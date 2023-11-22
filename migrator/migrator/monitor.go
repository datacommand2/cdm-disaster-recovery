package migrator

import (
	"10.1.1.220/cdm/cdm-cloud/common/broker"
	"10.1.1.220/cdm/cdm-cloud/common/database"
	"10.1.1.220/cdm/cdm-cloud/common/errors"
	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-cloud/common/store"
	"10.1.1.220/cdm/cdm-cloud/common/test/helper"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/constant"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/migrator"

	"github.com/jinzhu/gorm"

	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	defaultMonitorInterval      = 3 * time.Second
	scheduleRollbackWaitingTime = 5 * 60 // 5 minute
	retryMaxCount               = 100
)

// Monitor migrator 모니터링 구조체
type Monitor struct {
	subs []broker.Subscriber

	runningStop map[uint64]chan interface{}
	clearStop   map[uint64]chan interface{}
}

// AddLog lock
var addLogLock = sync.Mutex{}

func getJobTask(job *migrator.RecoveryJob, dependencyTask string) (*migrator.RecoveryJobTask, error) {
	clearTask, err := job.GetClearTask(dependencyTask)
	if err == nil {
		return clearTask, nil
	}
	if !errors.Equal(err, migrator.ErrNotFoundClearTask) {
		logger.Errorf("[getJobTask] Could not get clear task: dr.recovery.job/%d/clear_task/%s. Cause: %+v",
			job.RecoveryJobID, dependencyTask, err)
		return nil, err
	}

	task, err := job.GetTask(dependencyTask)
	if err != nil {
		logger.Errorf("[getJobTask] Could not get task: dr.recovery.job/%d/task/%s. Cause: %+v",
			job.RecoveryJobID, dependencyTask, err)
		return nil, err
	}

	return task, nil
}

