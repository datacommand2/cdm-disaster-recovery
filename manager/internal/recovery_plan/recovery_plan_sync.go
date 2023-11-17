package recoveryplan

import (
	"context"
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	"github.com/datacommand2/cdm-center/services/cluster-manager/storage"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/cluster"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/recovery_plan/assignee"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"github.com/jinzhu/gorm"
	"time"
)

func setVolumeRecoveryPlanUnavailableFlag(ctx context.Context, pg *model.ProtectionGroup, storagePlan *model.PlanStorage, storageID uint64, reasonCode string) (err error) {
	var volPlans []*model.PlanVolume
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.PlanVolume{RecoveryPlanID: storagePlan.RecoveryPlanID}).Find(&volPlans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	flag := true

	for _, volPlan := range volPlans {
		if volPlan.RecoveryClusterStorageID != nil && *volPlan.RecoveryClusterStorageID != storageID {
			continue
		}

		v, err := cluster.GetClusterVolume(ctx, pg.ProtectionClusterID, volPlan.ProtectionClusterVolumeID)
		if err != nil {
			return err
		}

		if v.Storage.Id != storagePlan.ProtectionClusterStorageID {
			continue
		}

		volPlan.UnavailableFlag = &flag
		volPlan.UnavailableReasonCode = &reasonCode

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(&volPlan).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[setVolumeRecoveryPlanUnavailableFlag] Sync the cluster(%d) volume(%s) plan(%d) by recovery storage(%d) changed.",
			pg.ProtectionClusterID, v.Uuid, volPlan.RecoveryPlanID, storageID)
	}

	return nil
}

// SetStorageRecoveryPlanUnavailableFlag recoveryClusterID 클러스터의 storageID 스토리지에 매핑된 스토리지 재해 복구 계획의
// unavailable_flag 를 true 로 설정하는 함수
func SetStorageRecoveryPlanUnavailableFlag(ctx context.Context, recoveryClusterID, storageID uint64, reasonCode string) (err error) {
	var plans []*model.PlanStorage
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanStorage{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_storage.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanStorage{RecoveryClusterStorageID: &storageID}).
			Find(&plans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	flag := true

	for _, plan := range plans {
		plan.UnavailableFlag = &flag
		plan.UnavailableReasonCode = &reasonCode

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(plan).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[SetStorageRecoveryPlanUnavailableFlag] Sync the cluster storage(%d) plan(%d) by recovery storage(%d) changed.",
			plan.ProtectionClusterStorageID, plan.RecoveryPlanID, storageID)

		var pg *model.ProtectionGroup
		if pg, err = getProtectionGroupByPlanID(plan.RecoveryPlanID); err != nil {
			logger.Warnf("[SetStorageRecoveryPlanUnavailableFlag] Could not get the protection group from plan(%d). Cause: %+v", plan.RecoveryPlanID, err)
			return err
		}

		if err = setVolumeRecoveryPlanUnavailableFlag(ctx, pg, plan, storageID, reasonCode); err != nil {
			logger.Errorf("[SetStorageRecoveryPlanUnavailableFlag] Could not set unavailable flag of the volume recovery plan: storage(%d). Cause: %+v", storageID, err)
			return err
		}

		if err = deleteMirrorEnvironment(
			&storage.ClusterStorage{ClusterID: pg.ProtectionClusterID, StorageID: plan.ProtectionClusterStorageID},
			&storage.ClusterStorage{ClusterID: recoveryClusterID, StorageID: *plan.RecoveryClusterStorageID},
		); err != nil {
			logger.Warnf("[SetStorageRecoveryPlanUnavailableFlag] Could not delete mirror environment mirroring. Cause: %+v", err)
		}
	}

	return nil
}

// SetVolumeRecoveryPlanUnavailableFlag recoveryClusterID 클러스터의 storageID 스토리지에 복제중인 볼륨 복구 계획의
// unavailable_flag 를 true 로 설정하는 함수
func SetVolumeRecoveryPlanUnavailableFlag(recoveryClusterID, storageID uint64, reasonCode string) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var volPlans []*model.PlanVolume
		if err = db.Table(model.PlanVolume{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_volume.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanVolume{RecoveryClusterStorageID: &storageID}).
			Find(&volPlans).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true

		for _, volPlan := range volPlans {
			volPlan.UnavailableFlag = &flag
			volPlan.UnavailableReasonCode = &reasonCode

			if err = db.Save(volPlan).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetVolumeRecoveryPlanUnavailableFlag] Sync the cluster volume(%d) plan(%d) by recovery storage(%d) changed.",
				volPlan.ProtectionClusterVolumeID, volPlan.RecoveryPlanID, storageID)
		}

		return nil
	})
}

// UnsetVolumeRecoveryPlanUnavailableFlag recoveryClusterID 클러스터의 storageID 스토리지에 복제중인 볼륨 복구 계획의
// unavailable_flag 를 false 로 설정하는 함수
// volumeID: source volume id, storageID: target storage id
func UnsetVolumeRecoveryPlanUnavailableFlag(pid, volumeID, storageID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var volPlans []*model.PlanVolume
		trueFlag := true
		falseFlag := false
		ReasonCode := constant.RecoveryPlanVolumeSourceInitCopyRequired

		if err = db.Table(model.PlanVolume{}.TableName()).
			Where(&model.PlanVolume{
				RecoveryPlanID:            pid,
				ProtectionClusterVolumeID: volumeID,
				RecoveryClusterStorageID:  &storageID,
				UnavailableFlag:           &trueFlag,
				UnavailableReasonCode:     &ReasonCode,
			}).Find(&volPlans).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		if len(volPlans) == 0 {
			logger.Infof("[UnsetVolumeRecoveryPlanUnavailableFlag] Plan is not changed: plan(%d) source volume(%d) target storage(%d)", pid, volumeID, storageID)
			return nil
		}

		for _, volPlan := range volPlans {
			volPlan.UnavailableFlag = &falseFlag
			volPlan.UnavailableReasonCode = nil

			if err = db.Save(volPlan).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[UnsetVolumeRecoveryPlanUnavailableFlag] Sync the cluster volume(%d) plan(%d): target storage(%d).", pid, volumeID, storageID)
		}

		return nil
	})
}

