package recoveryjob

import (
	"context"
	"fmt"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	schedulerConstants "github.com/datacommand2/cdm-cloud/services/scheduler/constants"
	types "github.com/datacommand2/cdm-cloud/services/scheduler/constants"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_job/queue"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
)

func getInstanceDependencies(i *cms.ClusterInstance, plan *drms.RecoveryPlan) []*cms.ClusterInstance {
	var dependencies []*cms.ClusterInstance

	for _, ii := range plan.Detail.Instances {
		for _, d := range ii.Dependencies {
			if i.Id == d.Id {
				dependencies = append(dependencies, ii.ProtectionClusterInstance)
				dependencies = append(dependencies, getInstanceDependencies(ii.ProtectionClusterInstance, plan)...)
				break
			}
		}
	}

	return dependencies
}

func checkUnavailableInstanceTenant(i *drms.InstanceRecoveryPlan, plan *drms.RecoveryPlan) []*drms.Message {
	var reasons []*drms.Message

	for _, t := range plan.Detail.Tenants {
		if i.ProtectionClusterInstance.Tenant.Id != t.ProtectionClusterTenant.Id {
			continue
		}

		if t.RecoveryClusterTenantMirrorNameUpdateFlag {
			reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_tenant.failure-recovery_cluster_tenant_mirror_name_update"})
		}
		break
	}

	return reasons
}

func checkUnavailableInstanceAvailabilityZone(i *drms.InstanceRecoveryPlan, plan *drms.RecoveryPlan) []*drms.Message {
	var reasons []*drms.Message

	if i.RecoveryClusterAvailabilityZone != nil {
		if i.RecoveryClusterAvailabilityZoneUpdateFlag {
			reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_availability_zone.failure-recovery_cluster_availability_zone_update"})
		}
	} else {
		for _, z := range plan.Detail.AvailabilityZones {
			if i.ProtectionClusterInstance.AvailabilityZone.Id != z.ProtectionClusterAvailabilityZone.Id {
				continue
			}

			if z.RecoveryClusterAvailabilityZoneUpdateFlag {
				reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_availability_zone.failure-recovery_cluster_tenant_mirror_name_update"})
			}
			break
		}
	}

	return reasons
}

func checkUnavailableInstanceNetworks(i *drms.InstanceRecoveryPlan, plan *drms.RecoveryPlan) []*drms.Message {
	var reasons []*drms.Message

	for _, n := range i.ProtectionClusterInstance.Networks {
		if n.FloatingIp == nil {
			continue
		}

		for _, fp := range plan.Detail.FloatingIps {
			if n.FloatingIp.Id != fp.ProtectionClusterFloatingIp.Id {
				continue
			}

			if fp.UnavailableFlag {
				reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_networks.failure-unavailable"})
			}
			break
		}
	}

	return reasons
}

func checkUnavailableInstanceRouters(i *drms.InstanceRecoveryPlan, plan *drms.RecoveryPlan) []*drms.Message {
	var reasons []*drms.Message

	for _, r := range i.ProtectionClusterInstance.Routers {
		var routerPlanExisted = false
		for _, rp := range plan.Detail.Routers {
			if r.Id != rp.ProtectionClusterRouter.Id {
				continue
			}

			routerPlanExisted = true
			if rp.RecoveryClusterExternalNetworkUpdateFlag {
				reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_routers.failure-recovery_cluster_external_network_update"})
			}
			break
		}

		if routerPlanExisted {
			continue
		}

		for _, en := range plan.Detail.ExternalNetworks {
			if r.ExternalRoutingInterfaces[0].Network.Id != en.ProtectionClusterExternalNetwork.Id {
				continue
			}

			if en.RecoveryClusterExternalNetworkUpdateFlag {
				reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_routers.failure-recovery_cluster_external_network_update"})
			}
			break
		}
	}

	return reasons
}

func checkUnavailableInstanceVolumes(i *drms.InstanceRecoveryPlan, plan *drms.RecoveryPlan) []*drms.Message {
	var reasons []*drms.Message

	for _, v := range i.ProtectionClusterInstance.Volumes {
		for _, vp := range plan.Detail.Volumes {
			if v.Volume.Id != vp.ProtectionClusterVolume.Id {
				continue
			}

			if vp.RecoveryClusterStorageUpdateFlag {
				reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_volumes.failure-recovery_cluster_storage_update"})
			}
			if vp.UnavailableFlag {
				reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_volumes.failure-unavailable"})
			}

			if vp.RecoveryClusterStorage == nil {
				for _, sp := range plan.Detail.Storages {
					if v.Storage.Id != sp.ProtectionClusterStorage.Id {
						continue
					}

					if sp.RecoveryClusterStorageUpdateFlag {
						reasons = append(reasons, &drms.Message{Code: "cdm-dr.manager.check_unavailable_instance_volumes.failure-recovery_cluster_storage_update"})
					}
					break
				}
			}
			break
		}
	}

	return reasons
}

