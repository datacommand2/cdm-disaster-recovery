package recoveryjob

import (
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	commonConstant "github.com/datacommand2/cdm-cloud/common/constant"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-cloud/common/store"
	identity "github.com/datacommand2/cdm-cloud/services/identity/proto"
	scheduler "github.com/datacommand2/cdm-cloud/services/scheduler/proto"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/cluster"
	protectionGroup "github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/protection_group"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/recovery_job/queue"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/recovery_job/queue/builder"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/recovery_plan"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/snapshot"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jinzhu/gorm"
	"github.com/micro/go-micro/v2/client/grpc"

	"context"
	"encoding/json"
	"math"
)

func getRecoveryJobList(filters ...recoveryJobFilter) ([]*model.Job, error) {
	var m []*model.Job
	var err error
	if err = database.Execute(func(db *gorm.DB) error {
		cond := db.Model(&model.Job{}).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan.id = cdm_disaster_recovery_job.recovery_plan_id")

		for _, f := range filters {
			if cond, err = f.Apply(cond); err != nil {
				return err
			}
		}

		return cond.Order("next_runtime").Find(&m).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return m, nil
}

func getRecoveryJob(gid, jid uint64) (*model.Job, error) {
	var m model.Job
	err := database.Execute(func(db *gorm.DB) error {
		return db.Model(&m).
			Select("*").
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan.id = cdm_disaster_recovery_job.recovery_plan_id").
			Where(&model.Plan{ProtectionGroupID: gid}).
			Where(&model.Job{ID: jid}).
			First(&m).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil, internal.NotFoundRecoveryJob(gid, jid)

	case err != nil:
		return nil, errors.UnusableDatabase(err)
	}

	return &m, nil
}

func getProtectionGroup(id, tid uint64) (*model.ProtectionGroup, error) {
	var m model.ProtectionGroup
	err := database.Execute(func(db *gorm.DB) error {
		return db.First(&m, &model.ProtectionGroup{ID: id, TenantID: tid}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil, internal.NotFoundProtectionGroup(id, tid)

	case err != nil:
		return nil, errors.UnusableDatabase(err)
	}
	return &m, nil
}

func getProtectionGroupSnapshot(ctx context.Context, pgid, pgsid uint64) (*drms.ProtectionGroupSnapshot, error) {
	//TODO: Not implemented
	return nil, nil
}

func getOperator(ctx context.Context, id uint64) (*identity.SimpleUser, error) {
	cli := identity.NewIdentityService(commonConstant.ServiceIdentity, grpc.NewClient())
	rsp, err := cli.GetSimpleUser(ctx, &identity.GetSimpleUserRequest{UserId: id})
	if errors.IsIPCFailed(err) {
		return nil, errors.IPCFailed(err)
	}

	return rsp.SimpleUser, nil
}

func getRecoveryJobsPagination(filters ...recoveryJobFilter) (*drms.Pagination, error) {
	var err error
	var offset, limit, total uint64
	if err = database.Execute(func(db *gorm.DB) error {
		cond := db.Model(&model.Job{}).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan.id = cdm_disaster_recovery_job.recovery_plan_id")

		for _, f := range filters {
			if _, ok := f.(*paginationFilter); ok {
				offset = f.(*paginationFilter).Offset
				limit = f.(*paginationFilter).Limit
				continue
			}

			if cond, err = f.Apply(cond); err != nil {
				return err
			}
		}

		return cond.Count(&total).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	if limit == 0 {
		return &drms.Pagination{
			Page:       &wrappers.UInt64Value{Value: 1},
			TotalPage:  &wrappers.UInt64Value{Value: 1},
			TotalItems: &wrappers.UInt64Value{Value: total},
		}, nil
	}

	return &drms.Pagination{
		Page:       &wrappers.UInt64Value{Value: offset/limit + 1},
		TotalPage:  &wrappers.UInt64Value{Value: (total + limit - 1) / limit},
		TotalItems: &wrappers.UInt64Value{Value: total},
	}, nil
}

func getSchedule(ctx context.Context, id uint64) (*scheduler.Schedule, int64, error) {
	cli := scheduler.NewSchedulerService(commonConstant.ServiceScheduler, grpc.NewClient())
	rsp, err := cli.GetSchedule(ctx, &scheduler.ScheduleRequest{Schedule: &scheduler.Schedule{Id: id}})
	if errors.IsIPCFailed(err) {
		return nil, 0, errors.IPCFailed(err)
	}

	return rsp.Schedule, rsp.NextRuntime, nil
}

func createJobSchedule(ctx context.Context, s *scheduler.Schedule, pgid, jid uint64) (uint64, int64, error) {
	b, err := json.Marshal(&internal.ScheduleMessage{
		ProtectionGroupID: pgid,
		RecoveryJobID:     jid,
	})
	if err != nil {
		return 0, 0, errors.Unknown(err)
	}

	s.Topic = constant.QueueTriggerRecoveryJob
	s.Message = string(b)
	s.EndAt = math.MaxInt64

	cli := scheduler.NewSchedulerService(commonConstant.ServiceScheduler, grpc.NewClient())
	rsp, err := cli.CreateSchedule(ctx, &scheduler.ScheduleRequest{Schedule: s})
	if errors.GetIPCStatusCode(err) == errors.IPCStatusBadRequest {
		return 0, 0, internal.IPCFailedBadRequest(err)
	} else if errors.IsIPCFailed(err) {
		return 0, 0, errors.IPCFailed(err)
	}
	return rsp.Schedule.Id, rsp.NextRuntime, nil
}

func updateJobSchedule(ctx context.Context, s *scheduler.Schedule, pgid, jid, sid uint64) (int64, error) {
	b, err := json.Marshal(&internal.ScheduleMessage{
		ProtectionGroupID: pgid,
		RecoveryJobID:     jid,
	})
	if err != nil {
		return 0, errors.Unknown(err)
	}

	s.Id = sid
	s.Topic = constant.QueueTriggerRecoveryJob
	s.Message = string(b)
	s.EndAt = math.MaxInt64

	cli := scheduler.NewSchedulerService(commonConstant.ServiceScheduler, grpc.NewClient())
	rsp, err := cli.UpdateSchedule(ctx, &scheduler.ScheduleRequest{Schedule: s})
	if errors.GetIPCStatusCode(err) == errors.IPCStatusBadRequest {
		return 0, internal.IPCFailedBadRequest(err)
	} else if errors.IsIPCFailed(err) {
		return 0, errors.IPCFailed(err)
	}

	return rsp.NextRuntime, nil
}

func calculateNextRuntime(ctx context.Context, s *scheduler.Schedule, fromNow bool) (int64, error) {
	s.Topic = constant.QueueTriggerRecoveryJob
	s.EndAt = math.MaxInt64

	cli := scheduler.NewSchedulerService(commonConstant.ServiceScheduler, grpc.NewClient())
	rsp, err := cli.CalculateNextRuntime(ctx, &scheduler.ScheduleNextRuntimeRequest{Schedule: s, FromNow: fromNow})
	if errors.GetIPCStatusCode(err) == errors.IPCStatusBadRequest {
		return 0, internal.IPCFailedBadRequest(err)
	} else if errors.IsIPCFailed(err) {
		return 0, errors.IPCFailed(err)
	}
	return rsp.NextRuntime, nil
}

func deleteJobSchedule(ctx context.Context, s *scheduler.Schedule) error {
	cli := scheduler.NewSchedulerService(commonConstant.ServiceScheduler, grpc.NewClient())
	_, err := cli.DeleteSchedule(ctx, &scheduler.ScheduleRequest{Schedule: s})
	if errors.IsIPCFailed(err) {
		return errors.IPCFailed(err)
	}
	return nil
}

// GetList 작업목록 조회
func GetList(ctx context.Context, req *drms.RecoveryJobListRequest) ([]*drms.RecoveryJob, *drms.Pagination, error) {
	var err error

	if err = validateRecoveryJobListRequest(req); err != nil {
		logger.Errorf("[RecoveryJob-GetList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-GetList] Errors occurred during checking accessible status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, nil, err
	}

	filters := makeRecoveryJobFilter(req)

	var jobs []*model.Job
	if jobs, err = getRecoveryJobList(filters...); err != nil {
		logger.Errorf("[RecoveryJob-GetList] Could not get the recovery job list. Cause: %+v", err)
		return nil, nil, err
	}

	var recoveryJobs []*drms.RecoveryJob
	for _, j := range jobs {
		var job drms.RecoveryJob
		if err = job.SetFromModel(j); err != nil {
			return nil, nil, err
		}

		if job.Operator, err = getOperator(ctx, j.OperatorID); err != nil {
			logger.Errorf("[RecoveryJob-GetList] Could not get the job(%d) operator(%d). Cause: %+v", j.ID, j.OperatorID, err)
			return nil, nil, err
		}

		if job.Group, err = protectionGroup.GetSimpleInfo(ctx, &drms.ProtectionGroupRequest{GroupId: req.GroupId}); err != nil {
			logger.Errorf("[RecoveryJob-GetList] Could not get the protection group(%d): job(%d). Cause: %+v", req.GroupId, j.ID, err)
			return nil, nil, err
		}

		if job.Plan, err = recoveryPlan.GetSimpleInfo(ctx, &drms.RecoveryPlanRequest{GroupId: req.GroupId, PlanId: j.RecoveryPlanID}); err != nil {
			logger.Errorf("[RecoveryJob-GetList] Could not get the recovery plan(%d): job(%d). Cause: %+v", j.RecoveryPlanID, j.ID, err)
			return nil, nil, err
		}

		if j.ScheduleID != nil && *j.ScheduleID != 0 {
			if job.Schedule, _, err = getSchedule(ctx, *j.ScheduleID); err != nil {
				logger.Errorf("[RecoveryJob-GetList] Could not get the recovery job(%d) schedule(%d): plan(%d:%s). Cause: %+v",
					j.ID, *j.ScheduleID, job.Plan.Id, job.Plan.Name, err)
				return nil, nil, err
			}
		}

		if job.StateCode, err = queue.GetJobStatus(j.ID); err != nil {
			logger.Errorf("[RecoveryJob-GetList] Could not get the recovery job(%d) status: plan(%d:%s). Cause: %+v",
				j.ID, job.Plan.Id, job.Plan.Name, err)
			return nil, nil, err
		}

		if job.OperationCode, err = queue.GetJobOperation(j.ID); err != nil {
			logger.Errorf("[RecoveryJob-GetList] Could not get the recovery job(%d) operation: plan(%d:%s). Cause: %+v",
				j.ID, job.Plan.Id, job.Plan.Name, err)
			return nil, nil, err
		}

		recoveryJobs = append(recoveryJobs, &job)
	}

	pagination, err := getRecoveryJobsPagination(filters...)
	if err != nil {
		logger.Errorf("[RecoveryJob-GetList] Could not get the recovery jobs pagination. Cause: %+v", err)
		return nil, nil, err
	}

	return recoveryJobs, pagination, nil
}

// Get 재해복구 작업을 조회하기위한 함수이다.
func Get(ctx context.Context, req *drms.RecoveryJobRequest) (*drms.RecoveryJob, error) {
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Get] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Get] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Get] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return nil, err
	}

	var job *model.Job
	if job, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Get] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return nil, err
	}

	var recoveryJob drms.RecoveryJob
	if err = recoveryJob.SetFromModel(job); err != nil {
		return nil, err
	}

	if recoveryJob.Operator, err = getOperator(ctx, job.OperatorID); err != nil {
		logger.Errorf("[RecoveryJob-Get] Could not get the job(%d) operator(%d). Cause: %+v", job.ID, job.OperatorID, err)
		return nil, err
	}

	if recoveryJob.Group, err = protectionGroup.Get(ctx, &drms.ProtectionGroupRequest{GroupId: req.GroupId}); err != nil {
		logger.Errorf("[RecoveryJob-Get] Could not get the protection group(%d): job(%d). Cause: %+v", req.GroupId, job.ID, err)
		return nil, err
	}

	// TODO: plan snapshot 의 plan 을 가져오는 경우 해당 snapshot 이 생성되는 시점 의 plan update flag 가 반영되어 현시점 기준의 plan 상태와 맞지 않게 되므로 이 부분 수정 필요
	if recoveryJob.RecoveryPointSnapshot != nil {
		recoveryJob.Plan, err = snapshot.GetPlanSnapshot(ctx, req.GroupId, recoveryJob.RecoveryPointSnapshot.Id, job.RecoveryPlanID)
		if err != nil {
			logger.Errorf("[RecoveryJob-Get] Could not get the recovery plan(%d): job(%d). Cause: %+v", job.RecoveryPlanID, job.ID, err)
			return nil, err
		}

		var mirroring []*drms.Message
		recoveryJob.Plan.MirrorStateCode, mirroring, err = recoveryPlan.GetVolumeMirrorStateCode(recoveryJob.Plan.Detail.Volumes)
		if err != nil {
			logger.Errorf("[RecoveryJob-Get] Could not get the recovery plan(%d) mirror state: job(%d). Cause: %+v", job.RecoveryPlanID, job.ID, err)
			return nil, err
		}

		if len(mirroring) > 0 {
			recoveryJob.Plan.AbnormalStateReasons.Mirroring = mirroring
		}

	} else {
		recoveryJob.Plan, err = recoveryPlan.Get(ctx, &drms.RecoveryPlanRequest{GroupId: req.GroupId, PlanId: job.RecoveryPlanID})
		if err != nil {
			logger.Errorf("[RecoveryJob-Get] Could not get the recovery plan(%d): job(%d). Cause: %+v", job.RecoveryPlanID, job.ID, err)
			return nil, err
		}
	}

	if job.ScheduleID != nil && *job.ScheduleID != 0 {
		if recoveryJob.Schedule, _, err = getSchedule(ctx, *job.ScheduleID); err != nil {
			logger.Errorf("[RecoveryJob-Get] Could not get recovery job(%d) schedule(%d). Cause: %+v", job.ID, *job.ScheduleID, err)
			return nil, err
		}
	}

	recoveryJob.StateCode, err = queue.GetJobStatus(job.ID)
	if err != nil {
		logger.Errorf("[RecoveryJob-Get] Could not get the recovery job(%d) status. Cause: %+v", job.ID, err)
		return nil, err
	}

	recoveryJob.OperationCode, err = queue.GetJobOperation(job.ID)
	if err != nil {
		logger.Errorf("[RecoveryJob-Get] Could not get the recovery job(%d) operation. Cause: %+v", job.ID, err)
		return nil, err
	}

	return &recoveryJob, nil
}

// Add 복구작업 등록
func Add(ctx context.Context, req *drms.AddRecoveryJobRequest) (*drms.RecoveryJob, error) {
	logger.Infof("[RecoveryJob-Add] Start: plan(%d)", req.GetJob().GetPlan().GetId())
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Add] Could not check the accessible status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	if err = validateRecoveryJobRequest(ctx, req.GroupId, req.Job); err != nil {
		logger.Errorf("[RecoveryJob-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = validateScheduleAddRecoveryJob(ctx, req); err != nil {
		logger.Errorf("[RecoveryJob-Add] Errors occurred during validating the job schedule. Cause: %+v", err)
		return nil, err
	}

	// 즉시 실행하거나 특정일시로 실행을 예약할 때, 모의훈련 혹은 재해복구가 불가능한 인스턴스가 존재한다면 에러와 함께 인스턴스 목록과 원인은 반환해야하며,
	// 사용자는 force 옵션을 지정하여 에러를 무시하고 모의훈련 혹은 재해복구 작업을 생성할 수 있어야 한다.
	if !req.Force {
		if err = checkUnavailableInstanceList(ctx, req.GroupId, req.Job.Plan.Id); err != nil {
			logger.Errorf("[RecoveryJob-Add] Errors occurred during checking unavailable instance list: group(%d) plan(%d). Cause: %v",
				req.GroupId, req.Job.Plan.Id, err)
			return nil, err
		}
	}

	var job *model.Job
	if job, err = req.Job.Model(); err != nil {
		return nil, err
	}

	// set job operator
	user, _ := metadata.GetAuthenticatedUser(ctx)
	job.OperatorID = user.Id
	job.NextRuntime = time.Now().Unix()

	if err = database.GormTransaction(func(db *gorm.DB) error {
		return db.Save(job).Error
	}); err != nil {
		logger.Errorf("[RecoveryJob-Add] Could not create the recovery job: group(%d) plan(%d:%s). Cause: %+v",
			req.GroupId, req.Job.Plan.Id, req.Job.Plan.Name, err)
		return nil, errors.UnusableDatabase(err)
	}

	defer func() {
		if err == nil {
			return
		}
		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Delete(job).Error
		}); err != nil {
			logger.Errorf("[RecoveryJob-Add] Could not delete recovery job: plan(%d:%s). Cause: %+v", req.Job.Plan.Id, req.Job.Plan.Name, err)
		}
	}()

	if req.Job.Schedule != nil {
		var (
			i uint64
			n int64
		)
		i, n, err = createJobSchedule(ctx, req.Job.Schedule, req.GroupId, job.ID)
		if err != nil {
			logger.Errorf("[RecoveryJob-Add] Could not create the recovery job(%d) schedule: plan(%d:%s). Cause: %+v",
				job.ID, req.Job.Plan.Id, req.Job.Plan.Name, err)
			return nil, err
		}
		logger.Infof("[RecoveryJob-Add] Done - create job(%d) schedule(%d)", job.ID, i)

		defer func() {
			if err == nil {
				return
			}
			if err = deleteJobSchedule(ctx, req.Job.Schedule); err != nil {
				logger.Warnf("[RecoveryJob-Add] Could not delete job(%d) schedule: plan(%d:%s). Cause: %+v",
					job.ID, req.Job.Plan.Id, req.Job.Plan.Name, err)
			}
		}()

		job.ScheduleID = &i
		job.NextRuntime = n

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(job).Error
		}); err != nil {
			logger.Errorf("[RecoveryJob-Add] Could not add the recovery job: plan(%d:%s). Cause: %+v", req.Job.Plan.Id, req.Job.Plan.Name, err)
			return nil, errors.UnusableDatabase(err)
		}
	}

	var j *drms.RecoveryJob
	if j, err = Get(ctx, &drms.RecoveryJobRequest{GroupId: req.GroupId, JobId: job.ID}); err != nil {
		logger.Errorf("[RecoveryJob-Add] Could not get the recovery job(%d): plan(%d:%s). Cause: %+v",
			job.ID, req.Job.Plan.Id, req.Job.Plan.Name, err)
		return nil, err
	}

	// 볼륨 스토리지(UUID) 별 복구될 총 볼륨 size 와 생성될 스냅샷의 size 계산한다.
	storageSizeMap := make(map[string]uint64)
	for _, v := range j.Plan.Detail.Volumes {
		storageSizeMap[v.GetProtectionClusterVolume().GetStorage().GetUuid()] += v.GetProtectionClusterVolume().GetSizeBytes()
	}

	for _, s := range j.Plan.Detail.Storages {
		var rsp *cms.ClusterStorage
		// plan 의 detail 의 recovery storage 정보는 plan 생성 시점의 정보이므로,
		// 현재 recovery cluster 의 storage 정보를 가져온다.
		rsp, err = cluster.GetClusterStorage(ctx, j.Plan.RecoveryCluster.Id, s.RecoveryClusterStorage.Id)
		if err != nil {
			logger.Errorf("[RecoveryJob-Add] Could not get the recovery cluster(%d) storage(%d). Cause: %+v", j.Plan.RecoveryCluster.Id, s.RecoveryClusterStorage.Id, err)
			return nil, err
		}

		// 동기화 recovery storage 의 free size 와 비교하여 free size 가 부족한 경우 실패 처리
		free := rsp.CapacityBytes - rsp.UsedBytes
		if storageSizeMap[s.GetProtectionClusterStorage().GetUuid()] > free {
			err = internal.InsufficientStorageSpace(req.GetGroupId(), rsp.GetId(), storageSizeMap[s.GetProtectionClusterStorage().GetUuid()], free)
			logger.Errorf("[RecoveryJob-Add] Errors occurred during checking the capacity of the recovery storage(%s). Cause: %+v", s.GetProtectionClusterStorage().GetUuid(), err)
			return nil, err
		}
	}

	// 재해 복구 작업 시점이 최신 데이터 일 경우
	// 보호 그룹의 인스턴스 목록이 재해 복구 계획에 인스턴스 목록과 일치 하는지 확인 한다.
	if j.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
		for _, i := range j.Plan.Detail.Instances {
			var matched = false
			for _, v := range j.Group.Instances {
				matched = matched || (v.Id == i.GetProtectionClusterInstance().GetId())
			}

			if !matched {
				err = internal.DifferentInstanceList(req.GetGroupId(), req.GetJob().GetPlan().GetId())
				logger.Errorf("[RecoveryJob-Add] Errors occurred during checking the instance list. Cause: %+v", err)
				return nil, err
			}
		}

		for _, i := range j.Group.Instances {
			var matched = false
			for _, v := range j.Plan.Detail.Instances {
				matched = matched || (i.Id == v.GetProtectionClusterInstance().GetId())
			}

			if !matched {
				err = internal.DifferentInstanceList(req.GetGroupId(), req.GetJob().GetPlan().GetId())
				logger.Errorf("[RecoveryJob-Add] Errors occurred during checking the instance list. Cause: %+v", err)
				return nil, err
			}
		}
	}

	logger.Infof("[RecoveryJob-Add] Success: job(%d) plan(%d:%s)", j.Id, j.Plan.GetId(), j.Plan.GetName())
	return j, nil
}

