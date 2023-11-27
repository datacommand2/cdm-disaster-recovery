package handler

import (
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	recoveryJob "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_job"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"

	"github.com/jinzhu/gorm"

	"context"
	"encoding/json"
	"fmt"
	"path"
	"runtime/debug"
	"strings"
	"time"
)

// 메모리에 저장할 Key 값. migrator 에 저장된 값과 똑같지만 handler 에 따로 선언함
const (
	recoveryJobKeyPrefix          = "dr.recovery.job/%d/"
	recoveryJobTaskKeyFormat      = "dr.recovery.job/%d/task/%s"
	recoveryJobClearTaskKeyFormat = "dr.recovery.job/%d/clear_task/%s"

	recoveryJobDetailKeyFormat    = "dr.recovery.job/%d/detail"
	recoveryJobOperationKeyFormat = "dr.recovery.job/%d/operation"
	recoveryJobStatusKeyFormat    = "dr.recovery.job/%d/status"
	recoveryJobResultKeyFormat    = "dr.recovery.job/%d/result"

	recoveryJobVolumeStatusKeyFormat   = "dr.recovery.job/%d/result/volume/%d"
	recoveryJobInstanceStatusKeyFormat = "dr.recovery.job/%d/result/instance/%d"

	migrationTaskTypeCreate = "dr.migration.task.create"
	migrationTaskTypeDelete = "dr.migration.task.delete"
)

// AddRecoveryJobQueue 재해복구작업을 대기열에 추가하는 함수
func (h *DisasterRecoveryManagerHandler) AddRecoveryJobQueue(e broker.Event) error {
	var err error
	var msg internal.ScheduleMessage
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Errorf("[Monitor-AddRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Transaction(func(db *gorm.DB) error {
		if ctx, err = helper.DefaultContext(db); err != nil {
			return err
		}

		if err = validateRequest(ctx); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return createError(ctx, "cdm-dr.manager.add_recovery_job_queue.failure-add", err)
	}

	job, err := recoveryJob.AddQueue(ctx, msg.ProtectionGroupID, msg.RecoveryJobID)
	if err != nil {
		switch {
		case errors.Equal(err, errors.ErrUnauthorizedRequest),
			errors.Equal(err, internal.ErrNotFoundProtectionGroup),
			errors.Equal(err, internal.ErrNotFoundRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundTenantRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundAvailabilityZoneRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundExternalNetworkRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundStorageRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundVolumeRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundInstanceRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundFloatingIPRecoveryPlan),
			errors.Equal(err, internal.ErrNotFoundRecoveryJob),
			errors.Equal(err, internal.ErrNotRunnableRecoveryJob),
			errors.Equal(err, internal.ErrNotFoundRecoverySecurityGroup):
			logger.Warnf("[Monitor-AddRecoveryJobQueue] Canceled adding recovery job(%d) queue: group(%d). Cause: %+v",
				msg.RecoveryJobID, msg.ProtectionGroupID, err)
			return nil

		default:
			logger.Errorf("[Monitor-AddRecoveryJobQueue] Could not add recovery job(%d) queue: group(%d). Cause: %+v",
				msg.RecoveryJobID, msg.ProtectionGroupID, err)
			return createError(ctx, "cdm-dr.manager.add_recovery_job_queue.failure-add", err)
		}
	}

	if job == nil {
		// 같은 id 의 job 이 이미 존재함
		return nil
	}

	// job 상태가 waiting, pending, running 상태를 관리한다.
	go recoveryJob.HandleRunningRecoveryJob(ctx, job)

	logger.Infof("[Monitor-AddRecoveryJobQueue] Add recovery job(%d) queue: group(%d)", msg.RecoveryJobID, msg.ProtectionGroupID)
	return nil
}

// WaitRecoveryJobQueue 재해복구작업의 상태를 waiting 과 pending, running 으로 바꿔주는 함수를 실행하는 함수
func (h *DisasterRecoveryManagerHandler) WaitRecoveryJobQueue(e broker.Event) error {
	var (
		err error
		msg migrator.RecoveryJob
	)
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Warnf("[Monitor-WaitRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		logger.Warnf("[Monitor-WaitRecoveryJobQueue] Context Error. Cause: %v", err)
		return nil
	}

	// job 상태가 waiting, pending, running 상태를 관리한다.
	go recoveryJob.HandleRunningRecoveryJob(ctx, &msg)

	return nil
}

