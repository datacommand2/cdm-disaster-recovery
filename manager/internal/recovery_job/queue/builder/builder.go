package builder

import (
	"context"
	"fmt"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-cloud/common/sync"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
)

const recoveryJobBuilderLock = "dr.recovery.job.recovery_cluster/%d"

// lock store transaction 이 commit 되는 시점에만 lock 이 잡힘으로
// 같은 recovery cluster 로 수행 되는 재해복구 작업 관련 global lock 추가
func lock(recoveryClusterID uint64) (func(), error) {
	path := fmt.Sprintf(recoveryJobBuilderLock, recoveryClusterID)
	m, err := sync.Lock(context.Background(), path)
	if err != nil {
		logger.Errorf("[builder-lock] cloud not lock(%s). Cause:%+v", path, err)
		return nil, errors.Unknown(err)
	}

	return func() {
		if err := m.Unlock(context.Background()); err != nil {
			logger.Errorf("[builder-lock] cloud not unlock(%s). Cause:%+v", path, err)
		}
	}, nil
}

// BuildTasks 재해복구작업의 Task 들을 생성한다.
func BuildTasks(ctx context.Context, txn store.Txn, job *migrator.RecoveryJob, detail *drms.RecoveryJob) error {
	logger.Infof("[BuildTasks] Start: job(%d)", job.RecoveryJobID)

	unlock, err := lock(job.RecoveryCluster.Id)
	if err != nil {
		return err
	}

	defer unlock()

	b, err := newTaskBuilder(ctx, job)
	if err != nil {
		logger.Errorf("[BuildTasks] Errors occurred during new task builder: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	b.RecoveryJobDetail = detail
	if err = b.buildTasks(ctx, txn); err != nil {
		return err
	}

	logger.Infof("[BuildTasks] Put metadata: job(%d)", job.RecoveryJobID)
	return b.putMetadata(txn)
}

// BuildClearTasks 재해복구작업의 Clear Task 들을 생성한다.
func BuildClearTasks(ctx context.Context, txn store.Txn, job *migrator.RecoveryJob, opts ...TaskBuilderOption) error {
	logger.Infof("[BuildClearTasks] Start: job(%d)", job.RecoveryJobID)

	unlock, err := lock(job.RecoveryCluster.Id)
	if err != nil {
		return err
	}

	defer unlock()

	b, err := newClearTaskBuilder(ctx, job, opts...)
	if err != nil {
		logger.Errorf("[BuildClearTasks] Errors occurred during new clear task builder: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	if err = b.buildClearTasks(ctx, txn); err != nil {
		return err
	}

	logger.Infof("[BuildClearTasks] Put metadata: job(%d)", job.RecoveryJobID)
	return b.putMetadata(txn)
}

// RebuildFailedClearTasks 재해복구작업의 실패한 Clear Task 들을 재생성한다.
// 실제로 Clear Task 를 다시 만들지는 않고, 기존 Clear Task 의 결과와 상태를 초기화한다.
func RebuildFailedClearTasks(ctx context.Context, txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[RebuildFailedClearTasks] Start: job(%d)", job.RecoveryJobID)

	unlock, err := lock(job.RecoveryCluster.Id)
	if err != nil {
		return err
	}

	defer unlock()

	b, err := newClearTaskBuilder(ctx, job)
	if err != nil {
		logger.Errorf("[RebuildFailedClearTasks] Errors occurred during new clear task builder: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	if err = b.rebuildFailedClearTasks(txn); err != nil {
		return err
	}

	logger.Infof("[RebuildFailedClearTasks] Put metadata: job(%d)", job.RecoveryJobID)
	return b.putMetadata(txn)
}

// RebuildClearTasks 재해복구작업의 Clear Task 들을 재생성한다.
// 기존 성공한 Clear Task 들을 제외하고 새로운 Clear Task들을 만든다.
func RebuildClearTasks(ctx context.Context, txn store.Txn, job *migrator.RecoveryJob) error {
	logger.Infof("[RebuildClearTasks] Start: job(%d)", job.RecoveryJobID)

	unlock, err := lock(job.RecoveryCluster.Id)
	if err != nil {
		return err
	}
	defer unlock()

	b, err := newClearTaskBuilder(ctx, job)
	if err != nil {
		logger.Errorf("[RebuildClearTasks] Errors occurred during new clear task builder: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}
	if err = b.rebuildClearTasks(ctx, txn); err != nil {
		return err
	}

	logger.Infof("[RebuildClearTasks] Put metadata: job(%d)", job.RecoveryJobID)
	return b.putMetadata(txn)
}