// Update 재해복구 작업 수정
func Update(ctx context.Context, req *drms.UpdateRecoveryJobRequest) (*drms.RecoveryJob, error) {
	logger.Infof("[RecoveryJob-Update] Start: job(%d) plan(%d)", req.GetJobId(), req.GetJob().GetPlan().GetId())

	var pg *model.ProtectionGroup
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)
	if pg, err = getProtectionGroup(req.GroupId, tid); err != nil {
		logger.Errorf("[RecoveryJob-Update] Could not get the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.JobId, err)
		return nil, err
	}

	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, pg.OwnerGroupID) {
		logger.Errorf("[RecoveryJob-Update] Errors occurred during checking the authentication of the user.")
		return nil, errors.UnauthorizedRequest(ctx)
	}

	if err = validateRecoveryJobRequest(ctx, req.GroupId, req.Job); err != nil {
		logger.Errorf("[RecoveryJob-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = validateScheduleUpdateRecoveryJob(ctx, req); err != nil {
		logger.Errorf("[RecoveryJob-Update] Errors occurred during validating the job schedule. Cause: %+v", err)
		return nil, err
	}

	var orig *model.Job
	if orig, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Update] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return nil, err
	}

	if err = checkUpdatableRecoveryJob(ctx, orig, req); err != nil {
		logger.Errorf("[RecoveryJob-Update] Errors occurred during checking updatable status of the recovery job(%d). Cause: %+v", req.JobId, err)
		return nil, err
	}

	if !req.Force {
		if err = checkUnavailableInstanceList(ctx, req.GroupId, req.Job.Plan.Id); err != nil {
			logger.Errorf("[RecoveryJob-Update] Errors occurred during checking unavailable instance list: group(%d) plan(%d:%s). Cause: %+v",
				req.GroupId, req.Job.Plan.Id, req.Job.Plan.Name, err)
			return nil, err
		}
	}

	// 함수 실패 시 스케줄러 원상 복구를 위해 임시 저장
	s, _, err := getSchedule(ctx, *orig.ScheduleID)
	if err != nil {
		logger.Errorf("[RecoveryJob-Update] Could not get the recovery job schedule(%d). Cause: %+v", *orig.ScheduleID, err)
		return nil, err
	}

	n, err := updateJobSchedule(ctx, req.Job.Schedule, req.GroupId, orig.ID, *orig.ScheduleID)
	if err != nil {
		logger.Errorf("[RecoveryJob-Update] Could not update the recovery job(%d) schedule(%d). Cause: %+v", orig.ID, *orig.ScheduleID, err)
		return nil, err
	}
	logger.Infof("[RecoveryJob-Update] Done - update job(%d) schedule(%d)", orig.ID, *orig.ScheduleID)

	defer func() {
		if err == nil {
			return
		}

		// 함수 실패 시 스케줄러 정보 원상 복구
		if _, e := updateJobSchedule(ctx, s, req.GroupId, orig.ID, *orig.ScheduleID); e != nil {
			logger.Warnf("[RecoveryJob-Update] Could not rollback job schedule(%d). Cause: %+v", orig.ScheduleID, e)
		}
	}()

	orig.RecoveryPointTypeCode = req.Job.RecoveryPointTypeCode
	orig.NextRuntime = n

	if err = database.GormTransaction(func(db *gorm.DB) error {
		return db.Save(&orig).Error
	}); err != nil {
		logger.Errorf("[RecoveryJob-Update] Could not update the recovery job(%d). Cause: %+v", orig.ID, err)
		return nil, errors.UnusableDatabase(err)
	}

	var j *drms.RecoveryJob
	if j, err = Get(ctx, &drms.RecoveryJobRequest{GroupId: req.GroupId, JobId: orig.ID}); err != nil {
		logger.Errorf("[RecoveryJob-Update] Could not get the recovery job(%d). Cause: %+v", orig.ID, err)
		return nil, err
	}

	logger.Infof("[RecoveryJob-Update] Success: job(%d) plan(%d:%s)", j.Id, j.Plan.GetId(), j.Plan.GetName())
	return j, nil
}