// DoneRecoveryJobQueue 재해복구작업이 완료되어 성공, 실패 확인 후 job 상태 complete 변경
func (h *DisasterRecoveryManagerHandler) DoneRecoveryJobQueue(e broker.Event) error {
	var (
		err error
		msg migrator.RecoveryJob
	)
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Warnf("[Monitor-DoneRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		logger.Warnf("[Monitor-DoneRecoveryJobQueue] Context Error. Cause: %v", err)
		return nil
	}

	logger.Infof("[Monitor-DoneRecoveryJobQueue] Run - job done: group(%d) job(%d)", msg.ProtectionGroupID, msg.RecoveryJobID)
	go recoveryJob.HandleDoneRecoveryJob(ctx, &msg)

	return nil
}

// CancelRecoveryJobQueue 재해복구작업 상태가 cancel 일때 작업 수행
func (h *DisasterRecoveryManagerHandler) CancelRecoveryJobQueue(e broker.Event) error {
	var (
		err error
		msg migrator.RecoveryJob
	)
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Warnf("[Monitor-CancelRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		logger.Warnf("[Monitor-CancelRecoveryJobQueue] Context Error. Cause: %v", err)
		return nil
	}

	logger.Infof("[Monitor-CancelRecoveryJobQueue] Run - job cancel: group(%d) job(%d)", msg.ProtectionGroupID, msg.RecoveryJobID)
	go recoveryJob.HandleCancelingRecoveryJob(ctx, &msg)

	return nil
}

// ClearFailedRecoveryJobQueue 재해복구작업 clear 가 완료되었지만 실패하여 job 상태를 clear failed 으로 변경 후 작업
func (h *DisasterRecoveryManagerHandler) ClearFailedRecoveryJobQueue(e broker.Event) error {
	var (
		err error
		msg migrator.RecoveryJob
	)
	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Warnf("[Monitor-ClearFailedRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		logger.Warnf("[Monitor-ClearFailedRecoveryJobQueue] Context Error. Cause: %v", err)
		return nil
	}

	logger.Infof("[Monitor-ClearFailedRecoveryJobQueue] Run - job clear failed: group(%d) job(%d)", msg.ProtectionGroupID, msg.RecoveryJobID)
	// ClearFailed 상태에서 RetryRollback, IgnoreRollback, RetryConfirm, CancelConfirm operation 대기
	go recoveryJob.HandleRecoveryJob(ctx, &msg, constant.RecoveryJobStateCodeClearFailed)

	return nil
}

// ReportRecoveryJobQueue 재해복구작업 clear 가 완료되어 job 상태를 reporting 으로 변경 후 리포트 생성
func (h *DisasterRecoveryManagerHandler) ReportRecoveryJobQueue(e broker.Event) error {
	var (
		err error
		msg migrator.RecoveryJob
	)

	if err = json.Unmarshal(e.Message().Body, &msg); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Warnf("[Monitor-CancelRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		logger.Warnf("[Monitor-CancelRecoveryJobQueue] Context Error. Cause: %v", err)
		return nil
	}

	// Reporting 상태에서 reporting, finished 동작
	go recoveryJob.HandelFinishRecoveryJob(ctx, &msg)

	return nil
}

// RecoveryJobStatusMonitor 재해복구작업에서 Job 의 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobStatusMonitor(e broker.Event) error {
	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-RecoveryJobStatusMonitor] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	var msg migrator.RecoveryJobMessage
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobStatusMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	if msg.Status != nil {
		h.Status[fmt.Sprintf(recoveryJobStatusKeyFormat, msg.JobID)] = &migrator.RecoveryJobStatus{
			StateCode:   msg.Status.StateCode,
			StartedAt:   msg.Status.StartedAt,
			PausedTime:  msg.Status.PausedTime,
			FinishedAt:  msg.Status.FinishedAt,
			ElapsedTime: msg.Status.ElapsedTime,
			PausedAt:    msg.Status.PausedAt,
			ResumeAt:    msg.Status.ResumeAt,
			RollbackAt:  msg.Status.RollbackAt,
		}
		logger.Infof("[Monitor-RecoveryJobStatusMonitor] Completed saving the Job(%d) Status(%s) to memory.", msg.JobID, msg.Status.StateCode)
	}

	if msg.Operation != nil {
		h.Operation[fmt.Sprintf(recoveryJobOperationKeyFormat, msg.JobID)] = &migrator.RecoveryJobOperation{Operation: msg.Operation.Operation}
		logger.Infof("[Monitor-RecoveryJobStatusMonitor] Completed saving the Job(%d) Operation(%s) to memory.", msg.JobID, msg.Operation.Operation)
	}

	if msg.Detail != nil {
		// Detail 에 값을 한번에 넣은 이유는 들어오는 값이 무조건 drms.RecoveryJob 값 전체가 들어오기 때문
		h.Detail[fmt.Sprintf(recoveryJobDetailKeyFormat, msg.JobID)] = &drms.RecoveryJob{
			Id:                    msg.Detail.Id,
			Operator:              msg.Detail.Operator,
			Group:                 msg.Detail.Group,
			Plan:                  msg.Detail.Plan,
			TypeCode:              msg.Detail.TypeCode,
			RecoveryPointTypeCode: msg.Detail.RecoveryPointTypeCode,
			RecoveryPointSnapshot: msg.Detail.RecoveryPointSnapshot,
			Schedule:              msg.Detail.Schedule,
			NextRuntime:           msg.Detail.NextRuntime,
			OperationCode:         msg.Detail.OperationCode,
			StateCode:             msg.Detail.StateCode,
			CreatedAt:             msg.Detail.CreatedAt,
			UpdatedAt:             msg.Detail.UpdatedAt,
		}
		logger.Infof("[Monitor-RecoveryJobStatusMonitor] Completed saving the Job(%d) Detail to memory.", msg.JobID)
	}

	if msg.Result != nil {
		h.Result[fmt.Sprintf(recoveryJobResultKeyFormat, msg.JobID)] = &migrator.RecoveryJobResult{
			ResultCode:     msg.Result.ResultCode,
			WarningReasons: msg.Result.WarningReasons,
			FailedReasons:  msg.Result.FailedReasons,
		}
		logger.Infof("[Monitor-RecoveryJobStatusMonitor] Completed saving the Job(%d) Result(%s) to memory.", msg.JobID, msg.Result.ResultCode)
	}

	return nil
}