// SetStorageRecoveryPlanRecoveryClusterStorageUpdateFlag recoveryClusterID 클러스터의 storageID 스토리지에 매핑된 스토리지 재해 복구 계획의
// recovery_cluster_storage_id 를 nil, recovery_cluster_storage_update_flag 를 true 로 설정하는 함수
func SetStorageRecoveryPlanRecoveryClusterStorageUpdateFlag(ctx context.Context, recoveryClusterID, storageID uint64, reasonCode string) (err error) {
	var plans []*model.PlanStorage
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanStorage{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_storage.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanStorage{RecoveryClusterStorageID: &storageID}).
			Find(&plans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	flag := true

	for _, plan := range plans {
		plan.RecoveryClusterStorageID = nil
		plan.RecoveryClusterStorageUpdateFlag = &flag
		plan.RecoveryClusterStorageUpdateReasonCode = &reasonCode
		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(plan).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[SetStorageRecoveryPlanRecoveryClusterStorageUpdateFlag] Sync the cluster storage(%d) plan(%d) by recovery storage(%d) changed.",
			plan.ProtectionClusterStorageID, plan.RecoveryPlanID, storageID)

		var pg *model.ProtectionGroup
		if pg, err = getProtectionGroupByPlanID(plan.RecoveryPlanID); err != nil {
			logger.Warnf("[SetStorageRecoveryPlanRecoveryClusterStorageUpdateFlag] Could not get the protection group from plan. Cause: %+v", err)
			continue
		}

		if err := setVolumeRecoveryPlanUnavailableFlag(ctx, pg, plan, storageID, reasonCode); err != nil {
			return err
		}

		if err := deleteMirrorEnvironment(
			&storage.ClusterStorage{ClusterID: pg.ProtectionClusterID, StorageID: plan.ProtectionClusterStorageID},
			&storage.ClusterStorage{ClusterID: recoveryClusterID, StorageID: *plan.RecoveryClusterStorageID},
		); err != nil {
			logger.Warnf("[SetStorageRecoveryPlanRecoveryClusterStorageUpdateFlag] Could not delete mirror environment mirroring. Cause: %+v", err)
		}
	}

	return nil
}

// SetVolumeRecoveryPlanRecoveryClusterStorageNilAndUpdateFlagTrue recoveryClusterID 클러스터의 storageID 스토리지에 복제중인 볼륨 복구 계획의
// recovery_cluster_storage_id 를 nil, recovery_cluster_storage_update_flag 를 true 로 설정하는 함수
func SetVolumeRecoveryPlanRecoveryClusterStorageNilAndUpdateFlagTrue(recoveryClusterID, storageID uint64, reasonCode string) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var volPlans []*model.PlanVolume
		if err = db.Table(model.PlanVolume{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_volume.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanVolume{RecoveryClusterStorageID: &storageID}).
			Find(&volPlans).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true

		for _, volPlan := range volPlans {
			volPlan.RecoveryClusterStorageID = nil
			volPlan.RecoveryClusterStorageUpdateFlag = &flag
			volPlan.RecoveryClusterStorageUpdateReasonCode = &reasonCode

			if err = db.Save(volPlan).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetVolumeRecoveryPlanRecoveryClusterStorageNilAndUpdateFlagTrue] Sync the cluster volume(%d) plan(%d) by recovery storage(%d) changed.",
				volPlan.ProtectionClusterVolumeID, volPlan.RecoveryPlanID, storageID)
		}

		return nil
	})
}