// Delete 재해복구 작업 삭제
func Delete(ctx context.Context, req *drms.RecoveryJobRequest, opts ...bool) error {
	logger.Infof("[RecoveryJob-Delete] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Delete] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	job, err := getRecoveryJob(req.GroupId, req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-Delete] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreTryLock(job.ID)
	if err != nil {
		logger.Errorf("[RecoveryJob-Delete] Could not lock store of the recovery job(%d). Cause: %+v", job.ID, err)
		return err
	}

	var forceDelete bool
	for _, o := range opts {
		forceDelete = o
	}

	if forceDelete {
		logger.Infof("[RecoveryJob-Delete] Force delete flag ON: job(%d)", job.ID)
	}

	defer unlock()

	var status string
	if status, err = queue.GetJobStatus(job.ID); err != nil {
		logger.Errorf("[RecoveryJob-Delete] Could not get the recovery job(%d) status. Cause: %+v", job.ID, err)
		return err
	}

	// 아직 실행되지 않았거나 대기 중인 모든 보호그룹 재해복구계획을 삭제할 수 있다.
	// 강제 삭제 옵션인 경우, 작업 상태와 상관없이 재해복구작업을 삭제할 수 있다.
	switch {
	case status == constant.RecoveryJobStateCodeWaiting, status == constant.RecoveryJobStateCodePending:
		break

	case forceDelete:
		var j *migrator.RecoveryJob
		j, err = migrator.GetJob(job.ID)
		if errors.Equal(err, migrator.ErrNotFoundJob) {
			logger.Infof("[RecoveryJob-Delete] Success - not found job: dr.recovery.job/%d", job.ID)
			return nil
		} else if err != nil {
			logger.Errorf("[RecoveryJob-Delete] Could not get job: dr.recovery.job/%d. Cause: %+v", job.ID, err)
			return err
		}

		if status == constant.RecoveryJobStateCodeRunning {
			if err = builder.RunningJobDecreaseReferenceCount(j); err != nil {
				logger.Warnf("[RecoveryJob-Delete] Could not decrease reference count: job(%d). Cause: %+v", job.ID, err)
			}
		} else if status == constant.RecoveryJobStateCodeClearing || status == constant.RecoveryJobStateCodeClearFailed {
			if err = builder.ClearingJobDecreaseReferenceCount(j); err != nil {
				logger.Warnf("[RecoveryJob-Delete] Could not decrease reference count: job(%d). Cause: %+v", job.ID, err)
			}
		} else {
			break
		}

	default:
		err = internal.UndeletableRecoveryJob(req.GroupId, req.JobId, status)
		logger.Errorf("[RecoveryJob-Delete] Could not delete: job(%d) status(%s) forceFlag(%t). Cause: %+v", job.ID, status, forceDelete, err)
		return err
	}

	if job.ScheduleID != nil && *job.ScheduleID != 0 {
		// schedule 이 없는 경우 이미 없으므로 pass
		s, _, err := getSchedule(ctx, *job.ScheduleID)
		if err == nil {
			if err = deleteJobSchedule(ctx, s); err != nil {
				logger.Warnf("[RecoveryJob-Delete] Could not delete the recovery job(%d) schedule(%d). Cause: %+v", job.ID, *job.ScheduleID, err)
			}
			logger.Infof("[RecoveryJob-Delete] Done - delete job(%d) schedule(%d)", job.ID, *job.ScheduleID)
		} else {
			logger.Warnf("[RecoveryJob-Delete] Could not get the recovery job(%d) schedule(%d). Cause: %+v", job.ID, *job.ScheduleID, err)
		}
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		return db.Where(&model.Job{ID: req.JobId}).Delete(&model.Job{}).Error
	}); err != nil {
		logger.Errorf("[RecoveryJob-Delete] Could not delete the recovery job(%d) database. Cause: %+v", job.ID, err)
		return errors.UnusableDatabase(err)
	}
	logger.Infof("[RecoveryJob-Delete] Done - deleted job(%d) database", job.ID)

	var j *migrator.RecoveryJob
	j, err = migrator.GetJob(job.ID)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		logger.Infof("[RecoveryJob-Delete] Success - not found job: dr.recovery.job/%d", job.ID)
		return nil
	} else if err != nil {
		logger.Warnf("[RecoveryJob-Delete] Could not get job: dr.recovery.job/%d. Cause: %+v", job.ID, err)
		return err
	}

	if err = store.Transaction(func(txn store.Txn) error {
		if forceDelete {
			if status == constant.RecoveryJobStateCodeClearing ||
				status == constant.RecoveryJobStateCodeClearFailed ||
				status == constant.RecoveryJobStateCodeReporting {
				//  job clear 의 count 를 감소시킨다.
				if err = j.DecreaseClearJobCount(txn); err != nil {
					logger.Errorf("[RecoveryJob-Delete] Could not decrease clear job count: dr.recovery.job/cluster/%d/clear. Cause: %+v", j.RecoveryCluster.Id, err)
					return err
				}
				logger.Infof("[RecoveryJob-Delete] Done - Decrease clear job(%d) count: dr.recovery.job/cluster/%d/clear", j.RecoveryJobID, j.RecoveryCluster.Id)
			}
		}

		queue.DeleteJob(txn, j)
		return nil
	}); err != nil {
		return err
	}

	if err = internal.PublishMessage(constant.QueueTriggerRecoveryJobDeleted, job.ID); err != nil {
		logger.Warnf("[RecoveryJob-Delete] Could not publish job(%d) deleted. Cause: %+v", job.ID, err)
	}

	logger.Infof("[RecoveryJob-Delete] Success: job(%d)", job.ID)

	return nil
}

