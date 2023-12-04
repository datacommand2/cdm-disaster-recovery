package queue

import (
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_job/queue/builder"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"

	"context"
	"time"
)

// GetJobList 재해복구작업 목록을 조회하는 함수
func GetJobList() ([]*migrator.RecoveryJob, error) {
	return migrator.GetJobList()
}

// GetJob 재해복구작업의 상태를 조회하는 함수
func GetJob(id uint64) (*migrator.RecoveryJob, error) {
	return migrator.GetJob(id)
}

// GetJobStatus 재해복구작의 상태를 조회하는 함수
func GetJobStatus(id uint64) (string, error) {
	job, err := migrator.GetJob(id)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		return constant.RecoveryJobStateCodeWaiting, nil

	} else if err != nil {
		logger.Errorf("[GetJobStatus] Could not get job: dr.recovery.job/%d. Cause: %+v", id, err)
		return "", err
	}

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[GetJobStatus] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", id, err)
		return "", err
	}

	return status.StateCode, nil
}

// GetJobOperation 재해복구작의 Operation 을 조회하는 함수
func GetJobOperation(id uint64) (string, error) {
	job, err := migrator.GetJob(id)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		return "", nil

	} else if err != nil {
		logger.Errorf("[GetJobOperation] Could not get job: dr.recovery.job/%d. Cause: %+v", id, err)
		return "", err
	}

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[GetJobOperation] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", id, err)
		return "", err
	}

	return op.Operation, nil
}

// AddJob 재해복구작업을 대기열에 넣고, 작업의 Task 들을 생성한다.
func AddJob(ctx context.Context, job *drms.RecoveryJob) (*migrator.RecoveryJob, error) {
	logger.Infof("[queue-AddJob] Start: job(%d)", job.Id)

	// 이미 동일한 job id의 job 이 추가 되어있다면 추가하지 않음.
	_, err := migrator.GetJob(job.Id)
	if err == nil {
		logger.Infof("[queue-AddJob] Already added job(%d)", job.Id)
		return nil, nil
	}

	hash, err := internal.EncodeProtectionGroupName(ctx, job.Group.Id)
	if err != nil {
		logger.Errorf("[queue-AddJob] Could not get the hash: job(%d) group(%d). Cause: %+v", job.Id, job.Group.Id, err)
		return nil, err
	}

	j := &migrator.RecoveryJob{
		RecoveryJobID:         job.Id,
		RecoveryJobHash:       hash,
		ProtectionGroupID:     job.Group.Id,
		ProtectionCluster:     job.Plan.ProtectionCluster,
		RecoveryCluster:       job.Plan.RecoveryCluster,
		RecoveryJobTypeCode:   job.TypeCode,
		RecoveryTimeObjective: job.Group.RecoveryTimeObjective,
		RecoveryPointTypeCode: job.RecoveryPointTypeCode,
		RecoveryPointSnapshot: job.RecoveryPointSnapshot,
		RecoveryPoint:         job.RecoveryPointSnapshot.GetCreatedAt(),
		TriggeredAt:           time.Now().Unix(),
	}

	// 최신데이터를 사용하여 재해복구하는 작업의 경우, 복구데이터 시점을 현재 시점으로 업데이트한다.
	if j.RecoveryJobTypeCode == constant.RecoveryTypeCodeMigration && j.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
		j.RecoveryPoint = j.TriggeredAt
	}

	if err = store.Transaction(func(txn store.Txn) error {
		if err = j.Put(txn); err != nil {
			logger.Errorf("[queue-AddJob] Could not job put: dr.recovery.job/%d. Cause: %+v", job.Id, err)
			return err
		}
		logger.Infof("[queue-AddJob] Done - put: dr.recovery.job/%d", job.Id)

		// detail
		if err = j.SetDetail(txn, job); err != nil {
			logger.Errorf("[queue-AddJob] Could not set job detail: dr.recovery.job/%d/detail. Cause: %+v", job.Id, err)
			return err
		}
		logger.Infof("[queue-AddJob] Done - set detail: dr.recovery.job/%d/detail", job.Id)

		// state 를 waiting 으로 변경
		if err = j.SetStatus(txn, &migrator.RecoveryJobStatus{StateCode: constant.RecoveryJobStateCodeWaiting}); err != nil {
			logger.Errorf("[queue-AddJob] Could not set job status: dr.recovery.job/%d/status. Cause: %+v", job.Id, err)
			return err
		}
		logger.Infof("[queue-AddJob] Done - set status: dr.recovery.job/%d/status {%s}", job.Id, constant.RecoveryJobStateCodeWaiting)

		// operation 을 run 으로 변경
		if err = j.SetOperation(txn, constant.RecoveryJobOperationRun); err != nil {
			logger.Errorf("[queue-AddJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.Id, err)
			return err
		}
		logger.Infof("[queue-AddJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.Id, constant.RecoveryJobOperationRun)

		return nil

	}); err != nil {
		return nil, err
	}

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     j.RecoveryJobID,
		Detail:    job,
		Status:    &migrator.RecoveryJobStatus{StateCode: constant.RecoveryJobStateCodeWaiting},
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRun},
	}); err != nil {
		logger.Warnf("[queue-AddJob] Could not publish job(%d) detail, status, operation. Cause: %+v", j.RecoveryJobID, err)
	}

	logger.Infof("[queue-AddJob] Success: job(%d) hash(%s)", job.Id, hash)
	return j, nil
}