func mirrorStopVolumeInVolumePlan(volumeID, sourceStorageID, targetStorageID uint64) error {
	mv, err := mirror.GetVolume(sourceStorageID, targetStorageID, volumeID)
	if errors.Equal(err, mirror.ErrNotFoundMirrorVolume) {
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("[mirrorStopVolumeInVolumePlan] - MirrorVolumeOperationStopAndDelete")
	if err := mv.SetOperation(constant.MirrorVolumeOperationStopAndDelete); err != nil {
		return err
	}

	timeout := time.After(30 * time.Second)
	for {
		_, err := mirror.GetVolume(sourceStorageID, targetStorageID, volumeID)
		if errors.Equal(err, mirror.ErrNotFoundMirrorVolume) {
			return nil

		} else if err != nil {
			logger.Warnf("[mirrorStopVolumeInVolumePlan] Could not stop volume mirroring: protection(%d) recovery(%d) storage. Cause: %+v", sourceStorageID, targetStorageID, err)
			return nil
		}

		select {
		case <-time.After(3 * time.Second):
		case <-timeout:
			logger.Warnf("[mirrorStopVolumeInVolumePlan] Could not stop volume mirroring: protection(%d) recovery(%d) storage. Cause: timeout occurred", sourceStorageID, targetStorageID)
			return nil
		}
	}
}

func getProtectionGroupByPlanID(planID uint64) (*model.ProtectionGroup, error) {
	var pg model.ProtectionGroup
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Table(model.ProtectionGroup{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan.protection_group_id = cdm_disaster_recovery_protection_group.id").
			Where(&model.Plan{ID: planID}).
			First(&pg).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return &pg, nil
}

// GetForPlanSync 재해복구계획의 가용구역, 인스턴스 정보만 조회하는 함수
func GetForPlanSync(ctx context.Context, req *drms.RecoveryPlanRequest) (*drms.RecoveryPlan, error) {
	var err error
	var p *model.Plan
	var pg *model.ProtectionGroup

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		return nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)
	if pg, err = getProtectionGroup(req.GroupId, tid); err != nil {
		return nil, err
	}

	if p, err = getPlan(req.GroupId, req.PlanId); err != nil {
		return nil, err
	}

	var recoveryPlan = drms.RecoveryPlan{Detail: new(drms.RecoveryPlanDetail)}
	if err = recoveryPlan.SetFromModel(p); err != nil {
		return nil, err
	}

	if recoveryPlan.ProtectionCluster, err = cluster.GetCluster(ctx, pg.ProtectionClusterID); err != nil {
		return nil, err
	}
	if recoveryPlan.RecoveryCluster, err = cluster.GetCluster(ctx, p.RecoveryClusterID); err != nil {
		return nil, err
	}

	var zones []model.PlanAvailabilityZone
	var instances []model.PlanInstance
	if err = database.Execute(func(db *gorm.DB) error {
		if zones, err = p.GetPlanAvailabilityZones(db); err != nil {
			return errors.UnusableDatabase(err)
		}
		if instances, err = p.GetPlanInstances(db); err != nil {
			return errors.UnusableDatabase(err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if err = appendAvailabilityZoneRecoveryPlansWithDeletedInfo(ctx, &recoveryPlan, zones); err != nil {
		return nil, err
	}
	if err = appendInstanceRecoveryPlansWithDeletedInfo(ctx, &recoveryPlan, instances); err != nil {
		return nil, err
	}

	return &recoveryPlan, nil
}

// SetAvailabilityZoneRecoveryPlanUpdateFlag 재해 복구 계획 availability_zone update_flag set 함수
func SetAvailabilityZoneRecoveryPlanUpdateFlag(ctx context.Context, recoveryClusterID, availabilityZoneID uint64) (err error) {
	var list []*model.PlanAvailabilityZone

	flag := true
	reasonCode := constant.RecoveryPlanAvailabilityZoneTargetDeleted

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Table(model.PlanAvailabilityZone{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_availability_zone.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanAvailabilityZone{RecoveryClusterAvailabilityZoneID: &availabilityZoneID}).
			Find(&list).Error; err != nil {
			return err
		}

		for _, azPlan := range list {
			azPlan.RecoveryClusterAvailabilityZoneUpdateFlag = &flag
			azPlan.RecoveryClusterAvailabilityZoneUpdateReasonCode = &reasonCode

			if err = db.Save(azPlan).Error; err != nil {
				return err
			}

			logger.Infof("[SetAvailabilityZoneRecoveryPlanUpdateFlag] Sync the cluster availability zone(%d) plan(%d) by recovery availability zone(%d) changed.",
				azPlan.ProtectionClusterAvailabilityZoneID, azPlan.RecoveryPlanID, availabilityZoneID)
		}

		return nil
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	var plans []*model.Plan
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Find(&plans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, p := range plans {
		plan, err := GetForPlanSync(ctx, &drms.RecoveryPlanRequest{
			GroupId: p.ProtectionGroupID,
			PlanId:  p.ID,
		})
		if err != nil {
			return errors.UnusableDatabase(err)
		}

		instList := getInstancePlanListByAvailabilityZoneID(plan, availabilityZoneID)

		if err = database.GormTransaction(func(db *gorm.DB) error {
			for _, i := range instList {
				i.RecoveryClusterHypervisor = nil
				return updateInstanceRecoveryPlanHypervisorID(db, p.ID, i, reasonCode)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// updateInstanceRecoveryPlanHypervisorID 재해복구 계획 instance recovery_hypervisor_id set 함수
func updateInstanceRecoveryPlanHypervisorID(db *gorm.DB, planID uint64, instPlan *drms.InstanceRecoveryPlan, reasonCode string) error {
	var cols = make(map[string]interface{})

	if instPlan.RecoveryClusterHypervisor.GetId() == 0 {
		cols["recovery_cluster_hypervisor_id"] = nil
		if instPlan.RecoveryClusterAvailabilityZone.GetId() != 0 {
			cols["recovery_cluster_availability_zone_update_flag"] = true
			cols["recovery_cluster_availability_zone_update_reason_code"] = reasonCode
			cols["recovery_cluster_availability_zone_update_reason_contents"] = nil
		}
	} else {
		cols["recovery_cluster_hypervisor_id"] = instPlan.RecoveryClusterHypervisor.Id
		cols["recovery_cluster_availability_zone_update_flag"] = false
		cols["recovery_cluster_availability_zone_update_reason_code"] = nil
		cols["recovery_cluster_availability_zone_update_reason_contents"] = nil
	}

	if len(cols) > 0 {
		err := db.Model(&model.PlanInstance{}).
			Where(&model.PlanInstance{RecoveryPlanID: planID, ProtectionClusterInstanceID: instPlan.ProtectionClusterInstance.Id}).
			Updates(cols).Error
		if err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateInstanceRecoveryPlanHypervisorID] Sync the cluster instance(%d) plan(%d) by recovery hypervisor changed.", instPlan.ProtectionClusterInstance.Id, planID)
	}

	return nil
}

// getInstancePlanListByAvailabilityZoneID plan 에 recoveryAvailabilityZone 이 availabilityZoneID 인 instance List 얻는 함수
func getInstancePlanListByAvailabilityZoneID(plan *drms.RecoveryPlan, availabilityZoneID uint64) []*drms.InstanceRecoveryPlan {
	azMap := make(map[uint64]uint64)
	for _, az := range plan.Detail.AvailabilityZones {
		azMap[az.ProtectionClusterAvailabilityZone.Id] = az.RecoveryClusterAvailabilityZone.Id
	}

	var instanceList []*drms.InstanceRecoveryPlan
	for _, instance := range plan.Detail.Instances {
		if instance.RecoveryClusterAvailabilityZone.GetId() == 0 {
			if azMap[instance.ProtectionClusterInstance.GetAvailabilityZone().GetId()] == availabilityZoneID {
				instanceList = append(instanceList, instance)
			}
		} else {
			if instance.RecoveryClusterAvailabilityZone.Id == availabilityZoneID {
				instanceList = append(instanceList, instance)
			}
		}
	}
	return instanceList
}

// getInstancePlanByID instancePlanList 에서 instance id 로 get instance
func getInstancePlanByID(instPlanList []*drms.InstanceRecoveryPlan, id uint64) *drms.InstanceRecoveryPlan {
	for _, i := range instPlanList {
		if i.GetProtectionClusterInstance().GetId() == id {
			return i
		}
	}
	return nil
}

// setAvailabilityZoneRecoveryPlanUpdateFlagByPlanID 재해 복구 계획 availability_zone update_flag set 함수
func setAvailabilityZoneRecoveryPlanUpdateFlagByPlanID(db *gorm.DB, planID, availabilityZoneID uint64) error {
	var list []*model.PlanAvailabilityZone
	if err := db.Where(&model.PlanAvailabilityZone{
		RecoveryPlanID:                    planID,
		RecoveryClusterAvailabilityZoneID: &availabilityZoneID,
	}).Find(&list).Error; err != nil {
		return errors.UnusableDatabase(err)
	}

	flag := true
	reasonCode := constant.RecoveryPlanAvailabilityZoneTargetHypervisorInsufficientResource

	for _, azPlan := range list {
		azPlan.RecoveryClusterAvailabilityZoneUpdateFlag = &flag
		azPlan.RecoveryClusterAvailabilityZoneUpdateReasonCode = &reasonCode

		if err := db.Save(azPlan).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[setAvailabilityZoneRecoveryPlanUpdateFlagByPlanID] Sync the cluster availability zone(%d) plan(%d) by recovery availability zone(%d) changed.",
			azPlan.ProtectionClusterAvailabilityZoneID, planID, availabilityZoneID)
	}
	return nil
}

// unsetAvailabilityZoneRecoveryPlanUpdateFlagByPlanID 재해 복구 계획 availability_zone update_flag unset 함수
func unsetAvailabilityZoneRecoveryPlanUpdateFlagByPlanID(db *gorm.DB, planID, availabilityZoneID uint64) error {
	trueFlag := true
	falseFlag := false

	var list []*model.PlanAvailabilityZone
	if err := db.Where(&model.PlanAvailabilityZone{
		RecoveryPlanID:                            planID,
		RecoveryClusterAvailabilityZoneID:         &availabilityZoneID,
		RecoveryClusterAvailabilityZoneUpdateFlag: &trueFlag,
	}).Find(&list).Error; err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, azPlan := range list {
		azPlan.RecoveryClusterAvailabilityZoneUpdateFlag = &falseFlag
		azPlan.RecoveryClusterAvailabilityZoneUpdateReasonCode = nil
		azPlan.RecoveryClusterAvailabilityZoneUpdateReasonContents = nil

		if err := db.Save(azPlan).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[unsetAvailabilityZoneRecoveryPlanUpdateFlagByPlanID] Sync the cluster availability zone(%d) plan(%d) by recovery availability zone(%d) changed.",
			azPlan.ProtectionClusterAvailabilityZoneID, planID, availabilityZoneID)
	}
	return nil
}

// SetInstanceRecoveryPlanUpdateFlagAndNil 재해 복구 계획 instance update_flag set, hypervisor id nil 설정 함수
func SetInstanceRecoveryPlanUpdateFlagAndNil(recoveryClusterID, hypervisorID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var list []*model.PlanInstance
		if err = db.Table(model.PlanInstance{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_instance.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanInstance{RecoveryClusterHypervisorID: &hypervisorID}).
			Find(&list).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanInstanceTargetHypervisorDeleted

		for _, plan := range list {
			plan.RecoveryClusterHypervisorID = nil
			plan.RecoveryClusterAvailabilityZoneUpdateFlag = &flag
			plan.RecoveryClusterAvailabilityZoneUpdateReasonCode = &reasonCode
			plan.RecoveryClusterAvailabilityZoneUpdateReasonContents = nil

			if err = db.Save(plan).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetInstanceRecoveryPlanUpdateFlagAndNil] Sync the cluster instance(%d) plan(%d) by recovery hypervisor(%d) changed(reasonCode: %s).",
				plan.ProtectionClusterInstanceID, plan.RecoveryPlanID, hypervisorID, reasonCode)
		}

		return nil
	})
}

// SetInstanceRecoveryPlanUpdateFlag 재해 복구 계획 instance update_flag set 함수
func SetInstanceRecoveryPlanUpdateFlag(recoveryClusterID, hypervisorID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var list []*model.PlanInstance
		if err = db.Table(model.PlanInstance{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_instance.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanInstance{RecoveryClusterHypervisorID: &hypervisorID}).
			Find(&list).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanInstanceTargetHypervisorUnusableState

		for _, plan := range list {
			plan.RecoveryClusterAvailabilityZoneUpdateFlag = &flag
			plan.RecoveryClusterAvailabilityZoneUpdateReasonCode = &reasonCode
			plan.RecoveryClusterAvailabilityZoneUpdateReasonContents = nil

			if err = db.Save(plan).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetInstanceRecoveryPlanUpdateFlag] Sync the cluster instance(%d) plan(%d) by recovery hypervisor(%d) changed(reasonCode: %s).",
				plan.ProtectionClusterInstanceID, plan.RecoveryPlanID, hypervisorID, reasonCode)
		}

		return nil
	})
}

// UnsetInstanceRecoveryPlanUpdateFlag 재해 복구 계획 instance update_flag unset 함수
func UnsetInstanceRecoveryPlanUpdateFlag(recoveryClusterID, hypervisorID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var list []*model.PlanInstance
		trueFlag := true
		falseFlag := false
		reasonCode := constant.RecoveryPlanInstanceTargetHypervisorUnusableState

		if err = db.Table(model.PlanInstance{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_instance.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanInstance{
				RecoveryClusterHypervisorID:                     &hypervisorID,
				RecoveryClusterAvailabilityZoneUpdateFlag:       &trueFlag,
				RecoveryClusterAvailabilityZoneUpdateReasonCode: &reasonCode,
			}).Find(&list).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, plan := range list {
			plan.RecoveryClusterAvailabilityZoneUpdateFlag = &falseFlag
			plan.RecoveryClusterAvailabilityZoneUpdateReasonCode = nil
			plan.RecoveryClusterAvailabilityZoneUpdateReasonContents = nil

			if err = db.Save(plan).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[UnsetInstanceRecoveryPlanUpdateFlag] Sync the cluster instance(%d) plan(%d) by recovery hypervisor(%d) status(enable) changed.",
				plan.ProtectionClusterInstanceID, plan.RecoveryPlanID, hypervisorID, reasonCode)
		}

		return nil
	})
}

// ReassignHypervisor planInstance 에 Hypervisor 재설정
func ReassignHypervisor(ctx context.Context, recoveryClusterID uint64, availabilityZoneID uint64) (err error) {
	var groupPlans []struct {
		ProtectionGroup model.ProtectionGroup `gorm:"embedded"`
		RecoveryPlan    model.Plan            `gorm:"embedded"`
	}

	// TODO: 동일한 보호그룹에 대한 재해복구계획의 복구대상 클러스터는 서로 달라야 한다. <- 가 되면 DISTINCT 가 필요 없어진다.
	if err = database.Execute(func(db *gorm.DB) error {
		return db.
			Raw("SELECT DISTINCT cdm_disaster_recovery_protection_group.id, cdm_disaster_recovery_plan.id "+
				"FROM cdm_disaster_recovery_plan "+
				"JOIN cdm_disaster_recovery_protection_group on cdm_disaster_recovery_plan.protection_group_id = cdm_disaster_recovery_protection_group.id "+
				"WHERE cdm_disaster_recovery_plan.recovery_cluster_id = ? ",
				// TODO: cdm_disaster_recovery_protection_group 에 priority_seq 추가
				//"ORDER BY cdm_disaster_recovery_protection_group.priority_seq",
				recoveryClusterID).
			Scan(&groupPlans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	if len(groupPlans) == 0 {
		return nil
	}

	hypervisorList, err := cluster.GetClusterHypervisorList(ctx, &cms.ClusterHypervisorListRequest{
		ClusterId:                 recoveryClusterID,
		ClusterAvailabilityZoneId: availabilityZoneID,
	})
	if err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, groupPlan := range groupPlans {
		plan, err := GetForPlanSync(ctx, &drms.RecoveryPlanRequest{
			GroupId: groupPlan.ProtectionGroup.ID,
			PlanId:  groupPlan.RecoveryPlan.ID,
		})
		if err != nil {
			return errors.UnusableDatabase(err)
		}

		instList := getInstancePlanListByAvailabilityZoneID(plan, availabilityZoneID)

		if len(instList) == 0 {
			continue
		}

		// instance priority sorting
		instList = sortInstancePlanList(instList)

		// assign hypervisor
		instList, err = assignee.AssignHypervisor(instList, hypervisorList)
		if err != nil {
			logger.Warnf("[ReassignHypervisor] Could not assign hypervisor group(%d) plan(%d). cause(%+v)", groupPlan.ProtectionGroup.ID, plan.Id, err)
			return err
		}

		azPlanUpdateFlag := false
		reservedCPUMap := make(map[uint64]uint32)
		reservedMemMap := make(map[uint64]uint64)

		if err = database.GormTransaction(func(db *gorm.DB) error {
			// update instance plan hypervisor
			for _, i := range instList {
				if i.GetRecoveryClusterHypervisor().GetId() == 0 && i.GetRecoveryClusterAvailabilityZone().GetId() == 0 {
					azPlanUpdateFlag = true
				}

				err = updateInstanceRecoveryPlanHypervisorID(db, plan.Id, i, constant.RecoveryPlanInstanceTargetHypervisorInsufficientResource)
				if err != nil {
					logger.Warnf("[ReassignHypervisor] Could not update recovery plan(%d) instance(%d). cause(%+v)", plan.Id, i.ProtectionClusterInstance.Id, err)
					return err
				}

				reservedCPUMap[i.GetRecoveryClusterHypervisor().GetId()] += i.GetProtectionClusterInstance().GetSpec().GetVcpuTotalCnt()
				reservedMemMap[i.GetRecoveryClusterHypervisor().GetId()] += i.GetProtectionClusterInstance().GetSpec().GetMemTotalBytes()
			}

			if azPlanUpdateFlag {
				err = setAvailabilityZoneRecoveryPlanUpdateFlagByPlanID(db, plan.Id, availabilityZoneID)
			} else {
				err = unsetAvailabilityZoneRecoveryPlanUpdateFlagByPlanID(db, plan.Id, availabilityZoneID)
			}
			if err != nil {
				logger.Warnf("[ReassignHypervisor] Could not update recovery plan(%d) availability zone(%d). cause(%+v)", plan.Id, availabilityZoneID, err)
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		for _, h := range hypervisorList {
			h.VcpuUsedCnt += reservedCPUMap[h.Id]
			h.MemUsedBytes += reservedMemMap[h.Id]
		}
	}

	return nil
}

// reassignHypervisorByPlan plan 에 등록된 복구대상 가용구역별 hypervisor 재할당
func reassignHypervisorByPlan(ctx context.Context, plan *drms.RecoveryPlan) error {
	azMap := make(map[uint64]bool)
	for _, az := range plan.Detail.AvailabilityZones {
		if az.RecoveryClusterAvailabilityZone.GetId() != 0 {
			azMap[az.RecoveryClusterAvailabilityZone.Id] = true
		}
	}

	for _, i := range plan.Detail.Instances {
		if i.RecoveryClusterAvailabilityZone.GetId() != 0 {
			azMap[i.RecoveryClusterAvailabilityZone.Id] = true
		}
	}

	for azID := range azMap {
		if err := ReassignHypervisor(ctx, plan.RecoveryCluster.Id, azID); err != nil {
			return err
		}
	}

	return nil
}

// checkSwap sortGroup 에서 swap 여부 체크: ture -> swap
func checkSwap(instPlanList []*drms.InstanceRecoveryPlan, instPlanA *drms.InstanceRecoveryPlan, instPlanB *drms.InstanceRecoveryPlan) bool {
	for _, d := range instPlanA.GetDependencies() {
		// instPlanA 의 dependencies 중 instPlanB 의 id가 있다면 swap
		if d.GetId() == instPlanB.GetProtectionClusterInstance().GetId() {
			return true
		}
	}

	for _, d := range instPlanA.GetDependencies() {
		depPlan := getInstancePlanByID(instPlanList, d.GetId())
		return checkSwap(instPlanList, depPlan, instPlanB)
	}

	return false
}

// sortGroup group 내에서 정렬
func sortGroup(instPlanList []*drms.InstanceRecoveryPlan, g []*drms.InstanceRecoveryPlan) {
	l := len(g)
	for i := 1; i < l; i++ {
		for j := i; j > 0; j-- {
			if g[j].GetDependencies() == nil || checkSwap(instPlanList, g[j-1], g[j]) {
				g[j-1], g[j] = g[j], g[j-1]
			}
		}
	}
}

// sortInstancePlanList 정렬된 instanceInfoList 얻기 위한 함수
func sortInstancePlanList(instPlanList []*drms.InstanceRecoveryPlan) []*drms.InstanceRecoveryPlan {
	var (
		groupList [][]*drms.InstanceRecoveryPlan
		noGroup   []*drms.InstanceRecoveryPlan
	)

	// map 초기화
	checkedMap := make(map[uint64]bool)

	// grouping
	for _, i := range instPlanList {
		// autoFlag 가 false 인 경우는 noGroup
		if !i.GetAutoStartFlag() {
			noGroup = append(noGroup, i)
			continue
		}

		if group := makeGroupByDependencies(instPlanList, checkedMap, i.GetProtectionClusterInstance().GetId()); group != nil {
			groupList = append(groupList, group)
		}
	}

	// list 에 instancePlan 추가
	var list []*drms.InstanceRecoveryPlan
	for _, g := range groupList {
		// grouping 된 list 정렬
		sortGroup(instPlanList, g)
		list = append(list, g...)
	}

	// noGroup list 에 추가
	list = append(list, noGroup...)

	return list
}

// makeGroupByDependencies 해당 인스턴스(id)와 dependencies 로 연결여부에 따라 group 추가
func makeGroupByDependencies(instanceList []*drms.InstanceRecoveryPlan, checkedMap map[uint64]bool, id uint64) []*drms.InstanceRecoveryPlan {
	if checkedMap[id] {
		return nil
	}

	// 해당 instance 추가
	checkedMap[id] = true
	instance := getInstancePlanByID(instanceList, id)

	var group []*drms.InstanceRecoveryPlan
	// 해당 instance autoStartFlag = true 이면 grouping 에 추가
	group = append(group, instance)

	// 해당 instance 의 dependency 의 인스턴스 추가
	for _, d := range instance.GetDependencies() {
		g := makeGroupByDependencies(instanceList, checkedMap, d.GetId())
		group = append(group, g...)
	}

	// 해당 instance 를 dependency 로 갖는 인스턴스 추가
	for _, i := range instanceList {
		for _, d := range i.GetDependencies() {
			if d.GetId() == id {
				g := makeGroupByDependencies(instanceList, checkedMap, i.GetProtectionClusterInstance().GetId())
				group = append(group, g...)
			}
		}
	}

	return group
}

// SetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag 재해 복구 계획 external network update_flag set 함수
func SetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag(recoveryClusterID, networkID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planNetworks []*model.PlanExternalNetwork
		if err = db.Table(model.PlanExternalNetwork{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_external_network.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanExternalNetwork{RecoveryClusterExternalNetworkID: &networkID}).
			Find(&planNetworks).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanNetworkTargetExternalFlagChanged

		for _, planNetwork := range planNetworks {
			planNetwork.RecoveryClusterExternalNetworkUpdateFlag = &flag
			planNetwork.RecoveryClusterExternalNetworkUpdateReasonCode = &reasonCode

			if err = db.Save(planNetwork).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag] Sync the cluster network(%d) plan(%d) by recovery network(%d) changed.",
				planNetwork.ProtectionClusterExternalNetworkID, planNetwork.RecoveryPlanID, networkID)
		}

		return nil
	})
}

// UnsetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag 재해 복구 계획 external network update_flag unset 함수
func UnsetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag(recoveryClusterID, networkID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planNetworks []*model.PlanExternalNetwork
		trueFlag := true
		falseFlag := false
		if err = db.Table(model.PlanExternalNetwork{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_external_network.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanExternalNetwork{RecoveryClusterExternalNetworkID: &networkID, RecoveryClusterExternalNetworkUpdateFlag: &trueFlag}).
			Find(&planNetworks).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, planNetwork := range planNetworks {
			planNetwork.RecoveryClusterExternalNetworkUpdateFlag = &falseFlag
			planNetwork.RecoveryClusterExternalNetworkUpdateReasonCode = nil
			planNetwork.RecoveryClusterExternalNetworkUpdateReasonContents = nil

			if err = db.Save(planNetwork).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[UnsetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag] Sync the cluster network(%d) plan(%d) by recovery network(%d) changed.",
				planNetwork.ProtectionClusterExternalNetworkID, planNetwork.RecoveryPlanID, networkID)
		}

		return nil
	})
}

// SetExternalNetworkRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue 재해 복구 계획 external network 의 recovery network id 초기화, update_flag set 함수
func SetExternalNetworkRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue(recoveryClusterID, networkID uint64) error {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planNetworks []*model.PlanExternalNetwork
		if err := db.Table(model.PlanExternalNetwork{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_external_network.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanExternalNetwork{RecoveryClusterExternalNetworkID: &networkID}).
			Find(&planNetworks).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanNetworkTargetDeleted

		for _, planNetwork := range planNetworks {
			planNetwork.RecoveryClusterExternalNetworkID = nil
			planNetwork.RecoveryClusterExternalNetworkUpdateFlag = &flag
			planNetwork.RecoveryClusterExternalNetworkUpdateReasonCode = &reasonCode
			planNetwork.RecoveryClusterExternalNetworkUpdateReasonContents = nil

			if err := db.Save(planNetwork).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetExternalNetworkRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue] Sync the cluster network(%d) plan(%d) by recovery network(%d) changed.",
				planNetwork.ProtectionClusterExternalNetworkID, planNetwork.RecoveryPlanID, networkID)
		}

		return nil
	})
}

