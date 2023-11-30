package recoveryjob

import (
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	drCluster "github.com/datacommand2/cdm-disaster-recovery/common/cluster"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	protectionGroup "github.com/datacommand2/cdm-disaster-recovery/manager/internal/protection_group"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_job/queue"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_job/queue/builder"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	recoveryReport "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_report"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"

	"github.com/jinzhu/gorm"

	"context"
	"fmt"
	"time"
)

const (
	defaultPausingTime                = 60 * 60 // 60 minute
	defaultRollbackWaitingTime        = 60 * 60 // 60 minute
	scheduleRollbackWaitingTime       = 5 * 60  // 5 minute
	defaultRecoveryJobHandlerInterval = 3
)

type RecoveryJob struct {
}

var lastLogJobTime = make(map[uint64]int64)

// 재해복구 작업을 취소하고 취소 완료 상태가 될 때까지 검사하다가 취소 완료가 되면 작업을 롤백한다.
func cancelAndRollbackRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[cancelAndRollbackRecoveryJob] Start: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)

	timeout := time.After(time.Hour)

	// 작업을 취소한다.
	if err := Cancel(ctx, &drms.RecoveryJobRequest{
		GroupId: job.ProtectionGroupID,
		JobId:   job.RecoveryJobID,
	}); err != nil {
		logger.Errorf("[cancelAndRollbackRecoveryJob] Could not cancel the job: group(%d) job(%d). Cause: %+v", job.ProtectionGroupID, job.RecoveryJobID, err)
		return err
	}

	// 작업 취소가 완료되거나 timeout(1시간) 될 때까지 반복하여 검사한다.
	for {
		op, err := job.GetOperation()
		if err != nil {
			logger.Errorf("[cancelAndRollbackRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
			return err
		}

		status, err := job.GetStatus()
		if err != nil {
			logger.Errorf("[cancelAndRollbackRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
			return err
		}

		// 작업의 operation 이 cancel 이고,
		// 작업의 상태가 completed 인 경우 작업 취소가 완료 되었다고 판단한다.
		if op.Operation == constant.RecoveryJobOperationCancel &&
			status.StateCode == constant.RecoveryJobStateCodeCompleted {
			logger.Infof("[cancelAndRollbackRecoveryJob] Job operation is canceled and job state is completed: group(%d) job(%d).", job.ProtectionGroupID, job.RecoveryJobID)
			break
		}

		select {
		case <-time.After(5 * time.Second):
		case <-timeout:
			break
		}
	}

	// 작업을 롤백한다.
	if err := Rollback(ctx, &drms.RecoveryJobRequest{
		GroupId: job.ProtectionGroupID,
		JobId:   job.RecoveryJobID,
	}, constant.RecoveryTypeCodeSimulation); err != nil {
		logger.Errorf("[cancelAndRollbackRecoveryJob] Could not rollback the job: group(%d) job(%d). Cause: %+v", job.ProtectionGroupID, job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[cancelAndRollbackRecoveryJob] Success: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)
	return nil
}

func cancelJobAndCreateResult(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[cancelJobAndCreateResult] Start: job(%d)", job.RecoveryJobID)
	var err error
	var result *drms.RecoveryResult

	// db 에서 job 정리
	if err = deleteJob(job); err != nil {
		logger.Warnf("[cancelJobAndCreateResult] Could not delete the job(%d) completely. Cause: %+v", job.RecoveryJobID, err)
	}

	if result, err = recoveryReport.CreateCanceledJob(ctx, job, constant.RecoveryResultCodeTimePassedCanceled); err != nil {
		logger.Warnf("[cancelJobAndCreateResult] Could not create recovery result by job auto canceled: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	if result != nil {
		recoveryReport.CreateRecoveryResultRaw(job.RecoveryJobID, result.Id)
	}

	// queue 에서 job 삭제
	_ = store.Transaction(func(txn store.Txn) error {
		queue.DeleteJob(txn, job)
		return nil
	})

	if err = internal.PublishMessage(constant.QueueTriggerRecoveryJobDeleted, job.RecoveryJobID); err != nil {
		logger.Warnf("[cancelJobAndCreateResult] Could not publish job(%d) deleted. Cause: %+v", job.RecoveryJobID, err)
	}

	logger.Infof("[cancelJobAndCreateResult] Success: job(%d) result(%d)", job.RecoveryJobID, result.GetId())
	return nil
}

func deleteJob(job *migrator.RecoveryJob) error {
	var err error
	var jobDetail drms.RecoveryJob
	if err = job.GetDetail(&jobDetail); err != nil {
		logger.Errorf("[deleteJob] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		return db.Where(&model.Job{ID: jobDetail.Id}).Delete(&model.Job{}).Error
	}); err != nil {
		logger.Errorf("[deleteJob] Could not delete job(%d). Cause: %+v", jobDetail.Id, err)
		return err
	}
	logger.Infof("[deleteJob] Done - Job deleted(db): job(%d)", jobDetail.Id)

	return nil
}

func prepareWaitingRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) (bool, error) {
	logger.Infof("[prepareWaitingRecoveryJob] Start: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)

	// 작업 예정시간을 (임시) 10분 초과한 job 은  queue 에서 삭제
	if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeSimulation {
		waitingTimeLimit := int64(10 * 60) // 10 분
		// trigger time + 10분 < 현재시간 이면 -> 현재시간이 trigger time 보다 10분이 넘게 지났다면
		if time.Unix(job.TriggeredAt+waitingTimeLimit, 0).Before(time.Now()) {
			if err := cancelJobAndCreateResult(ctx, job); err != nil {
				logger.Errorf("[prepareWaitingRecoveryJob] Could not cancel the passed waiting time job(%d). Cause: %+v", job.RecoveryJobID, err)
				return false, err
			}
			logger.Infof("[prepareWaitingRecoveryJob] Job(%d) canceled and result created by passed waiting time.", job.RecoveryJobID)
			return false, nil
		}
	}

	jobs, err := queue.GetJobList()
	if err != nil {
		logger.Errorf("[prepareWaitingRecoveryJob] Could not get job list: dr.recovery.job/. Cause: %+v", err)
		return false, err
	}

	for _, j := range jobs {
		// 같은 보호 그룹의 작업이 아니면 건너뛴다.
		if job.ProtectionGroupID != j.ProtectionGroupID {
			continue
		}

		// 지금 검사중인 작업이면 건너뛴다.
		if job.RecoveryJobID == j.RecoveryJobID {
			continue
		}

		s, err := j.GetStatus()
		if err != nil {
			logger.Errorf("[prepareWaitingRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", j.RecoveryJobID, err)
			return false, err
		}

		// 같은 보호 그룹의 다른 작업이 waiting 상태가 아니면 지금 작업을 build 할 수 없다.
		if s.StateCode != constant.RecoveryJobStateCodeWaiting {
			logger.Infof("[prepareWaitingRecoveryJob] Pass >> Same group other job(%d) state code is %s: group(%d) job(%d).",
				j.RecoveryJobID, s.StateCode, j.ProtectionGroupID, job.RecoveryJobID)
			return false, errors.New("cannot build the job unless other jobs in the same protection group are waiting")
		}

		// 같은 보호 그룹의 다른 작업의 trigger 시간이 더 빠른 경우 지금 작업을 build 할 수 없다.
		if j.TriggeredAt < job.TriggeredAt {
			logger.Infof("[prepareWaitingRecoveryJob] Pass >> Could not build job. trigger time: (%d:%+v) < (%d:%+v).",
				j.RecoveryJobID, time.Unix(j.TriggeredAt, 0), job.RecoveryJobID, time.Unix(job.TriggeredAt, 0))
			return false, errors.New("cannot build the job if other jobs in the same protection group have earlier trigger times")
		}
	}

	logger.Infof("[prepareWaitingRecoveryJob] Success: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)
	return true, nil
}

func preparePendingRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) (bool, error) {
	logger.Infof("[preparePendingRecoveryJob] Start: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)

	jobs, err := queue.GetJobList()
	if err != nil {
		logger.Errorf("[preparePendingRecoveryJob] Could not get job list(dr.recovery.job/). Cause: %+v", err)
		return false, err
	}

	for _, j := range jobs {
		// 같은 보호 그룹의 작업이 아니면 건너뛴다.
		if job.ProtectionGroupID != j.ProtectionGroupID {
			continue
		}

		// 지금 검사중인 작업이면 건너뛴다.
		if job.RecoveryJobID == j.RecoveryJobID {
			continue
		}

		s, err := j.GetStatus()
		if err != nil {
			logger.Errorf("[preparePendingRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", j.RecoveryJobID, err)
			return false, err
		}

		// 지금 작업이 재해복구 유형이고, 같은 보호 그룹의 다른 작업이 running 상태인 경우 작업을 취소하고 롤백한다.
		if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeMigration &&
			s.StateCode == constant.RecoveryJobStateCodeRunning {
			logger.Infof("[preparePendingRecoveryJob] Job(%d) type code is migration and state code is running.", job.RecoveryJobID)
			go func() {
				if err = cancelAndRollbackRecoveryJob(ctx, j); err != nil {
					logger.Warnf("[preparePendingRecoveryJob] Could not rollback recovery job(%d). Cause: %+v", j.RecoveryJobID, err)
					logger.Warnf("[preparePendingRecoveryJob] Please delete the job(%d) manually.", j.RecoveryJobID)
				}
			}()
		}

		// 같은 보호 그룹의 다른 작업이 waiting 이나 pending 상태가 아니면 지금 작업을 run 할 수 없다.
		if !(s.StateCode == constant.RecoveryJobStateCodeWaiting ||
			s.StateCode == constant.RecoveryJobStateCodePending) {
			logger.Infof("[preparePendingRecoveryJob] Pass >> Job state code is %s: group(%d) job(%d).", s.StateCode, j.ProtectionGroupID, j.RecoveryJobID)
			return false, errors.New("cannot run the job unless other jobs in the same protection group are not waiting or pending")
		}

		// 같은 보호 그룹의 다른 작업의 trigger 시간이 더 빠른 경우 지금 작업을 run 할 수 없다.
		if j.TriggeredAt < job.TriggeredAt {
			logger.Infof("[preparePendingRecoveryJob] Pass >> Could not build job. trigger time: (%d:%+v) < (%d:%+v).",
				j.RecoveryJobID, time.Unix(j.TriggeredAt, 0), job.RecoveryJobID, time.Unix(job.TriggeredAt, 0))
			return false, errors.New("cannot run the job if other jobs in the same protection group have earlier trigger times")
		}
	}

	logger.Infof("[preparePendingRecoveryJob] Success: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)
	return true, nil
}

func getJobResultWarningReasons(job *migrator.RecoveryJob) ([]*migrator.Message, error) {
	var reasons []*migrator.Message

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[getJobResultWarningReasons] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	// 목표복구시간을 준수하지 못함
	if status.ElapsedTime > int64(job.RecoveryTimeObjective)*60 /*min*/ {
		reasons = append(reasons, &migrator.Message{Code: "cdm-dr.manager.get_job_result_warning_reasons.failure-rpo_timeout"})
	}

	return reasons, err
}

func getJobResultFailedReasons(job *migrator.RecoveryJob) ([]*migrator.Message, error) {
	var reasons []*migrator.Message

	tasks, err := job.GetTaskList()
	if err != nil {
		logger.Errorf("[getJobResultFailedReasons] Could not get task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	for _, task := range tasks {
		result, err := task.GetResult()
		if err != nil {
			logger.Errorf("[getJobResultFailedReasons] Could not get job (%d) task (%s) result. Cause: %+v", job.RecoveryJobID, task.RecoveryJobTaskID, err)
			return nil, err
		}

		if result.ResultCode != constant.MigrationTaskResultCodeSuccess {
			reasons = append(reasons, result.FailedReason)
		}
	}

	return reasons, nil
}

func getJobInstanceResultSummary(job *migrator.RecoveryJob) (successCount int, failedCount int, err error) {
	var j drms.RecoveryJob
	if err := job.GetDetail(&j); err != nil {
		logger.Errorf("[getJobInstanceResultSummary] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return 0, 0, err
	}

	for _, i := range j.Plan.Detail.Instances {
		status, err := job.GetInstanceStatus(i.ProtectionClusterInstance.Id)
		if err != nil {
			logger.Errorf("[getJobInstanceResultSummary] Could not get instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
				job.RecoveryJobID, i.ProtectionClusterInstance.Id, err)
			return 0, 0, err
		}

		if status.StateCode == constant.RecoveryJobInstanceStateCodeSuccess {
			successCount = successCount + 1
		} else {
			failedCount = failedCount + 1
		}
	}

	return successCount, failedCount, nil
}

// 보호그룹의 최신 스냅샷을 반환한다.
func getLatestSnapshot(ctx context.Context, gid uint64) (*drms.ProtectionGroupSnapshot, error) {
	// TODO: Not implemented
	return nil, nil
}

// plan 의 대기중이거나 예약된 모의훈련을 모두 취소하고, 모의훈련 스케쥴들을 삭제한다.
func deleteRecoveryJobListInRecoveryPlan(ctx context.Context, groupID, planID uint64) error {
	logger.Infof("[deleteRecoveryJobListInRecoveryPlan] Start: group(%d) plan(%d)", groupID, planID)

	filters := makeRecoveryJobFilter(&drms.RecoveryJobListRequest{GroupId: groupID, PlanId: planID})
	jobs, err := getRecoveryJobList(filters...)
	if err != nil {
		logger.Errorf("[deleteRecoveryJobListInRecoveryPlan] Could not get job list: group(%d). Cause: %+v", groupID, err)
		return err
	}

	// 재해복구 유형의 작업은 보호그룹 당 1개만 존재하므로 유형에 대한 체크는 하지 않는다.
	for _, job := range jobs {
		// 작업이 DB 에는 존재하고 etcd 에는 없는 경우
		// 대기중이거나 예약된 모의훈련 작업인 경우이므로, 해당 작업을 삭제한다.
		if err = Delete(ctx, &drms.RecoveryJobRequest{GroupId: groupID, JobId: job.ID}); err != nil {
			logger.Errorf("[deleteRecoveryJobListInRecoveryPlan] Could not delete the delete job request: group(%d) job(%d). Cause: %+v", groupID, job.ID, err)
			return err
		}
		logger.Infof("[deleteRecoveryJobListInRecoveryPlan] Done - deleted in db: group(%d) job(%d)", groupID, job.ID)
	}

	logger.Infof("[deleteRecoveryJobListInRecoveryPlan] Success: group(%d) plan(%d)", groupID, planID)
	return err
}

// 성공한 인스턴스들로 Fail-back 을 위한 보호그룹을 생성한다.
func createFailbackProtectionGroup(ctx context.Context, jobDetail *drms.RecoveryJob, result *drms.RecoveryResult) (*drms.ProtectionGroup, error) {
	var (
		err       error
		groupName string
		instances []*cms.ClusterInstance
		pg        *drms.ProtectionGroup
	)

	// 재해복구에 성공한 인스턴스 목록 생성
	for _, i := range result.GetInstances() {
		if i.ResultCode == constant.InstanceRecoveryResultCodeSuccess {
			// result.instances.RecoveryClusterInstance 에 instance pgId 가 저장되어있지 않아 instance 조회
			recoveryInstance, err := cluster.GetClusterInstanceByUUID(ctx, result.RecoveryClusterId, i.RecoveryClusterInstance.Uuid, true)
			if err != nil {
				logger.Warnf("[createFailbackProtectionGroup] Could not get cluster instance: cluster(%d) instance(%s). Cause: %+v", result.RecoveryClusterId, i.RecoveryClusterInstance.Uuid, err)
				continue
			}
			instances = append(instances, recoveryInstance)
		}
	}

	if len(instances) == 0 {
		return nil, nil
	}

	// 기존의 해당 protection group 의 failback group 이 생성되었는지 조회
	pgId, err := migrator.GetFailbackProtectionGroup(jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id)
	if err != nil {
		logger.Errorf("[createFailbackProtectionGroup] Could not get failback protection group info: protection group(%d) recovery cluster(%d) job(%d). Cause: %+v",
			jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id, jobDetail.Id, err)
		return nil, err
	}

	// 존재하지 않음
	if pgId == 0 {
		// 보호그룹 이름의 중복을 방지하기 위해 생성날짜를 붙여줌
		groupName = jobDetail.Group.Name + ".failback_" + time.Now().Format("20060102")

		// 성공한 인스턴스들로 보호그룹 생성
		if pg, err = protectionGroup.Add(ctx, &drms.AddProtectionGroupRequest{Group: &drms.ProtectionGroup{
			OwnerGroup:                 jobDetail.Group.OwnerGroup,
			ProtectionCluster:          jobDetail.Plan.RecoveryCluster,
			Name:                       groupName,
			Remarks:                    jobDetail.Group.Remarks,
			RecoveryPointObjectiveType: jobDetail.Group.RecoveryPointObjectiveType,
			RecoveryPointObjective:     jobDetail.Group.RecoveryPointObjective,
			RecoveryTimeObjective:      jobDetail.Group.RecoveryTimeObjective,
			SnapshotIntervalType:       jobDetail.Group.SnapshotIntervalType,
			SnapshotInterval:           jobDetail.Group.SnapshotInterval,
			Instances:                  instances,
		}}); err != nil {
			logger.Errorf("[createFailbackProtectionGroup] Could not create the failback protection group(%d): recovery cluster(%d) job(%d). Cause: %+v",
				jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id, jobDetail.Id, err)
			return nil, err
		}
		logger.Infof("[createFailbackProtectionGroup] Create the protection group(%d:%s) in target cluster.", pg.Id, groupName)

		if err = migrator.PutFailbackProtectionGroup(jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id, pgId); err != nil {
			logger.Warnf("[createFailbackProtectionGroup] Could not put failback protection group info: protection group(%d) recovery cluster(%d) job(%d). Cause: %+v",
				jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id, jobDetail.Id, err)
		}

		return pg, nil
	}

	pg, err = protectionGroup.Get(ctx, &drms.ProtectionGroupRequest{GroupId: pgId})
	if err != nil {
		logger.Errorf("[createFailbackProtectionGroup] Could not get the protection group(%d). Cause: %+v", pg.Id, err)
		return nil, err
	}

	pg.Instances = instances
	// 보호그룹의 보호대상 인스턴스 목록 업데이트
	pg, err = protectionGroup.Update(ctx, &drms.UpdateProtectionGroupRequest{
		GroupId: pg.Id,
		Group:   pg,
	})
	if err != nil {
		logger.Errorf("[createFailbackProtectionGroup] Could not update the protection group(%d). Cause: %+v", pg.Id, err)
		return nil, err
	}

	logger.Infof("[createFailbackProtectionGroup] Update the protection group(%d:%s) in target cluster.", pg.Id, pg.Name)
	return pg, nil
}

func getRecoveryResultInstance(result *drms.RecoveryResult, id uint64) (*drms.RecoveryResultInstance, error) {
	for _, i := range result.Instances {
		if i.ResultCode != constant.InstanceRecoveryResultCodeSuccess {
			continue
		}

		if i.ProtectionClusterInstance.Id == id {
			return i, nil
		}
	}

	return nil, errors.Unknown(errors.New("not found instance recovery result"))
}

func getRecoveryResultVolume(result *drms.RecoveryResult, id uint64) (*drms.RecoveryResultVolume, error) {
	for _, v := range result.Volumes {
		if v.ResultCode != constant.VolumeRecoveryResultCodeSuccess {
			continue
		}

		if v.ProtectionClusterVolume.Id == id {
			return v, nil
		}
	}

	return nil, errors.Unknown(errors.New("not found volume recovery result"))
}

func getRecoveryResultRouter(result *drms.RecoveryResult, id uint64) (*drms.RecoveryResultRouter, error) {
	for _, r := range result.Routers {
		if r.ProtectionClusterRouter.Id == id {
			return r, nil
		}
	}

	return nil, errors.Unknown(errors.New("not found router recovery result"))
}

// Fail-back 재해복구계획의 상세계획을 생성한다
func appendFailbackRecoveryPlanDetail(plan *drms.RecoveryPlan, jobDetail *drms.RecoveryJob, result *drms.RecoveryResult) error {
	var err error

	var tenantMap = make(map[uint64]bool)
	var zoneMapping = make(map[uint64]uint64)
	var extNetMapping = make(map[uint64]uint64)
	var storageMapping = make(map[uint64]uint64)

	var volRsltMap = make(map[uint64]*drms.RecoveryResultVolume)
	var routerRsltMap = make(map[uint64]*drms.RecoveryResultRouter)

	for _, insRslt := range result.Instances {
		if insRslt.ResultCode != constant.InstanceRecoveryResultCodeSuccess {
			continue
		}

		// 테넌트 복구 계획 생성
		if !tenantMap[insRslt.RecoveryClusterInstance.Tenant.Id] {
			plan.Detail.Tenants = append(plan.Detail.Tenants, &drms.TenantRecoveryPlan{
				RecoveryTypeCode:                constant.TenantRecoveryPlanTypeMirroring,
				ProtectionClusterTenant:         insRslt.RecoveryClusterInstance.Tenant,
				RecoveryClusterTenantMirrorName: fmt.Sprintf("%s (failbacked from %s cluster)", insRslt.ProtectionClusterInstance.Tenant.Name, jobDetail.Plan.RecoveryCluster.Name),
			})
			tenantMap[insRslt.RecoveryClusterInstance.Tenant.Id] = true
		}

		// 가용구역 매핑 계획 생성
		if _, ok := zoneMapping[insRslt.ProtectionClusterInstance.AvailabilityZone.Id]; !ok {
			zonePlan := internal.GetAvailabilityZoneRecoveryPlan(
				jobDetail.Plan,
				insRslt.ProtectionClusterInstance.AvailabilityZone.Id,
			)

			plan.Detail.AvailabilityZones = append(plan.Detail.AvailabilityZones, &drms.AvailabilityZoneRecoveryPlan{
				RecoveryTypeCode:                  constant.AvailabilityZoneRecoveryPlanTypeMapping,
				ProtectionClusterAvailabilityZone: zonePlan.RecoveryClusterAvailabilityZone,
				RecoveryClusterAvailabilityZone:   zonePlan.ProtectionClusterAvailabilityZone,
			})

			zoneMapping[insRslt.ProtectionClusterInstance.AvailabilityZone.Id] = zonePlan.RecoveryClusterAvailabilityZone.Id
		}

		// 외부 네트워크 매핑 계획 생성
		for _, r := range insRslt.ProtectionClusterInstance.Routers {
			if _, ok := routerRsltMap[r.Id]; ok {
				continue
			}

			if routerRsltMap[r.Id], err = getRecoveryResultRouter(result, r.Id); err != nil {
				logger.Errorf("[appendFailbackRecoveryPlanDetail] Could not get recovery result router(%d). Cause: %+v", r.Id, err)
				return err
			}

			if _, ok := extNetMapping[r.ExternalRoutingInterfaces[0].Network.Id]; !ok {
				extNetPlan := internal.GetExternalNetworkRecoveryPlan(
					jobDetail.Plan,
					r.ExternalRoutingInterfaces[0].Network.Id,
				)

				plan.Detail.ExternalNetworks = append(plan.Detail.ExternalNetworks, &drms.ExternalNetworkRecoveryPlan{
					RecoveryTypeCode:                 constant.ExternalNetworkRecoveryPlanTypeMapping,
					ProtectionClusterExternalNetwork: extNetPlan.RecoveryClusterExternalNetwork,
					RecoveryClusterExternalNetwork:   extNetPlan.ProtectionClusterExternalNetwork,
				})

				extNetMapping[r.ExternalRoutingInterfaces[0].Network.Id] = extNetPlan.RecoveryClusterExternalNetwork.Id
			}
		}

		// 스토리지 매핑 계획 생성
		for _, v := range insRslt.ProtectionClusterInstance.Volumes {
			if _, ok := volRsltMap[v.Volume.Id]; ok {
				continue
			}

			if volRsltMap[v.Volume.Id], err = getRecoveryResultVolume(result, v.Volume.Id); err != nil {
				logger.Errorf("[appendFailbackRecoveryPlanDetail] Could not get recovery result volume(%d). Cause: %+v", v.Volume.Id, err)
				return err
			}

			if _, ok := storageMapping[v.Storage.Id]; !ok {
				storagePlan := internal.GetStorageRecoveryPlan(jobDetail.Plan, v.Storage.Id)

				plan.Detail.Storages = append(plan.Detail.Storages, &drms.StorageRecoveryPlan{
					RecoveryTypeCode:         constant.StorageRecoveryPlanTypeMapping,
					ProtectionClusterStorage: storagePlan.RecoveryClusterStorage,
					RecoveryClusterStorage:   storagePlan.ProtectionClusterStorage,
				})

				storageMapping[v.Storage.Id] = storagePlan.RecoveryClusterStorage.Id
			}
		}
	}

	// 라우터 복구계획 생성
	for _, routerRslt := range routerRsltMap {
		// 매핑된 외부 네트워크가 아닌 다른 외부 네트워크에 연결되었다면, 기존 외부 네트워크에 Fail-back 되도록 복구계획 추가
		if extNetMapping[routerRslt.ProtectionClusterRouter.ExternalRoutingInterfaces[0].Network.Id] == routerRslt.RecoveryClusterRouter.ExternalRoutingInterfaces[0].Network.Id {
			continue
		}

		// 라우터 복구계획 추가
		plan.Detail.Routers = append(plan.Detail.Routers, &drms.RouterRecoveryPlan{
			RecoveryTypeCode:               constant.RouterRecoveryPlanTypeMirroring,
			ProtectionClusterRouter:        routerRslt.RecoveryClusterRouter,
			RecoveryClusterExternalNetwork: routerRslt.ProtectionClusterRouter.ExternalRoutingInterfaces[0].Network,
		})
	}

	// 볼륨 복구계획 생성
	for _, volRslt := range volRsltMap {
		// 매핑된 스토리지가 아닌 다른 스토리지에 복구되었다면, 기존 스토리지에 Fail-back 되도록 설정
		var cs *cms.ClusterStorage
		if storageMapping[volRslt.ProtectionClusterVolume.Storage.Id] != volRslt.RecoveryClusterVolume.Storage.Id {
			cs = volRslt.ProtectionClusterVolume.Storage
		}

		// 볼륨 복구 계획 추가
		plan.Detail.Volumes = append(plan.Detail.Volumes, &drms.VolumeRecoveryPlan{
			RecoveryTypeCode:        constant.VolumeRecoveryPlanTypeMirroring,
			ProtectionClusterVolume: volRslt.RecoveryClusterVolume,
			RecoveryClusterStorage:  cs,
		})
	}

	// 인스턴스 복구계획 생성
	for _, insRslt := range result.Instances {
		if insRslt.ResultCode != constant.InstanceRecoveryResultCodeSuccess {
			continue
		}

		// 매핑된 가용구역이 아닌 다른 가용구역에 복구되었다면, 기존 가용구역에 Fail-back 되도록 설정
		var zone *cms.ClusterAvailabilityZone
		if zoneMapping[insRslt.ProtectionClusterInstance.AvailabilityZone.Id] != insRslt.RecoveryClusterInstance.AvailabilityZone.Id {
			zone = insRslt.ProtectionClusterInstance.AvailabilityZone
		}

		// 의존성 설정
		var dependencies []*cms.ClusterInstance
		for _, dep := range insRslt.Dependencies {
			depRslt, err := getRecoveryResultInstance(result, dep.Id)
			if err != nil {
				logger.Errorf("[appendFailbackRecoveryPlanDetail] Could not get recovery result instance: dependency instance(%d). Cause: %+v", dep.Id, err)
				return err
			}
			dependencies = append(dependencies, depRslt.RecoveryClusterInstance)
		}

		plan.Detail.Instances = append(plan.Detail.Instances, &drms.InstanceRecoveryPlan{
			RecoveryTypeCode:                constant.InstanceRecoveryPlanTypeMirroring,
			ProtectionClusterInstance:       insRslt.RecoveryClusterInstance,
			RecoveryClusterAvailabilityZone: zone,
			RecoveryClusterHypervisor:       insRslt.ProtectionClusterInstance.Hypervisor,
			AutoStartFlag:                   insRslt.AutoStartFlag,
			DiagnosisFlag:                   insRslt.DiagnosisFlag,
			DiagnosisMethodCode:             insRslt.DiagnosisMethodCode,
			DiagnosisMethodData:             insRslt.DiagnosisMethodData,
			DiagnosisTimeout:                insRslt.DiagnosisTimeout,
			Dependencies:                    dependencies,
		})
	}

	return nil
}

// 성공한 인스턴스들로 Fail-back 을 위한 재해복구계획을 생성한다.
func createFailbackRecoveryPlan(ctx context.Context, pg *drms.ProtectionGroup, jobDetail *drms.RecoveryJob, result *drms.RecoveryResult) error {
	plan := drms.RecoveryPlan{
		ProtectionCluster: jobDetail.Plan.RecoveryCluster,
		RecoveryCluster:   jobDetail.Group.ProtectionCluster,
		Name:              jobDetail.Plan.Name,
		Remarks:           jobDetail.Plan.Remarks,
		Detail:            new(drms.RecoveryPlanDetail),
	}

	if jobDetail.Plan.DirectionCode == constant.RecoveryJobDirectionCodeFailback {
		plan.DirectionCode = constant.RecoveryJobDirectionCodeFailover
	} else {
		plan.DirectionCode = constant.RecoveryJobDirectionCodeFailback
	}

	if err := appendFailbackRecoveryPlanDetail(&plan, jobDetail, result); err != nil {
		return err
	}

	_, err := recoveryPlan.Add(ctx, &drms.AddRecoveryPlanRequest{
		GroupId: pg.Id,
		Plan:    &plan,
		Force:   true,
	})

	return err
}

// 재해복구에 사용하지 않은 재해복구계획들을 제거한다.
func deleteUnusedRecoveryPlans(ctx context.Context, jobDetail *drms.RecoveryJob) error {
	logger.Infof("[deleteUnusedRecoveryPlans] Start: group(%d)", jobDetail.Group.Id)

	plans, _, err := recoveryPlan.GetList(ctx, &drms.RecoveryPlanListRequest{
		GroupId: jobDetail.Group.Id,
	})
	if errors.Equal(err, internal.ErrNotFoundProtectionGroup) {
		logger.Infof("[deleteUnusedRecoveryPlans] Not found protection group(%d)", jobDetail.Group.Id)
		return nil
	} else if err != nil {
		logger.Errorf("[deleteUnusedRecoveryPlans] Could not get plan list: group(%d). Cause: %+v", jobDetail.Group.Id, err)
		return err
	}

	for _, p := range plans {
		// 재해복구에 사용된 재해복구계획은 제거하지 않는다.
		if p.Id == jobDetail.Plan.Id {
			continue
		}

		if err = recoveryPlan.Delete(ctx, &drms.RecoveryPlanRequest{
			GroupId: jobDetail.Group.Id,
			PlanId:  p.Id,
		}); errors.Equal(err, internal.ErrNotFoundRecoveryPlan) {
			logger.Infof("[deleteUnusedRecoveryPlans] Not found recovery plan(%d): protection group(%d)", p.Id, jobDetail.Group.Id)
			continue
		} else if err != nil {
			logger.Errorf("[deleteUnusedRecoveryPlans] Could not delete the plan: group(%d) plan(%d). Cause: %+v", jobDetail.Group.Id, p.Id, err)
			return err
		}
		logger.Infof("[deleteUnusedRecoveryPlans] Done - delete plan: group(%d) plan(%d)", jobDetail.Group.Id, p.Id)
	}

	logger.Infof("[deleteUnusedRecoveryPlans] Success: group(%d)", jobDetail.Group.Id)
	return nil
}

// 재해복구에 사용한 재해복구계획과 보호그룹을 제거한다.
func deleteUsedProtectionGroupAndRecoveryPlan(ctx context.Context, jobDetail *drms.RecoveryJob) error {
	logger.Infof("[deleteUsedProtectionGroupAndRecoveryPlan] Start: group(%d) plan(%d)", jobDetail.Group.Id, jobDetail.Plan.Id)

	//var op string
	//if jobDetail.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
	//	if doUseReplicator(jobDetail) {
	//		op = constant.MirrorVolumeOperationStopAndDelete
	//	} else {
	//		op = constant.MirrorVolumeOperationStop
	//	}
	//} else {
	//	op = constant.MirrorVolumeOperationStopAndDelete
	//}

	logger.Infof("[deleteUsedProtectionGroupAndRecoveryPlan] - %s", jobDetail.RecoveryPointTypeCode)
	if err := recoveryPlan.Delete(ctx, &drms.RecoveryPlanRequest{
		GroupId: jobDetail.Group.Id,
		PlanId:  jobDetail.Plan.Id,
	}, jobDetail.RecoveryPointTypeCode); err != nil {
		logger.Errorf("[deleteUsedProtectionGroupAndRecoveryPlan] Could not delete the plan: group(%d) plan(%d). Cause: %+v", jobDetail.Group.Id, jobDetail.Plan.Id, err)
		return err
	}
	logger.Infof("[deleteUsedProtectionGroupAndRecoveryPlan] Done - delete plan: group(%d) plan(%d).", jobDetail.Group.Id, jobDetail.Plan.Id)

	if err := protectionGroup.Delete(ctx, &drms.DeleteProtectionGroupRequest{
		GroupId: jobDetail.Group.Id,
	}); err != nil {
		logger.Errorf("[deleteUsedProtectionGroupAndRecoveryPlan] Could not delete the protection group: group(%d). Cause: %+v", jobDetail.Group.Id, err)
	}
	logger.Infof("[deleteUsedProtectionGroupAndRecoveryPlan] Done - delete protection group: group(%d).", jobDetail.Group.Id)

	if err := migrator.DeleteFailbackProtectionGroup(jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id); err != nil {
		logger.Warnf("[deleteUsedProtectionGroupAndRecoveryPlan] Could not delete failback protection group info: protection group(%d) recovery cluster(%d) job(%d). Cause: %+v",
			jobDetail.Group.Id, jobDetail.Plan.RecoveryCluster.Id, jobDetail.Id, err)
	}

	logger.Infof("[deleteUsedProtectionGroupAndRecoveryPlan] Success: group(%d) plan(%d)", jobDetail.Group.Id, jobDetail.Plan.Id)
	return nil
}

// 실패한 인스턴스가 있다면, 성공한 인스턴스들을 보호그룹의 보호대상 인스턴스에서 제외한다.
func excludeProtectionInstances(ctx context.Context, jobDetail *drms.RecoveryJob, result *drms.RecoveryResult) error {
	logger.Infof("[excludeProtectionInstances] Start: group(%d)", jobDetail.Group.Id)

	// detail 의 Group 은 재해복구작업이 시작되는 시점의 정보이므로, 최신 정보를 다시 가져와서 업데이트한다.
	group, err := protectionGroup.Get(ctx, &drms.ProtectionGroupRequest{
		GroupId: jobDetail.Group.Id,
	})
	if err != nil {
		logger.Errorf("[excludeProtectionInstances] Could not get the protection group(%d). Cause: %+v", jobDetail.Group.Id, err)
		return err
	}

	exists := func(instanceID uint64) bool {
		for _, i := range group.Instances {
			if i.Id == instanceID {
				return true
			}
		}
		return false
	}

	// 재해복구결과를 통해 기존 보호대상 인스턴스 목록을 실패한 인스턴스들로 재구성
	// 재구성 하는 인스턴스은 현재 보호 그룹 인스턴스 목록에 있는 인스턴스들로 재구성 한다.
	var instances []*cms.ClusterInstance
	for _, i := range result.GetInstances() {
		if i.ResultCode != constant.InstanceRecoveryResultCodeSuccess && exists(i.ProtectionClusterInstance.Id) {
			instances = append(instances, i.ProtectionClusterInstance)
		}
	}
	group.Instances = instances

	// 재구성 하는 인스턴스 목록이 없는 경우 보호 그룹을 삭제한다.
	if len(group.Instances) == 0 {
		if err = protectionGroup.Delete(ctx, &drms.DeleteProtectionGroupRequest{GroupId: group.Id}); err != nil {
			logger.Errorf("[excludeProtectionInstances] Could not delete the protection group(%d). Cause: %+v", group.Id, err)
			return err
		}
		logger.Infof("[excludeProtectionInstances] Success - delete the protection group: group(%d)", group.Id)
		return nil
	}
	// 보호그룹의 보호대상 인스턴스 목록 업데이트
	_, err = protectionGroup.Update(ctx, &drms.UpdateProtectionGroupRequest{
		GroupId: group.Id,
		Group:   group,
	})
	if err != nil {
		logger.Errorf("[excludeProtectionInstances] Could not update the protection group(%d). Cause: %+v", group.Id, err)
		return err
	}
	logger.Infof("[excludeProtectionInstances] Done - update the protection group: group(%d)", group.Id)

	logger.Infof("[excludeProtectionInstances] Success: group(%d)", jobDetail.Group.Id)
	return nil
}

// 재해복구 확정에 대한 처리
func handleConfirmRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[handleConfirmRecoveryJob] Run: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)

	var (
		err       error
		jobDetail drms.RecoveryJob
		result    *drms.RecoveryResult
	)

	if err = job.GetDetail(&jobDetail); err != nil {
		logger.Errorf("[handleConfirmRecoveryJob] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 해당 plan 의 대기중이거나 예약된 모의훈련을 모두 취소하고, 모의훈련 스케쥴들을 삭제한다.
	if err = deleteRecoveryJobListInRecoveryPlan(ctx, jobDetail.Group.Id, jobDetail.Plan.Id); err != nil {
		logger.Errorf("[handleConfirmRecoveryJob] Could not delete the recovery job list in protection group(%d). Cause: %+v", jobDetail.Group.Id, err)
		return err
	}

	// 재해복구에 사용하지 않은 재해복구계획들을 제거한다.
	if err = deleteUnusedRecoveryPlans(ctx, &jobDetail); err != nil {
		logger.Errorf("[handleConfirmRecoveryJob] Could not delete the unused recovery plans: group(%d) Cause: %+v", jobDetail.Group.Id, err)
		return err
	}

	if result, err = recoveryReport.Get(ctx, &drms.RecoveryReportRequest{
		GroupId:  jobDetail.Group.Id,
		ResultId: job.RecoveryResultID,
	}); err != nil {
		logger.Warnf("[handleConfirmRecoveryJob] Could not get the job report(%d): group(%d) job(%d). Cause: %+v",
			job.RecoveryResultID, jobDetail.Group.Id, job.RecoveryJobID, err)
		result = &drms.RecoveryResult{ResultCode: constant.RecoveryResultCodeSuccess}
	}

	// 성공한 인스턴스들로 Fail-back 을 위한 보호그룹을 생성한다.
	// 동일이름의 보호그룹이 있는지 확인하고 있으면 인스턴스만 추가
	// 없으면 동일한 조건의 보호그룹을 recovery cluster 에 생성한다.
	if _, err = createFailbackProtectionGroup(ctx, &jobDetail, result); err != nil {
		logger.Errorf("[handleConfirmRecoveryJob] Could not create the protection group: cluster(%d) job(%d). Cause: %+v",
			jobDetail.Plan.RecoveryCluster.Id, jobDetail.Id, err)
		return err
	}

	// TODO : Failback 을 위한 재해복구계획을 재생성해야한다.

	if result.ResultCode == constant.RecoveryResultCodeSuccess {
		logger.Infof("[handleConfirmRecoveryJob] Run next - deleteUsedProtectionGroupAndRecoveryPlan: job(%d)", job.RecoveryJobID)
		// 실패한 인스턴스가 없다면, 재해복구에 사용한 재해복구계획과 보호그룹을 제거한다.
		if err = deleteUsedProtectionGroupAndRecoveryPlan(ctx, &jobDetail); err != nil {
			return err
		}
	} else {
		logger.Infof("[handleConfirmRecoveryJob] Run next - excludeProtectionInstances: job(%d) resultCode(%s)", job.RecoveryJobID, result.ResultCode)
		// 실패한 인스턴스가 있다면, 성공한 인스턴스들을 보호그룹의 보호대상 인스턴스에서 제외한다.
		if err = excludeProtectionInstances(ctx, &jobDetail, result); err != nil {
			return err
		}
	}

	if job.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
		return nil
	}

	return nil
}

// handleWaitingRecoveryJob 재해복구작업의 task 를 build 하고 operation 을 run 으로 변경하고 state 를 pending 으로 변경
// 동일한 보호그룹에 대해 실행중인 재해복구작업이 없고, 대기열에 제일 먼저 추가된 작업의 state 를 running 으로 변경
func handleWaitingRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) (bool, error) {
	logger.Infof("[handleWaitingRecoveryJob] Run: job(%d)", job.RecoveryJobID)

	var jobDetail drms.RecoveryJob
	err := job.GetDetail(&jobDetail)
	if err != nil {
		logger.Errorf("[handleWaitingRecoveryJob] Could not get job detail: dr.recovery.job/%d/detail. Cause: %v", job.RecoveryJobID, err)
		return false, err
	}

	// recovery cluster 의 storage 가 모두 사용 가능한 상태가 아닐 경우 재해 복구를 진행할 수 없다.
	var storages []uint64
	for _, s := range jobDetail.GetPlan().GetDetail().GetStorages() {
		if s.GetRecoveryClusterStorage().GetStatus() != constant.ClusterStorageStateAvailable {
			storages = append(storages, s.GetRecoveryClusterStorage().GetId())
			logger.Warnf("[handleWaitingRecoveryJob] Storage(%d) is not available state: job(%d).",
				s.GetRecoveryClusterStorage().GetId(), job.RecoveryJobID)
		}
	}

	if len(storages) > 0 {
		return false, drCluster.UnavailableStorageExisted(storages)
	}

	var status *migrator.RecoveryJobStatus
	// 대기열의 재해복구작업의 task 를 생성한다.
	if err = store.Transaction(func(txn store.Txn) error {
		// task 생성
		if err = builder.BuildTasks(ctx, txn, job, &jobDetail); err != nil {
			logger.Errorf("[handleWaitingRecoveryJob] Could not build tasks: job(%d). Cause: %+v", job.RecoveryJobID, err)
			return err
		}
		logger.Infof("[handleWaitingRecoveryJob] Done - Build tasks: job(%d)", job.RecoveryJobID)

		// run job 의 count 를 증가시킨다.
		if err = job.IncreaseRunJobCount(txn); err != nil {
			logger.Errorf("[handleWaitingRecoveryJob] Could not increase run job count: dr.recovery.job/cluster/%d/run. Cause: %+v", job.RecoveryCluster.Id, err)
			return err
		}
		logger.Infof("[handleWaitingRecoveryJob] Done - Increase run job(%d) count: dr.recovery.job/cluster/%d/run", job.RecoveryJobID, job.RecoveryCluster.Id)

		// operation 을 run 으로 변경
		if err = job.SetOperation(txn, constant.RecoveryJobOperationRun); err != nil {
			logger.Errorf("[handleWaitingRecoveryJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
			return err
		}
		logger.Infof("[handleWaitingRecoveryJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationRun)

		logger.Infof("[handleWaitingRecoveryJob] Run next - SetJobStatePending: job(%d)", job.RecoveryJobID)
		if status, err = queue.SetJobStatePending(txn, job); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return false, err
	}

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:     job.RecoveryJobID,
		Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRun},
		Status:    status,
	}); err != nil {
		logger.Warnf("[handleWaitingRecoveryJob] Could not publish job(%d) operation, status. Cause: %+v", job.RecoveryJobID, err)
	}

	return false, nil
}

// handlePendingRecoveryJob 동일한 보호그룹에 대해 실행중인 재해복구작업이 없고, 대기열에 제일 먼저 추가된 작업의 state 를 running 으로 변경
func handlePendingRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[handlePendingRecoveryJob] Run: job(%d)", job.RecoveryJobID)

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[handlePendingRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	if op.Operation != constant.RecoveryJobOperationRun {
		logger.Errorf("[handlePendingRecoveryJob] Unexpected job(%d) operation: %s", job.RecoveryJobID, op.Operation)
		return internal.UnexpectedJobOperation(job.RecoveryJobID, constant.RecoveryJobStateCodePending, op.Operation)
	}

	// 먼저 큐에 들어온 동일한 보호그룹의 작업이 먼저 실행되어야 한다.
	if ok, err := preparePendingRecoveryJob(ctx, job); err != nil && !ok {
		return err
	}

	var detail drms.RecoveryJob
	if err := job.GetDetail(&detail); err != nil {
		logger.Errorf("[handlePendingRecoveryJob] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	// 재해복구 작업 유형이 재해 복구일 경우 대상 볼륨의 복제를 일시정지한다.
	if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeMigration {
		logger.Infof("[handlePendingRecoveryJob] Job(%d) type code is migration.", job.RecoveryJobID)

		// TODO : 추후 resume 기능 활성화 시, plan 의 activation flag 를 다시 활성화 해야 함
		// plan 의 activation flag 를 비활성화 하여 해당 플랜에 대해서 보호 그룹 스냅샷이 생성되지 않도록 한다.
		if err = database.GormTransaction(func(db *gorm.DB) error {
			if err := db.Table(model.Plan{}.TableName()).
				Where(&model.Plan{ID: detail.Plan.Id}).
				Update("activation_flag", false).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[handlePendingRecoveryJob] Done - Update plan active flag > false: job(%d) plan(%d)", job.RecoveryJobID, detail.Plan.Id)

			// plan 의 볼륨 mirroring 을 pause 한다.
			return pausePlanVolumes(detail.Plan)

		}); err != nil {
			logger.Errorf("[handlePendingRecoveryJob] Could not pause plan(%d) volumes. Cause: %+v", detail.Plan.Id, err)
			return err
		}
	}

	if err = checkRecoveryClusterHypervisorResource(ctx, &detail); errors.Equal(err, internal.ErrInsufficientRecoveryHypervisorResource) {
		logger.Warnf("[handlePendingRecoveryJob] Recovery hypervisor resource is insufficient: job(%d) plan(%d). Cause: %+v", job.RecoveryJobID, detail.Plan.Id, err)
		reportEvent(ctx, "cdm-dr.manager.during_recovery_job_running.warning", "insufficient_recovery_hypervisor_resource", err)
	}

	// 대기열의 재해복구작업을 시작한다.
	logger.Infof("[handlePendingRecoveryJob] Run next - SetJobStateRunning: job(%d)", job.RecoveryJobID)
	return store.Transaction(func(txn store.Txn) error {
		return queue.SetJobStateRunning(txn, job)
	})
}

// HandleRunningRecoveryJob waiting, pending running 상태를 바라보고 각 상태에 따라 수행합니다.
func HandleRunningRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) {
	for {
		select {
		case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
			// 먼저 큐에 들어온 동일한 보호그룹의 작업이 먼저 build 되어야 한다.
			ok, err := prepareWaitingRecoveryJob(ctx, job)
			if err != nil && !ok {
				continue
			} else if err == nil && !ok {
				// err 는 nil 이고 ok 가 false 라면 작업 예정시간을 (임시) 10분 초과한 job 은  queue 에서 삭제되었기 때문에
				// 지금 job 루틴을 끝낸다.
				return
			}

			var runCount uint64
			if runCount, err = job.GetRunJobCount(); err != nil {
				logger.Errorf("[HandleRunningRecoveryJob] Could not get run job(%d) count: dr.recovery.job/cluster/%d/run. Cause: %+v",
					job.RecoveryJobID, job.RecoveryCluster.Id, err)
				continue
			}

			if runCount != 0 {
				logger.Infof("[HandleRunningRecoveryJob] Done - Run job(%d) already existed: dr.recovery.job/cluster/%d/run {%d}",
					job.RecoveryJobID, job.RecoveryCluster.Id, runCount)
				continue
			}

			var clearCount uint64
			if clearCount, err = job.GetClearJobCount(); err != nil {
				logger.Errorf("[HandleRunningRecoveryJob] Could not get clear job(%d) count: dr.recovery.job/cluster/%d/clear. Cause: %+v",
					job.RecoveryJobID, job.RecoveryCluster.Id, err)
				continue
			}

			// 동일한 recovery cluster 대상으로 clear job 이 수행 중인 경우 모든 clear job 이 종료될 때까지 기다린다.
			if clearCount != 0 {
				logger.Infof("[HandleRunningRecoveryJob] Done - Clear job(%d) already existed: dr.recovery.job/cluster/%d/clear {%d}",
					job.RecoveryJobID, job.RecoveryCluster.Id, clearCount)
				continue
			}

			s, err := job.GetStatus()
			if errors.Equal(err, migrator.ErrNotFoundKey) {
				logger.Errorf("[HandleRunningRecoveryJob] Could not get job(%d) status. Cause: %+v", job.RecoveryJobID, err)
				return
			} else if err != nil {
				logger.Errorf("[HandleRunningRecoveryJob] Could not get job(%d) status. Cause: %+v", job.RecoveryJobID, err)
				continue
			}

			logger.Infof("[HandleRunningRecoveryJob] Run - job status(%s) : group(%d) job(%d)", s.StateCode, job.ProtectionGroupID, job.RecoveryJobID)

			// 재시작 됐을 때 상태가 pending 부터 작업일 때를 대비하여 재확인
			switch s.StateCode {
			case constant.RecoveryJobStateCodeWaiting:
				if ok, err := handleWaitingRecoveryJob(ctx, job); err != nil && !ok {
					if errors.Equal(err, migrator.ErrNotFoundKey) {
						return
					}
					continue
				} else if err == nil && ok {
					return
				}

				if err = handlePendingRecoveryJob(ctx, job); errors.Equal(err, migrator.ErrNotFoundKey) {
					return
				} else if err != nil {
					continue
				}

			case constant.RecoveryJobStateCodePending:
				if err = handlePendingRecoveryJob(ctx, job); errors.Equal(err, migrator.ErrNotFoundKey) {
					return
				} else if err != nil {
					continue
				}
			}

			// Publish to migrator
			if err = internal.PublishMessage(constant.QueueTriggerRunRecoveryJob, job); err != nil {
				logger.Errorf("[HandleRunningRecoveryJob] Could not publish job(%d) to migrator. Cause: %+v", job.RecoveryJobID, err)
				continue
			}

			// running 상태에서 Cancel, Pause operation 모니터링
			go HandleRecoveryJob(ctx, job, constant.RecoveryJobStateCodeRunning)

			return
		}
	}
}

// monitorRunningRecoveryJob operation 이 pause 인 재해복구작업의 state 를 paused 로 변경
// operation 이 cancel 인 재해복구작업의 state 를 canceling 으로 변경
func monitorRunningRecoveryJob(job *migrator.RecoveryJob) (bool, error) {
	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[handleRunningRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return false, err
	}

	if status.StateCode != constant.RecoveryJobStateCodeRunning {
		return true, nil
	}

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[handleRunningRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return false, err
	}

	if op.Operation == constant.RecoveryJobOperationCancel {
		logger.Infof("[handleRunningRecoveryJob] Run next - SetJobStateCanceling: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)
		if err = store.Transaction(func(txn store.Txn) error { return queue.SetJobStateCanceling(txn, job) }); err != nil {
			return false, err
		}

		return false, nil
	}

	// FIXME: Pause 기능을 지금 사용하지 않음.
	//if op.Operation == constant.RecoveryJobOperationPause {
	//	logger.Infof("[handleRunningRecoveryJob] Run next - SetJobStatePaused: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)
	//	return store.Transaction(func(txn store.Txn) error {return queue.SetJobStatePaused(txn, job, defaultPausingTime)})
	//}

	return false, nil
}

// HandleDoneRecoveryJob migrator 에서 running 상태의 작업을 수행 후 완료 되었을 때 수행하는 함수
// 모든 Task 가 완료된 작업의 state 를 completed 로 변경하고, result code 를 success, partial_success, failed 중 하나로 입력
func HandleDoneRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) {
	for {
		select {
		case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
			if err := store.Transaction(func(txn store.Txn) error {
				warningReasons, err := getJobResultWarningReasons(job)
				if err != nil {
					return err
				}

				failedReasons, err := getJobResultFailedReasons(job)
				if err != nil {
					return err
				}

				successCount, failedCount, err := getJobInstanceResultSummary(job)
				if err != nil {
					return err
				}

				if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeMigration {
					// 재해복구의 결과는 성공, 부분성공, 실패이며, 각각의 조건은 다음과 같다. (목표복구시간 미준수시 +경고)
					// - 성공: 모든 보호대상 인스턴스가 생성(+기동)되고, 모든 기동대상 인스턴스들이 정상동작한다고 판단
					// - 부분성공: 일부 보호대상 인스턴스를 복구하지 못함
					// - 실패: 모든 보호대상 인스턴스를 복구하지 못함
					var resultCode string
					if failedCount == 0 {
						resultCode = constant.RecoveryResultCodeSuccess
					} else if successCount == 0 {
						resultCode = constant.RecoveryResultCodeFailed
					} else {
						resultCode = constant.RecoveryResultCodePartialSuccess
					}
					if err = job.SetResult(txn, resultCode, warningReasons, failedReasons); err != nil {
						logger.Errorf("[HandleDoneRecoveryJob] Could not set job result: dr.recovery.job/%d/result. Cause: %+v", job.RecoveryJobID, err)
						return err
					}

					logger.Infof("[HandleDoneRecoveryJob] Done - set result: job(%d) resultCode(%s) warningReasons(%+v) failedReasons(%+v)",
						job.RecoveryJobID, resultCode, warningReasons, failedReasons)

					if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
						JobID: job.RecoveryJobID,
						Result: &migrator.RecoveryJobResult{
							ResultCode:     resultCode,
							WarningReasons: warningReasons,
							FailedReasons:  failedReasons,
						},
					}); err != nil {
						logger.Warnf("[HandleDoneRecoveryJob] Could not publish job(%d) result. Cause: %+v", job.RecoveryJobID, err)
					}

				} else {
					// 모의훈련은 목표복구시간 내에 모든 보호대상 인스턴스가 생성(+기동)되고, 모든 기동대상 인스턴스들이 정상동작한다고 판단되면 성공이다.
					if failedCount == 0 && len(warningReasons) == 0 {
						if err = job.SetResult(txn, constant.RecoveryResultCodeSuccess, nil, nil); err != nil {
							logger.Errorf("[HandleDoneRecoveryJob] Could not set job result: dr.recovery.job/%d/result. Cause: %+v", job.RecoveryJobID, err)
							return err
						}

						logger.Infof("[HandleDoneRecoveryJob] Done - set result: job(%d) resultCode(%s)",
							job.RecoveryJobID, constant.RecoveryResultCodeSuccess)

						if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
							JobID:  job.RecoveryJobID,
							Result: &migrator.RecoveryJobResult{ResultCode: constant.RecoveryResultCodeSuccess},
						}); err != nil {
							logger.Warnf("[HandleDoneRecoveryJob] Could not publish job(%d) result. Cause: %+v", job.RecoveryJobID, err)
						}

					} else {
						if err = job.SetResult(txn, constant.RecoveryResultCodeFailed, nil, append(warningReasons, failedReasons...)); err != nil {
							logger.Errorf("[HandleDoneRecoveryJob] Could not set job result: dr.recovery.job/%d/result. Cause: %+v", job.RecoveryJobID, err)
							return err
						}

						logger.Infof("[HandleDoneRecoveryJob] Done - set result: job(%d) resultCode(%s) failedReasons(%+v)",
							job.RecoveryJobID, constant.RecoveryResultCodeFailed, warningReasons)

						if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
							JobID: job.RecoveryJobID,
							Result: &migrator.RecoveryJobResult{
								ResultCode:    constant.RecoveryResultCodeFailed,
								FailedReasons: append(warningReasons, failedReasons...),
							},
						}); err != nil {
							logger.Warnf("[HandleDoneRecoveryJob] Could not publish job(%d) result. Cause: %+v", job.RecoveryJobID, err)
						}
					}
				}

				var jobDetail drms.RecoveryJob
				if err = job.GetDetail(&jobDetail); err != nil {
					logger.Errorf("[HandleDoneRecoveryJob] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
					return err
				}

				// 데이터 정리 시간을 10분
				var rollbackTime int64
				rollbackTime = defaultRollbackWaitingTime

				// 모든 run task 가 수행된 job 의 count 를 감소시킨다.
				if err = job.DecreaseRunJobCount(txn); err != nil {
					logger.Errorf("[HandleDoneRecoveryJob] Could not decrease run job count: dr.recovery.job/cluster/%d/run. Cause: %+v", job.RecoveryCluster.Id, err)
					return err
				}
				logger.Infof("[HandleDoneRecoveryJob] Done - Decrease run job(%d) count: dr.recovery.job/cluster/%d/run", job.RecoveryJobID, job.RecoveryCluster.Id)

				logger.Infof("[HandleDoneRecoveryJob] Run next - SetJobStateComplete: job(%d)", job.RecoveryJobID)
				return queue.SetJobStateComplete(txn, job, rollbackTime)
			}); err != nil {
				continue
			}

			// completed 상태에서 Retry, Rollback, Confirm operation 모니터링
			go HandleRecoveryJob(ctx, job, constant.RecoveryJobStateCodeCompleted)

			return
		}
	}
}

// HandleCancelingRecoveryJob 모든 Task 가 완료된 작업의 state 를 completed 로 변경하고, result code 를 canceled 로 입력
func HandleCancelingRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) {
	var err error
	for {
		select {
		case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
			if err = store.Transaction(func(txn store.Txn) error {
				if err = job.SetResult(txn, constant.RecoveryResultCodeCanceled, nil, nil); err != nil {
					logger.Errorf("[HandleCancelingRecoveryJob] Could not set job result: dr.recovery.job/%d/result. Cause: %+v", job.RecoveryJobID, err)
					return err
				}

				if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
					JobID:  job.RecoveryJobID,
					Result: &migrator.RecoveryJobResult{ResultCode: constant.RecoveryResultCodeCanceled},
				}); err != nil {
					logger.Warnf("[HandleCancelingRecoveryJob] Could not publish job(%d) result. Cause: %+v", job.RecoveryJobID, err)
				}

				logger.Infof("[HandleCancelingRecoveryJob] Done - set result: job(%d) resultCode(%s)",
					job.RecoveryJobID, constant.RecoveryResultCodeCanceled)

				// 모든 task 가 취소(완료)된 job 의 count 를 감소시킨다.
				if err = job.DecreaseRunJobCount(txn); err != nil {
					logger.Errorf("[HandleCancelingRecoveryJob] Could not decrease run job count: dr.recovery.job/cluster/%d/run. Cause: %+v", job.RecoveryCluster.Id, err)
					return err
				}
				logger.Infof("[HandleCancelingRecoveryJob] Done - Decrease run job(%d) count: dr.recovery.job/cluster/%d/run", job.RecoveryJobID, job.RecoveryCluster.Id)

				logger.Infof("[HandleCancelingRecoveryJob] Run next - SetJobStateComplete: job(%d)", job.RecoveryJobID)
				return queue.SetJobStateComplete(txn, job, defaultRollbackWaitingTime)
			}); err != nil {
				continue
			}

			// Cancel 상태에서 Retry, Rollback, Confirm operation 모니터링
			go HandleRecoveryJob(ctx, job, constant.RecoveryJobStateCodeCompleted)

			return
		}
	}
}