// AddQueue 재해복구작업을 대기열에 넣는다.
func AddQueue(ctx context.Context, gid, jid uint64) (*migrator.RecoveryJob, error) {
	var job *drms.RecoveryJob
	var err error
	if job, err = Get(ctx, &drms.RecoveryJobRequest{GroupId: gid, JobId: jid}); err != nil {
		return nil, err
	}

	return queue.AddJob(ctx, job)
}

// Pause 복구작업 일시중지
func Pause(ctx context.Context, req *drms.PauseRecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-Pause] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Pause] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Pause] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Pause] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Pause] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-Pause] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotPausableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-Pause] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-Pause] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	logger.Infof("[RecoveryJob-Pause] Pause recovery job: job(%d) group(%d)", job.RecoveryJobID, job.ProtectionGroupID)
	return queue.PauseJob(job)
}

// ExtendPausingTime 복구작업 일시중지 시간 연장
func ExtendPausingTime(ctx context.Context, req *drms.ExtendRecoveryJobPausingTimeRequest) error {
	logger.Infof("[RecoveryJob-ExtendPausingTime] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.ExtendTime < 30 || req.ExtendTime > 180 {
		err = errors.OutOfRangeParameterValue("extend_time", req.ExtendTime, 30, 180)
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotExtendablePausingTimeState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-ExtendPausingTime] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	logger.Infof("[RecoveryJob-ExtendPausingTime] Extend recovery job(protection_group.id:%d, job.id:%d) pausing time", job.ProtectionGroupID, job.RecoveryJobID)

	return queue.ExtendJobPausingTime(job, req.ExtendTime*60)
}