// SetRouterRecoveryPlanExternalNetworkUpdateFlag 재해 복구 계획 router external_network update_flag set 함수
func SetRouterRecoveryPlanExternalNetworkUpdateFlag(recoveryClusterID, networkID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planRouters []*model.PlanRouter
		if err = db.Table(model.PlanRouter{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_router.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanRouter{RecoveryClusterExternalNetworkID: &networkID}).
			Find(&planRouters).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanRouterTargetExternalFlagChanged

		for _, planRouter := range planRouters {
			planRouter.RecoveryClusterExternalNetworkUpdateFlag = &flag
			planRouter.RecoveryClusterExternalNetworkUpdateReasonCode = &reasonCode

			if err = db.Save(planRouter).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetRouterRecoveryPlanExternalNetworkUpdateFlag] Sync the cluster router(%d) plan(%d) by recovery network(%d) changed.", planRouter.ProtectionClusterRouterID, planRouter.RecoveryPlanID, networkID)
		}

		return nil
	})
}

// UnsetRouterRecoveryPlanExternalNetworkUpdateFlag 재해 복구 계획 router external_network update_flag unset 함수
func UnsetRouterRecoveryPlanExternalNetworkUpdateFlag(recoveryClusterID, networkID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planRouters []*model.PlanRouter
		trueFlag := true
		falseFlag := false
		if err = db.Table(model.PlanRouter{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_router.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanRouter{RecoveryClusterExternalNetworkID: &networkID, RecoveryClusterExternalNetworkUpdateFlag: &trueFlag}).
			Find(&planRouters).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, planRouter := range planRouters {
			planRouter.RecoveryClusterExternalNetworkUpdateFlag = &falseFlag
			planRouter.RecoveryClusterExternalNetworkUpdateReasonCode = nil
			planRouter.RecoveryClusterExternalNetworkUpdateReasonContents = nil

			if err = db.Save(planRouter).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[UnsetRouterRecoveryPlanExternalNetworkUpdateFlag] Sync the cluster router(%d) plan(%d) by recovery network(%d) changed.", planRouter.ProtectionClusterRouterID, planRouter.RecoveryPlanID, networkID)
		}

		return nil
	})
}

// SetRouterRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue 재해 복구 계획 router 의 recovery external network id 초기화, update_flag set 함수
func SetRouterRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue(recoveryClusterID, networkID uint64) error {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planRouters []*model.PlanRouter
		if err := db.Table(model.PlanRouter{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_router.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Where(&model.PlanRouter{RecoveryClusterExternalNetworkID: &networkID}).
			Find(&planRouters).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanRouterTargetDeleted

		for _, planRouter := range planRouters {
			planRouter.RecoveryClusterExternalNetworkID = nil
			planRouter.RecoveryClusterExternalNetworkUpdateFlag = &flag
			planRouter.RecoveryClusterExternalNetworkUpdateReasonCode = &reasonCode
			planRouter.RecoveryClusterExternalNetworkUpdateReasonContents = nil

			if err := db.Save(planRouter).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetRouterRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue] Sync the cluster router(%d) plan(%d) by recovery network(%d) changed.", planRouter.ProtectionClusterRouterID, planRouter.RecoveryPlanID, networkID)
		}

		return nil
	})
}

// DeleteExternalRoutingInterfacePlanRecord 재해 복구 계획 external_subnet_routing_interface record delete 함수
func DeleteExternalRoutingInterfacePlanRecord(recoveryClusterID, subnetID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		var planRouters []*model.PlanRouter
		if err = db.Table(model.PlanRouter{}.TableName()).
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan_router.recovery_plan_id = cdm_disaster_recovery_plan.id").
			Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).
			Find(&planRouters).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, r := range planRouters {
			if err = db.Where(&model.PlanExternalRoutingInterface{
				RecoveryPlanID:                  r.RecoveryPlanID,
				ProtectionClusterRouterID:       r.ProtectionClusterRouterID,
				RecoveryClusterExternalSubnetID: subnetID,
			}).Delete(&model.PlanExternalRoutingInterface{}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[DeleteExternalRoutingInterfacePlanRecord] Sync the cluster routing interface plan(%d) by recovery subnet(%d) changed.(protectionRouter:%d)", r.RecoveryPlanID, subnetID, r.ProtectionClusterRouterID)
		}

		return nil
	})
}

