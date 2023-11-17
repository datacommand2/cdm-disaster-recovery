package builder

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
)

// buildClearTasks 재해복구작업의 Clear Task 들을 생성한다.
func (b *TaskBuilder) buildClearTasks(ctx context.Context, txn store.Txn) error {
	logger.Infof("[buildClearTasks] Start: job(%d)", b.RecoveryJob.RecoveryJobID)

	for _, task := range b.RecoveryJobTaskMap {
		if _, err := b.buildReverseTask(txn, task); err != nil && err != errTaskBuildSkipped {
			return err
		}
	}

	if len(b.DeletedSharedTaskMap) != 0 {
		return b.loadMetadata(ctx)
	}

	logger.Infof("[buildClearTasks] Success: job(%d)", b.RecoveryJob.RecoveryJobID)
	return nil
}

// rebuildFailedClearTasks 재해복구작업의 실패한 Clear Task 들을 재생성한다.
// 실제로 Clear Task 를 다시 만들지는 않고, 기존 Clear Task 의 결과와 상태를 초기화한다.
func (b *TaskBuilder) rebuildFailedClearTasks(txn store.Txn) error {
	logger.Infof("[rebuildFailedClearTasks] Start: job(%d)", b.RecoveryJob.RecoveryJobID)

	for _, task := range b.RecoveryJobClearTaskMap {
		result, err := task.GetResult()
		if err != nil && !errors.Equal(err, migrator.ErrNotFoundTaskResult) {
			logger.Errorf("[rebuildFailedClearTasks] Could not get task result. Cause: %+v", err)
			return err
		}

		// 성공한 Clear Task 는 재생성하지 않는다.
		if err == nil && result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			continue
		}

		// Clear Task 의 결과를 삭제한다.
		if err = task.DeleteResult(txn); err != nil {
			logger.Errorf("[rebuildFailedClearTasks] Could not delete task result. Cause: %+v", err)
			return err
		}
		logger.Infof("[rebuildFailedClearTasks] Done - delete clear task result: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)

		// Clear Task 를 실행대기 상태로 변경한다.
		if err = task.SetStatusWaiting(txn); err != nil {
			logger.Errorf("[rebuildFailedClearTasks] Could not set status waiting. Cause: %+v", err)
			return err
		}
		logger.Infof("[rebuildFailedClearTasks] Done - set task status waiting: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)
	}

	logger.Infof("[rebuildFailedClearTasks] Success: job(%d)", b.RecoveryJob.RecoveryJobID)
	return nil
}

// rebuildClearTasks 재해복구작업의 Clear Task 들을 재생성한다.
// 기존 성공한 Clear Task 제외한 새로운 Clear Task 들을 생성한다.
func (b *TaskBuilder) rebuildClearTasks(ctx context.Context, txn store.Txn) error {
	if err := b.rebuildFailedClearTasks(txn); err != nil {
		return err
	}

	return b.buildClearTasks(ctx, txn)
}
