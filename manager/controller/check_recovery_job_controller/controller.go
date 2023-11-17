package checkrecoveryjobcontroller

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/controller"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	recoveryjob "github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/recovery_job"
	"github.com/jinzhu/gorm"
)

var defaultController checkRecoveryJobController // singleton

type checkRecoveryJobController struct {
}

func init() {
	controller.RegisterController(&defaultController)
}

// Run distributorController 초기화 및 queue 생성과 controller 실행
func (c *checkRecoveryJobController) Run() {
	logger.Info("[checkRecoveryJobController-Run] Starting check recovery job controller.")

	var (
		err error
		ctx context.Context
	)
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		logger.Errorf("[checkRecoveryJobController] Context Error. Cause: %v", err)
		return
	}

	go func() {
		jobs, err := migrator.GetJobList()
		if err != nil {
			logger.Errorf("[checkRecoveryJobController-run] Could not get job list. Cause: %+v", err)
			return
		}

		for _, job := range jobs {
			if err = checkRecoveryJob(ctx, job); err != nil {
				logger.Errorf("[checkRecoveryJobController-run] Could not handle recovery job (%d). Cause: %+v", job.RecoveryJobID, err)
			}
		}
	}()
}

// Close close 채널 전송과 unsubscribe
func (c *checkRecoveryJobController) Close() {
	logger.Info("[checkRecoveryJobController-Close] Stopping check recovery job controller")
}

func checkRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) error {
	s, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[CheckRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 상태 값 중 running, canceling, clearing 는 migrator 에서 작업하기때문에 요기선 처리할 필요 없음
	// 만약 서비스가 재시작 됐을 때 진행됐었던 job 을 다시 재실행
	switch s.StateCode {
	case constant.RecoveryJobStateCodeWaiting, constant.RecoveryJobStateCodePending, constant.RecoveryJobStateCodeRunning:
		logger.Infof("[CheckRecoveryJob] Re-run job(%d) status(%s).", job.RecoveryJobID, s.StateCode)
		if err = internal.PublishMessage(constant.QueueTriggerWaitingRecoveryJob, job); err != nil {
			logger.Errorf("[CheckRecoveryJob] Could not publish recovery job(%d). Cause: %+v", job.RecoveryJobID, err)
			return errors.UnusableBroker(err)
		}

	case constant.RecoveryJobStateCodeCanceling:
		logger.Infof("[CheckRecoveryJob] Re-run job(%d) status(%s).", job.RecoveryJobID, s.StateCode)
		if err = internal.PublishMessage(constant.QueueTriggerCancelRecoveryJob, job); err != nil {
			logger.Errorf("[CheckRecoveryJob] Could not publish recovery job(%d). Cause: %+v", job.RecoveryJobID, err)
			return errors.UnusableBroker(err)
		}

	case constant.RecoveryJobStateCodeCompleted:
		logger.Infof("[CheckRecoveryJob] Re-run job(%d) status(%s).", job.RecoveryJobID, s.StateCode)
		// completed 상태에서 Retry, Rollback, Confirm operation 모니터링
		go recoveryjob.HandleRecoveryJob(ctx, job, constant.RecoveryJobStateCodeCompleted)

	case constant.RecoveryJobStateCodeClearFailed:
		logger.Infof("[CheckRecoveryJob] Re-run job(%d) status(%s).", job.RecoveryJobID, s.StateCode)
		if err = internal.PublishMessage(constant.QueueTriggerClearFailedRecoveryJob, job); err != nil {
			logger.Errorf("[CheckRecoveryJob] Could not publish recovery job(%d). Cause: %+v", job.RecoveryJobID, err)
			return errors.UnusableBroker(err)
		}

	case constant.RecoveryJobStateCodeReporting, constant.RecoveryJobStateCodeFinished:
		logger.Infof("[CheckRecoveryJob] Re-run job(%d) status(%s).", job.RecoveryJobID, s.StateCode)
		if err = internal.PublishMessage(constant.QueueTriggerReportingRecoveryJob, job); err != nil {
			logger.Errorf("[CheckRecoveryJob] Could not publish recovery job(%d). Cause: %+v", job.RecoveryJobID, err)
			return errors.UnusableBroker(err)
		}
	}

	return nil
}