// RecoveryJobTaskMonitor 재해복구작업에서 Task 의 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobTaskMonitor(e broker.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-RecoveryJobTaskMonitor] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	var msg migrator.RecoveryJobTask
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobTaskMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return err
	}

	h.Task[msg.RecoveryJobTaskKey] = &migrator.RecoveryJobTask{
		RecoveryJobTaskKey: msg.RecoveryJobTaskKey,
		RecoveryJobID:      msg.RecoveryJobID,
		RecoveryJobTaskID:  msg.RecoveryJobTaskID,
		ReverseTaskID:      msg.ReverseTaskID,
		TypeCode:           msg.TypeCode,
		ResourceID:         msg.ResourceID,
		ResourceName:       msg.ResourceName,
		Input:              msg.Input,
	}

	return nil
}

// RecoveryJobClearTaskMonitor 재해복구작업에서 ClearTask 의 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobClearTaskMonitor(e broker.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-RecoveryJobClearTaskMonitor] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	var msg migrator.RecoveryJobTask
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobClearTaskMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return err
	}

	h.ClearTask[msg.RecoveryJobTaskKey] = &migrator.RecoveryJobTask{
		RecoveryJobTaskKey: msg.RecoveryJobTaskKey,
		RecoveryJobID:      msg.RecoveryJobID,
		RecoveryJobTaskID:  msg.RecoveryJobTaskID,
		ReverseTaskID:      msg.ReverseTaskID,
		TypeCode:           msg.TypeCode,
		ResourceID:         msg.ResourceID,
		ResourceName:       msg.ResourceName,
		Input:              msg.Input,
	}

	return nil
}

// RecoveryJobTaskStatusMonitor 재해복구작업에서 Task 의 상태 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobTaskStatusMonitor(e broker.Event) error {
	var msg migrator.RecoveryJobTaskStatusMessage
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobTaskStatusMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return err
	}

	h.TaskStatus[msg.RecoveryJobTaskStatusKey] = &migrator.RecoveryJobTaskStatus{
		StateCode:  msg.RecoveryJobTaskStatus.StateCode,
		StartedAt:  msg.RecoveryJobTaskStatus.StartedAt,
		FinishedAt: msg.RecoveryJobTaskStatus.FinishedAt,
	}

	logger.Infof("[Monitor-RecoveryJobTaskStatusMonitor] Completed saving the Task(%s) Status(%s) to memory.", msg.TypeCode, msg.RecoveryJobTaskStatus.StateCode)

	return nil
}

// RecoveryJobTaskResultMonitor 재해복구작업에서 Task 의 결과 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobTaskResultMonitor(e broker.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-RecoveryJobTaskResultMonitor] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	var msg migrator.RecoveryJobTaskResultMessage
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobTaskResultMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return err
	}

	if !msg.IsDeleted {
		h.TaskResult[msg.RecoveryJobTaskResultKey] = &migrator.RecoveryJobTaskResult{
			ResultCode:   msg.RecoveryJobTaskResult.ResultCode,
			FailedReason: msg.RecoveryJobTaskResult.FailedReason,
			Output:       msg.RecoveryJobTaskResult.Output,
		}

		logger.Infof("[Monitor-RecoveryJobTaskResultMonitor] Completed saving the Task(%s) Result(%s) to memory.", msg.TypeCode, msg.RecoveryJobTaskResult.ResultCode)

	} else {
		// IsDeleted 가 true 면, 데이터 삭제
		if _, ok := h.TaskResult[msg.RecoveryJobTaskResultKey]; ok {
			delete(h.TaskResult, msg.RecoveryJobTaskResultKey)
		}
		logger.Infof("[Monitor-RecoveryJobTaskResultMonitor] Completed deleting the Task(%s) Result to memory.", msg.TypeCode)
	}

	return nil
}