// PauseJob 재해복구작업을 일시중지한다.
func PauseJob(job *migrator.RecoveryJob) error {
	logger.Infof("[queue-PauseJob] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-PauseJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업이 실행중이 아니라면 일시중지할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeRunning {
		logger.Errorf("[queue-PauseJob] Could not pause job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotPausableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		return job.SetOperation(txn, constant.RecoveryJobOperationPause)
	}); err != nil {
		logger.Errorf("[queue-PauseJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
			job.RecoveryJobID, constant.RecoveryJobOperationPause, err)
		return err
	}

	logger.Infof("[queue-PauseJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationPause)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationPause},
	}); err != nil {
		logger.Warnf("[queue-PauseJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// ExtendJobPausingTime 작업의 일시중지 시간을 연장한다.
func ExtendJobPausingTime(job *migrator.RecoveryJob, extendTime uint32) error {
	logger.Infof("[queue-ExtendJobPausingTime] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-ExtendJobPausingTime] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업이 일시중지 상태가 아닌 경우 일시중지 시간을 연장할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodePaused {
		logger.Errorf("[queue-ExtendJobPausingTime] Could not extend job(%d) pausing time. Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotExtendablePausingTimeState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	status.ResumeAt = status.ResumeAt + int64(extendTime)

	if err = store.Transaction(func(txn store.Txn) error {
		return job.SetStatus(txn, status)
	}); err != nil {
		logger.Errorf("[queue-ExtendJobPausingTime] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-ExtendJobPausingTime] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-ExtendJobPausingTime] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// ResumeJob 일시중지된 재해복구작업을 재개한다.
func ResumeJob(job *migrator.RecoveryJob) error {
	logger.Infof("[queue-ResumeJob] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-ResumeJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업이 일시중지 상태가 아니라면 재개할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodePaused {
		logger.Errorf("[queue-ResumeJob] Could not resume job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotResumableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		return job.SetOperation(txn, constant.RecoveryJobOperationRun)
	}); err != nil {
		logger.Errorf("[queue-ResumeJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
			job.RecoveryJobID, constant.RecoveryJobOperationRun, err)
		return err
	}

	logger.Infof("[queue-ResumeJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationRun)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRun},
	}); err != nil {
		logger.Warnf("[queue-ResumeJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// CancelJob 실행중인(혹은 일시중지 상태인) 재해복구작업을 취소한다.
func CancelJob(job *migrator.RecoveryJob) error {
	logger.Infof("[queue-CancelJob] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-CancelJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업이 실행중이거나 일시중지 상태가 아니라면 취소할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeRunning &&
		status.StateCode != constant.RecoveryJobStateCodePaused {
		logger.Errorf("[queue-CancelJob] Could not cancel job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotCancelableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		return job.SetOperation(txn, constant.RecoveryJobOperationCancel)
	}); err != nil {
		logger.Errorf("[queue-CancelJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
			job.RecoveryJobID, constant.RecoveryJobOperationCancel, err)
		return err
	}

	logger.Infof("[queue-CancelJob] Done - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationCancel)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationCancel},
	}); err != nil {
		logger.Warnf("[queue-CancelJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// RollbackJob 재해복구작업을 롤백하기 위한 Clear Task 들을 생성한다.
func RollbackJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-RollbackJob] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-RollbackJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업이 완료 상태가 아니라면 롤백할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeCompleted {
		logger.Errorf("[queue-RollbackJob] Could not rollback job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotRollbackableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		// 모든 Task 들에 대한 Clear Task 들을 생성한다.
		if err = builder.BuildClearTasks(ctx, txn, job); err != nil {
			logger.Errorf("[queue-RollbackJob] Could not build clear tasks: job(%d). Cause: %+v", job.RecoveryJobID, err)
			if errors.Equal(err, migrator.ErrNotFoundTask) {
				if err = SetJobStateClearFailedOperationIgnoreRollback(txn, job); err != nil {
					return err
				}
			}
			return err
		}
		logger.Infof("[queue-RollbackJob] Build clear tasks Done: job(%d)", job.RecoveryJobID)

		if err = job.SetOperation(txn, constant.RecoveryJobOperationRollback); err != nil {
			logger.Errorf("[queue-RollbackJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
				job.RecoveryJobID, constant.RecoveryJobOperationRollback, err)
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[queue-RollbackJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationRollback)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRollback},
	}); err != nil {
		logger.Warnf("[queue-RollbackJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// ExtendJobRollbackTime 모의훈련 시, 작업의 롤백 대기시간 연장한다
func ExtendJobRollbackTime(job *migrator.RecoveryJob, extendTime uint32) error {
	logger.Infof("[queue-ExtendJobRollbackTime] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-ExtendJobRollbackTime] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업의 유형이 재해복구인 경우, 롤백실패 상태에서만 롤백 대기시간을 연장할 수 있다.
	if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeMigration &&
		status.StateCode != constant.RecoveryJobStateCodeClearFailed {
		logger.Errorf("[queue-ExtendJobRollbackTime] Job(%d) type code is migration and state is clear failed.", job.RecoveryJobID)
		return internal.NotExtendableRollbackTimeState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업이 완료 상태거나 롤백실패 상태가 아닌 경우 롤백 대기시간을 연장할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeCompleted &&
		status.StateCode != constant.RecoveryJobStateCodeClearFailed {
		logger.Errorf("[queue-ExtendJobRollbackTime] Could not extend job(%d) rollback time. Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotExtendableRollbackTimeState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	status.RollbackAt = status.RollbackAt + int64(extendTime)

	if err = store.Transaction(func(txn store.Txn) error {
		return job.SetStatus(txn, status)
	}); err != nil {
		logger.Errorf("[queue-ExtendJobRollbackTime] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-ExtendJobRollbackTime] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-ExtendJobRollbackTime] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// IgnoreRollbackJob 재해복구작업 롤백을 무시한다.
// 최소 1회 롤백에 실패한 경우, 롤백을 무시할 수 있다.
func IgnoreRollbackJob(job *migrator.RecoveryJob) error {
	logger.Infof("[queue-IgnoreRollbackJob] Start: job(%d)", job.RecoveryJobID)

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[queue-IgnoreRollbackJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-IgnoreRollbackJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업의 Operation 이 롤백이나 확정이 아니라면 롤백을 무시할 수 없다.
	if op.Operation != constant.RecoveryJobOperationRollback &&
		op.Operation != constant.RecoveryJobOperationConfirm {
		logger.Errorf("[queue-IgnoreRollbackJob] Could not ignore rollback job(%d). Cause: operation is %s", job.RecoveryJobID, op.Operation)
		return internal.NotRollbackIgnorableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업이 롤백실패 상태가 아니라면 롤백을 무시할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeClearFailed {
		logger.Errorf("[queue-IgnoreRollbackJob] Could not ignore rollback job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotRollbackIgnorableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		return job.SetOperation(txn, constant.RecoveryJobOperationIgnoreRollback)
	}); err != nil {
		logger.Errorf("[queue-IgnoreRollbackJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
			job.RecoveryJobID, constant.RecoveryJobOperationIgnoreRollback, err)
		return err
	}

	logger.Infof("[queue-IgnoreRollbackJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationIgnoreRollback)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationIgnoreRollback},
	}); err != nil {
		logger.Warnf("[queue-IgnoreRollbackJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// RetryRollbackJob 재해복구작업 롤백을 재시도하기 위한 Clear Task 들을 생성한다.
func RetryRollbackJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-RetryRollbackJob] Start: job(%d)", job.RecoveryJobID)

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[queue-RetryRollbackJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-RetryRollbackJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업의 Operation 이 롤백이나 확정이 아니라면 롤백을 재시도할 수 없다.
	if op.Operation != constant.RecoveryJobOperationRollback &&
		op.Operation != constant.RecoveryJobOperationConfirm {
		logger.Errorf("[queue-RetryRollbackJob] Could not retry rollback job(%d). Cause: operation is %s", job.RecoveryJobID, op.Operation)
		return internal.NotRollbackRetryableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업이 롤백실패 상태가 아니라면 롤백을 재시도할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeClearFailed {
		logger.Errorf("[queue-RetryRollbackJob] Could not retry rollback job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotRollbackRetryableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		// 실패한 Clear Task 들을 다시 생성한다.
		if err = builder.RebuildFailedClearTasks(ctx, txn, job); err != nil {
			logger.Errorf("[queue-RetryRollbackJob] Could not rebuild failed clear tasks: job(%d). Cause: %+v", job.RecoveryJobID, err)
			if errors.Equal(err, migrator.ErrNotFoundTask) {
				if err = SetJobStateClearFailedOperationIgnoreRollback(txn, job); err != nil {
					return err
				}
			}
			return err
		}
		logger.Infof("[queue-RetryRollbackJob] Rebuild failed clear tasks Done: job(%d)", job.RecoveryJobID)

		if err = job.SetOperation(txn, constant.RecoveryJobOperationRetryRollback); err != nil {
			logger.Errorf("[queue-RetryRollbackJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
				job.RecoveryJobID, constant.RecoveryJobOperationRetryRollback, err)
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[queue-RetryRollbackJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationRetryRollback)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRetryRollback},
	}); err != nil {
		logger.Warnf("[queue-RetryRollbackJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// ConfirmJob 재해복구작업을 확정하기 위한 Clear Task 들을 생성한다.
// 성공한 인스턴스는 Clear Task 를 생성하지 않는다.
func ConfirmJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-ConfirmJob] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-ConfirmJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	result, err := job.GetResult()
	if err != nil {
		logger.Errorf("[queue-ConfirmJob] Could not get result: dr.recovery.job/%d/result. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업의 유형이 재해복구가 아닌 경우 확정할 수 없다.
	if job.RecoveryJobTypeCode != constant.RecoveryTypeCodeMigration {
		logger.Errorf("[queue-ConfirmJob] Could not confirm job(%d). Cause: job type code is %s", job.RecoveryJobID, job.RecoveryJobTypeCode)
		return internal.NotConfirmableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업이 완료 상태가 아니라면 확정할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeCompleted {
		logger.Errorf("[queue-ConfirmJob] Could not confirm job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotConfirmableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업 결과가 성공이거나 부분성공이 아니라면 확정할 수 없다.
	if result.ResultCode != constant.RecoveryResultCodeSuccess &&
		result.ResultCode != constant.RecoveryResultCodePartialSuccess {
		logger.Errorf("[queue-ConfirmJob] Could not confirm job(%d). Cause: result code is %s", job.RecoveryJobID, result.ResultCode)
		return internal.NotConfirmableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구 승인자를 넣어준다.
	job.Approver, _ = metadata.GetAuthenticatedUser(ctx)
	if err = store.Transaction(func(txn store.Txn) error {
		if err = job.Put(txn); err != nil {
			logger.Errorf("[queue-ConfirmJob] Could not job put: dr.recovery.job/%d. Cause: %+v", job.RecoveryJobID, err)
			return err
		}

		// 성공한 인스턴스 생성 Task 를 제외한 모든 Task 들에 대한 Clear Task 들을 생성한다.
		// 성공한 인스턴스의 볼륨 스냅샷 중 사용하지 않는 스냅샷(recovery point 이 후의 스냅샷)을 제거하지않음
		//  * 스토리지 종류별로 사용하지 않는 스냅샷을 제거하기 위해 전달해야 하는 값이 달라 Clear Task 의 input 을 표준화할 수 없음
		//    - nfs: copy volume task output 의 volume.uuid 필요
		//    - ceph: import volume task output 의 volume.uuid 필요
		if err = builder.BuildClearTasks(ctx, txn, job, builder.SkipSuccessInstances()); err != nil {
			logger.Errorf("[queue-ConfirmJob] Could not build clear tasks(skipSuccessInstances). Cause: %+v", err)
			if errors.Equal(err, migrator.ErrNotFoundTask) {
				if err = SetJobStateClearFailedOperationIgnoreRollback(txn, job); err != nil {
					return err
				}
			}
			return err
		}
		logger.Infof("[queue-ConfirmJob] Build clear tasks(skipSuccessInstances) Done: job(%d)", job.RecoveryJobID)

		if err = job.SetOperation(txn, constant.RecoveryJobOperationConfirm); err != nil {
			logger.Errorf("[queue-ConfirmJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
				job.RecoveryJobID, constant.RecoveryJobOperationConfirm, err)
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[queue-ConfirmJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationConfirm)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationConfirm},
	}); err != nil {
		logger.Warnf("[queue-ConfirmJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// RetryConfirmJob 재해복구작업을 확정하기 위한 Clear Task 들을 재생성한다.
// 성공한 Clear Task 는 재생성하지 않는다.
func RetryConfirmJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-RetryConfirmJob] Start: job(%d)", job.RecoveryJobID)

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[queue-RetryConfirmJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-RetryConfirmJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업의 유형이 재해복구가 아닌 경우 확정을 재시도할 수 없다.
	if job.RecoveryJobTypeCode != constant.RecoveryTypeCodeMigration {
		logger.Errorf("[queue-RetryConfirmJob] Could not retry confirm job(%d). Cause: job type code is %s", job.RecoveryJobID, job.RecoveryJobTypeCode)
		return internal.NotConfirmRetryableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업의 Operation 이 확정이 아니라면 확정을 재시도할 수 없다.
	if op.Operation != constant.RecoveryJobOperationConfirm {
		logger.Errorf("[queue-RetryConfirmJob] Could not retry confirm job(%d). Cause: operation is %s", job.RecoveryJobID, job.RecoveryJobTypeCode)
		return internal.NotConfirmRetryableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업이 Clear Failed 상태가 아니라면 확정을 재시도할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeClearFailed {
		logger.Errorf("[queue-RetryConfirmJob] Could not retry confirm job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotConfirmRetryableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		// 실패한 Clear Task 들을 다시 생성한다.
		if err = builder.RebuildFailedClearTasks(ctx, txn, job); err != nil {
			logger.Errorf("[queue-RetryConfirmJob] Could not rebuild failed clear tasks: job(%d). Cause: %+v", job.RecoveryJobID, err)
			if errors.Equal(err, migrator.ErrNotFoundTask) {
				if err = SetJobStateClearFailedOperationIgnoreRollback(txn, job); err != nil {
					return err
				}
			}
			return err
		}
		logger.Infof("[queue-RetryConfirmJob] Rebuild failed clear tasks Done: job(%d)", job.RecoveryJobID)

		if err = job.SetOperation(txn, constant.RecoveryJobOperationRetryConfirm); err != nil {
			logger.Errorf("[queue-RetryConfirmJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
				job.RecoveryJobID, constant.RecoveryJobOperationRetryConfirm, err)
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[queue-RetryConfirmJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationRetryConfirm)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRetryConfirm},
	}); err != nil {
		logger.Warnf("[queue-RetryConfirmJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// CancelConfirmJob 재해복구작업 확정을 취소하기 위한 Clear Task 들을 생성한다.
// 기존 Clear Task 들을 모두 제거하고, 모든 Task 들에 대한 Clear Task 들을 생성한다.
func CancelConfirmJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-CancelConfirmJob] Start: job(%d)", job.RecoveryJobID)

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[queue-CancelConfirmJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-CancelConfirmJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구작업의 유형이 재해복구가 아닌 경우 확정을 취소할 수 없다.
	if job.RecoveryJobTypeCode != constant.RecoveryTypeCodeMigration {
		logger.Errorf("[queue-CancelConfirmJob] Could not cancel confirm job(%d). Cause: job type code is %s", job.RecoveryJobID, job.RecoveryJobTypeCode)
		return internal.NotConfirmCancelableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업의 Operation 이 확정이 아니라면 확정을 취소할 수 없다.
	if op.Operation != constant.RecoveryJobOperationConfirm {
		logger.Errorf("[queue-CancelConfirmJob] Could not cancel confirm job(%d). Cause: operation is %s", job.RecoveryJobID, job.RecoveryJobTypeCode)
		return internal.NotConfirmCancelableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	// 재해복구작업이 Clear Failed 상태가 아니라면 확정을 취소할 수 없다.
	if status.StateCode != constant.RecoveryJobStateCodeClearFailed {
		logger.Errorf("[queue-CancelConfirmJob] Could not cancel confirm job(%d). Cause: state code is %s", job.RecoveryJobID, status.StateCode)
		return internal.NotConfirmCancelableState(job.ProtectionGroupID, job.RecoveryJobID)
	}

	if err = store.Transaction(func(txn store.Txn) error {
		// 기존 Clear Task 들을 지우고, 모든 Task 들에 대한 Clear Task 들을 다시 생성한다.
		if err = builder.RebuildClearTasks(ctx, txn, job); err != nil {
			logger.Errorf("[queue-CancelConfirmJob] Could not rebuild clear tasks: job(%d). Cause: %+v", job.RecoveryJobID, err)
			if errors.Equal(err, migrator.ErrNotFoundTask) {
				if err = SetJobStateClearFailedOperationIgnoreRollback(txn, job); err != nil {
					return err
				}
			}
			return err
		}
		logger.Infof("[queue-CancelConfirmJob] Rebuild clear tasks Done: job(%d)", job.RecoveryJobID)

		if err = job.SetOperation(txn, constant.RecoveryJobOperationCancelConfirm); err != nil {
			logger.Errorf("[queue-CancelConfirmJob] Could not set operation: dr.recovery.job/%d/operation {%s}. Cause: %+v",
				job.RecoveryJobID, constant.RecoveryJobOperationCancelConfirm, err)
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[queue-CancelConfirmJob] Success - set operation: dr.recovery.job/%d/operation {%s}",
		job.RecoveryJobID, constant.RecoveryJobOperationCancelConfirm)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationCancelConfirm},
	}); err != nil {
		logger.Warnf("[queue-CancelConfirmJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// DeleteJob 재해복구작업을 대기열에서 제거한다.
func DeleteJob(txn store.Txn, job *migrator.RecoveryJob) {
	logger.Infof("[queue-DeleteJob] Delete job: dr.recovery.job/%d", job.RecoveryJobID)
	job.Delete(txn)
}

// SetJobStatePending 작업을 보류 상태로 만든다.
func SetJobStatePending(txn store.Txn, job *migrator.RecoveryJob) (*migrator.RecoveryJobStatus, error) {
	logger.Infof("[queue-SetJobStatePending] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStatePending] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	status.StateCode = constant.RecoveryJobStateCodePending
	status.StartedAt = time.Now().Unix()

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStatePending] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return nil, err
	}

	logger.Infof("[queue-SetJobStatePending] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	return status, nil
}

// SetJobStateRunning 작업을 시작한다.
func SetJobStateRunning(txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-SetJobStateRunning] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateRunning] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeRunning
	status.StartedAt = time.Now().Unix()

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateRunning] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-SetJobStateRunning] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-SetJobStateRunning] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// SetJobStateCanceling 작업을 취소한다.
func SetJobStateCanceling(txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-SetJobStateCanceling] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateCanceling] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeCanceling
	if status.PausedAt != 0 {
		pausingTime := time.Now().Unix() - status.PausedAt
		status.PausedTime = status.PausedTime + pausingTime
		status.PausedAt = 0
	}

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateCanceling] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-SetJobStateCanceling] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-SetJobStateCanceling] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// SetJobStateComplete 작업을 완료한다.
func SetJobStateComplete(txn store.Txn, job *migrator.RecoveryJob, rollbackWaitingTime int64) error {
	logger.Infof("[queue-SetJobStateComplete] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateComplete] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeCompleted
	status.PausedAt = time.Now().Unix()
	status.FinishedAt = time.Now().Unix()
	status.RollbackAt = time.Now().Unix() + rollbackWaitingTime

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateComplete] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-SetJobStateComplete] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-SetJobStateComplete] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// SetJobStateRunningByRetry 작업을 재시도 한다.
func SetJobStateRunningByRetry(txn store.Txn, job *migrator.RecoveryJob) (*migrator.RecoveryJobStatus, error) {
	logger.Infof("[queue-SetJobStateRunningByRetry] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateRunningByRetry] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	status.StateCode = constant.RecoveryJobStateCodeRunning
	pausingTime := time.Now().Unix() - status.PausedAt
	status.PausedTime = status.PausedTime + pausingTime
	status.PausedAt = 0
	status.FinishedAt = 0

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateRunningByRetry] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return nil, err
	}

	logger.Infof("[queue-SetJobStateRunningByRetry] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	return status, nil
}

// SetJobStateClearing 작업을 정리한다.
func SetJobStateClearing(txn store.Txn, job *migrator.RecoveryJob) (*migrator.RecoveryJobStatus, error) {
	logger.Infof("[queue-SetJobStateClearing] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateClearing] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	status.StateCode = constant.RecoveryJobStateCodeClearing

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateClearing] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return nil, err
	}

	logger.Infof("[queue-SetJobStateClearing] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	return status, nil
}

// SetJobStateClearFailed 작업 정리에 실패했다.
func SetJobStateClearFailed(txn store.Txn, job *migrator.RecoveryJob, rollbackWaitingTime int64) error {
	logger.Infof("[queue-SetJobStateClearFailed] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateClearFailed] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeClearFailed
	status.RollbackAt = time.Now().Unix() + rollbackWaitingTime

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateClearFailed] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-SetJobStateClearFailed] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-SetJobStateClearFailed] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// SetJobStateReporting 작업결과 보고서 생성
func SetJobStateReporting(txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-SetJobStateReporting] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateReporting] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeReporting

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateReporting] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-SetJobStateReporting] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[queue-SetJobStateReporting] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	return nil
}

// SetJobStateFinished 작업이 종료되었다.
func SetJobStateFinished(txn store.Txn, job *migrator.RecoveryJob) (*migrator.RecoveryJobStatus, error) {
	logger.Infof("[queue-SetJobStateFinished] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateFinished] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	status.StateCode = constant.RecoveryJobStateCodeFinished

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateFinished] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return nil, err
	}

	logger.Infof("[queue-SetJobStateFinished] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	return status, nil
}

// SetJobStateClearFailedOperationIgnoreRollback 작업 정리 및 ignore rollback
func SetJobStateClearFailedOperationIgnoreRollback(txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[queue-SetJobStateClearFailedOperationIgnoreRollback] Start: job(%d)", job.RecoveryJobID)

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[queue-SetJobStateClearFailedOperationIgnoreRollback] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	status.StateCode = constant.RecoveryJobStateCodeClearFailed

	if err = job.SetStatus(txn, status); err != nil {
		logger.Errorf("[queue-SetJobStateClearFailedOperationIgnoreRollback] Could not set status: dr.recovery.job/%d/status {%s}. Cause: %+v",
			job.RecoveryJobID, status.StateCode, err)
		return err
	}

	logger.Infof("[queue-SetJobStateClearFailedOperationIgnoreRollback] Success - set status: dr.recovery.job/%d/status {%s}",
		job.RecoveryJobID, status.StateCode)

	if err = job.SetOperation(txn, constant.RecoveryJobOperationIgnoreRollback); err != nil {
		logger.Errorf("[queue-SetJobStateClearFailedOperationIgnoreRollback] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[queue-SetJobStateClearFailedOperationIgnoreRollback] Success - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationIgnoreRollback)

	logger.Infof("[queue-SetJobStateClearFailedOperationIgnoreRollback] Success: job(%d)", job.RecoveryJobID)

	return nil
}
