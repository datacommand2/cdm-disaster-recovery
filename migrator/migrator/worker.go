package migrator

import (
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"path"

	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

//const defaultLeaderElectionPath = "migrator_workers"

var workerMapLock = sync.Mutex{}
var workerMap = make(map[string]*Worker)

// Worker task 수행을 위한 job 과 task 정보
type Worker struct {
	job  *migrator.RecoveryJob
	task *migrator.RecoveryJobTask
}

type taskResultOption struct {
	msg *migrator.Message
}

// TaskResultOptions taskResultOption 옵션 함수
type TaskResultOptions func(o *taskResultOption)

// WithTaskResult 공유 task 일 경우 공유 task result, status 등을 조회 하도록 설정 하는 함수
func WithTaskResult(msg *migrator.Message) TaskResultOptions {
	return func(o *taskResultOption) {
		o.msg = msg
	}
}

func extractOutputMessage(data interface{}) (*migrator.Message, error) {
	marshal, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Message migrator.Message `json:"message"`
	}

	if err := json.Unmarshal(marshal, &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Message, nil
}

func (w *Worker) setInstanceStatusRecovering(txn store.Txn) error {
	var in migrator.InstanceCreateTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setInstanceStatusRecovering] Could not get input data: job(%d) instance(%d). Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status, err := w.job.GetInstanceStatus(in.InstanceID)
	if err != nil {
		logger.Errorf("[setInstanceStatusRecovering] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobInstanceStateCodeBooting
	status.StartedAt = time.Now().Unix()

	if err = w.job.SetInstanceStatus(txn, in.InstanceID, status); err != nil {
		logger.Errorf("[setInstanceStatusRecovering] Could not set instance status: dr.recovery.job/%d/result/instance/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, status.StateCode, err)
		return err
	}

	logger.Infof("[setInstanceStatusRecovering] Done - set instance status: dr.recovery.job/%d/result/instance/%d {%s}.",
		w.job.RecoveryJobID, in.InstanceID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
		RecoveryJobID:  w.job.RecoveryJobID,
		InstanceID:     in.InstanceID,
		InstanceStatus: status,
	}); err != nil {
		logger.Warnf("[setInstanceStatusRecovering] Could not publish instance(%d) status. Cause: %+v", in.InstanceID, err)
	}

	return nil
}

func (w *Worker) setInstanceStatusCanceled(txn store.Txn) error {
	var in migrator.InstanceCreateTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setInstanceStatusCanceled] Could not get input data: job(%d) instance(%d). Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status, err := w.job.GetInstanceStatus(in.InstanceID)
	if err != nil {
		logger.Errorf("[setInstanceStatusCanceled] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobInstanceStateCodeFailed
	status.ResultCode = constant.InstanceRecoveryResultCodeCanceled
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = &migrator.Message{Code: "cdm-dr.migrator.run_task.canceled"}

	if err = w.job.SetInstanceStatus(txn, in.InstanceID, status); err != nil {
		logger.Errorf("[setInstanceStatusCanceled] Could not set instance status: dr.recovery.job/%d/result/instance/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, status.StateCode, err)
		return err
	}

	logger.Infof("[setInstanceStatusCanceled] Done - set instance status: dr.recovery.job/%d/result/instance/%d {%s}.",
		w.job.RecoveryJobID, in.InstanceID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
		RecoveryJobID:  w.job.RecoveryJobID,
		InstanceID:     in.InstanceID,
		InstanceStatus: status,
	}); err != nil {
		logger.Warnf("[setInstanceStatusCanceled] Could not publish instance(%d) status. Cause: %+v", in.InstanceID, err)
	}

	return nil
}

func (w *Worker) setInstanceStatusIgnored(txn store.Txn) error {
	var in migrator.InstanceCreateTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setInstanceStatusIgnored] Could not get input data: job(%d) instance(%d). Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status, err := w.job.GetInstanceStatus(in.InstanceID)
	if err != nil {
		logger.Errorf("[setInstanceStatusIgnored] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobInstanceStateCodeIgnored
	status.ResultCode = constant.InstanceRecoveryResultCodeFailed
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = &migrator.Message{Code: "cdm-dr.migrator.run_task.ignored-dependency_task_failed"}

	if err = w.job.SetInstanceStatus(txn, in.InstanceID, status); err != nil {
		logger.Errorf("[setInstanceStatusIgnored] Could not set instance status: dr.recovery.job/%d/result/instance/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, status.StateCode, err)
		return err
	}

	logger.Infof("[setInstanceStatusIgnored] Done - set instance status: dr.recovery.job/%d/result/instance/%d {%s}.",
		w.job.RecoveryJobID, in.InstanceID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
		RecoveryJobID:  w.job.RecoveryJobID,
		InstanceID:     in.InstanceID,
		InstanceStatus: status,
	}); err != nil {
		logger.Warnf("[setInstanceStatusIgnored] Could not publish instance(%d) status. Cause: %+v", in.InstanceID, err)
	}

	return nil
}

func (w *Worker) setInstanceStatusSuccess(txn store.Txn) error {
	var in migrator.InstanceCreateTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setInstanceStatusSuccess] Could not get input data: job(%d) instance(%d). Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status, err := w.job.GetInstanceStatus(in.InstanceID)
	if err != nil {
		logger.Errorf("[setInstanceStatusSuccess] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobInstanceStateCodeSuccess
	status.ResultCode = constant.InstanceRecoveryResultCodeSuccess
	status.FinishedAt = time.Now().Unix()

	if err = w.job.SetInstanceStatus(txn, in.InstanceID, status); err != nil {
		logger.Errorf("[setInstanceStatusSuccess] Could not set instance status: dr.recovery.job/%d/result/instance/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, status.StateCode, err)
		return err
	}

	logger.Infof("[setInstanceStatusSuccess] Done - set instance status: dr.recovery.job/%d/result/instance/%d {%s}.",
		w.job.RecoveryJobID, in.InstanceID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
		RecoveryJobID:  w.job.RecoveryJobID,
		InstanceID:     in.InstanceID,
		InstanceStatus: status,
	}); err != nil {
		logger.Warnf("[setInstanceStatusSuccess] Could not publish instance(%d) status. Cause: %+v", in.InstanceID, err)
	}

	return nil
}

func (w *Worker) setInstanceStatusFailed(txn store.Txn, reason *migrator.Message) error {
	var in migrator.InstanceCreateTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setInstanceStatusFailed] Could not get input data: job(%d) instance(%d). Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status, err := w.job.GetInstanceStatus(in.InstanceID)
	if err != nil {
		logger.Errorf("[setInstanceStatusFailed] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobInstanceStateCodeFailed
	status.ResultCode = constant.InstanceRecoveryResultCodeFailed
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = reason

	if err = w.job.SetInstanceStatus(txn, in.InstanceID, status); err != nil {
		logger.Errorf("[setInstanceStatusFailed] Could not set instance status: dr.recovery.job/%d/result/instance/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.InstanceID, status.StateCode, err)
		return err
	}

	logger.Infof("[setInstanceStatusFailed] Done - set instance status: dr.recovery.job/%d/result/instance/%d {%s}.",
		w.job.RecoveryJobID, in.InstanceID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
		RecoveryJobID:  w.job.RecoveryJobID,
		InstanceID:     in.InstanceID,
		InstanceStatus: status,
	}); err != nil {
		logger.Warnf("[setInstanceStatusFailed] Could not publish instance(%d) status. Cause: %+v", in.InstanceID, err)
	}

	return nil
}

func (w *Worker) setVolumeStatusRecovering(txn store.Txn) error {
	var in migrator.VolumeImportTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setVolumeStatusRecovering] Could not get input data: job(%d) volume(%d). Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status, err := w.job.GetVolumeStatus(in.VolumeID)
	if err != nil {
		logger.Errorf("[setVolumeStatusRecovering] Could not get volume status: dr.recovery.job/%d/result/volume/%d. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobVolumeStateCodePreparing
	status.StartedAt = time.Now().Unix()

	if err = w.job.SetVolumeStatus(txn, in.VolumeID, status); err != nil {
		logger.Errorf("[setVolumeStatusRecovering] Could not set volume status: dr.recovery.job/%d/result/volume/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, status.StateCode, err)
		return err
	}

	logger.Infof("[setVolumeStatusRecovering] Done - set volume status: dr.recovery.job/%d/result/volume/%d {%s}.",
		w.job.RecoveryJobID, in.VolumeID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
		RecoveryJobID: w.job.RecoveryJobID,
		VolumeID:      in.VolumeID,
		VolumeStatus:  status,
	}); err != nil {
		logger.Warnf("[setVolumeStatusRecovering] Could not publish volume(%d) status. Cause: %+v", in.VolumeID, err)
	}

	return nil
}

func (w *Worker) setVolumeStatusCanceled(txn store.Txn) error {
	var in migrator.VolumeImportTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setVolumeStatusCanceled] Could not get input data: job(%d) volume(%d). Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status, err := w.job.GetVolumeStatus(in.VolumeID)
	if err != nil {
		logger.Errorf("[setVolumeStatusCanceled] Could not get volume status: dr.recovery.job/%d/result/volume/%d. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobVolumeStateCodeFailed
	status.ResultCode = constant.VolumeRecoveryResultCodeCanceled
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = &migrator.Message{Code: "cdm-dr.migrator.run_task.canceled"}

	if err = w.job.SetVolumeStatus(txn, in.VolumeID, status); err != nil {
		logger.Errorf("[setVolumeStatusCanceled] Could not set volume status: dr.recovery.job/%d/result/volume/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, status.StateCode, err)
		return err
	}

	logger.Infof("[setVolumeStatusCanceled] Done - set volume status: dr.recovery.job/%d/result/volume/%d {%s}.",
		w.job.RecoveryJobID, in.VolumeID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
		RecoveryJobID: w.job.RecoveryJobID,
		VolumeID:      in.VolumeID,
		VolumeStatus:  status,
	}); err != nil {
		logger.Warnf("[setVolumeStatusCanceled] Could not publish volume(%d) status. Cause: %+v", in.VolumeID, err)
	}

	return nil
}

func (w *Worker) setVolumeStatusSuccess(txn store.Txn) error {
	var in migrator.VolumeImportTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setVolumeStatusSuccess] Could not get input data: job(%d) volume(%d). Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status, err := w.job.GetVolumeStatus(in.VolumeID)
	if err != nil {
		logger.Errorf("[setVolumeStatusSuccess] Could not get volume status: dr.recovery.job/%d/result/volume/%d. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobVolumeStateCodeSuccess
	status.ResultCode = constant.VolumeRecoveryResultCodeSuccess
	status.FinishedAt = time.Now().Unix()

	if err = w.job.SetVolumeStatus(txn, in.VolumeID, status); err != nil {
		logger.Errorf("[setVolumeStatusSuccess] Could not set volume status: dr.recovery.job/%d/result/volume/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, status.StateCode, err)
		return err
	}

	logger.Infof("[setVolumeStatusSuccess] Done - set volume status: dr.recovery.job/%d/result/volume/%d {%s}.",
		w.job.RecoveryJobID, in.VolumeID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
		RecoveryJobID: w.job.RecoveryJobID,
		VolumeID:      in.VolumeID,
		VolumeStatus:  status,
	}); err != nil {
		logger.Warnf("[setVolumeStatusSuccess] Could not publish volume(%d) status. Cause: %+v", in.VolumeID, err)
	}

	return nil
}

func (w *Worker) setVolumeStatusFailed(txn store.Txn, reason *migrator.Message) error {
	var in migrator.VolumeImportTaskInputData
	if err := w.task.GetInputData(&in); err != nil {
		logger.Errorf("[setVolumeStatusFailed] Could not get input data: job(%d) volume(%d). Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status, err := w.job.GetVolumeStatus(in.VolumeID)
	if err != nil {
		logger.Errorf("[setVolumeStatusFailed] Could not get volume status: dr.recovery.job/%d/result/volume/%d. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobVolumeStateCodeFailed
	status.ResultCode = constant.VolumeRecoveryResultCodeFailed
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = reason

	if err = w.job.SetVolumeStatus(txn, in.VolumeID, status); err != nil {
		logger.Errorf("[setVolumeStatusFailed] Could not set volume status: dr.recovery.job/%d/result/volume/%d {%s}. Cause: %+v",
			w.job.RecoveryJobID, in.VolumeID, status.StateCode, err)
		return err
	}

	logger.Infof("[setVolumeStatusFailed] Done - set volume status: dr.recovery.job/%d/result/volume/%d {%s}.",
		w.job.RecoveryJobID, in.VolumeID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
		RecoveryJobID: w.job.RecoveryJobID,
		VolumeID:      in.VolumeID,
		VolumeStatus:  status,
	}); err != nil {
		logger.Warnf("[setVolumeStatusFailed] Could not publish volume(%d) status. Cause: %+v", in.VolumeID, err)
	}

	return nil
}

// 성공 task 의 result 설정
func (w *Worker) setTaskResultSuccess(txn store.Txn, output interface{}) error {
	switch w.task.TypeCode {
	case constant.MigrationTaskTypeImportVolume:
		if err := w.setVolumeStatusSuccess(txn); err != nil {
			logger.Errorf("[setTaskResultSuccess] Could not set volume status success: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

	case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
		if err := w.setInstanceStatusSuccess(txn); err != nil {
			logger.Errorf("[setTaskResultSuccess] Could not set instance status success: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
	}

	// 공유 task type 의 task 성공 시, 공유 task 의 reference count 를 가져오고,
	// 0 인 경우 현재 리소스를 사용하고 있지 않으므로 공유 task 정보를 삭제한다.
	isShared := false
	if w.task.SharedTaskKey != "" {
		count, err := w.task.GetRefCount()
		if err != nil {
			logger.Errorf("[setTaskResultSuccess] Could not get reference count: job(%d) typeCode(%s) taskKey(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskKey, err)
			return err
		}

		if count < 1 {
			taskID := strings.Split(w.task.SharedTaskKey, "/")[1]
			// subnet, security group rule 은 network, security group 삭제 시 여기서 shared task 정보를 지움
			if err = migrator.DeleteReverseSharedTask(txn, w.task.TypeCode, taskID, w.job.RecoveryCluster.Id); err != nil && !errors.Equal(err, migrator.ErrNotSharedTaskType) {
				logger.Errorf("[setTaskResultSuccess] Cloud not delete reverse shared task: job(%d) typeCode(%s) taskKey(%s). Cause:%+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.SharedTaskKey, err)
				return err
			}
			logger.Infof("[setTaskResultSuccess] Done - delete reverse shared task: job(%d) typeCode(%s) taskKey(%s).",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.SharedTaskKey)
		} else {
			isShared = true
		}
	}

	if err := w.task.SetResultSuccess(txn, output, migrator.WithSharedTask(isShared)); err != nil {
		logger.Errorf("[setTaskResultSuccess] Could not set result(success) withShared(%t): job(%d) typeCode(%s) task(%s). Cause: %+v",
			isShared, w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return err
	}
	logger.Infof("[setTaskResultSuccess] Done - set result(success): job(%d) typeCode(%s) task(%s)",
		w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

	msg, err := extractOutputMessage(output)
	if err != nil {
		logger.Warnf("[setTaskResultSuccess] Could not extract message(%+v): job(%d) typeCode(%s) task(%s). Cause: %+v",
			output, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
	} else {
		content := msg.Contents
		if isConcurrent(w.task.TypeCode) {
			content = w.task.RecoveryJobTaskID
		}

		seq := getLogSeq(w.job)
		addLogLock.Lock()
		if err := w.job.AddLog(txn, seq, msg.Code, content); err != nil {
			logger.Warnf("[setTaskResultSuccess] Could not add log[%d]: code(%s) job(%d) typeCode(%s) task(%s) msg(%+v). Cause: %+v",
				seq, msg.Code, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, msg, err)
		} else {
			logger.Infof("[setTaskResultSuccess] Done - Add log[%d]: code(%s) job(%d) typeCode(%s) task(%s).",
				seq, msg.Code, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
		}
		addLogLock.Unlock()
	}

	if err = w.task.SetStatusFinished(txn, migrator.WithSharedTask(isShared)); err != nil {
		logger.Errorf("[setTaskResultSuccess] Cloud not set status finished: job(%d) typeCode(%s) task(%s). Cause:%+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return err
	}
	logger.Infof("[setTaskResultSuccess] Done - set status(done) withShared(%t): job(%d) typeCode(%s) task(%s)",
		isShared, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

	setSuccessTaskResultMap(w.job.RecoveryJobID, w.task.RecoveryJobTaskID)

	// security group delete -> security group rule, network delete -> subnet
	// task reference count decrease 해 줄 수 있게 해줌
	decreaseRelatedSharedTaskRefCount(w.job, w.task)
	return nil
}

// 무시 task 의 result 설정
func (w *Worker) setTaskResultIgnored(txn store.Txn, reason interface{}) error {
	b, err := json.Marshal(reason)
	if err != nil {
		return err
	}

	switch w.task.TypeCode {
	case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
		if failed, err := w.isFailedInstanceCreationTask(); err != nil {
			logger.Errorf("[setTaskResultIgnored] Could not get failed instance creation task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		} else if failed {
			if err = w.setInstanceStatusFailed(txn, &migrator.Message{Code: taskIgnoredLogCodeMap[w.task.TypeCode], Contents: string(b)}); err != nil {
				logger.Errorf("[setTaskResultIgnored] Could not set instance status failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				return err
			}
		} else {
			if err = w.setInstanceStatusIgnored(txn); err != nil {
				logger.Errorf("[setTaskResultIgnored] Could not set instance status ignored: job(%d) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				return err
			}
		}

	case constant.MigrationTaskTypeImportVolume:
		if err = w.setVolumeStatusFailed(txn, &migrator.Message{Code: taskIgnoredLogCodeMap[w.task.TypeCode], Contents: string(b)}); err != nil {
			logger.Errorf("[setTaskResultIgnored] Could not set volume status failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

	}

	if err = w.task.SetResultIgnored(txn, taskIgnoredLogCodeMap[w.task.TypeCode], reason, migrator.WithSharedTask(true)); err != nil {
		logger.Errorf("[setTaskResultIgnored] Could not set result ignored: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return err
	}
	logger.Infof("[setTaskResultIgnored] Done - set result(ignored): job(%d) typeCode(%s) task(%s)",
		w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

	seq := getLogSeq(w.job)
	addLogLock.Lock()
	if err = w.job.AddLog(txn, seq, taskIgnoredLogCodeMap[w.task.TypeCode], reason); err != nil {
		logger.Warnf("[setTaskResultIgnored] Could not add log[%d]: code(%s) contents(%+v) job(%d) typeCode(%s) task(%s). Cause: %+v",
			seq, taskIgnoredLogCodeMap[w.task.TypeCode], reason, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
	} else {
		logger.Infof("[setTaskResultIgnored] Done - Add log[%d]: code(%s) contents(%+v) job(%d) typeCode(%s) task(%s).",
			seq, taskIgnoredLogCodeMap[w.task.TypeCode], reason, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
	}
	addLogLock.Unlock()

	// task 상태를 done 으로 변경
	if err = w.task.SetStatusFinished(txn, migrator.WithSharedTask(true)); err != nil {
		logger.Warnf("[setTaskResultIgnored] Could not set status finished: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
	}

	logger.Infof("[setTaskResultIgnored] Done - set status(done): job(%d) typeCode(%s) task(%s)",
		w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

	setFailedTaskResultMap(w.job.RecoveryJobID, w.task)
	return nil
}

func (w *Worker) setTaskStatusCanceled(txn store.Txn) error {
	switch w.task.TypeCode {
	case constant.MigrationTaskTypeImportVolume:
		if err := w.setVolumeStatusCanceled(txn); err != nil {
			logger.Errorf("[setTaskResultCanceled] Could not set volume status canceled: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

	case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
		if err := w.setInstanceStatusCanceled(txn); err != nil {
			logger.Errorf("[setTaskResultCanceled] Could not set instance status canceled: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

	}

	return nil
}

func (w *Worker) setTaskResultCanceled() error {
	return store.Transaction(func(txn store.Txn) error {
		if err := w.task.SetResultCanceled(txn, taskCanceledLogCodeMap[w.task.TypeCode]); err != nil {
			logger.Errorf("[setTaskResultCanceled] Could not set result canceled: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
		logger.Infof("[setTaskResultCanceled] Done - set result(canceled): job(%d) typeCode(%s) task(%s)",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

		seq := getLogSeq(w.job)
		addLogLock.Lock()
		if err := w.job.AddLog(txn, seq, taskCanceledLogCodeMap[w.task.TypeCode], nil); err != nil {
			logger.Warnf("[setTaskResultCanceled] Could not add log[%d]: code(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				seq, taskCanceledLogCodeMap[w.task.TypeCode], w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		} else {
			logger.Infof("[setTaskResultCanceled] Done - Add log[%d]: code(%s) job(%d) typeCode(%s) task(%s).",
				seq, taskCanceledLogCodeMap[w.task.TypeCode], w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
		}
		addLogLock.Unlock()

		// 공유 task type 의 task 취소 시, 공유 task 의 reference count 를 가져오고,
		// 1 보다 작거나 같은 경우(바로 cancel 되는 job 은 reference 정보가 없음) 다른 작업에서 현재 리소스를 사용하고 있지 않으므로 공유 task 정보를 삭제한다.
		isShared := false
		if w.task.SharedTaskKey != "" {
			count, err := w.task.GetRefCount()
			if err != nil {
				logger.Errorf("[setTaskResultCanceled] Could not get reference count: job(%d) typeCode(%s) taskKey(%s). Cause: %+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskKey, err)
				return err
			}

			if count < 1 {
				if err = migrator.DeleteSharedTask(txn, w.task.TypeCode, w.task.RecoveryJobTaskID, w.job.RecoveryCluster.Id); err != nil {
					logger.Errorf("[setTaskResultCanceled] Cloud not delete shared task: job(%d) typeCode(%s) taskKey(%s). Cause:%+v",
						w.task.RecoveryJobID, w.task.TypeCode, w.task.SharedTaskKey, err)
					return err
				}
				logger.Infof("[setTaskResultCanceled] Done - delete shared task: job(%d) typeCode(%s) taskKey(%s).",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.SharedTaskKey)
			} else {
				isShared = true
			}
		}

		// task 상태를 done 으로 변경
		if err := w.task.SetStatusFinished(txn, migrator.WithSharedTask(isShared)); err != nil {
			logger.Warnf("[setTaskResultCanceled] Could not set status finished: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
		logger.Infof("[setTaskResultCanceled] Done - set status(done) withShared(%t): job(%d) typeCode(%s) task(%s)",
			isShared, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

		setFailedTaskResultMap(w.job.RecoveryJobID, w.task)
		return nil
	})

}

// 실패 task 의 result 설정
func (w *Worker) setTaskResultFailed(txn store.Txn, reason interface{}) error {
	b, err := json.Marshal(reason)
	if err != nil {
		return err
	}

	m := &migrator.Message{Code: taskFailedLogCodeMap[w.task.TypeCode], Contents: string(b)}
	switch w.task.TypeCode {
	case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
		if err = w.setInstanceStatusFailed(txn, m); err != nil {
			logger.Errorf("[setTaskResultFailed] Could not set instance status failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

	case constant.MigrationTaskTypeImportVolume:
		if err = w.setVolumeStatusFailed(txn, m); err != nil {
			logger.Errorf("[setTaskResultFailed] Could not set volume status failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
	}

	if err = w.task.SetResultFailed(txn, m.Code, m.Contents, migrator.WithSharedTask(true)); err != nil {
		logger.Errorf("[setTaskResultFailed] Could not set result failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return err
	}

	seq := getLogSeq(w.job)
	addLogLock.Lock()
	if err = w.job.AddLog(txn, seq, m.Code, m.Contents); err != nil {
		logger.Warnf("[setTaskResultFailed] Could not add log[%d]: code(%s) contents(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
			seq, m.Code, m.Contents, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
	} else {
		logger.Infof("[setTaskResultFailed] Done - Add log[%d]: code(%s) contents(%s) job(%d) typeCode(%s) task(%s).",
			seq, m.Code, m.Contents, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
	}
	addLogLock.Unlock()

	// task 상태를 done 으로 변경
	if err = w.task.SetStatusFinished(txn, migrator.WithSharedTask(true)); err != nil {
		logger.Warnf("[setTaskResultFailed] Could not set status finished: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
	}

	logger.Infof("[setTaskResultFailed] Done - set result(failed) set status(done): job(%d) typeCode(%s) task(%s)",
		w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

	if err = rollbackSharedTaskReferenceCount(txn, w.job, w.task); err != nil {
		return err
	}

	setFailedTaskResultMap(w.job.RecoveryJobID, w.task)
	return nil
}

func (w *Worker) setTaskStatusStarted(txn store.Txn) (err error) {
	switch w.task.TypeCode {
	case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
		if err = w.setInstanceStatusRecovering(txn); err != nil {
			logger.Errorf("[setTaskStatusStarted] Could not set instance status recovering: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

	case constant.MigrationTaskTypeImportVolume:
		if err = w.setVolumeStatusRecovering(txn); err != nil {
			logger.Errorf("[setTaskStatusStarted] Could not set volume status recovering: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
	}

	// task status 를 running 으로 변경
	if err = w.task.SetStatusStarted(txn, migrator.WithSharedTask(true)); err != nil {
		logger.Warnf("[setTaskStatusStarted] Could not set status finished: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
	}

	logger.Infof("[setTaskStatusStarted] Done - set status(running): job(%d) typeCode(%s) task(%s)",
		w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
	return nil
}

// 현재 task 의 dependency task 중 실패한 task 가 있는지 확인한다.
func (w *Worker) checkDependencyTaskFailed() error {
	var tasks []string

	if w.task.TypeCode == constant.MigrationTaskTypeDeleteTenant {
		taskList, err := w.job.GetClearTaskList()
		if err != nil {
			logger.Errorf("[checkDependencyTaskFailed-DeleteTenant] Could not get GetClearTaskList job(%d) task(%s). Cause: %+v",
				w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
		} else {
			for _, task := range taskList {
				if w.task.TypeCode == task.TypeCode {
					continue
				}
				status, err := task.GetStatus()
				if err != nil {
					logger.Errorf("[checkDependencyTaskFailed-DeleteTenant] Could not get dependency result: job(%d) task(%s). Cause: %+v",
						w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
					continue //기능 점검을 위한 임시
				}
				result, err := task.GetResult()
				if err != nil {
					logger.Errorf("[checkDependencyTaskFailed-DeleteTenant] Could not get dependency result: job(%d) task(%s). Cause: %+v",
						w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
					continue //기능 점검을 위한 임시
				}

				if status.StateCode == constant.MigrationTaskStateCodeDone && result.ResultCode == constant.MigrationTaskResultCodeSuccess {
					continue
				}

				logger.Warnf("[checkDependencyTaskFailed-DeleteTenant] DependencyTaskFailed: job(%d) task(%s) status(%s) result(%s).",
					w.job.RecoveryJobID, w.task.RecoveryJobTaskID, status.StateCode, result.ResultCode)
				//tasks = append(tasks, w.task.RecoveryJobTaskID)

			}
			/*if len(tasks) > 0 {
				return DependencyTaskFailed(w.task.RecoveryJobID, w.task.RecoveryJobTaskID, tasks)
			}
			return nil*/
		}

	}

	for _, d := range w.task.Dependencies {
		depTask, err := getJobTask(w.job, d)
		if err != nil {
			logger.Errorf("[checkDependencyTaskFailed] Could not get dependency task(%s): job(%d) task(%s). Cause: %+v",
				d, w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
			return err
		}

		result, err := depTask.GetResult()
		if err != nil {
			logger.Errorf("[checkDependencyTaskFailed] Could not get dependency task(%s) result: job(%d) task(%s). Cause: %+v",
				d, w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
			return err
		}

		if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			continue
		}

		logger.Warnf("[checkDependencyTaskFailed] Dependency task(%s) is %s >> DependencyTaskFailed: job(%d) task(%s).",
			depTask.RecoveryJobTaskID, result.ResultCode, w.job.RecoveryJobID, w.task.RecoveryJobTaskID)
		tasks = append(tasks, depTask.RecoveryJobTaskID)
	}

	if len(tasks) > 0 {
		return DependencyTaskFailed(w.task.RecoveryJobID, w.task.RecoveryJobTaskID, tasks)
	}

	return nil
}

func (w *Worker) isFailedInstanceCreationTask() (bool, error) {
	for _, d := range w.task.Dependencies {
		depTask, err := getJobTask(w.job, d)
		if err != nil {
			logger.Errorf("[isFailedInstanceCreationTask] Could not get dependency task(%s): job(%d) task(%s). Cause: %+v",
				d, w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
			return false, err
		}

		result, err := depTask.GetResult()
		if err != nil {
			logger.Errorf("[isFailedInstanceCreationTask] Could not get dependency task(%s) result: job(%d) task(%s). Cause: %+v",
				d, w.job.RecoveryJobID, w.task.RecoveryJobTaskID, err)
			return false, err
		}

		if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			continue
		}

		// 의존하는 Task 중 인스턴스 생성이 아닌 다른 리소스 생성에 실패한 경우 실패임
		if depTask.TypeCode != constant.MigrationTaskTypeCreateAndDiagnosisInstance {
			logger.Infof("[isFailedInstanceCreationTask] Dependency task(%s:%s) is failed: job(%d) task(%s).",
				d, depTask.TypeCode, w.job.RecoveryJobID, w.task.RecoveryJobTaskID)
			return true, nil
		}
	}

	return false, nil
}

func (w *Worker) setTaskStatusAndResultSameAsSharedTask(sharedTaskStatus *migrator.RecoveryJobTaskStatus) error {
	var (
		err    error
		result *migrator.RecoveryJobTaskResult
		code   string
		reason interface{}
	)

	if err = store.Transaction(func(txn store.Txn) error {
		// 현재 task 의 status 를 공유 task 의 status 로 채운다.
		if err = w.task.SetStatus(txn, sharedTaskStatus); err != nil {
			logger.Errorf("[setTaskStatusAndResultSameAsSharedTask] Could not set task status(%s): job(%d) typeCode(%s) task(%s). Cause: %+v",
				sharedTaskStatus.StateCode, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
		logger.Infof("[setTaskStatusAndResultSameAsSharedTask] Done - set task status(%s): job(%d) typeCode(%s) task(%s)",
			sharedTaskStatus.StateCode, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

		// 공유 task 의 result 를 가져온다.
		if result, err = w.task.GetResult(migrator.WithSharedTask(true)); err != nil {
			logger.Errorf("[setTaskStatusAndResultSameAsSharedTask] Could not get task result: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}

		// 현재 task 의 result 를 공유 task 의 result 로 채운다.
		if err = w.task.SetResult(txn, result); err != nil {
			logger.Errorf("[setTaskStatusAndResultSameAsSharedTask] Could not set task result(%s): job(%d) typeCode(%s) task(%s). Cause: %+v",
				result, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return err
		}
		logger.Infof("[setTaskStatusAndResultSameAsSharedTask] Done - set task result(%s): job(%d) typeCode(%s) task(%s)",
			result.ResultCode, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

		if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			msg, err := extractOutputMessage(result.Output)
			if err != nil {
				logger.Warnf("[setTaskStatusAndResultSameAsSharedTask] Could not extract message: job(%d) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				code = w.task.TypeCode
			}
			code = msg.Code
			reason = msg.Contents
		} else {
			code = result.FailedReason.Code
			reason = result.FailedReason.Contents
		}

		seq := getLogSeq(w.job)
		addLogLock.Lock()
		if err = w.job.AddLog(txn, seq, code, reason); err != nil {
			logger.Warnf("[setTaskStatusAndResultSameAsSharedTask] Could not add log[%d]: code(%s) reason(%+v) job(%d) typeCode(%s) task(%s). Cause: %+v",
				seq, code, reason, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		} else {
			logger.Infof("[setTaskStatusAndResultSameAsSharedTask] Done - Add log[%d]: code(%s) reason(%+v) job(%d) typeCode(%s) task(%s).",
				seq, code, reason, w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
		}
		addLogLock.Unlock()
		return nil
	}); err != nil {
		logger.Errorf("[setTaskStatusAndResultSameAsSharedTask] Cloud not run task: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return err
	}

	if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
		setSuccessTaskResultMap(w.job.RecoveryJobID, w.task.RecoveryJobTaskID)
		// security group delete -> security group rule, network delete -> subnet
		// task reference count decrease 해 줄 수 있게 해줌
		decreaseRelatedSharedTaskRefCount(w.job, w.task)
	} else {
		setFailedTaskResultMap(w.job.RecoveryJobID, w.task)
	}

	if err = publishMessage(constant.QueueRecoveryJobTaskStatusMonitor, migrator.RecoveryJobTaskStatusMessage{
		RecoveryJobTaskStatusKey: path.Join(w.task.RecoveryJobTaskKey, "status"),
		TypeCode:                 w.task.TypeCode,
		RecoveryJobTaskStatus:    sharedTaskStatus,
	}); err != nil {
		logger.Warnf("[setTaskStatusAndResultSameAsSharedTask] Could not publish task(%s) status(%s). Cause: %+v", w.task.TypeCode, sharedTaskStatus.StateCode, err)
	}

	if err = publishMessage(constant.QueueRecoveryJobTaskResultMonitor, migrator.RecoveryJobTaskResultMessage{
		RecoveryJobTaskResultKey: path.Join(w.task.RecoveryJobTaskKey, "result"),
		TypeCode:                 w.task.TypeCode,
		RecoveryJobTaskResult:    result,
	}); err != nil {
		logger.Warnf("[setTaskStatusAndResultSameAsSharedTask] Could not publish task(%s) result(%s). Cause: %+v", w.task.TypeCode, result.ResultCode, err)
	}

	return nil
}

// job 이 running, clearing 상태일 때 task 수행
func (w *Worker) runTask(ctx context.Context) {
	var err error
	timeout := time.After(5 * time.Minute)

	// TODO : task 수행 결과를 저장 하지 못하는 경우 처리 추가
	// task function 수행 후 성공 or 실패 결과 등록
	logger.Infof("[Worker-runTask] Start: job(%d) typeCode(%s) task(%s).",
		w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)

	output, taskErr := taskFuncMap[w.task.TypeCode](ctx, w)
	for {

		if err = store.Transaction(func(txn store.Txn) error {
			if taskErr != nil {
				logger.Warnf("[Worker-runTask] Task is failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, taskErr)
				if err := w.setTaskResultFailed(txn, taskErr); err != nil {
					logger.Errorf("[Worker-runTask] Could not set task result failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
						w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
					return err
				}

				return nil
			}

			logger.Infof("[Worker-runTask] Task is success: job(%d) typeCode(%s) task(%s).",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
			if err := w.setTaskResultSuccess(txn, output); err != nil {
				logger.Errorf("[Worker-runTask] Could not set task result success: job(%d) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				return err
			}

			return nil

		}); err == nil {
			return
		}

		select {
		case <-time.After(1 * time.Second):
		case <-timeout:
			logger.Errorf("[Worker-runTask] Could not set task status and result: job(%d) typeCode(%s) task(%s). Cause: time out",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
			return
		}
	}
}

// task worker go routine 실행
// prefix log format : [job_id:task_id]
func (w *Worker) run(ctx context.Context, wg *sync.WaitGroup) {
	logger.Infof("[Worker-run] Start: job(%d) typeCode(%s) task(%s)", w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
	defer wg.Done()
	// task 수행이 완료되면 성공 여부와 상관없이 worker thread 를 삭제한다.
	defer deleteWorker(w.task)

	// FIXME: Leader 부분 주석 처리
	//// task 를 수행할 leader 를 선출한다
	//l, err := cloudSync.CampaignLeader(ctx, path.Join(defaultLeaderElectionPath, w.task.GetKey()))
	//if err != nil {
	//	logger.Errorf("[Worker-run] Could not get campaign leader(key: %s). Cause: %+v", w.task.GetKey(), err)
	//	return
	//}
	//
	//done := make(chan interface{})
	//defer func() {
	//	close(done)
	//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	//	_ = l.Resign(ctx)
	//	_ = l.Close()
	//	cancel()
	//}()
	//
	//// hanging 일 경우, 프로세스 종료
	//go func() {
	//	select {
	//	case <-l.Status():
	//		logger.Fatal("[Worker-run] Leader resigned. Process is forcibly terminated.")
	//
	//	case <-done:
	//		return
	//	}
	//}()

	// task 의 중복 실행을 막기 위한 lock
	unlock, err := w.task.Lock()
	if err != nil {
		logger.Errorf("[Worker-run] Could not acquire lock: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return
	}
	defer unlock()

	/*
		// migrator container 여러개 사용시 주석 해제필요
		// 현재 task 의 status 를 가져온다.
		taskStatus, err := w.task.GetStatus()
		if err != nil {
			logger.Errorf("[Worker-run] Could not get task status: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return
		}

		// 현재 task 가 running 이거나 done status 인 경우 task 를 run 할 필요가 없다.
		if taskStatus.StateCode != constant.MigrationTaskStateCodeWaiting {
			logger.Infof("[Worker-run] Nothing to do: taskState(%s) job(%d) typeCode(%s) task(%s)",
				taskStatus.StateCode, w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
			return
		}
	*/
	// job status 에 따른 task 동작 수행
	jobStatus, err := w.job.GetStatus()
	if err != nil {
		logger.Errorf("[Worker-run] Could not get job status: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return
	}

	switch jobStatus.StateCode {
	// 이미 생성된 공유 자원을 생성하는 task 인 경우 reference count 를 증가시킨다.
	case constant.RecoveryJobStateCodeRunning:
		if err = increaseSharedTaskRefCount(w.job, w.task); err != nil {
			logger.Errorf("[Worker-run] Could not increase shared task reference count: job(%d) state(%s) typeCode(%s) task(%s). Cause:%+v",
				w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return
		}

	// 아직 공유 중인 자원을 삭제하는 task 인 경우 reference count 를 감소시킨다.
	case constant.RecoveryJobStateCodeClearing:
		if err = decreaseSharedTaskRefCount(w.job, w.task); err != nil {
			logger.Errorf("[Worker-run] Could not decrease shared task reference count: job(%d) state(%s) typeCode(%s) task(%s). Cause:%+v",
				w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return
		}

	// job 의 상태가 canceling 이면 해당 task 의 result 를 취소 값으로 변경한다.
	case constant.RecoveryJobStateCodeCanceling:
		if err = w.setTaskResultCanceled(); err != nil {
			logger.Errorf("[Worker-run] Could not set task result canceled: job(%d) state(%s) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		}
		return
	}

	// 공유 task 의 status 를 가져온다.
	sharedTaskStatus, err := w.task.GetStatus(migrator.WithSharedTask(true))
	if err != nil {
		logger.Errorf("[Worker-run] Could not get task status: job(%d) typeCode(%s) task(%s). Cause: %+v",
			w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
		return
	}

	// 공유 task 가 이미 done 상태인지 확인한다.
	if sharedTaskStatus.StateCode == constant.MigrationTaskStateCodeDone {
		// 이미 done 상태이고, 공유 task 가 아니면 이미 수행이 완료된 task 이므로 return 한다.
		if w.task.SharedTaskKey == "" {
			logger.Infof("[Worker-run] Done - SharedTaskKey empty: task state code(%s) job(%d) typeCode(%s) task(%s)",
				sharedTaskStatus.StateCode, w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
			return
		}

		_, err = w.task.GetResult()
		// task 의 result 가 존재하지 않을 때 shared task 의 status 와 result 로 설정한다.
		if errors.Equal(err, migrator.ErrNotFoundTaskResult) {
			if err = w.setTaskStatusAndResultSameAsSharedTask(sharedTaskStatus); err != nil {
				return
			}
		} else if err != nil {
			logger.Errorf("[Worker-run] Could not get task result: job(%d) typeCode(%s) task(%s). Cause: %+v",
				w.task.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
			return
		}

		// 공유 task 가 이미 done 상태이면 task 를 수행하지 않고 return 한다.
		logger.Infof("[Worker-run] Success - task is already done: job(%d) typeCode(%s) task(%s)",
			w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
		return
	}
	//checkDependency 기록
	var isRunnable bool
	switch jobStatus.StateCode {
	// job 의 상태가 running 이거나 clearing 이면 task 를 수행한다.
	case constant.RecoveryJobStateCodeRunning, constant.RecoveryJobStateCodeClearing:
		if err = store.Transaction(func(txn store.Txn) error {

			err = w.checkDependencyTaskFailed()
			switch {
			// 실패한 dependency task 가 존재한다면 해당 task 는 무시(ignore)
			case errors.Equal(err, ErrDependencyTaskFailed):
				logger.Warnf("[Worker-run] Task is ignored: job(%d) state(%s) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				isRunnable = false
				if err = w.setTaskResultIgnored(txn, err); err != nil {
					logger.Errorf("[Worker-run] Could not set task result ignored: job(%d) state(%s) typeCode(%s) task(%s). Cause: %+v",
						w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
					return err
				}
				return nil

			case err != nil:
				logger.Errorf("[Worker-run] Could not check dependency task failed: job(%d) state(%s) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				return err
			}

			isRunnable = true

			if err = w.setTaskStatusStarted(txn); err != nil {
				logger.Errorf("[Worker-run] Could not set task status started: job(%d) state(%s) typeCode(%s) task(%s). Cause: %+v",
					w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID, err)
				return err
			}

			return nil

		}); err != nil {
			return
		}

		if isRunnable == false {
			logger.Warnf("[Worker-run] job(%d) state(%s) typeCode(%s) task(%s).", w.task.RecoveryJobID, jobStatus.StateCode, w.task.TypeCode, w.task.RecoveryJobTaskID)
			return
		}

		w.runTask(ctx)
	}

	logger.Infof("[Worker-run] Success: job(%d) typeCode(%s) task(%s)", w.job.RecoveryJobID, w.task.TypeCode, w.task.RecoveryJobTaskID)
}

// task 의 key 값으로 worker thread 를 생성한다
func newWorker(j *migrator.RecoveryJob, t *migrator.RecoveryJobTask) *Worker {
	workerMapLock.Lock()
	defer workerMapLock.Unlock()

	if _, ok := workerMap[t.GetKey()]; ok {
		return nil
	}

	workerMap[t.GetKey()] = &Worker{
		job:  j,
		task: t,
	}

	return workerMap[t.GetKey()]
}

// 워커 thread 삭제한다
func deleteWorker(t *migrator.RecoveryJobTask) {
	workerMapLock.Lock()
	defer workerMapLock.Unlock()

	delete(workerMap, t.GetKey())
}

func setTaskResultSuccess(txn store.Txn, job *migrator.RecoveryJob, task *migrator.RecoveryJobTask, options ...TaskResultOptions) error {
	var option = taskResultOption{
		msg: &migrator.Message{},
	}

	for _, o := range options {
		o(&option)
	}

	logger.Infof("[setTaskResultSuccess] Start: job(%d) typeCode(%s) task(%s)", task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if err := task.SetResultSuccess(txn, option.msg, migrator.WithSharedTask(true)); err != nil {
		logger.Errorf("[setTaskResultSuccess] Could not set result success: job(%d) typeCode(%s) task(%s). Cause: %+v",
			job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}
	logger.Infof("[setTaskResultSuccess] Done - set result success(%+v): job(%d) typeCode(%s) task(%s)",
		option.msg, job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if err := task.SetStatusFinished(txn, migrator.WithSharedTask(true)); err != nil {
		logger.Errorf("[setTaskResultSuccess] Could not set status finished: job(%d) typeCode(%s) task(%s). Cause: %+v",
			job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}
	logger.Infof("[setTaskResultSuccess] Done - set status done: job(%d) typeCode(%s) task(%s)",
		job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	seq := getLogSeq(job)
	addLogLock.Lock()
	if err := job.AddLog(txn, seq, option.msg.Code, option.msg.Contents); err != nil {
		logger.Warnf("[setTaskResultSuccess] Could not add log[%d]: code(%s) contents(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
			seq, option.msg.Code, option.msg.Contents, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
	} else {
		logger.Infof("[setTaskResultSuccess] Done - Add log[%d]: code(%s) contents(%s) job(%d) typeCode(%s) task(%s).",
			seq, option.msg.Code, option.msg.Contents, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	}
	addLogLock.Unlock()

	setSuccessTaskResultMap(job.RecoveryJobID, task.RecoveryJobTaskID)
	logger.Infof("[setTaskResultSuccess] Success: job(%d) typeCode(%s) task(%s)", job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return nil
}

func increaseSharedTaskRefCount(job *migrator.RecoveryJob, task *migrator.RecoveryJobTask) error {
	// 공유 리소스와 관련된 task 가 아닌 경우 건너뛴다.
	if task.SharedTaskKey == "" {
		return nil
	}

	// etcd 에서 task type 에 따라 해당 공유 task 의 정보를 가져온다.
	sharedTaskID := strings.Split(task.SharedTaskKey, "/")[1]
	sharedTask, err := migrator.GetSharedTask(task.TypeCode, sharedTaskID)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundSharedTask):
		// shared task 를 가져올 수 없는 경우 더이상 진행 불가능으로 판단하여 task done, result failed 처리
		content := fmt.Sprintf("%s.not_found_shared_task_during_increase_shared_task_reference_count", sharedTaskID)
		_ = setTaskStatusDoneAndResultFailed(job, task, content)
		return err
	case err != nil:
		logger.Errorf("[increaseSharedTaskRefCount] Could not get shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
		return err
	}

	if err = store.Transaction(func(txn store.Txn) error {
		// 공유 task 의 reference count 를 증가시킨다.
		return sharedTask.IncreaseRefCount(txn)
	}); err != nil {
		logger.Errorf("[increaseSharedTaskRefCount] Could not increase reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
		return err
	}

	count, err := sharedTask.GetRefCount()
	if err != nil {
		logger.Errorf("[increaseSharedTaskRefCount] Could not get reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
	}

	logger.Infof("[increaseSharedTaskRefCount] Done - increase shared task reference count: job(%d) typeCode(%s) task(%s) count(%d)",
		task.RecoveryJobID, task.TypeCode, sharedTaskID, count)

	return nil
}

// 공유 task 의 reference count 를 감소시킨다.
func decreaseSharedTaskRefCount(job *migrator.RecoveryJob, task *migrator.RecoveryJobTask) error {
	// 공유 리소스와 관련된 task 가 아닌 경우 건너뛴다.
	if task.SharedTaskKey == "" {
		return nil
	}

	// etcd 에서 task type 에 따라 해당 공유 task 의 정보를 가져온다.
	sharedTaskID := strings.Split(task.SharedTaskKey, "/")[1]
	sharedTask, err := migrator.GetReverseSharedTask(task.TypeCode, sharedTaskID)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundSharedTask):
		// shared task 를 가져올 수 없는 경우 이미 decrease 가 되었다고 판단하여 성공
		return store.Transaction(func(txn store.Txn) error {
			return setTaskResultSuccess(txn, job, task)
		})

	case err != nil:
		logger.Errorf("[decreaseSharedTaskRefCount] Could not get reverse shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
		return err
	}

	return store.Transaction(func(txn store.Txn) error {
		// 공유 task 의 reference count 를 감소시킨다.
		count, err := sharedTask.DecreaseRefCount(txn)
		if err != nil {
			logger.Errorf("[decreaseSharedTaskRefCount] Could not decrease reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
			return err
		}

		logger.Infof("[decreaseSharedTaskRefCount] Done - decrease shared task reference count: job(%d) typeCode(%s) task(%s) count(%d)",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, count)

		// 감소 시킨 공유 task 의 reference count 가 0 일 경우에만 clear_task 를 수행한다.
		// 0 이 아닌경우 다른 복구 작업에서 리소스를 사용 중이므로, task 의 result 를 "success", status 를 "done" 으로 변경하여
		// clear task 가 수행하지 않도록 한다.
		if count > 0 {
			msg := &migrator.Message{Code: taskSkippedLogCodeMap[task.TypeCode], Contents: fmt.Sprintf("%s.in_used", task.SharedTaskKey)}
			if err = setTaskResultSuccess(txn, job, task, WithTaskResult(msg)); err != nil {
				return err
			}

			// security group delete -> security group rule, network delete -> subnet
			// task reference count decrease 해 줄 수 있게 해줌
			decreaseRelatedSharedTaskRefCount(job, task)
		}
		return nil
	})
}

func rollbackSharedTaskReferenceCount(txn store.Txn, job *migrator.RecoveryJob, task *migrator.RecoveryJobTask) error {
	if task.SharedTaskKey == "" {
		return nil
	}

	// job status 에 따른 task 동작 수행
	jobStatus, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[rollbackSharedTaskReferenceCount] Could not get job status: job(%d) typeCode(%s) task(%s). Cause: %+v",
			task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	switch jobStatus.StateCode {
	case constant.RecoveryJobStateCodeRunning:
		// etcd 에서 task type 에 따라 해당 공유 task 의 정보를 가져온다.
		sharedTaskID := strings.Split(task.SharedTaskKey, "/")[1]
		sharedTask, err := migrator.GetSharedTask(task.TypeCode, sharedTaskID)
		if err != nil {
			logger.Errorf("[rollbackSharedTaskReferenceCount] Could not get shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
			return err
		}

		// 공유 task 의 reference count 를 감소시킨다.
		count, err := sharedTask.DecreaseRefCount(txn)
		if err != nil {
			logger.Errorf("[rollbackSharedTaskReferenceCount] Could not decrease reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
			return err
		}

		logger.Infof("[rollbackSharedTaskReferenceCount] Done - decrease shared task reference count: job(%d) typeCode(%s) task(%s) count(%d)",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, count)

		if count < 1 {
			// subnet, security group rule 은 network, security group 삭제 시 여기서 shared task 정보를 지움
			if err = migrator.DeleteReverseSharedTask(txn, task.TypeCode, sharedTaskID, job.RecoveryCluster.Id); err != nil && !errors.Equal(err, migrator.ErrNotSharedTaskType) {
				logger.Warnf("[rollbackSharedTaskReferenceCount] Cloud not delete reverse shared task: job(%d) typeCode(%s) taskKey(%s). Cause:%+v",
					task.RecoveryJobID, task.TypeCode, task.SharedTaskKey, err)
				return err
			}
			logger.Infof("[rollbackSharedTaskReferenceCount] Done - delete reverse shared task: job(%d) typeCode(%s) taskKey(%s).",
				task.RecoveryJobID, task.TypeCode, task.SharedTaskKey)
		}

	case constant.RecoveryJobStateCodeClearing:
		// etcd 에서 task type 에 따라 해당 공유 task 의 정보를 가져온다.
		sharedTaskID := strings.Split(task.SharedTaskKey, "/")[1]
		sharedTask, err := migrator.GetReverseSharedTask(task.TypeCode, sharedTaskID)
		if err != nil {
			logger.Errorf("[rollbackSharedTaskReferenceCount] Could not get reverse shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
			return err
		}

		// 공유 task 의 reference count 를 증가시킨다.
		if err = sharedTask.IncreaseRefCount(txn); err != nil {
			logger.Errorf("[rollbackSharedTaskReferenceCount] Could not increase reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
			return err
		}

		count, err := sharedTask.GetRefCount()
		if err != nil {
			logger.Errorf("[rollbackSharedTaskReferenceCount] Could not get reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, sharedTaskID, err)
		}

		logger.Infof("[rollbackSharedTaskReferenceCount] Done - increase shared task reference count: job(%d) typeCode(%s) task(%s) count(%d)",
			task.RecoveryJobID, task.TypeCode, sharedTaskID, count)
	}
	return nil
}

// Security Group Rule 또는 Subnet 공유 task 의 reference count 를 감소시킨다.
func decreaseRelatedSharedTaskRefCount(job *migrator.RecoveryJob, doneTask *migrator.RecoveryJobTask) {
	var key, typeCode string
	switch doneTask.TypeCode {
	case constant.MigrationTaskTypeDeleteSecurityGroup:
		key = "cdm.dr.manager.recovery_job.builder.security_group_rule_task_id_map"
		typeCode = constant.MigrationTaskTypeCreateSecurityGroupRule

	case constant.MigrationTaskTypeDeleteNetwork:
		key = "cdm.dr.manager.recovery_job.builder.subnet_task_id_map"
		typeCode = constant.MigrationTaskTypeCreateSubnet

	default:
		// security group, network 삭제 가 아닌 경우 진행하지 않음
		return
	}

	if doneTask.SharedTaskKey == "" {
		return
	}

	// SharedTaskKey 의 구조는 아래와 같이 때문에 TaskID를 얻기 위해서는 '/' 를 기준으로 split 하여 뒷 값을 사용
	// SharedTaskKey:cdm.dr.manager.recovery_job.builder.shared_security_group_task_map/cd6a0e65-9c6e-4cdf-9c0a-4410464af5de
	doneTaskID := strings.Split(doneTask.SharedTaskKey, "/")[1]

	// 해당 job metadata 조회
	// "id":"taskID" 구조이기 때문에 id map 으로 받아서 사용
	idMap := make(map[uint64]string)
	if err := job.GetMetadata(key, &idMap); err != nil {
		logger.Warnf("[decreaseRelatedSharedTaskRefCount] Could not get job metadata: job(%d). Cause: %+v", job.RecoveryJobID, err)
	}

	for _, taskID := range idMap {
		// taskID 로 sharedTask 조회
		sharedTask, err := migrator.GetSharedTask(typeCode, taskID)
		if err != nil {
			logger.Warnf("[decreaseRelatedSharedTaskRefCount] Could not get shared task: job(%d) doneTask(%s:%s). Cause: %+v", job.RecoveryJobID, typeCode, taskID, err)
			continue
		}

		// 조회하는 sharedTask 의 dependency 에 doneTask ID가 있는지 확인
		isDep := false
		for _, dep := range sharedTask.Dependencies {
			if dep == doneTaskID {
				isDep = true
				break
			}
		}

		// 없으면 pass
		if !isDep {
			continue
		}

		// 있다면 reference count 를 감소
		store.Transaction(func(txn store.Txn) error {
			count, err := sharedTask.DecreaseRefCount(txn)
			if err != nil {
				logger.Errorf("[decreaseRelatedSharedTaskRefCount] Could not decrease reference count: job(%d) task(%s:%s). Cause: %+v",
					doneTask.RecoveryJobID, typeCode, taskID, err)
			} else {
				logger.Infof("[decreaseRelatedSharedTaskRefCount] Done - decrease shared task(%s:%s) reference count(%d) by task(%s:%s) success: job(%d).",
					typeCode, taskID, count, doneTask.TypeCode, doneTask.RecoveryJobTaskID, doneTask.RecoveryJobID)
			}
			return nil
		})
	}
}