// DeleteRecoveryJobQueue 재해복구작업이 삭제되었을때 메모리 데이터 삭제
func (h *DisasterRecoveryManagerHandler) DeleteRecoveryJobQueue(e broker.Event) error {
	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-DeleteRecoveryJobQueue] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	var err error
	var RecoveryJobID uint64
	if err = json.Unmarshal(e.Message().Body, &RecoveryJobID); err != nil {
		// 파싱할 수 없는 메시지라면 다른 subscriber 도 파싱할 수 없으므로
		// 에러 로깅만 하고 requeue 는 하지 않음
		logger.Errorf("[Monitor-DeleteRecoveryJobQueue] Could not parse broker message. Cause: %+v", err)
		return nil
	}

	if _, ok := h.Status[fmt.Sprintf(recoveryJobStatusKeyFormat, RecoveryJobID)]; ok {
		delete(h.Status, fmt.Sprintf(recoveryJobStatusKeyFormat, RecoveryJobID))
	}
	if _, ok := h.Operation[fmt.Sprintf(recoveryJobOperationKeyFormat, RecoveryJobID)]; ok {
		delete(h.Operation, fmt.Sprintf(recoveryJobOperationKeyFormat, RecoveryJobID))
	}
	if _, ok := h.Result[fmt.Sprintf(recoveryJobResultKeyFormat, RecoveryJobID)]; ok {
		delete(h.Result, fmt.Sprintf(recoveryJobResultKeyFormat, RecoveryJobID))
	}
	if _, ok := h.Detail[fmt.Sprintf(recoveryJobDetailKeyFormat, RecoveryJobID)]; ok {
		delete(h.Detail, fmt.Sprintf(recoveryJobDetailKeyFormat, RecoveryJobID))
	}

	// SecurityGroup, Instance, Volume, Task 를 저장한 Key 값들은 RecoveryJobID 말고 다른 ID 값도 필요하지만
	// 기본값이 되는 RecoveryJob 이 삭제가 되면 다 삭제하기 위한 방법
	if h.VolumeStatus != nil {
		for k := range h.VolumeStatus {
			if strings.HasPrefix(k, fmt.Sprintf(recoveryJobKeyPrefix, RecoveryJobID)) {
				if _, ok := h.VolumeStatus[k]; ok {
					delete(h.VolumeStatus, k)
				}
			}
		}
	}

	if h.InstanceStatus != nil {
		for k := range h.InstanceStatus {
			if strings.HasPrefix(k, fmt.Sprintf(recoveryJobKeyPrefix, RecoveryJobID)) {
				if _, ok := h.InstanceStatus[k]; ok {
					delete(h.InstanceStatus, k)
				}
			}
		}
	}

	if h.Task != nil {
		for k := range h.Task {
			if strings.HasPrefix(k, fmt.Sprintf(recoveryJobKeyPrefix, RecoveryJobID)) {
				if _, ok := h.Task[k]; ok {
					delete(h.Task, k)
				}
				if _, ok := h.ClearTask[k]; ok {
					delete(h.ClearTask, k)
				}
				if _, ok := h.TaskStatus[k]; ok {
					delete(h.TaskStatus, k)
				}
				if _, ok := h.TaskResult[k]; ok {
					delete(h.TaskResult, k)
				}
			}
		}
	}

	logger.Infof("[Monitor-DeleteRecoveryJobQueue] Completed deleting the Job(%d) to memory.", RecoveryJobID)

	return nil
}

// RecoveryJobVolumeMonitor 재해복구작업에서 Volume 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobVolumeMonitor(e broker.Event) error {
	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-RecoveryJobVolumeMonitor] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	var msg migrator.RecoveryJobVolumeMessage
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobVolumeMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return err
	}

	h.VolumeStatus[fmt.Sprintf(recoveryJobVolumeStatusKeyFormat, msg.RecoveryJobID, msg.VolumeID)] = &migrator.RecoveryJobVolumeStatus{
		RecoveryJobTaskID:     msg.VolumeStatus.RecoveryJobTaskID,
		RecoveryPointTypeCode: msg.VolumeStatus.RecoveryPointTypeCode,
		RecoveryPoint:         msg.VolumeStatus.RecoveryPoint,
		StateCode:             msg.VolumeStatus.StateCode,
		ResultCode:            msg.VolumeStatus.ResultCode,
		StartedAt:             msg.VolumeStatus.StartedAt,
		FinishedAt:            msg.VolumeStatus.FinishedAt,
		FailedReason:          msg.VolumeStatus.FailedReason,
	}

	logger.Infof("[Monitor-RecoveryJobVolumeMonitor] Completed saving the Volume(%d) status(%s), result(%s) to memory.",
		msg.VolumeID, msg.VolumeStatus.StateCode, msg.VolumeStatus.ResultCode)

	return nil
}