func setJobStateClearFailed(txn store.Txn, job *migrator.RecoveryJob, rollbackWaitingTime int64) error {
	logger.Infof("[setJobStateClearFailed] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[setJobStateClearFailed] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeClearFailed
	status.RollbackAt = time.Now().Unix() + rollbackWaitingTime

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[setJobStateClearFailed] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[setJobStateClearFailed] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[setJobStateClearFailed] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

func setJobStateReporting(txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[setJobStateReporting] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[setJobStateReporting] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeReporting

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[setJobStateReporting] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[setJobStateReporting] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[setJobStateReporting] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

func setVolumeStatusFailed(txn store.Txn, job *migrator.RecoveryJob, task *migrator.RecoveryJobTask, reason *migrator.Message) error {
	var in migrator.VolumeImportTaskInputData
	if err := task.GetInputData(&in); err != nil {
		logger.Errorf("[setVolumeStatusFailed] Could not get input data: job(%d) volume(%d). Cause: %+v",
			job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status, err := job.GetVolumeStatus(in.VolumeID)
	if err != nil {
		logger.Errorf("[setVolumeStatusFailed] Could not get volume status: dr.recovery.job/%d/result/volume/%d. Cause: %+v",
			job.RecoveryJobID, in.VolumeID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobVolumeStateCodeFailed
	status.ResultCode = constant.VolumeRecoveryResultCodeFailed
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = reason

	if err = job.SetVolumeStatus(txn, in.VolumeID, status); err != nil {
		logger.Errorf("[setVolumeStatusFailed] Could not set volume status: dr.recovery.job/%d/result/volume/%d {%s}. Cause: %+v",
			job.RecoveryJobID, in.VolumeID, status.StateCode, err)
		return err
	}

	logger.Infof("[setVolumeStatusFailed] Done - set volume status: dr.recovery.job/%d/result/volume/%d {%s}.",
		job.RecoveryJobID, in.VolumeID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
		RecoveryJobID: job.RecoveryJobID,
		VolumeID:      in.VolumeID,
		VolumeStatus:  status,
	}); err != nil {
		logger.Warnf("[setVolumeStatusFailed] Could not publish volume(%d) status. Cause: %+v", in.VolumeID, err)
	}

	return nil
}

func setInstanceStatusFailed(txn store.Txn, job *migrator.RecoveryJob, task *migrator.RecoveryJobTask, reason *migrator.Message) error {
	var in migrator.InstanceCreateTaskInputData
	if err := task.GetInputData(&in); err != nil {
		logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not get input data: job(%d) instance(%d). Cause: %+v",
			job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status, err := job.GetInstanceStatus(in.InstanceID)
	if err != nil {
		logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			job.RecoveryJobID, in.InstanceID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobInstanceStateCodeFailed
	status.ResultCode = constant.InstanceRecoveryResultCodeFailed
	status.FinishedAt = time.Now().Unix()
	status.FailedReason = reason

	if err = job.SetInstanceStatus(txn, in.InstanceID, status); err != nil {
		logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not set instance status: dr.recovery.job/%d/result/instance/%d {%s}. Cause: %+v",
			job.RecoveryJobID, in.InstanceID, status.StateCode, err)
		return err
	}

	logger.Infof("[setTaskStatusDoneAndResultFailed] Done - set instance status: dr.recovery.job/%d/result/instance/%d {%s}.",
		job.RecoveryJobID, in.InstanceID, status.StateCode)

	if err = publishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
		RecoveryJobID:  job.RecoveryJobID,
		InstanceID:     in.InstanceID,
		InstanceStatus: status,
	}); err != nil {
		logger.Warnf("[setTaskStatusDoneAndResultFailed] Could not publish instance(%d) status. Cause: %+v", in.InstanceID, err)
	}

	return nil
}

func isDoneRecoveryJobTasks(job *migrator.RecoveryJob) bool {
	tasks, err := job.GetTaskList()
	if err != nil {
		logger.Warnf("[isDoneRecoveryJobTasks] Could not get task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return false
	}

	for _, task := range tasks {
		if status, err := task.GetStatus(); err != nil {
			logger.Warnf("[isDoneRecoveryJobTasks] Could not get job (%d) task (%s) status. Cause: %+v", job.RecoveryJobID, task.RecoveryJobTaskID, err)
			return false

		} else if status.StateCode != constant.MigrationTaskStateCodeDone {
			return false
		}
	}

	return true
}

func isDoneRecoveryJobClearTasks(job *migrator.RecoveryJob) bool {
	tasks, err := job.GetClearTaskList()
	if err != nil {
		logger.Warnf("[isDoneRecoveryJobClearTasks] Could not get clear task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return false
	}

	for _, task := range tasks {
		if status, err := task.GetStatus(); err != nil {
			logger.Warnf("[isDoneRecoveryJobClearTasks] Could not get job (%d) clear task (%s) status. Cause: %+v",
				job.RecoveryJobID, task.RecoveryJobTaskID, err)
			return false

		} else if status.StateCode != constant.MigrationTaskStateCodeDone {
			logger.Infof("[isDoneRecoveryJobClearTasks] Pass >> task(%s) state(%s) - job(%d) taskID(%s)",
				task.TypeCode, status.StateCode, job.RecoveryJobID, task.RecoveryJobTaskID)
			return false
		}
	}

	return true
}

func isSuccessRecoveryJobClearTasks(job *migrator.RecoveryJob) (bool, error) {
	tasks, err := job.GetClearTaskList()
	if err != nil {
		logger.Errorf("[isSuccessRecoveryJobClearTasks] Could not get clear task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return false, err
	}

	for _, task := range tasks {
		if result, err := task.GetResult(); err != nil {
			logger.Errorf("[isSuccessRecoveryJobClearTasks] Could not get job (%d) task (%s) result. Cause: %+v", job.RecoveryJobID, task.RecoveryJobTaskID, err)
			return false, err

		} else if result.ResultCode != constant.MigrationTaskResultCodeSuccess {
			return false, nil
		}
	}

	return true, nil
}

// instance task(stop instance 는 제외) 이면 true
// volume copy 관련 task 이면 true
func isConcurrent(typeCode string) bool {
	return typeCode == constant.MigrationTaskTypeCreateAndDiagnosisInstance || typeCode == constant.MigrationTaskTypeDeleteInstance ||
		typeCode == constant.MigrationTaskTypeCopyVolume || typeCode == constant.MigrationTaskTypeDeleteVolumeCopy || typeCode == constant.MigrationTaskTypeImportVolume
}

func setTaskStatusDoneAndResultFailed(job *migrator.RecoveryJob, task *migrator.RecoveryJobTask, contents string) error {
	logger.Infof("[setTaskStatusDoneAndResultFailed] Start: job(%d) typeCode(%s) task(%s)", task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	msg := &migrator.Message{Code: taskFailedLogCodeMap[task.TypeCode], Contents: contents}

	if err := store.Transaction(func(txn store.Txn) error {
		switch task.TypeCode {
		case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
			if err := setInstanceStatusFailed(txn, job, task, msg); err != nil {
				logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not set volume status failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
					task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return err
			}

		case constant.MigrationTaskTypeImportVolume:
			if err := setVolumeStatusFailed(txn, job, task, msg); err != nil {
				logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not set volume status failed: job(%d) typeCode(%s) task(%s). Cause: %+v",
					task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return err
			}
		}

		if err := task.SetResultFailed(txn, msg.Code, msg.Contents, migrator.WithSharedTask(true)); err != nil {
			logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not set task result (failed): job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}
		logger.Infof("[setTaskStatusDoneAndResultFailed] Done - set task result (failed): job(%d) typeCode(%s) task(%s)",
			task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

		if err := task.SetStatusFinished(txn, migrator.WithSharedTask(true)); err != nil {
			logger.Errorf("[setTaskStatusDoneAndResultFailed] Could not set task status (done): job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}
		logger.Infof("[setTaskStatusDoneAndResultFailed] Done - set task status (done): job(%d) typeCode(%s) task(%s)",
			task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

		seq := getLogSeq(job)
		addLogLock.Lock()
		if err := job.AddLog(txn, seq, msg.Code, msg.Contents); err != nil {
			logger.Warnf("[setTaskStatusDoneAndResultFailed] Could not add log[%d]: code(%s) contents(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				seq, msg.Code, msg.Contents, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		} else {
			logger.Infof("[setTaskStatusDoneAndResultFailed] Done - Add log[%d]: code(%s) contents(%s) job(%d) typeCode(%s) task(%s).",
				seq, msg.Code, msg.Contents, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		}
		addLogLock.Unlock()

		return nil
	}); err != nil {
		return err
	}

	setFailedTaskResultMap(job.RecoveryJobID, task)
	logger.Infof("[setTaskStatusDoneAndResultFailed] Success: job(%d) typeCode(%s) task(%s)", task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return nil
}

// job 을 수행
func (m *Monitor) clearJob(job *migrator.RecoveryJob) error {
	var err error

	jobStatus, err := job.GetStatus()
	if err != nil {
		logger.Warnf("[clearJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[clearJob] Start: job(%d) status(%s)", job.RecoveryJobID, jobStatus.StateCode)

	var taskList jobTaskList
	if jobStatus.StateCode != constant.RecoveryJobStateCodeClearing {
		resetJobInfo(job.RecoveryJobID)
		return err
	}

	updateJobInfo(job.RecoveryJobID)

	// job 의 clear task 목록을 가져온다.
	taskList, err = job.GetClearTaskList()
	if err != nil {
		logger.Warnf("[clearJob] Could not get job clear task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	makeTaskResultMap(job, taskList)

	// task list 정렬
	sort.Sort(&taskList)

	runTask(job, taskList, jobStatus)

	// 재해복구작업정리 완료 여부 확인
	if !isDoneRecoveryJobClearTasks(job) {
		return errors.Unknown(errors.New("task not completed"))
	}

	// 모든 task 가 수행된 job 의 성공 여부를 파악한다.
	success, err := isSuccessRecoveryJobClearTasks(job)
	if err != nil {
		return err
	}

	// job 이 실패인 경우 job clear failed 로 메시지 전송
	if !success {
		logger.Infof("[clearJob] Clear Failed: job(%d) status(%s)", job.RecoveryJobID, jobStatus.StateCode)
		// job clear 가 성공했기 때문에 state 를 report 로 변경한다.
		if err = store.Transaction(func(txn store.Txn) error {
			if err = setJobStateClearFailed(txn, job, scheduleRollbackWaitingTime); err != nil {
				logger.Errorf("[clearJob] Could not set job(%d) status. Cause: &+v", job.RecoveryJobID, err)
				return err
			}

			if err = publishMessage(constant.QueueTriggerClearFailedRecoveryJob, job); err != nil {
				logger.Errorf("[clearJob] Could not publish job(%d). Cause: %+v", job.RecoveryJobID, err)
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	}

	logger.Infof("[clearJob] Clear Success: job(%d) status(%s)", job.RecoveryJobID, jobStatus.StateCode)
	// job clear 가 성공했기 때문에 state 를 report 로 변경한다.
	if err = store.Transaction(func(txn store.Txn) error {
		if err = setJobStateReporting(txn, job); err != nil {
			logger.Errorf("[clearJob] Could not publish job(%d). Cause: %+v", job.RecoveryJobID, err)
			return err
		}

		// job 의 결과 보고서를 생성을 위해 메시지 전송
		if err = publishMessage(constant.QueueTriggerReportingRecoveryJob, job); err != nil {
			logger.Errorf("[clearJob] Could not publish job(%d). Cause: %+v", job.RecoveryJobID, err)
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// job 이 running 과 cancel 상태일때 수행
func (m *Monitor) runJob(job *migrator.RecoveryJob) error {
	var err error

	jobStatus, err := job.GetStatus()
	if err != nil {
		logger.Warnf("[runJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[runJob] Start: job(%d) status(%s)", job.RecoveryJobID, jobStatus.StateCode)

	var taskList jobTaskList
	// job 의 상태가 running, cancel 이 아닌 경우 메모리 정리 및 goroutine 종료
	if jobStatus.StateCode != constant.RecoveryJobStateCodeRunning && jobStatus.StateCode != constant.RecoveryJobStateCodeCanceling {
		resetJobInfo(job.RecoveryJobID)
		return err
	}

	updateJobInfo(job.RecoveryJobID)

	// job 의 task 목록을 가져온다.
	newTaskResultMap(job.RecoveryJobID)
	taskList, err = job.GetTaskList()
	if err != nil {
		logger.Warnf("[runJob] Could not get job task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// task list 정렬
	sort.Sort(&taskList)

	runTask(job, taskList, jobStatus)

	// 재해복구작업 완료 여부 확인
	if !isDoneRecoveryJobTasks(job) {
		logger.Warnf("[runJob] All tasks in the job(%d) are not completed.", job.RecoveryJobID)
		return errors.Unknown(errors.New("task not completed"))
	}

	// 작업이 완료가 된 후 다시 job 의 상태를 구하는 이유는 runJob 함수 처음에 job 의 상태를 구하는 부분에서는 running 일지라도
	// 작업이 완료가 된 시점에는 canceling 상태 일 수도 있기 때문에 다시 구합니다.
	status, err := job.GetStatus()
	if err != nil {
		logger.Warnf("[runJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 작업이 완료 되어 task 모두 done 일 때 manager 로 메시지 전송
	switch status.StateCode {
	case constant.RecoveryJobStateCodeRunning:
		logger.Infof("[runJob] Success: job(%d) status(%s)", job.RecoveryJobID, status.StateCode)
		if err = publishMessage(constant.QueueTriggerDoneRecoveryJob, job); err != nil {
			logger.Errorf("[runJob] Could not publish job(%d). Cause: %+v", job.RecoveryJobID, err)
			return err
		}

	case constant.RecoveryJobStateCodeCanceling:
		logger.Infof("[runJob] Success: job(%d) status(%s)", job.RecoveryJobID, status.StateCode)
		if err = publishMessage(constant.QueueTriggerCancelRecoveryJob, job); err != nil {
			logger.Errorf("[runJob] Could not publish job(%d). Cause: %+v", job.RecoveryJobID, err)
			return err
		}
	}

	return nil
}

func runTask(job *migrator.RecoveryJob, taskList jobTaskList, jobStatus *migrator.RecoveryJobStatus) {
	var wg sync.WaitGroup
	for _, task := range taskList {
		if _, ok := getTaskResultMap(job.RecoveryJobID, task.RecoveryJobTaskID); ok {
			continue
		}

		taskStatus, err := task.GetStatus()
		if err != nil {
			logger.Warnf("[runTask] Could not get task status: job(%d) typeCode(%s) task(%s). Cause: %+v",
				task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			if isConcurrent(task.TypeCode) {
				continue
			}
			return
		}

		logger.Infof("[runTask] runCount(%d) job(%d) typeCode(%s) status(%s) task(%s)",
			getRunCount(job.RecoveryJobID), task.RecoveryJobID, task.TypeCode, taskStatus.StateCode, task.RecoveryJobTaskID)

		if taskStatus.StateCode == constant.MigrationTaskStateCodeDone {
			if _, ok := getTaskResultMap(job.RecoveryJobID, task.RecoveryJobTaskID); !ok {
				setTaskResultMap(job.RecoveryJobID, task)
			}
			continue
		}

		// retryMaxCount 가 넘으면 failed 처리
		if getRunCount(job.RecoveryJobID) > retryMaxCount {
			_ = setTaskStatusDoneAndResultFailed(job, task, "task_run_retry_count_is_passed")
			continue
		}

		// running 인 task 가 동시진행 가능 task 가 아니면 task done 이 되어야 다음 task 진행 가능
		if taskStatus.StateCode == constant.MigrationTaskStateCodeRunning {
			if isConcurrent(task.TypeCode) {
				continue
			}
			return
		}

		if jobStatus.StateCode != constant.RecoveryJobStateCodeCanceling {
			if isTaskFailed(job.RecoveryJobID, task) {
				content := fmt.Sprintf("be_preceded_task_failed")
				_ = setTaskStatusDoneAndResultFailed(job, task, content)
			}

			isRunnable := false
			for _, d := range task.Dependencies {
				// dependency task 가 아직 done 이 아니거나 result 가 success 가 아닌 경우
				if r, ok := getTaskResultMap(job.RecoveryJobID, d); !r {
					// dependency task 의 가 done 이면서 result 가 false 인 경우
					if ok {
						content := fmt.Sprintf("%s.dependency_task_failed", d)
						_ = setTaskStatusDoneAndResultFailed(job, task, content)
					}

					// 동시진행 가능 task 일때는 pass
					if isConcurrent(task.TypeCode) {
						isRunnable = true
						break
					}
					// 동시진행 가능 task 가 아닐땐 다음 task 확인할거 없이 return
					return
				}
			}
			if isRunnable {
				continue
			}
		}

		w := newWorker(job, task)
		if w == nil {
			// worker is already exist
			logger.Infof("[runTask] Worker is already existed: job(%d) task(%s)", job.RecoveryJobID, task.RecoveryJobTaskID)
			continue
		}

		var ctx context.Context
		if err = database.Transaction(func(db *gorm.DB) error {
			ctx, err = helper.DefaultContext(db)
			return err
		}); err != nil {
			return
		}

		if isConcurrent(task.TypeCode) {
			wg.Add(1)
			go w.run(ctx, &wg)
			if task.TypeCode == constant.MigrationTaskTypeImportVolume {
				time.Sleep(1500 * time.Millisecond)
			}
		} else {
			wg.Add(1)
			w.run(ctx, &wg)
		}
	}
	wg.Wait()
}

func (m *Monitor) handleRecoveryJob(job *migrator.RecoveryJob, state string) {
	switch state {
	case constant.RecoveryJobStateCodeRunning:
		for {
			select {
			case <-m.runningStop[job.RecoveryJobID]:
				logger.Infof("[Monitor-handleRecoveryJob] Close Running handler for job(%d).", job.RecoveryJobID)
				resetJobInfo(job.RecoveryJobID)
				return

			case <-time.After(defaultMonitorInterval):
				if err := m.runJob(job); err != nil {
					continue
				}

				close(m.runningStop[job.RecoveryJobID])
			}
		}

	case constant.RecoveryJobStateCodeClearing:
		for {
			select {
			case <-m.clearStop[job.RecoveryJobID]:
				logger.Infof("[Monitor-handleRecoveryJob] Close Clearing handler for job(%d).", job.RecoveryJobID)
				resetJobInfo(job.RecoveryJobID)
				return

			case <-time.After(defaultMonitorInterval):
				if err := m.clearJob(job); err != nil {
					continue
				}

				close(m.clearStop[job.RecoveryJobID])
			}
		}
	}
}

// Run migrator 모니터링 시작 함수
func (m *Monitor) Run(e broker.Event) error {
	var err error
	var msg migrator.RecoveryJob
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Errorf("[Monitor-Run] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	m.runningStop[msg.RecoveryJobID] = make(chan interface{})

	go m.handleRecoveryJob(&msg, constant.RecoveryJobStateCodeRunning)

	go clearMap(&msg)

	return nil
}

// Clear migrator 모니터링 시작 함수
func (m *Monitor) Clear(e broker.Event) error {
	var err error
	var msg migrator.RecoveryJob
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Errorf("[Monitor-Clear] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	m.clearStop[msg.RecoveryJobID] = make(chan interface{})

	go m.handleRecoveryJob(&msg, constant.RecoveryJobStateCodeClearing)

	go clearMap(&msg)

	return nil
}

// Finish migrator 모니터링 시작 함수
func (m *Monitor) Finish(e broker.Event) error {
	var err error
	var msg migrator.RecoveryJob
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Errorf("[Monitor-Finish] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	deleteTaskResultMap(msg.RecoveryJobID)

	go clearMap(&msg)

	return nil
}

// Stop migrator 모니터링 종료 함수
func (m *Monitor) Stop() {
	// broker unsubscribe
	for _, sub := range m.subs {
		if err := sub.Unsubscribe(); err != nil {
			logger.Warnf("[Monitor-Close] Could not unsubscribe queue (%s). Cause: %+v", sub.Topic(), err)
		}
	}

	if m.runningStop != nil {
		for _, stop := range m.runningStop {
			close(stop)
		}
	}
	if m.clearStop != nil {
		for _, cancel := range m.clearStop {
			close(cancel)
		}
	}
}

// Run migrator 모니터링 시작 함수
func (m *Monitor) run() {
	jobList, err := migrator.GetJobList()
	if err != nil {
		logger.Errorf("[Monitor-run] Could not get job list. Cause: %+v", err)
		return
	}

	for _, job := range jobList {
		status, err := job.GetStatus()
		if err != nil {
			logger.Errorf("[Monitor-run] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
			return
		}

		switch status.StateCode {
		case constant.RecoveryJobStateCodeRunning, constant.RecoveryJobStateCodeCanceling:
			logger.Infof("[Monitor-run] A restart is initiated for the job(%d) that was in progress in a state(%s).", job.RecoveryJobID, status.StateCode)
			if err = publishMessage(constant.QueueTriggerRunRecoveryJob, job); err != nil {
				logger.Errorf("[Monitor-run] Could not publish job(%d) running. Cause: %+v", job.RecoveryJobID, err)
				return
			}

		case constant.RecoveryJobStateCodeClearing:
			logger.Infof("[Monitor-run] A restart is initiated for the job(%d) that was in progress in a state(%s).", job.RecoveryJobID, status.StateCode)
			if err = publishMessage(constant.QueueTriggerClearingRecoveryJob, job); err != nil {
				logger.Errorf("[Monitor-run] Could not publish job(%d) running. Cause: %+v", job.RecoveryJobID, err)
				return
			}

		case constant.RecoveryJobStateCodeFinished:
			logger.Infof("[Monitor-run] A restart is initiated for the job(%d) that was in progress in a state(%s).", job.RecoveryJobID, status.StateCode)
			deleteTaskResultMap(job.RecoveryJobID)
			fallthrough

		default:
			resetJobInfo(job.RecoveryJobID)
		}
	}
}

// NewMigratorMonitor migrator 모니터링 구조체 생성 함수
func NewMigratorMonitor() (*Monitor, error) {
	m := &Monitor{runningStop: make(map[uint64]chan interface{}), clearStop: make(map[uint64]chan interface{})}

	for topic, handlerFunc := range map[string]broker.Handler{
		constant.QueueTriggerRunRecoveryJob:      m.Run,
		constant.QueueTriggerClearingRecoveryJob: m.Clear,
		constant.QueueTriggerFinishedRecoveryJob: m.Finish,
	} {
		logger.Debugf("Subscribe cluster event (%s).", topic)

		sub, err := broker.SubscribePersistentQueue(topic, handlerFunc, true)
		if err != nil {
			logger.Errorf("[Monitor-NewMigratorMonitor] Could not start broker. Cause: %+v", errors.UnusableBroker(err))
			return nil, errors.UnusableBroker(err)
		}

		m.subs = append(m.subs, sub)
	}

	// 서비스 재시작 혹은 처음 시작할 때 실행할 Job 이 있다면 실행
	go m.run()

	return m, nil
}