// operation 이 run 인 재해복구작업의 state 를 running 으로 변경
// operation 이 cancel 인 재해복구작업의 state 를 canceling 으로 변경
// pause timeout 이 발생한 재해복구작업의 state 를 running 으로 변경
//func handlePausedRecoveryJob(job *migrator.RecoveryJob, status *migrator.RecoveryJobStatus) error {
//	return store.Transaction(func(txn store.Txn) error {
//		op, err := job.GetOperation()
//		if err != nil {
//			logger.Errorf("[handlePausedRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
//			return err
//		}
//
//		logger.Infof("[handlePausedRecoveryJob] Run: job(%d) operation(%d)", job.RecoveryJobID, op.Operation)
//
//		if op.Operation == constant.RecoveryJobOperationCancel {
//			logger.Infof("[handlePausedRecoveryJob] Run next - SetJobStateCanceling: job(%d)", job.RecoveryJobID)
//			return queue.SetJobStateCanceling(txn, job)
//		}
//
//		if op.Operation == constant.RecoveryJobOperationRun {
//			logger.Infof("[handlePausedRecoveryJob] Run next - SetJobStateRunningByResume: job(%d)", job.RecoveryJobID)
//			return queue.SetJobStateRunningByResume(txn, job)
//		}
//
//		if op.Operation != constant.RecoveryJobOperationPause {
//			logger.Errorf("[handlePausedRecoveryJob] Unexpected job(%d) operation: %s", job.RecoveryJobID, op.Operation)
//			return internal.UnexpectedJobOperation(job.RecoveryJobID, constant.RecoveryJobStateCodePaused, op.Operation)
//		}
//
//		now := time.Now().Unix()
//		if status.ResumeAt <= now {
//			logger.Infof("[handlePausedRecoveryJob] status.ResumeAt(%+v) <= now(%+v)", time.Unix(status.ResumeAt, 0), time.Unix(now, 0))
//			if err := job.SetOperation(txn, constant.RecoveryJobOperationRun); err != nil {
//				logger.Errorf("[handlePausedRecoveryJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
//				return err
//			}
//			logger.Infof("[handlePausedRecoveryJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationRun)
//
//			if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
//				JobID:     job.RecoveryJobID,
//				Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRun},
//			}); err != nil {
//				logger.Warnf("[handlePausedRecoveryJob] Could not publish job(%d) operation. Cause: %+v", job.RecoveryJobID, err)
//			}
//			logger.Infof("[handlePausedRecoveryJob] Run next - SetJobStateRunningByResume: job(%d)", job.RecoveryJobID)
//			return queue.SetJobStateRunningByResume(txn, job)
//		}
//
//		logger.Infof("[handlePausedRecoveryJob] Done: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)
//		return nil
//	})
//}