// SetFloatingIPRecoveryPlanUnavailableFlag 재해 복구 계획 floating ip unavailable_flag set 함수
func SetFloatingIPRecoveryPlanUnavailableFlag(ctx context.Context, recoveryClusterID uint64, floatingIPAddress string) (err error) {
	var plans []*model.Plan
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).Find(&plans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, plan := range plans {
		var planFloatingIPs []model.PlanInstanceFloatingIP
		if err = database.Execute(func(db *gorm.DB) error {
			if planFloatingIPs, err = plan.GetPlanInstanceFloatingIPs(db); err != nil {
				return errors.UnusableDatabase(err)
			}

			return nil
		}); err != nil {
			return err
		}

		var pg *model.ProtectionGroup
		if pg, err = getProtectionGroupByPlanID(plan.ID); err != nil {
			return errors.UnusableDatabase(err)
		}

		flag := true
		reasonCode := constant.RecoveryPlanFloatingIPTargetFloatingIPDuplicatedIPAddress

		for _, f := range planFloatingIPs {
			// 이미 UnavailableFlag 가 true 면 continue
			if f.UnavailableFlag != nil && *f.UnavailableFlag {
				continue
			}

			floatingIP, err := cluster.GetClusterFloatingIP(ctx, pg.ProtectionClusterID, f.ProtectionClusterInstanceFloatingIPID)
			if err != nil {
				return errors.UnusableDatabase(err)
			}

			if floatingIP.IpAddress != floatingIPAddress {
				continue
			}

			f.UnavailableFlag = &flag
			f.UnavailableReasonCode = &reasonCode

			if err = database.GormTransaction(func(db *gorm.DB) error {
				return db.Save(f).Error
			}); err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[SetFloatingIPRecoveryPlanUnavailableFlag] Sync the cluster floating IP(%d) plan(%d) by recovery floating IP(%s) changed.", f.ProtectionClusterInstanceFloatingIPID, f.RecoveryPlanID, floatingIPAddress)
		}
	}

	return nil
}

