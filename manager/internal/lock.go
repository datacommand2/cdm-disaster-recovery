package internal

import (
	"context"
	"fmt"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/sync"
	"time"
)

const (
	protectionGroupDBLockPathFormat = "dr.protection.group.db.locks/%d"
	recoveryPlanDBLockPathFormat    = "dr.recovery.plan.db.locks/%d"
	recoveryJobDBLockPathFormat     = "dr.recovery.job.db.locks/%d"
	recoveryReportDBLockPathFormat  = "dr.recovery.report.db.locks/%d"
	recoveryJobStoreLockPathFormat  = "dr.recovery.job.store.locks/%d"
)

// ProtectionGroupDBLock 보호그룹의 데이터 베이스 lock
func ProtectionGroupDBLock(pgid uint64) (func(), error) {
	p := fmt.Sprintf(protectionGroupDBLockPathFormat, pgid)
	lock, err := sync.Lock(context.Background(), p)
	if err != nil {
		_ = lock.Close()
		return nil, errors.Unknown(err)
	}

	return func() {
		if err = lock.Unlock(context.Background()); err != nil {
			logger.Warnf("Could not unlock (%s). Cause: %v", p, err)
		}
		if err = lock.Close(); err != nil {
			logger.Warnf("Could not close the session (%s). Cause: %v", p, err)
		}
	}, nil
}

// RecoveryPlanDBLock 재해복구 계획의 데이터 베이스 lock
func RecoveryPlanDBLock(pid uint64) (func(), error) {
	p := fmt.Sprintf(recoveryPlanDBLockPathFormat, pid)
	lock, err := sync.Lock(context.Background(), p)
	if err != nil {
		_ = lock.Close()
		return nil, errors.Unknown(err)
	}

	return func() {
		if err = lock.Unlock(context.Background()); err != nil {
			logger.Warnf("Could not unlock (%s). Cause: %v", p, err)
		}
		if err = lock.Close(); err != nil {
			logger.Warnf("Could not close the session (%s). Cause: %v", p, err)
		}
	}, nil
}

// RecoveryJobDBLock 재해복구 작업의 데이터 베이스 lock
func RecoveryJobDBLock(jid uint64) (func(), error) {
	p := fmt.Sprintf(recoveryJobDBLockPathFormat, jid)
	lock, err := sync.Lock(context.Background(), p)
	if err != nil {
		_ = lock.Close()
		return nil, errors.Unknown(err)
	}

	return func() {
		if err = lock.Unlock(context.Background()); err != nil {
			logger.Warnf("Could not unlock (%s). Cause: %v", p, err)
		}
		if err = lock.Close(); err != nil {
			logger.Warnf("Could not close the session (%s). Cause: %v", p, err)
		}
	}, nil
}

// RecoveryReportDBLock 결과 보고서의 데이터 베이스 lock
func RecoveryReportDBLock(rid uint64) (func(), error) {
	p := fmt.Sprintf(recoveryReportDBLockPathFormat, rid)
	lock, err := sync.Lock(context.Background(), p)
	if err != nil {
		_ = lock.Close()
		return nil, errors.Unknown(err)
	}

	return func() {
		if err = lock.Unlock(context.Background()); err != nil {
			logger.Warnf("Could not unlock (%s). Cause: %v", p, err)
		}
		if err = lock.Close(); err != nil {
			logger.Warnf("Could not close the session (%s). Cause: %v", p, err)
		}
	}, nil
}

// RecoveryJobStoreLock 재해복구 작업의 스토어 lock
func RecoveryJobStoreLock(jid uint64) (func(), error) {
	p := fmt.Sprintf(recoveryJobStoreLockPathFormat, jid)
	lock, err := sync.Lock(context.Background(), p)
	if err != nil {
		_ = lock.Close()
		return nil, errors.Unknown(err)
	}

	return func() {
		if err = lock.Unlock(context.Background()); err != nil {
			logger.Warnf("Could not unlock (%s). Cause: %v", p, err)
		}
		if err = lock.Close(); err != nil {
			logger.Warnf("Could not close the session (%s). Cause: %v", p, err)
		}
	}, nil
}

// RecoveryJobStoreTryLock 재해복구 작업의 스토어 try lock
func RecoveryJobStoreTryLock(jid uint64) (func(), error) {
	p := fmt.Sprintf(recoveryJobStoreLockPathFormat, jid)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	lock, err := sync.Lock(ctx, p)
	if err != nil {
		_ = lock.Close()
		return nil, ServerBusy(err)
	}

	return func() {
		if err = lock.Unlock(context.Background()); err != nil {
			logger.Warnf("Could not unlock (%s). Cause: %v", p, err)
		}
		if err = lock.Close(); err != nil {
			logger.Warnf("Could not close the session (%s). Cause: %v", p, err)
		}
	}, nil
}