// Resume 복구작업 재개
func Resume(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Info("[RecoveryJob-Resume] Start")

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Resume] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Resume] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Resume] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Resume] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-Resume] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotResumableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-Resume] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-Resume] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	logger.Infof("[RecoveryJob-Resume] Resume recovery job: job(%d) group(%d)", job.RecoveryJobID, job.ProtectionGroupID)
	return queue.ResumeJob(job)
}

// Cancel 복구작업 취소
func Cancel(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-Cancel] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Cancel] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Cancel] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Cancel] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Cancel] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-Cancel] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotCancelableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-Cancel] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-Cancel] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	logger.Infof("[RecoveryJob-Cancel] Cancel recovery job: job(%d) group(%d)", job.RecoveryJobID, job.ProtectionGroupID)
	return queue.CancelJob(job)
}

// IgnoreRollback 복구작업 롤백 무시
func IgnoreRollback(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-IgnoreRollback] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-IgnoreRollback] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-IgnoreRollback] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-IgnoreRollback] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-IgnoreRollback] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-IgnoreRollback] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotRollbackIgnorableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-IgnoreRollback] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-IgnoreRollback] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	return queue.IgnoreRollbackJob(job)
}

// ExtendRollbackTime 복구작업 롤백 대기시간 연장
func ExtendRollbackTime(ctx context.Context, req *drms.ExtendRecoveryJobRollbackTimeRequest) error {
	logger.Infof("[RecoveryJob-ExtendRollbackTime] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.ExtendTime < 30 || req.ExtendTime > 180 {
		err = errors.OutOfRangeParameterValue("extend_time", req.ExtendTime, 30, 180)
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotExtendableRollbackTimeState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-ExtendRollbackTime] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	logger.Infof("[RecoveryJob-ExtendRollbackTime] Extend recovery job(protection_group.id:%d, job.id:%d) rollback time", job.ProtectionGroupID, job.RecoveryJobID)
	return queue.ExtendJobRollbackTime(job, req.ExtendTime*60)
}