// UnsetFloatingIPRecoveryPlanUnavailableFlag 재해 복구 계획 floating ip unavailable_flag unset 함수
func UnsetFloatingIPRecoveryPlanUnavailableFlag(ctx context.Context, recoveryClusterID uint64, floatingIPAddress string) (err error) {
	var plans []*model.Plan
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.Plan{RecoveryClusterID: recoveryClusterID}).Find(&plans).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, plan := range plans {
		var planFloatingIPs []model.PlanInstanceFloatingIP
		if err = database.Execute(func(db *gorm.DB) error {
			if planFloatingIPs, err = plan.GetPlanInstanceFloatingIPs(db); err != nil {
				return errors.UnusableDatabase(err)
			}
			return nil
		}); err != nil {
			return err
		}

		var pg *model.ProtectionGroup
		if pg, err = getProtectionGroupByPlanID(plan.ID); err != nil {
			return errors.UnusableDatabase(err)
		}

		falseFlag := false

		for _, f := range planFloatingIPs {
			// UnavailableFlag 가 설정되지 않았거나 false 면 continue
			if f.UnavailableFlag == nil || !*f.UnavailableFlag {
				continue
			}

			floatingIP, err := cluster.GetClusterFloatingIP(ctx, pg.ProtectionClusterID, f.ProtectionClusterInstanceFloatingIPID)
			if err != nil {
				return errors.UnusableDatabase(err)
			}

			if floatingIP.IpAddress != floatingIPAddress {
				continue
			}

			if f.UnavailableReasonCode != nil &&
				*f.UnavailableReasonCode == constant.RecoveryPlanFloatingIPTargetRoutingInterfaceDuplicatedIPAddress {
				continue
			} else if f.UnavailableReasonCode != nil &&
				*f.UnavailableReasonCode == constant.RecoveryPlanFloatingIPTargetFloatingIPAndRoutingInterfaceDuplicatedIPAddress {
				reasonCode := constant.RecoveryPlanFloatingIPTargetRoutingInterfaceDuplicatedIPAddress
				f.UnavailableReasonCode = &reasonCode
				f.UnavailableReasonContents = nil
			} else {
				f.UnavailableFlag = &falseFlag
				f.UnavailableReasonCode = nil
				f.UnavailableReasonContents = nil
			}

			if err = database.GormTransaction(func(db *gorm.DB) error {
				return db.Save(f).Error
			}); err != nil {
				return errors.UnusableDatabase(err)
			}

			logger.Infof("[UnsetFloatingIPRecoveryPlanUnavailableFlag] Sync the cluster floating IP(%d) plan(%d) by recovery floating IP(%s) changed.", f.ProtectionClusterInstanceFloatingIPID, f.RecoveryPlanID, floatingIPAddress)
		}
	}

	return nil
}
