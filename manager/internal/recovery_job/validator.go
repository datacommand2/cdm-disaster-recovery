package recoveryjob

import (
	"context"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
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

func validateRecoveryJobRequest(pgid uint64, job *drms.RecoveryJob) error {
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

	if !internal.IsRecoveryPointTypeCode(job.RecoveryPointTypeCode) {
		return errors.UnavailableParameterValue("job.recovery_point_type_code", job.RecoveryPointTypeCode, internal.RecoveryPointTypeCodes)
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

func checkUpdatableRecoveryJob(orig *model.Job, req *drms.UpdateRecoveryJobRequest) error {
	if req.JobId != req.Job.Id || orig.ID != req.Job.Id {
		return errors.UnchangeableParameter("job.id")
	}

	if orig.RecoveryPlanID != req.Job.Plan.Id {
		return errors.UnchangeableParameter("job.plan.id")
	}

	if orig.TypeCode != req.Job.TypeCode {
		return errors.UnchangeableParameter("job.type_code")
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

	return nil
}