func checkUnavailableInstanceList(ctx context.Context, pgid, pid uint64) error {
	var plan *drms.RecoveryPlan
	var err error

	if plan, err = recoveryPlan.Get(ctx, &drms.RecoveryPlanRequest{GroupId: pgid, PlanId: pid}); err != nil {
		return err
	}

	var unavailableInstanceMap = make(map[uint64]*internal.UnavailableInstance)

	for _, i := range plan.Detail.Instances {
		var reasons []*drms.Message

		reasons = append(reasons, checkUnavailableInstanceTenant(i, plan)...)
		reasons = append(reasons, checkUnavailableInstanceAvailabilityZone(i, plan)...)
		reasons = append(reasons, checkUnavailableInstanceNetworks(i, plan)...)
		reasons = append(reasons, checkUnavailableInstanceRouters(i, plan)...)
		reasons = append(reasons, checkUnavailableInstanceVolumes(i, plan)...)

		if len(reasons) > 0 {
			unavailableInstanceMap[i.ProtectionClusterInstance.Id] = &internal.UnavailableInstance{
				Instance:           i.ProtectionClusterInstance,
				UnavailableReasons: reasons,
			}
		}
	}

	var dependencies []*cms.ClusterInstance
	for _, i := range unavailableInstanceMap {
		dependencies = append(dependencies, getInstanceDependencies(i.Instance, plan)...)
	}

	for _, i := range dependencies {
		if _, ok := unavailableInstanceMap[i.Id]; !ok {
			unavailableInstanceMap[i.Id] = &internal.UnavailableInstance{
				Instance:           i,
				UnavailableReasons: []*drms.Message{{Code: "cdm-dr.manager.check_unavailable_instance_list.failure-unavailable"}},
			}
		}
	}

	if len(unavailableInstanceMap) > 0 {
		return internal.UnavailableInstanceExisted(unavailableInstanceMap)
	}

	return nil
}

func validateRecoveryJobListRequest(req *drms.RecoveryJobListRequest) error {
	if req.GetGroupId() == 0 {
		return errors.RequiredParameter("group_id")
	}

	if len(req.Name) > 255 {
		return errors.LengthOverflowParameterValue("name", req.Name, 255)
	}

	if len(req.Type) != 0 && !internal.IsRecoveryJobTypeCode(req.Type) {
		return errors.UnavailableParameterValue("type", req.Type, internal.RecoveryJobTypeCodes)
	}

	return nil
}
func validateRetryRecoveryJob(req *drms.RetryRecoveryJobRequest) error {
	if req.GetGroupId() == 0 {
		return errors.RequiredParameter("group_id")
	}

	if req.GetJobId() == 0 {
		return errors.RequiredParameter("job_id")
	}

	if req.GetRecoveryPointSnapshot() == nil {
		return errors.RequiredParameter("recovery_point_snapshot")
	}

	if req.GetInstances() == nil {
		return errors.RequiredParameter("instances")
	}

	return nil
}
func validateRecoveryJobRequest(ctx context.Context, pgid uint64, job *drms.RecoveryJob) error {
	if job == nil {
		return errors.RequiredParameter("job")
	}

	if job.GetPlan().GetId() == 0 {
		return errors.RequiredParameter("job.plan.id")
	}

	err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.Plan{ID: job.Plan.Id, ProtectionGroupID: pgid}).First(&model.Plan{}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return errors.InvalidParameterValue("job.plan.id", job.Plan.Id, "invalid plan id")

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	if !internal.IsRecoveryJobTypeCode(job.TypeCode) {
		return errors.UnavailableParameterValue("job.type_code", job.TypeCode, internal.RecoveryJobTypeCodes)
	}

	// 재해복구는 즉시(schedule == nil) 또는 특정일시(ScheduleTypeSpecified)만 가능
	if job.TypeCode == constant.RecoveryTypeCodeMigration && job.Schedule != nil && job.Schedule.Type != types.ScheduleTypeSpecified {
		return errors.InvalidParameterValue("job.schedule.type", job.Schedule.Type, "periodic job is unavailable on migration type")
	}

	if !internal.IsRecoveryPointTypeCode(job.RecoveryPointTypeCode) {
		return errors.UnavailableParameterValue("job.recovery_point_type_code", job.RecoveryPointTypeCode, internal.RecoveryPointTypeCodes)
	}

	return nil
}