// Rollback 재해복구작업을 롤백한다.
func Rollback(ctx context.Context, req *drms.RecoveryJobRequest, jobTypeCode string) error {
	logger.Infof("[RecoveryJob-Rollback] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Rollback] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Rollback] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Rollback] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Rollback] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-Rollback] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotRollbackableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-Rollback] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-Rollback] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	if job.RecoveryJobTypeCode != jobTypeCode {
		err = internal.MismatchJobTypeCode(req.GroupId, req.JobId, job.RecoveryJobTypeCode)
		logger.Errorf("[RecoveryJob-Rollback] Errors occurred during checking the recovery job(%d) type code(%s:%s). Cause: %+v", req.JobId, job.RecoveryJobTypeCode, jobTypeCode, err)
		return err
	}

	return queue.RollbackJob(ctx, job)
}

// RetryRollback 재해복구작업 롤백을 재시도한다.
func RetryRollback(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-RetryRollback] Start: job(%d)", req.GetJobId())
	return nil
}

// Confirm 재해복구작업을 확정한다.
func Confirm(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-Confirm] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-Confirm] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-Confirm] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-Confirm] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-Confirm] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-Confirm] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotConfirmableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-Confirm] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-Confirm] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	return queue.ConfirmJob(ctx, job)
}

// RetryConfirm 재해복구작업 확정을 재시도한다.
func RetryConfirm(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-RetryConfirm] Start: job(%d)", req.GetJobId())
	return nil
}