// RecoveryJobInstanceMonitor 재해복구작업에서 Instance 정보를 메모리에 저장하는 함수
func (h *DisasterRecoveryManagerHandler) RecoveryJobInstanceMonitor(e broker.Event) error {
	var msg migrator.RecoveryJobInstanceMessage
	if err := json.Unmarshal(e.Message().Body, &msg); err != nil {
		logger.Errorf("[Monitor-RecoveryJobInstanceMonitor] Error occurred during unmarshal the message. Cause: %+v", err)
		return err
	}

	h.InstanceStatus[fmt.Sprintf(recoveryJobInstanceStatusKeyFormat, msg.RecoveryJobID, msg.InstanceID)] = &migrator.RecoveryJobInstanceStatus{
		RecoveryJobTaskID:     msg.InstanceStatus.RecoveryJobTaskID,
		RecoveryPointTypeCode: msg.InstanceStatus.RecoveryPointTypeCode,
		RecoveryPoint:         msg.InstanceStatus.RecoveryPoint,
		StateCode:             msg.InstanceStatus.StateCode,
		ResultCode:            msg.InstanceStatus.ResultCode,
		StartedAt:             msg.InstanceStatus.StartedAt,
		FinishedAt:            msg.InstanceStatus.FinishedAt,
		FailedReason:          msg.InstanceStatus.FailedReason,
	}

	logger.Infof("[Monitor-RecoveryJobInstanceMonitor] Completed saving the Instance(%d) status(%s), result(%s) to memory.",
		msg.InstanceID, msg.InstanceStatus.StateCode, msg.InstanceStatus.ResultCode)

	return nil
}

// getMemoryRecoveryJobStatus 재해복구작업에서 Job 의 정보를 메모리에서 가져와 출력하는 함수
func (h *DisasterRecoveryManagerHandler) getMemoryRecoveryJobStatus(jobID uint64) *drms.RecoveryJobStatus {
	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-getMemoryRecoveryJobStatus] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	op, ok := h.Operation[fmt.Sprintf(recoveryJobOperationKeyFormat, jobID)]
	if !ok {
		logger.Warnf("[Monitor-getMemoryRecoveryJobStatus] Could not get job(%d) operation from memory data. Cause: no data in memory", jobID)
		return nil
	}

	s, ok := h.Status[fmt.Sprintf(recoveryJobStatusKeyFormat, jobID)]
	if !ok {
		s = &migrator.RecoveryJobStatus{StateCode: constant.RecoveryJobStateCodeWaiting}
	}

	if s.StartedAt != 0 {
		now := time.Now().Unix() - s.PausedAt // PausedAt 은 일시중지 중에만 값이 있다
		if s.FinishedAt != 0 {
			now = s.FinishedAt
		}

		s.ElapsedTime = now - s.StartedAt - s.PausedTime
	}

	status := &drms.RecoveryJobStatus{
		StartedAt:     s.StartedAt,
		FinishedAt:    s.FinishedAt,
		ElapsedTime:   s.ElapsedTime,
		OperationCode: op.Operation,
		StateCode:     s.StateCode,
		ResumeAt:      s.ResumeAt,
		RollbackAt:    s.RollbackAt,
	}

	r, ok := h.Result[fmt.Sprintf(recoveryJobResultKeyFormat, jobID)]
	if !ok {
		logger.Warnf("[Monitor-getMemoryRecoveryJobStatus] Could not get recovery job(%d) result from memory data. Cause: no data in memory", jobID)
		return status
	}

	status.ResultCode = r.ResultCode

	if len(r.WarningReasons) > 0 {
		status.WarningFlag = true
	}

	for _, w := range r.WarningReasons {
		if w != nil {
			status.WarningReasons = append(status.WarningReasons, &drms.Message{
				Code:     w.Code,
				Contents: w.Contents,
			})
		} else {
			logger.Warn("[Monitor-getMemoryRecoveryJobStatus] WarningReasons is nil")
		}

	}

	for _, f := range r.FailedReasons {
		if f != nil {
			status.FailedReasons = append(status.FailedReasons, &drms.Message{
				Code:     f.Code,
				Contents: f.Contents,
			})
		} else {
			logger.Warn("[Monitor-getMemoryRecoveryJobStatus] FailedReasons is nil")
		}
	}

	return status
}