// monitorCompletedRecoveryJob operation 이 retry 인 재해복구작업의 state 를 running 으로 변경하고, operation 을 run 으로 변경 (부분 재시도)
// operation 이 rollback 인 재해복구작업의 state 를 clearing 으로 변경
// operation 이 confirm 인 재해복구작업의 state 를 clearing 으로 변경
// 모의훈련인 경우, rollback timeout 이 발생한 재해복구작업의 state 를 clearing 으로 변경
func monitorCompletedRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) (bool, error) {
	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[HandleCompletedRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return false, err
	}

	switch op.Operation {
	case constant.RecoveryJobOperationRetry:
		var s *migrator.RecoveryJobStatus
		if err = store.Transaction(func(txn store.Txn) error {
			if err = job.SetOperation(txn, constant.RecoveryJobOperationRun); err != nil {
				logger.Errorf("[HandleCompletedRecoveryJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
				return err
			}
			logger.Infof("[HandleCompletedRecoveryJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationRun)

			logger.Infof("[HandleCompletedRecoveryJob] Run next - SetJobStateRunningByRetry: job(%d)", job.RecoveryJobID)
			if s, err = queue.SetJobStateRunningByRetry(txn, job); err != nil {
				return err
			}

			// publish to migrator
			if err = internal.PublishMessage(constant.QueueTriggerRunRecoveryJob, job); err != nil {
				logger.Errorf("[HandleCompletedRecoveryJob] Could not publish job(%d) to migrator. Cause: %+v", job.RecoveryJobID, err)
				return err
			}

			return nil
		}); err != nil {
			return false, err
		}

		// publish to monitor
		if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
			JobID:     job.RecoveryJobID,
			Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRun},
			Status:    s,
		}); err != nil {
			logger.Warnf("[HandleCompletedRecoveryJob] Could not publish job(%d) operation, status. Cause: %+v", job.RecoveryJobID, err)
		}

		return true, nil

	case constant.RecoveryJobOperationRollback, constant.RecoveryJobOperationConfirm:
		var runCount uint64
		if runCount, err = job.GetRunJobCount(); err != nil {
			logger.Errorf("[HandleCompletedRecoveryJob] Could not get run job count: dr.recovery.job/cluster/%d/run operation(%s). Cause: %+v",
				job.RecoveryCluster.Id, op.Operation, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 run job 이 수행 중인 경우 모든 run job 이 종료될 때까지 기다린다.
		if runCount != 0 {
			logger.Infof("[HandleCompletedRecoveryJob] run job(%d) exists: dr.recovery.job/cluster/%d/run {%d} operation(%s)",
				job.RecoveryJobID, job.RecoveryCluster.Id, runCount, op.Operation)
			return false, nil
		}

		var clearCount uint64
		if clearCount, err = job.GetClearJobCount(); err != nil {
			logger.Errorf("[HandleCompletedRecoveryJob] Could not get clear job(%d) count: dr.recovery.job/cluster/%d/clear. Cause: %+v",
				job.RecoveryJobID, job.RecoveryCluster.Id, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 clear job 이 수행 중인 경우 모든 clear job 이 종료될 때까지 기다린다.
		if clearCount != 0 {
			logger.Infof("[HandleCompletedRecoveryJob] Done - Clear job(%d) already existed: dr.recovery.job/cluster/%d/clear {%d}",
				job.RecoveryJobID, job.RecoveryCluster.Id, clearCount)
			return false, nil
		}

		var s *migrator.RecoveryJobStatus
		if err = store.Transaction(func(txn store.Txn) error {
			// clear job 의 count 를 증가시킨다.
			if err = job.IncreaseClearJobCount(txn); err != nil {
				logger.Errorf("[HandleCompletedRecoveryJob] Could not increase clear job count: dr.recovery.job/cluster/%d/clear operation(%s). Cause: %+v",
					job.RecoveryCluster.Id, op.Operation, err)
				return err
			}
			logger.Infof("[HandleCompletedRecoveryJob] Done - Increase clear job(%d) count: dr.recovery.job/cluster/%d/clear", job.RecoveryJobID, job.RecoveryCluster.Id)

			logger.Infof("[HandleCompletedRecoveryJob] Run next - SetJobStateClearing: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)
			if s, err = queue.SetJobStateClearing(txn, job); err != nil {
				return err
			}

			// publish to migrator
			if err = internal.PublishMessage(constant.QueueTriggerClearingRecoveryJob, job); err != nil {
				logger.Errorf("[HandleCompletedRecoveryJob] Could not publish job(%d) to migrator. Cause: %+v", job.RecoveryJobID, err)
				return err
			}

			return nil
		}); err != nil {
			return false, err
		}

		// publish to monitor
		if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{JobID: job.RecoveryJobID, Status: s}); err != nil {
			logger.Warnf("[HandleCompletedRecoveryJob] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
		}

		return true, nil

	case constant.RecoveryJobOperationRun, constant.RecoveryJobOperationCancel:
		status, err := job.GetStatus()
		if err != nil {
			logger.Errorf("[HandleCompletedRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
			return false, err
		}

		// 모의훈련작업인 경우 일정시간이 경과하면 자동으로 롤백을 진행한다.
		now := time.Now().Unix()
		if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeSimulation && status.RollbackAt < now {
			logger.Infof("[HandleCompletedRecoveryJob] Run next - RollbackJob: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)
			return false, queue.RollbackJob(ctx, job)
		}

		if now-lastLogJobTime[job.RecoveryJobID] > 60 {
			logger.Infof("[HandleCompletedRecoveryJob] Job(%d) waiting for rollback time(rollBack: %+v >= now: %+v): operation(%s)",
				job.RecoveryJobID, time.Unix(status.RollbackAt, 0), time.Unix(now, 0), op.Operation)

			lastLogJobTime[job.RecoveryJobID] = now
		}

	default:
		logger.Errorf("[HandleCompletedRecoveryJob] Unexpected: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)
		return false, internal.UnexpectedJobOperation(job.RecoveryJobID, constant.RecoveryJobStateCodeCompleted, op.Operation)
	}
	return false, nil
}

// monitorClearFailedRecoveryJob operation 이 retry-rollback 인 재해복구작업의 state 를 clearing 으로 변경
// operation 이 ignore-rollback 인 재해복구작업의 state 를 reporting 으로 변경
// rollback timeout 이 발생한 재해복구작업의 state 를 clearing 으로 변경
func monitorClearFailedRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) (bool, error) {
	op, err := job.GetOperation()
	// 사용자가 job 을 강제 삭제 했을 경우를 대비하여 job 의 operation 을 조회할 때 notFoundKey 발생 시 종료한다.
	if errors.Equal(err, migrator.ErrNotFoundKey) {
		logger.Infof("[HandleClearFailedRecoveryJob] Close ClearFailedRecoveryJob because the job could not be found.", job.RecoveryJobID, err)
		return true, err
	} else if err != nil {
		logger.Errorf("[HandleClearFailedRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return false, err
	}

	status, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[HandleClearFailedRecoveryJob] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return false, err
	}

	// 롤백실패이고, 일정시간이 경과하면 자동으로 롤백을 재시도한다.
	if op.Operation == constant.RecoveryJobOperationRollback && status.RollbackAt < time.Now().Unix() {
		logger.Infof("[HandleClearFailedRecoveryJob] Run next - RetryRollbackJob: job(%d)", job.RecoveryJobID)
		return false, queue.RetryRollbackJob(ctx, job)
	}

	logger.Infof("[HandleClearFailedRecoveryJob] Run: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)

	switch op.Operation {
	case constant.RecoveryJobOperationRetryRollback:
		var runCount uint64
		if runCount, err = job.GetRunJobCount(); err != nil {
			logger.Errorf("[HandleClearFailedRecoveryJob] Could not get run job count: dr.recovery.job/cluster/%d/run operation(%s). Cause: %+v",
				job.RecoveryCluster.Id, op.Operation, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 run job 이 수행 중인 경우 모든 run job 이 종료될 때까지 기다린다.
		if runCount != 0 {
			logger.Infof("[HandleClearFailedRecoveryJob] run job(%d) exists: dr.recovery.job/cluster/%d/run {%d} operation(%s)",
				job.RecoveryJobID, job.RecoveryCluster.Id, runCount, op.Operation)
			return false, nil
		}

		var clearCount uint64
		if clearCount, err = job.GetClearJobCount(); err != nil {
			logger.Errorf("[HandleClearFailedRecoveryJob] Could not get clear job(%d) count: dr.recovery.job/cluster/%d/clear. Cause: %+v",
				job.RecoveryJobID, job.RecoveryCluster.Id, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 clear job 이 수행 중인 경우 모든 clear job 이 종료될 때까지 기다린다.
		if clearCount != 0 {
			logger.Infof("[HandleClearFailedRecoveryJob] Done - Clear job(%d) already existed: dr.recovery.job/cluster/%d/clear {%d}",
				job.RecoveryJobID, job.RecoveryCluster.Id, clearCount)
			return false, nil
		}

		if err = store.Transaction(func(txn store.Txn) error {
			if err = job.SetOperation(txn, constant.RecoveryJobOperationRollback); err != nil {
				logger.Errorf("[HandleClearFailedRecoveryJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
				return err
			}
			logger.Infof("[HandleClearFailedRecoveryJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationRollback)

			logger.Infof("[HandleClearFailedRecoveryJob] Run next - SetJobStateClearing: job(%d)", job.RecoveryJobID)
			s, err := queue.SetJobStateClearing(txn, job)
			if err != nil {
				return err
			}

			// publish to migrator
			if err = internal.PublishMessage(constant.QueueTriggerClearingRecoveryJob, job); err != nil {
				logger.Errorf("[HandleClearFailedRecoveryJob] Could not publish job(%d) to migrator. Cause: %+v", job.RecoveryJobID, err)
				return err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
				JobID:     job.RecoveryJobID,
				Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRetryRollback},
				Status:    s,
			}); err != nil {
				logger.Warnf("[HandleClearFailedRecoveryJob] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
			}

			return nil
		}); err != nil {
			return false, err
		}

		logger.Infof("[HandleClearFailedRecoveryJob] Done: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)

		return true, nil

	case constant.RecoveryJobOperationIgnoreRollback:
		if err = store.Transaction(func(txn store.Txn) error {
			if err = builder.ClearingJobDecreaseReferenceCount(job); err != nil {
				logger.Warnf("[HandleClearFailedRecoveryJob] Could not decrease the reference count during ignore rollback. Cause: %v", err)
			}

			logger.Infof("[HandleClearFailedRecoveryJob] Run next - SetJobStateReporting: job(%d)", job.RecoveryJobID)
			if err = queue.SetJobStateReporting(txn, job); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return false, err
		}

		// 상태를 reporting 으로 변경 되면 그 이후 작업을 바로 실행한다.
		go HandelFinishRecoveryJob(ctx, job)

		logger.Infof("[HandleClearFailedRecoveryJob] Done: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)
		return true, nil

	case constant.RecoveryJobOperationRetryConfirm:
		var runCount uint64
		if runCount, err = job.GetRunJobCount(); err != nil {
			logger.Errorf("[HandleClearFailedRecoveryJob] Could not get run job count: dr.recovery.job/cluster/%d/run operation(%s). Cause: %+v",
				job.RecoveryCluster.Id, op.Operation, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 run job 이 수행 중인 경우 모든 run job 이 종료될 때까지 기다린다.
		if runCount != 0 {
			logger.Infof("[HandleClearFailedRecoveryJob] run job(%d) exists: dr.recovery.job/cluster/%d/run {%d} operation(%s)",
				job.RecoveryJobID, job.RecoveryCluster.Id, runCount, op.Operation)
			return false, nil
		}

		var clearCount uint64
		if clearCount, err = job.GetClearJobCount(); err != nil {
			logger.Errorf("[HandleClearFailedRecoveryJob] Could not get clear job(%d) count: dr.recovery.job/cluster/%d/clear. Cause: %+v",
				job.RecoveryJobID, job.RecoveryCluster.Id, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 clear job 이 수행 중인 경우 모든 clear job 이 종료될 때까지 기다린다.
		if clearCount != 0 {
			logger.Infof("[HandleClearFailedRecoveryJob] Done - Clear job(%d) already existed: dr.recovery.job/cluster/%d/clear {%d}",
				job.RecoveryJobID, job.RecoveryCluster.Id, clearCount)
			return false, nil
		}

		if err = store.Transaction(func(txn store.Txn) error {
			if err = job.SetOperation(txn, constant.RecoveryJobOperationConfirm); err != nil {
				logger.Errorf("[HandleClearFailedRecoveryJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
				return err
			}
			logger.Infof("[HandleClearFailedRecoveryJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationConfirm)

			logger.Infof("[HandleClearFailedRecoveryJob] Run next - SetJobStateClearing: job(%d)", job.RecoveryJobID)
			s, err := queue.SetJobStateClearing(txn, job)
			if err != nil {
				return err
			}

			// publish to migrator
			if err = internal.PublishMessage(constant.QueueTriggerClearingRecoveryJob, job); err != nil {
				logger.Errorf("[HandleClearFailedRecoveryJob] Could not publish job(%d) to migrator. Cause: %+v", job.RecoveryJobID, err)
				return err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
				JobID:     job.RecoveryJobID,
				Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationConfirm},
				Status:    s,
			}); err != nil {
				logger.Warnf("[HandleClearFailedRecoveryJob] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
			}

			return nil
		}); err != nil {
			return false, err
		}

		logger.Infof("[HandleClearFailedRecoveryJob] Done: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)

		return true, nil

	case constant.RecoveryJobOperationCancelConfirm:
		var runCount uint64
		if runCount, err = job.GetRunJobCount(); err != nil {
			logger.Errorf("[HandleClearFailedRecoveryJob] Could not get run job count: dr.recovery.job/cluster/%d/run operation(%s). Cause: %+v",
				job.RecoveryCluster.Id, op.Operation, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 run job 이 수행 중인 경우 모든 run job 이 종료될 때까지 기다린다.
		if runCount != 0 {
			logger.Infof("[HandleClearFailedRecoveryJob] run job(%d) exists: dr.recovery.job/cluster/%d/run {%d} operation(%s)",
				job.RecoveryJobID, job.RecoveryCluster.Id, runCount, op.Operation)
			return false, nil
		}

		var clearCount uint64
		if clearCount, err = job.GetClearJobCount(); err != nil {
			logger.Errorf("[HandleClearFailedRecoveryJob] Could not get clear job(%d) count: dr.recovery.job/cluster/%d/clear. Cause: %+v",
				job.RecoveryJobID, job.RecoveryCluster.Id, err)
			return false, err
		}

		// 동일한 recovery cluster 대상으로 clear job 이 수행 중인 경우 모든 clear job 이 종료될 때까지 기다린다.
		if clearCount != 0 {
			logger.Infof("[HandleClearFailedRecoveryJob] Done - Clear job(%d) already existed: dr.recovery.job/cluster/%d/clear {%d}",
				job.RecoveryJobID, job.RecoveryCluster.Id, clearCount)
			return false, nil
		}

		if err = store.Transaction(func(txn store.Txn) error {
			if err = job.SetOperation(txn, constant.RecoveryJobOperationRollback); err != nil {
				logger.Errorf("[HandleClearFailedRecoveryJob] Could not set job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
				return err
			}
			logger.Infof("[HandleClearFailedRecoveryJob] Done - set operation: dr.recovery.job/%d/operation {%s}", job.RecoveryJobID, constant.RecoveryJobOperationRollback)

			logger.Infof("[HandleClearFailedRecoveryJob] Run next - SetJobStateClearing: job(%d)", job.RecoveryJobID)
			s, err := queue.SetJobStateClearing(txn, job)
			if err != nil {
				return err
			}

			// publish to migrator
			if err = internal.PublishMessage(constant.QueueTriggerClearingRecoveryJob, job); err != nil {
				logger.Errorf("[HandleClearFailedRecoveryJob] Could not publish job(%d) to migrator. Cause: %+v", job.RecoveryJobID, err)
				return err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
				JobID:     job.RecoveryJobID,
				Operation: &migrator.RecoveryJobOperation{Operation: constant.RecoveryJobOperationRollback},
				Status:    s,
			}); err != nil {
				logger.Warnf("[HandleClearFailedRecoveryJob] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
			}

			return nil
		}); err != nil {
			return false, err
		}

		logger.Infof("[HandleClearFailedRecoveryJob] Done: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)

		return true, nil
	}

	return false, nil
}

func HandelFinishRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) {
	logger.Infof("[Monitor-CancelRecoveryJobQueue] Run - job reporting: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)

	for {
		select {
		case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
			s, err := job.GetStatus()
			if err != nil {
				logger.Errorf("[HandelFinishRecoveryJob] Could not get job(%d) status. Cause: %+v", job.RecoveryJobID, err)
				continue
			}

			// 재시작 됐을 때 상태가 finished 부터 작업일 때를 대비
			switch s.StateCode {
			case constant.RecoveryJobStateCodeReporting:
				if err = handleReportingRecoveryJob(ctx, job); err != nil {
					continue
				}
				if err = handleFinishedRecoveryJob(ctx, job); err != nil {
					continue
				}

				return

			case constant.RecoveryJobStateCodeFinished:
				if err = handleFinishedRecoveryJob(ctx, job); err != nil {
					continue
				}

				return
			}
		}
	}
}

// handleReportingRecoveryJob 재해복구작업의 재해복구결과 보고서를 생성하고, state 를 finished 로 변경
func handleReportingRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) error {
	logger.Infof("[HandleReportingRecoveryJob] Start: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)

	report, err := recoveryReport.Create(ctx, job)
	if err != nil {
		logger.Errorf("[HandleReportingRecoveryJob] Could not create job(%d) report. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[HandleReportingRecoveryJob] Done - created result: job(%d) result(%d)", job.RecoveryJobID, report.Id)

	job.RecoveryResultID = report.Id
	recoveryReport.CreateRecoveryResultRaw(job.RecoveryJobID, job.RecoveryResultID)

	if err = builder.DeleteJobSharedTasks(job); err != nil {
		logger.Warnf("[HandleReportingRecoveryJob] Could not delete shared tasks: job(%d). Cause: %+v", job.RecoveryJobID, err)
	}

	var status *migrator.RecoveryJobStatus
	if err = store.Transaction(func(txn store.Txn) error {
		// job 의 RecoveryResultID 를 해당 정보에 업데이트 해주기 위함
		if err = job.Put(txn); err != nil {
			logger.Warnf("[HandleReportingRecoveryJob] Could not job put: dr.recovery.job/%d. Cause: %+v", job.RecoveryJobID, err)
		}

		if status, err = queue.SetJobStateFinished(txn, job); err != nil {
			return err
		}

		return nil
	}); err != nil {
		logger.Errorf("[HandleReportingRecoveryJob] Could not set job state finished: job(%d)", job.RecoveryJobID)
		return err
	}

	if err = internal.PublishMessage(constant.QueueRecoveryJobMonitor, migrator.RecoveryJobMessage{
		JobID:  job.RecoveryJobID,
		Status: status,
	}); err != nil {
		logger.Warnf("[HandleReportingRecoveryJob] Could not publish job(%d) status. Cause: %+v", job.RecoveryJobID, err)
	}

	logger.Infof("[HandleReportingRecoveryJob] Success: group(%d) job(%d)", job.ProtectionGroupID, job.RecoveryJobID)
	return nil
}

// handleFinishedRecoveryJob 재해복구작업을 삭제
func handleFinishedRecoveryJob(ctx context.Context, job *migrator.RecoveryJob) error {
	// 재해복구작업을 monitoring 중이라면 finished 상태가 클라이언트에 최소 1회 이상 전달되어야 함
	// max monitoring interval (30초) 만큼 기다렸다가 queue 에서 삭제하는 방식으로 처리
	time.Sleep(30 * time.Second)

	var err error
	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[handleFinishedRecoveryJob] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[handleFinishedRecoveryJob] Run: job(%d) operation(%s)", job.RecoveryJobID, op.Operation)

	if err = deleteJob(job); err != nil {
		logger.Warnf("[handleFinishedRecoveryJob] Could not delete the job(%d) completely. Cause: %+v", job.RecoveryJobID, err)
	}

	//if job.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
	//	// 최신데이터로 모의훈련을 진행한 작업인 경우, 사용한(생성한) 스냅샷을 제거한다.
	//	// 최신데이터로 재해복구을 진행한 작업이고 operation 이 확정이 아닌 경우 인스턴스의 볼륨들의 mirroring 을 재개한다.
	//	// finish 단계에 올 수 있는 작업의 operation 은 confirm, rollback, ignore-rollback 이 있다.
	//
	//	// 최신 데이터로 모의훈련은 지원하지 않음
	//	if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeSimulation {
	//		if err := snapshot.DeleteSnapshot(job.ProtectionGroupID, job.RecoveryPointSnapshot.Id); err != nil {
	//			return err
	//		}
	//	}
	//
	//	// protection cluster 가 재해 상황이므로 이므로 resume 할 수 없음
	//	else if job.RecoveryJobTypeCode == constant.RecoveryTypeCodeMigration &&
	//		op.Operation != constant.RecoveryJobOperationConfirm {
	//		resumePlanVolumes(jobDetail.Plan)
	//	}
	//}

	// 재해복구 확정인 경우, 확정에 대한 처리를 진행한다.
	if op.Operation == constant.RecoveryJobOperationConfirm {
		if err = handleConfirmRecoveryJob(ctx, job); err != nil {
			return err
		}

		tasks, err := job.GetTaskList()
		if err != nil {
			logger.Warnf("[handleFinishedRecoveryJob] Could not get task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		}

		for _, task := range tasks {
			result, err := task.GetResult(migrator.WithSharedTask(true))
			if err != nil {
				logger.Warnf("[handleFinishedRecoveryJob] Could not get task result: job(%d) task(%s:%s). Cause: %+v", job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				continue
			}

			if result.ResultCode != constant.MigrationTaskResultCodeSuccess {
				continue
			}

			if task.SharedTaskKey == "" {
				continue
			}

			if err = task.SetConfirm(&migrator.RecoveryJobTaskConfirm{RecoveryJobID: job.RecoveryJobID, ConfirmedAt: time.Now().Unix()}); err != nil {
				logger.Warnf("[handleFinishedRecoveryJob] Could not confirm the task: job(%d) task(%s:%s). Cause: %+v",
					job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			}

			logger.Infof("[handleFinishedRecoveryJob] Done - set task confirm: job(%d) task(%s:%s)",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		}
	}

	if err = store.Transaction(func(txn store.Txn) error {
		// job clear 의 count 를 감소시킨다.
		if err = job.DecreaseClearJobCount(txn); err != nil {
			logger.Errorf("[handleFinishedRecoveryJob] Could not decrease clear job count: dr.recovery.job/cluster/%d/clear. Cause: %+v",
				job.RecoveryCluster.Id, err)
			return err
		}
		logger.Infof("[handleFinishedRecoveryJob] Done - Decrease clear job(%d) count: dr.recovery.job/cluster/%d/clear",
			job.RecoveryJobID, job.RecoveryCluster.Id)

		queue.DeleteJob(txn, job)

		if err = internal.PublishMessage(constant.QueueTriggerRecoveryJobDeleted, job.RecoveryJobID); err != nil {
			logger.Errorf("[HandleFinishedRecoveryJob] Could not publish job(%d) deleted. Cause: %+v", job.RecoveryJobID, err)
			return err
		}
		if err = internal.PublishMessage(constant.QueueTriggerFinishedRecoveryJob, job); err != nil {
			logger.Errorf("[handleFinishedRecoveryJob] Could not publish job(%d) deleted to migrator. Cause: %+v", job.RecoveryJobID, err)
			return err
		}

		return nil
	}); err != nil {
		logger.Errorf("[handleFinishedRecoveryJob] Could not delete job(%d) queue. Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	logger.Infof("[handleFinishedRecoveryJob] Done: job(%d)", job.RecoveryJobID)
	return nil
}

// HandleRecoveryJob 재해복구작업의 상태를 핸들링한다.
func HandleRecoveryJob(ctx context.Context, job *migrator.RecoveryJob, state string) {
	switch state {
	case constant.RecoveryJobStateCodeRunning:
		logger.Infof("[HandleRecoveryJob] Start the Running Handler for a job(%d).", job.RecoveryJobID)
		go func() {
			for {
				select {
				case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
					if ok, _ := monitorRunningRecoveryJob(job); ok {
						// 상태가 Completed, Canceling 중 하나가 되었기 때문에 running 을 바라보고 있는 goroutine 종료.
						logger.Infof("[HandleRecoveryJob] Close the Running Handler for a job(%d).", job.RecoveryJobID)
						return
					}
				}
			}
		}()

	case constant.RecoveryJobStateCodeCompleted:
		logger.Infof("[HandleRecoveryJob] Start the Complete Handler for a job(%d).", job.RecoveryJobID)
		go func() {
			for {
				select {
				case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
					// true 를 반환하면 루틴 종료
					if ok, _ := monitorCompletedRecoveryJob(ctx, job); ok {
						// lastLogJobTime 데이터 삭제
						delete(lastLogJobTime, job.RecoveryJobID)

						logger.Infof("[HandleRecoveryJob] Close the Complete Handler for a job(%d).", job.RecoveryJobID)
						return
					}
				}
			}
		}()

	case constant.RecoveryJobStateCodeClearFailed:
		logger.Infof("[HandleRecoveryJob] Start the ClearFailed Handler for a job(%d).", job.RecoveryJobID)

		go func() {
			for {
				select {
				case <-time.After(time.Duration(defaultRecoveryJobHandlerInterval) * time.Second):
					if ok, _ := monitorClearFailedRecoveryJob(ctx, job); ok {
						// job 상태를 다시 clearing 으로 바꾸고 실행을 했기 때문에 지금 goroutine 은 종료한다.
						logger.Infof("[HandleRecoveryJob] Close the ClearFailed Handler for a job(%d).", job.RecoveryJobID)
						return
					}
				}
			}
		}()
	}
}