// CancelConfirm 재해복구작업 확정을 취소하고 롤백한다.
func CancelConfirm(ctx context.Context, req *drms.RecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-CancelConfirm] Start: job(%d)", req.GetJobId())

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryJob-CancelConfirm] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetJobId() == 0 {
		err = errors.RequiredParameter("job_id")
		logger.Errorf("[RecoveryJob-CancelConfirm] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryJob-CancelConfirm] Could not check the accessible status of the protection group(%d): job(%d). Cause: %+v", req.GroupId, req.GroupId, err)
		return err
	}

	if _, err = getRecoveryJob(req.GroupId, req.JobId); err != nil {
		logger.Errorf("[RecoveryJob-CancelConfirm] Could not get the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	unlock, err := internal.RecoveryJobStoreLock(req.JobId)
	if err != nil {
		logger.Errorf("[RecoveryJob-CancelConfirm] Could not lock store of the recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	}

	defer unlock()

	job, err := queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		err = internal.NotConfirmCancelableState(req.GroupId, req.JobId)
		logger.Errorf("[RecoveryJob-CancelConfirm] Not found recovery job(%d). Cause: %+v", req.JobId, err)
		return err
	case err != nil:
		logger.Errorf("[RecoveryJob-CancelConfirm] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return err
	}

	return queue.CancelConfirmJob(ctx, job)
}

// Retry 재해복구작업을 재시도한다.
func Retry(ctx context.Context, req *drms.RetryRecoveryJobRequest) error {
	logger.Infof("[RecoveryJob-Retry] Start: job(%d)", req.GetJobId())
	return nil
}

// Monitor 재해복구작업을 모니터링한다.
func Monitor(req *drms.MonitorRecoveryJobRequest) (*drms.RecoveryJobStatus, error) {
	job, err := queue.GetJob(req.JobId)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		return &drms.RecoveryJobStatus{StateCode: constant.RecoveryJobStateCodeWaiting}, nil
	} else if err != nil {
		logger.Errorf("[RecoveryJob-Monitor] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return nil, err
	}

	op, err := job.GetOperation()
	if err != nil {
		logger.Errorf("[RecoveryJob-Monitor] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", req.JobId, err)
		return nil, err
	}

	s, err := job.GetStatus()
	if err != nil {
		logger.Errorf("[RecoveryJob-Monitor] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", req.JobId, err)
		return nil, err
	}

	status := drms.RecoveryJobStatus{
		StartedAt:     s.StartedAt,
		FinishedAt:    s.FinishedAt,
		ElapsedTime:   s.ElapsedTime,
		OperationCode: op.Operation,
		StateCode:     s.StateCode,
		ResumeAt:      s.ResumeAt,
		RollbackAt:    s.RollbackAt,
	}

	r, err := job.GetResult()
	if errors.Equal(err, migrator.ErrNotFoundJobResult) {
		return &status, nil

	} else if err != nil {
		logger.Errorf("[RecoveryJob-Monitor] Could not get recovery job result: dr.recovery.job/%d/result. Cause: %+v", req.JobId, err)
		return nil, err
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
			logger.Warn("[RecoveryJob-Monitor] WarningReasons is nil")
		}

	}

	for _, f := range r.FailedReasons {
		if f != nil {
			status.FailedReasons = append(status.FailedReasons, &drms.Message{
				Code:     f.Code,
				Contents: f.Contents,
			})
		} else {
			logger.Warn("[RecoveryJob-Monitor] FailedReasons is nil")
		}

	}

	return &status, nil
}

// MonitorTaskLogs 재해복구작업 작업내역을 모니터링한다.
func MonitorTaskLogs(req *drms.MonitorRecoveryJobRequest, lastSeq uint64) ([]*drms.RecoveryTaskLog, uint64, error) {
	job, err := queue.GetJob(req.JobId)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		return []*drms.RecoveryTaskLog{}, 0, nil
	} else if err != nil {
		logger.Errorf("[RecoveryJob-MonitorTaskLogs] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return nil, 0, err
	}

	var logs []*migrator.RecoveryJobLog
	var taskLogs []*drms.RecoveryTaskLog

	logs, err = job.GetLogsAfter(lastSeq)
	if err != nil {
		logger.Errorf("[RecoveryJob-MonitorTaskLogs] Could not get recovery job log: dr.recovery.job/%d/log/last_log_seq. Cause: %+v", req.JobId, err)
		return nil, 0, err
	}

	for _, l := range logs {
		taskLogs = append(taskLogs, &drms.RecoveryTaskLog{
			Code:     l.Code,
			Contents: l.Contents,
			LogSeq:   l.LogSeq,
			LogDt:    l.LogDt,
		})

		lastSeq = l.LogSeq
	}

	return taskLogs, lastSeq, nil
}

// MonitorInstances 재해복구작업 인스턴스를 모니터링한다.
func MonitorInstances(req *drms.MonitorRecoveryJobRequest, prev map[uint64]string) ([]*drms.RecoveryJobInstanceStatus, error) {
	job, err := queue.GetJob(req.JobId)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		return []*drms.RecoveryJobInstanceStatus{}, nil
	} else if err != nil {
		logger.Errorf("[RecoveryJob-MonitorInstances] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return nil, err
	}

	var j drms.RecoveryJob
	if err = job.GetDetail(&j); err != nil {
		logger.Errorf("[RecoveryJob-MonitorInstances] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", req.JobId, err)
		return nil, err
	}

	var statusList []*drms.RecoveryJobInstanceStatus
	for _, i := range j.Plan.Detail.Instances {
		s, err := job.GetInstanceStatus(i.ProtectionClusterInstance.Id)
		if err != nil {
			logger.Errorf("[RecoveryJob-MonitorInstances] Could not get job instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
				req.JobId, i.ProtectionClusterInstance.Id, err)
			return nil, err
		}

		if prev[i.ProtectionClusterInstance.Id] == s.StateCode {
			continue
		}

		status := drms.RecoveryJobInstanceStatus{
			Instance:              i.ProtectionClusterInstance,
			RecoveryPointTypeCode: s.RecoveryPointTypeCode,
			RecoveryPoint:         s.RecoveryPoint,
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

		prev[i.ProtectionClusterInstance.Id] = s.StateCode
	}

	return statusList, nil
}

// MonitorVolumes 재해복구작업 볼륨을 모니터링한다.
func MonitorVolumes(req *drms.MonitorRecoveryJobRequest, prev map[uint64]string) ([]*drms.RecoveryJobVolumeStatus, error) {
	job, err := queue.GetJob(req.JobId)
	if errors.Equal(err, migrator.ErrNotFoundJob) {
		return []*drms.RecoveryJobVolumeStatus{}, nil
	} else if err != nil {
		logger.Errorf("[RecoveryJob-MonitorVolumes] Could not get job: dr.recovery.job/%d. Cause: %+v", req.JobId, err)
		return nil, err
	}

	var j drms.RecoveryJob
	if err = job.GetDetail(&j); err != nil {
		logger.Errorf("[RecoveryJob-MonitorVolumes] Could not get recovery job detail: dr.recovery.job/%d/detail. Cause: %+v", req.JobId, err)
		return nil, err
	}

	var statusList []*drms.RecoveryJobVolumeStatus
	for _, v := range j.Plan.Detail.Volumes {
		s, err := job.GetVolumeStatus(v.ProtectionClusterVolume.Id)
		if err != nil {
			logger.Errorf("[RecoveryJob-MonitorVolumes] Could not get job volume status: dr.recovery.job/%d/result/volume/%d. Cause: %+v",
				req.JobId, v.ProtectionClusterVolume.Id, err)
			return nil, err
		}

		if prev[v.ProtectionClusterVolume.Id] == s.StateCode {
			continue
		}

		status := drms.RecoveryJobVolumeStatus{
			Volume:                v.ProtectionClusterVolume,
			RecoveryPointTypeCode: s.RecoveryPointTypeCode,
			RecoveryPoint:         s.RecoveryPoint,
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

		prev[v.ProtectionClusterVolume.Id] = s.StateCode
	}

	return statusList, nil
}

// GetRecoveryClusterHypervisorResources 복구대상으로 지정된 하이퍼바이저의 리소스가 모두 충분한지 확인
func GetRecoveryClusterHypervisorResources(ctx context.Context, req *drms.RecoveryHypervisorResourceRequest) ([]*drms.RecoveryHypervisorResource, bool, error) {
	var (
		err       error
		plan      *drms.RecoveryPlan
		resources []*drms.RecoveryHypervisorResource
	)

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[GetRecoveryClusterHypervisorResources] Errors occurred during validating the request. Cause: %+v", err)
		return nil, false, err
	}

	if req.GetPlanId() == 0 {
		err = errors.RequiredParameter("plan_id")
		logger.Errorf("[GetRecoveryClusterHypervisorResources] Errors occurred while validating the request. Cause: %+v", err)
		return nil, false, err
	}

	if req.GetRecoveryPointTypeCode() == "" {
		err = errors.RequiredParameter("recovery_point_type_code")
		logger.Errorf("[GetRecoveryClusterHypervisorResources] Errors occurred while validating the request. Cause: %+v", err)
		return nil, false, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[GetRecoveryClusterHypervisorResources] Errors occurred during validating the request. Cause: %+v", err)
		return nil, false, err
	}

	if req.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
		// 최신 데이터
		plan, err = recoveryPlan.Get(ctx, &drms.RecoveryPlanRequest{GroupId: req.GroupId, PlanId: req.PlanId})
		if err != nil {
			return nil, false, err
		}
	} else {
		return nil, false, nil
	}

	hypervisorMap := make(map[uint64]*drms.RecoveryHypervisorResource)
	for _, i := range plan.Detail.GetInstances() {
		if i.GetRecoveryClusterHypervisor() == nil {
			continue
		}

		if _, ok := hypervisorMap[i.RecoveryClusterHypervisor.Id]; !ok {
			hypervisor, err := cluster.GetClusterHypervisorWithSync(ctx, plan.RecoveryCluster.Id, i.RecoveryClusterHypervisor.Id)
			if err != nil {
				return nil, false, err
			}

			hypervisorMap[i.RecoveryClusterHypervisor.Id] = &drms.RecoveryHypervisorResource{
				Id:                hypervisor.Id,
				Name:              hypervisor.Hostname,
				VcpuTotalCnt:      hypervisor.VcpuTotalCnt,
				VcpuUsedCnt:       hypervisor.VcpuUsedCnt,
				VcpuExpectedUsage: hypervisor.VcpuUsedCnt,
				MemTotalBytes:     hypervisor.MemTotalBytes,
				MemUsedBytes:      hypervisor.MemUsedBytes,
				MemExpectedUsage:  hypervisor.MemUsedBytes,
				DiskTotalBytes:    hypervisor.DiskTotalBytes,
				DiskUsedBytes:     hypervisor.DiskUsedBytes,
				DiskExpectedUsage: hypervisor.DiskUsedBytes,
			}
		}

		hypervisorMap[i.RecoveryClusterHypervisor.Id].VcpuExpectedUsage += i.ProtectionClusterInstance.Spec.VcpuTotalCnt
		hypervisorMap[i.RecoveryClusterHypervisor.Id].MemExpectedUsage += i.ProtectionClusterInstance.Spec.MemTotalBytes
		hypervisorMap[i.RecoveryClusterHypervisor.Id].DiskExpectedUsage += i.ProtectionClusterInstance.Spec.DiskTotalBytes
	}

	usable := true
	for _, v := range hypervisorMap {
		if v.VcpuExpectedUsage > v.VcpuTotalCnt || v.MemExpectedUsage > v.MemTotalBytes {
			usable = false
		}
		resources = append(resources, v)
	}

	return resources, usable, nil
}