func validateScheduleAddRecoveryJob(ctx context.Context, req *drms.AddRecoveryJobRequest) error {
	jobs, _, err := GetList(ctx, &drms.RecoveryJobListRequest{
		GroupId: req.GroupId,
	})
	if err != nil {
		return err
	}

	for _, job := range jobs {
		// job id가 같을 경우 check 하지 않음
		if job.Id == req.Job.Id {
			continue
		}

		// plan 이 다를 경우 check 하지 않음
		if job.Plan.Id != req.Job.Plan.Id {
			continue
		}

		// 해당 보호그룹의 재해복구작업이 진행중이거나 재해복구가 특정일시로 예약되어있다면 모의훈련 혹은 재해복구 작업을 생성할 수 없다.
		if job.TypeCode == constant.RecoveryTypeCodeMigration {
			return internal.AlreadyMigrationJobRegistered(req.GroupId)
		}

		// 복구 유형: 즉시
		if req.Job.Schedule == nil || job.Schedule == nil {
			return nil
		}

		// 복구 유형: 둘다 스케줄
		switch job.Schedule.Type {
		case types.ScheduleTypeSpecified:
			break

		default:
			if job.Schedule.Type != req.Job.Schedule.Type {
				break
			}

			if job.Schedule.Type == types.ScheduleTypeHourly && job.Schedule.StartAt != req.Job.Schedule.StartAt {
				break
			}

			// 같은 조건의 스케줄을 생성하는 경우
			if job.Schedule.DayOfMonth == req.Job.Schedule.DayOfMonth &&
				job.Schedule.DayOfWeek == req.Job.Schedule.DayOfWeek &&
				job.Schedule.IntervalMonth == req.Job.Schedule.IntervalMonth &&
				job.Schedule.IntervalWeek == req.Job.Schedule.IntervalWeek &&
				job.Schedule.IntervalDay == req.Job.Schedule.IntervalDay &&
				job.Schedule.IntervalHour == req.Job.Schedule.IntervalHour &&
				job.Schedule.Hour == req.Job.Schedule.Hour &&
				job.Schedule.Minute == req.Job.Schedule.Minute {
				return internal.RecoveryJobScheduleConflicted(fmt.Sprintf("%+v", req.Job.Schedule), job.Id, fmt.Sprintf("%+v", job.Schedule))
			}

		}

		// 복구 유형: 특정일시, 스케줄 (둘다 즉시 아닐때 모두)
		// 동일한 실행예정 시간으로는 복구 작업을 생성할 수 없다.
		nextRunTime, err := calculateNextRuntime(ctx, req.Job.Schedule, true)
		if err != nil {
			logger.Warnf("[validateScheduleAddRecoveryJob] Could not get job calculate next runtime. Cause: %+v", err)
		}

		// 기존 job 의 다음 진행 예정시간과 추가할 job 의 진행 예정시간이 같은 경우
		if job.NextRuntime == nextRunTime {
			return internal.RecoveryJobScheduleConflicted(fmt.Sprintf("%+v", req.Job.Schedule), job.Id, fmt.Sprintf("%+v", job.Schedule))
		}
	}

	return nil
}