// getMemoryTask 재해복구작업에서 Task 의 정보를 메모리에서 가져와 출력하는 함수
func (h *DisasterRecoveryManagerHandler) getMemoryTask(jobID uint64) ([]*drms.RecoveryJobTenantStatus, []*drms.RecoveryJobSecurityGroupStatus, []*drms.RecoveryJobNetworkStatus, []*drms.RecoveryJobSubnetStatus,
	[]*drms.RecoveryJobFloatingIPStatus, []*drms.RecoveryJobRouterStatus, []*drms.RecoveryJobKeypairStatus, []*drms.RecoveryJobSpecStatus) {
	var (
		tenants        []*drms.RecoveryJobTenantStatus
		securityGroups []*drms.RecoveryJobSecurityGroupStatus
		networks       []*drms.RecoveryJobNetworkStatus
		subnets        []*drms.RecoveryJobSubnetStatus
		floatingIps    []*drms.RecoveryJobFloatingIPStatus
		routers        []*drms.RecoveryJobRouterStatus
		keypairs       []*drms.RecoveryJobKeypairStatus
		specs          []*drms.RecoveryJobSpecStatus
	)

	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-getMemoryTask] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	if h.Task != nil {
		for k, v := range h.Task {
			var jid uint64
			var tid string

			// 메모리에 저장된 Task 중에서 jobID가 같은 Key 값의 정보를 가져온다.
			if _, err := fmt.Sscanf(k, recoveryJobTaskKeyFormat, &jid, &tid); err != nil {
				continue
			}

			if jid == jobID {
				switch v.TypeCode {
				case constant.MigrationTaskTypeCreateTenant:
					tenants = append(tenants, h.getTenant(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateSecurityGroup:
					securityGroups = append(securityGroups, h.getSecurityGroup(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateNetwork:
					networks = append(networks, h.getNetwork(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateSubnet:
					subnets = append(subnets, h.getSubnet(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateFloatingIP:
					floatingIps = append(floatingIps, h.getFloatingIp(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateRouter:
					routers = append(routers, h.getRouter(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateKeypair:
					keypairs = append(keypairs, h.getKeypair(v, migrationTaskTypeCreate))

				case constant.MigrationTaskTypeCreateSpec:
					specs = append(specs, h.getSpec(v, migrationTaskTypeCreate))
				}
			}
		}
	}

	return tenants, securityGroups, networks, subnets, floatingIps, routers, keypairs, specs
}

// getMemoryClearTask 재해복구작업에서 ClearTask 의 정보를 메모리에서 가져와 출력하는 함수
func (h *DisasterRecoveryManagerHandler) getMemoryClearTask(jobID uint64) ([]*drms.RecoveryJobTenantStatus, []*drms.RecoveryJobSecurityGroupStatus,
	[]*drms.RecoveryJobNetworkStatus, []*drms.RecoveryJobSubnetStatus, []*drms.RecoveryJobFloatingIPStatus, []*drms.RecoveryJobRouterStatus,
	[]*drms.RecoveryJobVolumeStatus, []*drms.RecoveryJobKeypairStatus, []*drms.RecoveryJobSpecStatus, []*drms.RecoveryJobInstanceStatus) {
	var (
		tenants        []*drms.RecoveryJobTenantStatus
		securityGroups []*drms.RecoveryJobSecurityGroupStatus
		networks       []*drms.RecoveryJobNetworkStatus
		subnets        []*drms.RecoveryJobSubnetStatus
		floatingIps    []*drms.RecoveryJobFloatingIPStatus
		routers        []*drms.RecoveryJobRouterStatus
		volumes        []*drms.RecoveryJobVolumeStatus
		keypairs       []*drms.RecoveryJobKeypairStatus
		specs          []*drms.RecoveryJobSpecStatus
		instances      []*drms.RecoveryJobInstanceStatus
	)

	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-getMemoryClearTask] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	if h.ClearTask != nil {
		for k, v := range h.ClearTask {
			var jid uint64
			var tid string

			if _, err := fmt.Sscanf(k, recoveryJobClearTaskKeyFormat, &jid, &tid); err != nil {
				continue
			}

			if jid == jobID {
				switch v.TypeCode {
				case constant.MigrationTaskTypeDeleteTenant:
					tenants = append(tenants, h.getTenant(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeDeleteSecurityGroup:
					securityGroups = append(securityGroups, h.getSecurityGroup(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeDeleteNetwork:
					networks = append(networks, h.getNetwork(v, migrationTaskTypeDelete))
					// Subnet 은 Network 삭제 Task 에서 같이 처리함

				case constant.MigrationTaskTypeDeleteFloatingIP:
					floatingIps = append(floatingIps, h.getFloatingIp(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeDeleteRouter:
					routers = append(routers, h.getRouter(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeUnmanageVolume:
					volumes = append(volumes, h.getVolume(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeDeleteKeypair:
					keypairs = append(keypairs, h.getKeypair(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeDeleteSpec:
					specs = append(specs, h.getSpec(v, migrationTaskTypeDelete))

				case constant.MigrationTaskTypeDeleteInstance:
					instances = append(instances, h.getInstance(v, migrationTaskTypeDelete))
				}
			}
		}
	}

	return tenants, securityGroups, networks, subnets, floatingIps, routers, volumes, keypairs, specs, instances
}

func (h *DisasterRecoveryManagerHandler) getMemoryInstancesStatus(jobID uint64) []*drms.RecoveryJobInstanceStatus {
	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-getMemoryInstancesStatus] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	j, ok := h.Detail[fmt.Sprintf(recoveryJobDetailKeyFormat, jobID)]
	if !ok {
		logger.Warnf("[Monitor-getMemoryInstancesStatus] Could not get job(%d) detail from memory data. Cause: no data in memory", jobID)
		return nil
	}

	var statusList []*drms.RecoveryJobInstanceStatus
	for _, i := range j.Plan.Detail.Instances {
		s, ok := h.InstanceStatus[fmt.Sprintf(recoveryJobInstanceStatusKeyFormat, jobID, i.ProtectionClusterInstance.Id)]
		if !ok {
			logger.Warnf("[Monitor-getMemoryInstancesStatus] Could not get job(%d) instance(%d) status from memory data. Cause: no data in memory",
				jobID, i.ProtectionClusterInstance.Id)
			return nil
		}

		if s.StartedAt != 0 {
			now := time.Now().Unix()
			if s.FinishedAt != 0 {
				now = s.FinishedAt
			}

			s.ElapsedTime = now - s.StartedAt
		}

		status := drms.RecoveryJobInstanceStatus{
			Instance:              i.ProtectionClusterInstance,
			RecoveryPointTypeCode: s.RecoveryPointTypeCode,
			RecoveryPoint:         s.RecoveryPoint,
			ElapsedTime:           s.ElapsedTime,
			StartedAt:             s.StartedAt,
			FinishedAt:            s.FinishedAt,
			StateCode:             s.StateCode,
			ResultCode:            s.ResultCode,
		}

		if s.FailedReason != nil {
			status.FailedReason = &drms.Message{
				Code:     s.FailedReason.Code,
				Contents: s.FailedReason.Contents,
			}
		}

		statusList = append(statusList, &status)
	}

	return statusList
}

func (h *DisasterRecoveryManagerHandler) getMemoryVolumesStatus(jobID uint64) []*drms.RecoveryJobVolumeStatus {
	defer func() {
		// panic 이 발생해서 서비스가 재시작되는 걸 방지
		if r := recover(); r != nil {
			logger.Errorf("[Monitor-getMemoryVolumesStatus] Panic occurred during execution. Cause: %+v. %s", r, debug.Stack())
		}
	}()

	j, ok := h.Detail[fmt.Sprintf(recoveryJobDetailKeyFormat, jobID)]
	if !ok {
		logger.Warnf("[Monitor-getMemoryVolumesStatus] Could not get job(%d) detail from memory data. Cause: no data in memory", jobID)
		return nil
	}

	var statusList []*drms.RecoveryJobVolumeStatus
	for _, v := range j.Plan.Detail.Volumes {
		s, ok := h.VolumeStatus[fmt.Sprintf(recoveryJobVolumeStatusKeyFormat, jobID, v.ProtectionClusterVolume.Id)]
		if !ok {
			logger.Warnf("[Monitor-getMemoryVolumesStatus] Could not get job(%d) volume(%d) status from memory data. Cause: no data in memory",
				jobID, v.ProtectionClusterVolume.Id)
			return nil
		}

		if s.StartedAt != 0 {
			now := time.Now().Unix()
			if s.FinishedAt != 0 {
				now = s.FinishedAt
			}

			s.ElapsedTime = now - s.StartedAt
		}

		status := drms.RecoveryJobVolumeStatus{
			Volume:                v.ProtectionClusterVolume,
			RecoveryPointTypeCode: s.RecoveryPointTypeCode,
			RecoveryPoint:         s.RecoveryPoint,
			ElapsedTime:           s.ElapsedTime,
			StartedAt:             s.StartedAt,
			FinishedAt:            s.FinishedAt,
			StateCode:             s.StateCode,
			ResultCode:            s.ResultCode,
		}

		if s.FailedReason != nil {
			status.FailedReason = &drms.Message{
				Code:     s.FailedReason.Code,
				Contents: s.FailedReason.Contents,
			}
		}

		statusList = append(statusList, &status)
	}

	return statusList
}

func (h *DisasterRecoveryManagerHandler) getTenant(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobTenantStatus {
	var tenant *drms.RecoveryJobTenantStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		tenant = &drms.RecoveryJobTenantStatus{
			Tenant:      &cms.ClusterTenant{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		tenant.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			tenant.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return tenant
}

func (h *DisasterRecoveryManagerHandler) getSecurityGroup(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobSecurityGroupStatus {
	var securityGroup *drms.RecoveryJobSecurityGroupStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		securityGroup = &drms.RecoveryJobSecurityGroupStatus{
			SecurityGroup: &cms.ClusterSecurityGroup{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime:   status.ElapsedTime,
			StartedAt:     status.StartedAt,
			FinishedAt:    status.FinishedAt,
			StateCode:     status.StateCode,
			TypeCode:      typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		securityGroup.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			securityGroup.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return securityGroup
}

func (h *DisasterRecoveryManagerHandler) getNetwork(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobNetworkStatus {
	var network *drms.RecoveryJobNetworkStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		network = &drms.RecoveryJobNetworkStatus{
			Network:     &cms.ClusterNetwork{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		network.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			network.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return network
}

func (h *DisasterRecoveryManagerHandler) getSubnet(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobSubnetStatus {
	var subnet *drms.RecoveryJobSubnetStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		subnet = &drms.RecoveryJobSubnetStatus{
			Subnet:      &cms.ClusterSubnet{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		subnet.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			subnet.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return subnet
}

func (h *DisasterRecoveryManagerHandler) getFloatingIp(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobFloatingIPStatus {
	var floatingIp *drms.RecoveryJobFloatingIPStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		floatingIp = &drms.RecoveryJobFloatingIPStatus{
			FloatingIp:  &cms.ClusterFloatingIP{Id: task.ResourceID, IpAddress: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		floatingIp.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			floatingIp.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return floatingIp
}

func (h *DisasterRecoveryManagerHandler) getRouter(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobRouterStatus {
	var router *drms.RecoveryJobRouterStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		router = &drms.RecoveryJobRouterStatus{
			Router:      &cms.ClusterRouter{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		router.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			router.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return router
}

func (h *DisasterRecoveryManagerHandler) getVolume(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobVolumeStatus {
	var volume *drms.RecoveryJobVolumeStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		volume = &drms.RecoveryJobVolumeStatus{
			Volume:      &cms.ClusterVolume{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		volume.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			volume.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return volume
}

func (h *DisasterRecoveryManagerHandler) getKeypair(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobKeypairStatus {
	var keypair *drms.RecoveryJobKeypairStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		keypair = &drms.RecoveryJobKeypairStatus{
			Keyfair:     &cms.ClusterKeypair{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		keypair.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			keypair.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return keypair
}

func (h *DisasterRecoveryManagerHandler) getSpec(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobSpecStatus {
	var spec *drms.RecoveryJobSpecStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		spec = &drms.RecoveryJobSpecStatus{
			InstanceSpec: &cms.ClusterInstanceSpec{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime:  status.ElapsedTime,
			StartedAt:    status.StartedAt,
			FinishedAt:   status.FinishedAt,
			StateCode:    status.StateCode,
			TypeCode:     typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		spec.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			spec.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return spec
}

func (h *DisasterRecoveryManagerHandler) getInstance(task *migrator.RecoveryJobTask, typeCode string) *drms.RecoveryJobInstanceStatus {
	var instance *drms.RecoveryJobInstanceStatus
	if status, ok := h.TaskStatus[path.Join(task.RecoveryJobTaskKey, "status")]; ok {
		if status.StartedAt != 0 {
			now := time.Now().Unix()
			if status.FinishedAt != 0 {
				now = status.FinishedAt
			}
			status.ElapsedTime = now - status.StartedAt
		}

		instance = &drms.RecoveryJobInstanceStatus{
			Instance:    &cms.ClusterInstance{Id: task.ResourceID, Name: task.ResourceName},
			ElapsedTime: status.ElapsedTime,
			StartedAt:   status.StartedAt,
			FinishedAt:  status.FinishedAt,
			StateCode:   status.StateCode,
			TypeCode:    typeCode,
		}
	}

	if result, ok := h.TaskResult[path.Join(task.RecoveryJobTaskKey, "result")]; ok {
		instance.ResultCode = result.ResultCode

		if result.FailedReason != nil {
			instance.FailedReason = &drms.Message{
				Code:     result.FailedReason.Code,
				Contents: result.FailedReason.Contents,
			}
		}
	}

	return instance
}