func validateScheduleUpdateRecoveryJob(ctx context.Context, req *drms.UpdateRecoveryJobRequest) error {
	jobs, _, err := GetList(ctx, &drms.RecoveryJobListRequest{
		GroupId: req.GroupId,
	})
	if err != nil {
		return err
	}

	for _, job := range jobs {
		// job id가 같을 경우 check 하지 않음
		if job.Id == req.Job.Id {
			continue
		}

		// plan 이 다를 경우 check 하지 않음
		if job.Plan.Id != req.Job.Plan.Id {
			continue
		}

		// 이미 존재하는 job 의 수정이기 때문에 job type 에 상관없이 schedule type check 만 한다.
		// 하지만 정책변화로 아래 조건 필요시 주석 해제
		// 해당 보호그룹의 재해복구작업이 진행중이거나 재해복구가 특정일시로 예약되어있다면 모의훈련 혹은 재해복구 작업을 생성할 수 없다.
		//if job.TypeCode == constant.RecoveryTypeCodeMigration {
		//	return internal.AlreadyMigrationJobRegistered(req.GroupId)
		//}

		// 복구 유형: 즉시
		if req.Job.Schedule == nil || job.Schedule == nil {
			return nil
		}

		// 복구 유형: 둘다 스케줄
		switch job.Schedule.Type {
		case types.ScheduleTypeSpecified:
			break

		default:
			if job.Schedule.Type != req.Job.Schedule.Type {
				break
			}

			if job.Schedule.Type == types.ScheduleTypeHourly && job.Schedule.StartAt != req.Job.Schedule.StartAt {
				break
			}

			// 같은 조건의 스케줄을 생성하는 경우
			if job.Schedule.DayOfMonth == req.Job.Schedule.DayOfMonth &&
				job.Schedule.DayOfWeek == req.Job.Schedule.DayOfWeek &&
				job.Schedule.IntervalMonth == req.Job.Schedule.IntervalMonth &&
				job.Schedule.IntervalWeek == req.Job.Schedule.IntervalWeek &&
				job.Schedule.IntervalDay == req.Job.Schedule.IntervalDay &&
				job.Schedule.IntervalHour == req.Job.Schedule.IntervalHour &&
				job.Schedule.Hour == req.Job.Schedule.Hour &&
				job.Schedule.Minute == req.Job.Schedule.Minute {
				return internal.RecoveryJobScheduleConflicted(fmt.Sprintf("%+v", req.Job.Schedule), job.Id, fmt.Sprintf("%+v", job.Schedule))
			}

		}

		// 복구 유형: 특정일시, 스케줄
		// 동일한 실행예정 시간으로는 복구 작업을 생성할 수 없다.
		nextRunTime, err := calculateNextRuntime(ctx, req.Job.Schedule, true)
		if err != nil {
			logger.Warnf("[validateScheduleUpdateRecoveryJob] Could not get job calculate next runtime. Cause: %+v", err)
		}

		// 기존 job 의 다음 진행 예정시간과 추가할 job 의 진행 예정시간이 같은 경우
		if job.NextRuntime == nextRunTime {
			return internal.RecoveryJobScheduleConflicted(fmt.Sprintf("%+v", req.Job.Schedule), job.Id, fmt.Sprintf("%+v", job.Schedule))
		}
	}

	return nil
}

// ValidateMonitorRecoveryJob 는 Job 모니터링 요청에대한 validation
func ValidateMonitorRecoveryJob(ctx context.Context, req *drms.MonitorRecoveryJobRequest) error {
	if req.GetGroupId() == 0 {
		return errors.RequiredParameter("group_id")
	}

	if req.GetJobId() == 0 {
		return errors.RequiredParameter("job_id")
	}

	if err := internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		return err
	}

	err := database.Execute(func(db *gorm.DB) error {
		return db.Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan.id = cdm_disaster_recovery_job.recovery_plan_id").
			Where(&model.Plan{ProtectionGroupID: req.GroupId}).
			Where(&model.Job{ID: req.JobId}).
			First(&model.Job{}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return internal.NotFoundRecoveryJob(req.GroupId, req.JobId)

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	return nil
}

func checkUpdatableRecoveryJob(ctx context.Context, orig *model.Job, req *drms.UpdateRecoveryJobRequest) error {
	if req.JobId != req.Job.Id || orig.ID != req.Job.Id {
		return errors.UnchangeableParameter("job.id")
	}

	if orig.RecoveryPlanID != req.Job.Plan.Id {
		return errors.UnchangeableParameter("job.plan.id")
	}

	if orig.TypeCode != req.Job.TypeCode {
		return errors.UnchangeableParameter("job.type_code")
	}

	// 실행중이거나 대기열에 추가된 작업은 수정할 수 없으므로 즉시실행은 수정할 수 없다.
	if orig.ScheduleID == nil {
		return internal.UnchangeableRecoveryJob(req.GroupId, req.JobId)
	}

	if req.Job.Schedule == nil {
		return errors.RequiredParameter("job.schedule")
	}

	unlock, err := internal.RecoveryJobStoreTryLock(req.JobId)
	if err != nil {
		return err
	}

	defer unlock()

	// 실행중이거나 대기열에 추가된 작업은 수정할 수 없다
	_, err = queue.GetJob(req.JobId)
	switch {
	case errors.Equal(err, migrator.ErrNotFoundJob):
		break

	case err == nil: // 실행중이거나 대기열에 추가된 작업
		return internal.UnchangeableRecoveryJob(req.GroupId, req.JobId)

	default:
		return err
	}

	s, _, err := getSchedule(ctx, *orig.ScheduleID)
	if errors.IsIPCFailed(err) {
		return errors.IPCFailed(err)
	}

	// 특정일시 스케줄은 특정 일시로만 변경 가능
	if s.Type == schedulerConstants.ScheduleTypeSpecified && s.Type != req.Job.Schedule.Type {
		return errors.UnchangeableParameter("job.schedule.type")
	}

	// 일정 간격 스케줄은 특정 일시를 제외한 스케줄 변경 가능
	if s.Type != schedulerConstants.ScheduleTypeSpecified && req.Job.Schedule.Type == schedulerConstants.ScheduleTypeSpecified {
		return errors.UnchangeableParameter("job.schedule.type")
	}

	return nil
}
