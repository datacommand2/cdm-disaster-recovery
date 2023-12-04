package protectiongroup

import (
	"context"
	"fmt"
	cmsModel "github.com/datacommand2/cdm-center/cluster-manager/database/model"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/event"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
)

type instanceFloatingIP struct {
	instanceID uint64
	floatingIP *cms.ClusterFloatingIP
}

type instanceVolume struct {
	tenant  *cms.ClusterTenant
	volume  *cms.ClusterVolume
	storage *cms.ClusterStorage
}

func reportEvent(ctx context.Context, eventCode, errorCode string, eventContents interface{}) {
	id, err := metadata.GetTenantID(ctx)
	if err != nil {
		logger.Warnf("[ProtectionGroupSync-reportEvent] Could not get the tenant ID. Cause: %v", err)
		return
	}

	err = event.ReportEvent(id, eventCode, errorCode, event.WithContents(eventContents))
	if err != nil {
		logger.Warnf("[ProtectionGroupSync-reportEvent] Could not report the event(%s:%s). Cause: %v", eventCode, errorCode, err)
	}
}

func getProtectionGroupInstanceMap(pg *drms.ProtectionGroup) map[uint64]*cms.ClusterInstance {
	result := make(map[uint64]*cms.ClusterInstance)
	for _, instance := range pg.Instances {
		result[instance.Id] = instance
	}

	return result
}

func getProtectionGroupExternalNetworkMap(pg *drms.ProtectionGroup) (map[uint64]*cms.ClusterNetwork, error) {
	result := make(map[uint64]*cms.ClusterNetwork)

	for _, instance := range pg.Instances {
		for _, r := range instance.Routers {
			if len(r.ExternalRoutingInterfaces) == 0 {
				return nil, internal.NotFoundExternalRoutingInterface(r.Id) // requeue 하면 안됨
			}
			result[r.ExternalRoutingInterfaces[0].Network.Id] = r.ExternalRoutingInterfaces[0].Network
		}
	}

	return result, nil
}

func getProtectionGroupRouterMap(pg *drms.ProtectionGroup) map[uint64]*cms.ClusterRouter {
	result := make(map[uint64]*cms.ClusterRouter)
	for _, instance := range pg.Instances {
		for _, r := range instance.Routers {
			result[r.Id] = r
		}
	}

	return result
}

func getProtectionGroupFloatingIPMap(pg *drms.ProtectionGroup) map[uint64]*instanceFloatingIP {
	result := make(map[uint64]*instanceFloatingIP)
	for _, i := range pg.Instances {
		for _, n := range i.Networks {
			if n.GetFloatingIp().GetId() == 0 {
				continue
			}

			result[n.FloatingIp.Id] = &instanceFloatingIP{
				instanceID: i.Id,
				floatingIP: n.FloatingIp,
			}
		}
	}

	return result
}

func isExistPlanExternalNetwork(id uint64, planExternalNetworks []*model.PlanExternalNetwork) bool {
	for _, planExternalNetwork := range planExternalNetworks {
		if id == planExternalNetwork.ProtectionClusterExternalNetworkID {
			return true
		}
	}

	return false
}

func updateExternalNetworkRecoveryPlanList(plan *model.Plan, extNetMap map[uint64]*cms.ClusterNetwork) error {
	var planExternalNetworks []*model.PlanExternalNetwork
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanExternalNetwork{}.TableName()).
			Where(&model.PlanExternalNetwork{RecoveryPlanID: plan.ID}).
			Find(&planExternalNetworks).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	// 네트워크 복구 계획 생성
	for id := range extNetMap {
		if isExistPlanExternalNetwork(id, planExternalNetworks) {
			continue
		}

		updateFlag := true
		reasonCode := constant.RecoveryPlanNetworkTargetRequired

		planExternalNetwork := model.PlanExternalNetwork{
			RecoveryPlanID:                                 plan.ID,
			ProtectionClusterExternalNetworkID:             id,
			RecoveryTypeCode:                               constant.ExternalNetworkRecoveryPlanTypeMapping,
			RecoveryClusterExternalNetworkUpdateFlag:       &updateFlag,
			RecoveryClusterExternalNetworkUpdateReasonCode: &reasonCode,
		}

		if err := database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(&planExternalNetwork).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateExternalNetworkRecoveryPlanList] Success to create the cluster network(%d) plan(%d).", id, plan.ID)
	}

	// 네트워크 복구 계획 삭제
	for _, extNetPlan := range planExternalNetworks {
		if extNetMap[extNetPlan.ProtectionClusterExternalNetworkID] != nil {
			continue
		}

		if err := database.GormTransaction(func(db *gorm.DB) error {
			return db.Where(&model.PlanExternalNetwork{
				RecoveryPlanID:                     plan.ID,
				ProtectionClusterExternalNetworkID: extNetPlan.ProtectionClusterExternalNetworkID,
			}).Delete(&model.PlanExternalNetwork{}).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateExternalNetworkRecoveryPlanList] Success to delete the cluster network(%d) plan(%d).", extNetPlan.ProtectionClusterExternalNetworkID, plan.ID)
	}

	logger.Infof("[updateExternalNetworkRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

func updateRouterRecoveryPlanList(plan *model.Plan, routerMap map[uint64]*cms.ClusterRouter) error {
	var planRouters []*model.PlanRouter
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanRouter{}.TableName()).
			Where(&model.PlanRouter{RecoveryPlanID: plan.ID}).
			Find(&planRouters).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	// 라우터 복구 계획 삭제
	for _, routerPlan := range planRouters {
		if routerMap[routerPlan.ProtectionClusterRouterID] != nil {
			continue
		}

		if err := database.GormTransaction(func(db *gorm.DB) error {
			if err := db.Where(&model.PlanExternalRoutingInterface{
				RecoveryPlanID:            plan.ID,
				ProtectionClusterRouterID: routerPlan.ProtectionClusterRouterID,
			}).Delete(&model.PlanExternalRoutingInterface{}).Error; err != nil {
				return err
			}

			logger.Infof("[updateRouterRecoveryPlanList] Success to delete the cluster routing interface plan(%d).(routerID:%d)", plan.ID, routerPlan.ProtectionClusterRouterID)

			if err := db.Where(&model.PlanRouter{
				RecoveryPlanID:            plan.ID,
				ProtectionClusterRouterID: routerPlan.ProtectionClusterRouterID,
			}).Delete(&model.PlanRouter{}).Error; err != nil {
				return err
			}

			return nil
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateRouterRecoveryPlanList] Success to delete the cluster router(%d) plan(%d).", routerPlan.ProtectionClusterRouterID, plan.ID)
	}

	logger.Infof("[updateRouterRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

func isExistPlanFloatingIP(id uint64, planInstanceFloatingIPS []*model.PlanInstanceFloatingIP) *model.PlanInstanceFloatingIP {
	for _, planInstanceFloatingIP := range planInstanceFloatingIPS {
		if id == planInstanceFloatingIP.ProtectionClusterInstanceFloatingIPID {
			return planInstanceFloatingIP
		}
	}

	return nil
}

func updateFloatingIPRecoveryPlanList(ctx context.Context, plan *model.Plan, floatingIPMap map[uint64]*instanceFloatingIP) error {
	var err error
	var planFloatingIPs []*model.PlanInstanceFloatingIP
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanInstanceFloatingIP{}.TableName()).
			Where(&model.PlanInstanceFloatingIP{RecoveryPlanID: plan.ID}).
			Find(&planFloatingIPs).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	// floating ip 복구 계획 생성 및 수정
	for id, f := range floatingIPMap {
		msg := "update"
		var planInstanceFloatingIP *model.PlanInstanceFloatingIP
		if planInstanceFloatingIP = isExistPlanFloatingIP(id, planFloatingIPs); planInstanceFloatingIP == nil {
			//continue
			// plan 이 이미 존재하는 경우 수정
			msg = "create"
			planInstanceFloatingIP = &model.PlanInstanceFloatingIP{
				RecoveryPlanID:                        plan.ID,
				ProtectionClusterInstanceID:           f.instanceID,
				ProtectionClusterInstanceFloatingIPID: f.floatingIP.Id,
			}
		}

		isExistFloating, err := cluster.CheckIsExistClusterFloatingIP(ctx, plan.RecoveryClusterID, f.floatingIP.IpAddress)
		if err != nil {
			return err
		}

		isExistRouting, err := cluster.CheckIsExistClusterRoutingInterface(ctx, plan.RecoveryClusterID, f.floatingIP.IpAddress)
		if err != nil {
			return err
		}

		flag := true
		switch {
		case isExistFloating && isExistRouting:
			reasonCode := constant.RecoveryPlanFloatingIPTargetFloatingIPAndRoutingInterfaceDuplicatedIPAddress
			planInstanceFloatingIP.UnavailableFlag = &flag
			planInstanceFloatingIP.UnavailableReasonCode = &reasonCode

		case isExistFloating:
			reasonCode := constant.RecoveryPlanFloatingIPTargetFloatingIPDuplicatedIPAddress
			planInstanceFloatingIP.UnavailableFlag = &flag
			planInstanceFloatingIP.UnavailableReasonCode = &reasonCode

		case isExistRouting:
			reasonCode := constant.RecoveryPlanFloatingIPTargetRoutingInterfaceDuplicatedIPAddress
			planInstanceFloatingIP.UnavailableFlag = &flag
			planInstanceFloatingIP.UnavailableReasonCode = &reasonCode

		default:
			planInstanceFloatingIP.UnavailableFlag = nil
			planInstanceFloatingIP.UnavailableReasonCode = nil
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(planInstanceFloatingIP).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateFloatingIPRecoveryPlanList] Success to %s the cluster floating IP(%d) plan(%d).", msg, f.floatingIP.Id, plan.ID)
	}

	// floating ip 복구 계획 삭제
	for _, floatingIPPlan := range planFloatingIPs {
		if floatingIPMap[floatingIPPlan.ProtectionClusterInstanceFloatingIPID] != nil {
			continue
		}

		if err := database.GormTransaction(func(db *gorm.DB) error {
			return db.Where(&model.PlanInstanceFloatingIP{
				RecoveryPlanID:                        plan.ID,
				ProtectionClusterInstanceFloatingIPID: floatingIPPlan.ProtectionClusterInstanceFloatingIPID,
			}).Delete(&model.PlanInstanceFloatingIP{}).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateFloatingIPRecoveryPlanList] Success to delete the cluster floating IP(%d) plan(%d).",
			floatingIPPlan.ProtectionClusterInstanceFloatingIPID, plan.ID)
	}

	logger.Infof("[updateFloatingIPRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

// UpdateNetworkRecoveryPlansByInstanceID 인스턴스 재해복구계획의 네트워크 관련 계획 업데이트 함수
func UpdateNetworkRecoveryPlansByInstanceID(ctx context.Context, protectionClusterID, instanceID uint64) error {
	// 해당 인스턴스가 포함된 pgID 조회
	pgID, err := getProtectionGroupIDByInstanceID(protectionClusterID, instanceID)
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	return updateNetworkRecoveryPlans(ctx, pgID)
}

// UpdateNetworkRecoveryPlansByProtectionClusterID protectionClusterID 로 조회되는 protection group 의 network 관련 복구계획에 대한 업데이트
func UpdateNetworkRecoveryPlansByProtectionClusterID(ctx context.Context, protectionClusterID uint64) error {
	var pgList []*model.ProtectionGroup
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.ProtectionGroup{ProtectionClusterID: protectionClusterID}).Find(&pgList).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, pg := range pgList {
		if err := updateNetworkRecoveryPlans(ctx, pg.ID); err != nil {
			return err
		}
	}

	return nil
}

// sync 쪽에서 protection group 을 get 할때 필요한 정보만 따로 모아 놓은 함수
func getProtectionGroupInstancesForSync(ctx context.Context, pgID uint64) (*drms.ProtectionGroup, error) {
	var err error
	var pg *model.ProtectionGroup
	tid, _ := metadata.GetTenantID(ctx)

	if pg, err = getProtectionGroup(tid, pgID); err != nil {
		logger.Errorf("[getProtectionGroupInstancesForSync] Could not get the protection group(%d). Cause: %+v", pgID, err)
		return nil, err
	}

	// 권한이 없는 보호그룹 조회시 UnauthorizedRequest error
	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, pg.OwnerGroupID) {
		logger.Errorf("[getProtectionGroupInstancesForSync] Errors occurred during checking the authentication of the user.")
		return nil, errors.UnauthorizedRequest(ctx)
	}

	var protectionGroup drms.ProtectionGroup
	if err = protectionGroup.SetFromModel(pg); err != nil {
		return nil, err
	}

	var list []model.ProtectionInstance
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Find(&list, &model.ProtectionInstance{ProtectionGroupID: pg.ID}).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, i := range list {
		rsp, err := cluster.GetClusterInstance(ctx, pg.ProtectionClusterID, i.ProtectionClusterInstanceID)
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			continue
		} else if err != nil {
			return nil, err
		}

		protectionGroup.Instances = append(protectionGroup.Instances, rsp)
	}

	return &protectionGroup, err
}

// updateNetworkRecoveryPlans 보호그룹의 재해복구계획들의 네트워크 관련 계획 업데이트 함수
// 외부 네트워크 매핑 계획, 라우터 복구 계획, FloatingIP 복구 계획에 대한 동기화를 진행한다.
func updateNetworkRecoveryPlans(ctx context.Context, pgID uint64) error {
	pg, err := getProtectionGroupInstancesForSync(ctx, pgID)
	if err != nil {
		return err
	}

	// 보호그룹에서 사용하는 모든 외부 네트워크, 라우터, FloatingIP 조회
	extNetMap, err := getProtectionGroupExternalNetworkMap(pg)
	if err != nil {
		return err
	}

	routerMap := getProtectionGroupRouterMap(pg)
	floatingIPMap := getProtectionGroupFloatingIPMap(pg)

	plans, err := getProtectionGroupRecoveryPlans(pgID)
	if err != nil {
		return err
	}

	for _, p := range plans {
		// plan 하나라도 동기화가 완료가 되면 기록이 남기 때문에 for 문 전체를 transaction 할 필요가 없다.
		if err = updateExternalNetworkRecoveryPlanList(&p, extNetMap); err != nil {
			return err
		}

		if err = updateRouterRecoveryPlanList(&p, routerMap); err != nil {
			return err
		}

		if err = updateFloatingIPRecoveryPlanList(ctx, &p, floatingIPMap); err != nil {
			return err
		}
	}

	return nil
}

func getProtectionGroupIDByInstanceID(clusterID, instanceID uint64) (uint64, error) {
	var pg model.ProtectionGroup

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Table(model.ProtectionGroup{}.TableName()).
			Select("cdm_disaster_recovery_protection_group.id").
			Joins("JOIN cdm_disaster_recovery_protection_instance ON cdm_disaster_recovery_protection_group.id = cdm_disaster_recovery_protection_instance.protection_group_id").
			Where(&model.ProtectionInstance{ProtectionClusterInstanceID: instanceID}).
			Where(&model.ProtectionGroup{ProtectionClusterID: clusterID}).
			First(&pg).Error
	}); err != nil {
		return 0, err
	}

	return pg.ID, nil
}

func getProtectionGroupRecoveryPlans(pgID uint64) ([]model.Plan, error) {
	var plans []model.Plan
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.Plan{ProtectionGroupID: pgID}).Find(&plans).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return plans, nil
}

// CreateVolumeRecoveryPlans 재해복구계획의 인스턴스 볼륨 attached 동기화 함수
func CreateVolumeRecoveryPlans(ctx context.Context, protectionClusterID, instanceID, volumeID uint64) error {
	// 해당 인스턴스가 포함된 pgID 조회
	pgID, err := getProtectionGroupIDByInstanceID(protectionClusterID, instanceID)
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	plans, err := getProtectionGroupRecoveryPlans(pgID)
	if err != nil {
		return err
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		for _, p := range plans {
			var planVolume model.PlanVolume
			if err = db.Where(&model.PlanVolume{RecoveryPlanID: p.ID, ProtectionClusterVolumeID: volumeID}).First(&planVolume).Error; err == nil {
				continue
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return errors.UnusableDatabase(err)
			}

			v, err := cluster.GetClusterVolume(ctx, protectionClusterID, volumeID)
			if err != nil {
				return err
			}

			if err = createStorageRecoveryPlan(db, p.ID, v.Storage); err != nil {
				return err
			}

			if err = createVolumeRecoveryPlan(db, p.ID, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// DeleteVolumeRecoveryPlans 재해복구계획의 인스턴스 볼륨 detached 동기화 함수
func DeleteVolumeRecoveryPlans(ctx context.Context, clusterID, storageID uint64, instanceVolume *cmsModel.ClusterInstanceVolume) error {
	// 해당 인스턴스가 포함된 pgID 조회
	pgID, err := getProtectionGroupIDByInstanceID(clusterID, instanceVolume.ClusterInstanceID)
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	// Sync 에서 보호그룹 정보중 필요한 것만 가져오기 위한 함수
	pg, err := getProtectionGroupInstancesForSync(ctx, pgID)
	if err != nil {
		return err
	}

	// 보호그룹에서 사용하는 모든 볼륨 조회
	storageMap, volumeMap := getProtectionGroupStorageAndVolumeMap(pg)

	plans, err := getProtectionGroupRecoveryPlans(pgID)
	if err != nil {
		return err
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		for _, p := range plans {
			var planVolume model.PlanVolume
			err = db.Where(&model.PlanVolume{RecoveryPlanID: p.ID, ProtectionClusterVolumeID: instanceVolume.ClusterVolumeID}).First(&planVolume).Error
			switch err {
			case nil:
				if volumeMap[planVolume.ProtectionClusterVolumeID] != nil {
					continue
				}

				if err = deleteVolumeRecoveryPlan(db, p.ID, &planVolume); err != nil {
					return err
				}

			case gorm.ErrRecordNotFound:
				continue

			default:
				return errors.UnusableDatabase(err)
			}

			if storageMap[storageID] != nil {
				continue
			}

			var planStorage model.PlanStorage
			err = db.Where(&model.PlanStorage{RecoveryPlanID: p.ID, ProtectionClusterStorageID: storageID}).First(&planStorage).Error
			switch err {
			case nil:
				if err = deleteStorageRecoveryPlan(db, p.ID, &planStorage); err != nil {
					return err
				}

			case gorm.ErrRecordNotFound:
				continue

			default:
				return errors.UnusableDatabase(err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func createStorageRecoveryPlan(db *gorm.DB, planID uint64, s *cms.ClusterStorage) error {
	var count uint64
	if err := db.Table(model.PlanStorage{}.TableName()).
		Where(&model.PlanStorage{RecoveryPlanID: planID, ProtectionClusterStorageID: s.Id}).
		Count(&count).Error; err != nil {
		return errors.UnusableDatabase(err)
	}

	if count > 0 {
		return nil
	}

	flag := true
	storageUpdateReasonCode := constant.RecoveryPlanStorageTargetRequired

	planStorage := &model.PlanStorage{
		RecoveryPlanID:                         planID,
		ProtectionClusterStorageID:             s.Id,
		RecoveryTypeCode:                       constant.StorageRecoveryPlanTypeMapping,
		RecoveryClusterStorageUpdateFlag:       &flag,
		RecoveryClusterStorageUpdateReasonCode: &storageUpdateReasonCode,
	}

	if s.TypeCode == storage.ClusterStorageTypeUnKnown {
		storageUnavailableReasonCode := constant.RecoveryPlanStorageSourceTypeUnknown
		planStorage.UnavailableFlag = &flag
		planStorage.UnavailableReasonCode = &storageUnavailableReasonCode
	}

	if err := db.Save(planStorage).Error; err != nil {
		return errors.UnusableDatabase(err)
	}

	logger.Infof("[createStorageRecoveryPlan] Success to create the cluster storage(%d) plan(%d).", s.Id, planID)

	return nil
}

// 볼륨 복구 계획 생성
func createVolumeRecoveryPlan(db *gorm.DB, planID uint64, v *cms.ClusterVolume) error {
	flag := true
	volumeUnavailableReasonCode := constant.RecoveryPlanVolumeSourceInitCopyRequired

	// 볼륨 복구 계획 추가 (초기 복제가 필요할 때, 지원하지 않는 볼륨 타입이거나 볼륨 타입 변경 필요 플래그가 설정되어 있는 경우 unavailable_flag = true)
	planVolume := &model.PlanVolume{
		RecoveryPlanID:            planID,
		ProtectionClusterVolumeID: v.Id,
		RecoveryTypeCode:          constant.VolumeRecoveryPlanTypeMirroring,
		UnavailableFlag:           &flag,
		UnavailableReasonCode:     &volumeUnavailableReasonCode,
	}

	var storagePlan model.PlanStorage
	err := db.Table(model.PlanStorage{}.TableName()).
		Where(&model.PlanStorage{RecoveryPlanID: planID, ProtectionClusterStorageID: v.Storage.Id}).
		First(&storagePlan).Error
	if err == nil {
		// 보호대상 volume 의 storage 에 대한 plan 이 존재할 때
		// 복구대상 storage 를 입력해주고 updateFlag 도 동일하게 입력해줌
		if storagePlan.RecoveryClusterStorageID != nil && *storagePlan.RecoveryClusterStorageID != 0 {
			planVolume.RecoveryClusterStorageID = storagePlan.RecoveryClusterStorageID
		}
		if storagePlan.RecoveryClusterStorageUpdateFlag != nil {
			planVolume.RecoveryClusterStorageUpdateFlag = storagePlan.RecoveryClusterStorageUpdateFlag
		}
		if storagePlan.RecoveryClusterStorageUpdateReasonCode != nil {
			planVolume.RecoveryClusterStorageUpdateReasonCode = storagePlan.RecoveryClusterStorageUpdateReasonCode
		}
	} else if err == gorm.ErrRecordNotFound {
		// 보호대상 volume 의 storage 에 대한 plan 이 존재하지 않을때
		storageUpdateReasonCode := constant.RecoveryPlanVolumeTargetRequired
		planVolume.RecoveryClusterStorageUpdateFlag = &flag
		planVolume.RecoveryClusterStorageUpdateReasonCode = &storageUpdateReasonCode
	} else {
		return errors.UnusableDatabase(err)
	}

	if err = db.Save(planVolume).Error; err != nil {
		return errors.UnusableDatabase(err)
	}
	logger.Infof("[createVolumeRecoveryPlan] Success to create the cluster volume(%d) plan(%d).", v.Id, planID)

	return nil
}

func getProtectionGroupStorageAndVolumeMap(pg *drms.ProtectionGroup) (map[uint64]*cms.ClusterStorage, map[uint64]*instanceVolume) {
	storageResult := make(map[uint64]*cms.ClusterStorage)
	volResult := make(map[uint64]*instanceVolume)

	for _, instance := range pg.Instances {
		for _, v := range instance.Volumes {
			storageResult[v.Storage.Id] = v.Storage
			v.Volume.Storage = v.Storage
			volResult[v.Volume.Id] = &instanceVolume{
				tenant:  instance.Tenant,
				volume:  v.Volume,
				storage: v.Storage,
			}
		}
	}

	return storageResult, volResult
}

func deleteStorageRecoveryPlan(db *gorm.DB, planID uint64, storagePlan *model.PlanStorage) error {
	if err := db.Where(&model.PlanStorage{
		RecoveryPlanID:             planID,
		ProtectionClusterStorageID: storagePlan.ProtectionClusterStorageID,
	}).Delete(&model.PlanStorage{}).Error; err != nil {
		return errors.UnusableDatabase(err)
	}
	logger.Infof("[deleteStorageRecoveryPlan] Success to delete the cluster storage(%d) plan(%d).", storagePlan.ProtectionClusterStorageID, planID)

	return nil
}

func deleteVolumeRecoveryPlan(db *gorm.DB, planID uint64, volumePlan *model.PlanVolume) error {
	if err := db.Where(&model.PlanVolume{
		RecoveryPlanID:            planID,
		ProtectionClusterVolumeID: volumePlan.ProtectionClusterVolumeID,
	}).Delete(&model.PlanVolume{}).Error; err != nil {
		return errors.UnusableDatabase(err)
	}
	logger.Infof("[deleteVolumeRecoveryPlan] Success to delete the cluster volume(%d) plan(%d).", volumePlan.ProtectionClusterVolumeID, planID)

	return nil
}

// ReassignHypervisorByProtectionInstance protection cluster instance 로 찾은 planInstance 에 Hypervisor 재설정
func ReassignHypervisorByProtectionInstance(ctx context.Context, protectionClusterID uint64, availabilityZoneID, instanceID uint64) error {
	// 해당 인스턴스가 포함된 pgID 조회
	pgID, err := getProtectionGroupIDByInstanceID(protectionClusterID, instanceID)
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	plans, err := getProtectionGroupRecoveryPlans(pgID)
	if err != nil {
		return err
	}

	for _, p := range plans {
		plan, err := recoveryPlan.GetForPlanSync(ctx, &drms.RecoveryPlanRequest{GroupId: pgID, PlanId: p.ID})
		if err != nil {
			return err
		}

		insPlan := internal.GetInstanceRecoveryPlan(plan, instanceID)

		var recoveryClusterAvailabilityZone *cms.ClusterAvailabilityZone
		if insPlan.RecoveryClusterAvailabilityZone.GetId() == 0 {
			azPlan := internal.GetAvailabilityZoneRecoveryPlan(plan, availabilityZoneID)

			recoveryClusterAvailabilityZone = azPlan.RecoveryClusterAvailabilityZone
		} else {
			recoveryClusterAvailabilityZone = insPlan.RecoveryClusterAvailabilityZone
		}

		if recoveryClusterAvailabilityZone == nil {
			logger.Warnf("[ReassignHypervisorByProtectionInstance] Could not reassign instance (%d) in recovery plan (%d). Cause: not found recovery cluster availability zone", instanceID, plan.Id)
			continue
		}

		if err = recoveryPlan.ReassignHypervisor(ctx, plan.RecoveryCluster.Id, recoveryClusterAvailabilityZone.Id); err != nil {
			return err
		}
	}

	return nil
}

// DeleteProtectionGroupInstance 보호 그룹 인스턴스가 제거 됐을 때 동기화 하는 함수
func DeleteProtectionGroupInstance(ctx context.Context, protectionClusterID uint64, instanceID uint64) error {
	// 해당 인스턴스가 포함된 pgID 조회
	pgID, err := getProtectionGroupIDByInstanceID(protectionClusterID, instanceID)
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	err = syncProtectionGroupDeletedInstance(ctx, &drms.ProtectionGroupRequest{GroupId: pgID}, instanceID)
	if err != nil {
		return err
	}

	return nil
}

func syncProtectionGroupDeletedInstance(ctx context.Context, req *drms.ProtectionGroupRequest, instanceID uint64) error {
	if req.GetGroupId() == 0 {
		return errors.RequiredParameter("group_id")
	}

	var (
		err error
		pg  *model.ProtectionGroup
	)

	tid, _ := metadata.GetTenantID(ctx)
	if pg, err = getProtectionGroup(tid, req.GroupId); err != nil {
		return err
	}

	// 권한이 없는 보호그룹 조회시 UnauthorizedRequest error
	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, pg.OwnerGroupID) {
		return errors.UnauthorizedRequest(ctx)
	}

	_, err = cluster.GetCluster(ctx, pg.ProtectionClusterID)
	if err != nil {
		return err
	}

	instances, err := getProtectionGroupInstancesWithoutDeletedInstance(ctx, pg, instanceID)
	if err != nil {
		return err
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = updateProtectionInstanceList(db, pg, instances); err != nil {
			logger.Errorf("[syncProtectionGroupDeletedInstance] Could not update the instance list of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	var rsp *drms.ProtectionGroup
	if rsp, err = getProtectionGroupInstancesForSync(ctx, pg.ID); err != nil {
		logger.Errorf("[syncProtectionGroupDeletedInstance] Errors occurred during getting the protection group(%d). Cause: %+v", pg.ID, err)
		return err
	}

	if err = updateRecoveryPlanList(ctx, rsp); err != nil {
		logger.Errorf("[syncProtectionGroupDeletedInstance] Could not update the recovery plan of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
		return err
	}

	return nil
}

func updateRecoveryPlanList(ctx context.Context, pg *drms.ProtectionGroup) (err error) {
	var plans []model.Plan
	plans, err = getProtectionGroupRecoveryPlans(pg.Id)
	if err != nil {
		return errors.UnusableDatabase(err)
	}

	for _, plan := range plans {
		// 리소스 복구 계획 동기화
		if err = updateRecoveryPlan(ctx, &plan, pg); err != nil {
			return err
		}
	}

	return nil
}

func getProtectionGroupInstancesWithoutDeletedInstance(ctx context.Context, pg *model.ProtectionGroup, instanceID uint64) ([]*cms.ClusterInstance, error) {
	var err error
	var list []model.ProtectionInstance
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Find(&list, &model.ProtectionInstance{ProtectionGroupID: pg.ID}).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	var instances []*cms.ClusterInstance
	for _, i := range list {
		if i.ProtectionClusterInstanceID == instanceID {
			continue
		}

		rsp, err := cluster.GetClusterInstance(ctx, pg.ProtectionClusterID, i.ProtectionClusterInstanceID)
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			continue
		} else if err != nil {
			return nil, err
		}

		instances = append(instances, rsp)
	}
	return instances, nil
}

func updateRecoveryPlan(ctx context.Context, plan *model.Plan, pg *drms.ProtectionGroup) error {
	// 보호그룹에서 사용하는 모든 외부 네트워크, 라우터, FloatingIP, 테넌트, 가용 구역, 볼륨, 인스턴스 동기화
	tenantMap := getProtectionGroupTenantMap(pg)
	azMap := getProtectionGroupAvailabilityZoneMap(pg)
	extNetMap, err := getProtectionGroupExternalNetworkMap(pg)
	if err != nil {
		return err
	}
	routerMap := getProtectionGroupRouterMap(pg)
	floatingIPMap := getProtectionGroupFloatingIPMap(pg)
	storageMap, volumeMap := getProtectionGroupStorageAndVolumeMap(pg)
	instanceMap := getProtectionGroupInstanceMap(pg)

	if err = updateTenantRecoveryPlanList(ctx, plan, tenantMap); err != nil {
		return err
	}

	if err = updateAvailabilityZoneRecoveryPlanList(plan, azMap); err != nil {
		return err
	}

	if err = updateExternalNetworkRecoveryPlanList(plan, extNetMap); err != nil {
		return err
	}

	if err = updateRouterRecoveryPlanList(plan, routerMap); err != nil {
		return err
	}

	if err = updateFloatingIPRecoveryPlanList(ctx, plan, floatingIPMap); err != nil {
		return err
	}

	if err = updateStorageRecoveryPlanList(plan, storageMap); err != nil {
		return err
	}

	if err = updateVolumeRecoveryPlanList(plan, volumeMap); err != nil {
		return err
	}

	if err = updateInstanceRecoveryPlanList(ctx, plan, instanceMap); err != nil {
		return err
	}

	return nil
}

func getProtectionGroupTenantMap(pg *drms.ProtectionGroup) map[uint64]*cms.ClusterTenant {
	result := make(map[uint64]*cms.ClusterTenant)

	for _, instance := range pg.Instances {
		// instance.Tenant.Cluster 는 Id 값만 가지고 있음
		// tenant plan 생성시 cluster name 필요
		instance.Tenant.Cluster = instance.Cluster
		result[instance.Tenant.Id] = instance.Tenant
	}

	return result
}

func isExistPlanTenant(id uint64, planTenants []*model.PlanTenant) bool {
	for _, tenant := range planTenants {
		if id == tenant.ProtectionClusterTenantID {
			return true
		}
	}

	return false
}

func updateTenantRecoveryPlanList(ctx context.Context, plan *model.Plan, tenantMap map[uint64]*cms.ClusterTenant) (err error) {
	var planTenants []*model.PlanTenant
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanTenant{}.TableName()).
			Where(&model.PlanTenant{RecoveryPlanID: plan.ID}).
			Find(&planTenants).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	// 테넌트 복구 계획 생성
	for id, tenant := range tenantMap {
		if isExistPlanTenant(id, planTenants) {
			continue
		}
		mirrorName := fmt.Sprintf("%s (recovered from %s cluster)", tenant.Name, tenant.Cluster.Name)
		planTenant := &model.PlanTenant{
			RecoveryPlanID:                  plan.ID,
			ProtectionClusterTenantID:       id,
			RecoveryTypeCode:                constant.TenantRecoveryPlanTypeMirroring,
			RecoveryClusterTenantMirrorName: &mirrorName,
		}

		isExist, err := cluster.CheckIsExistClusterTenant(ctx, plan.RecoveryClusterID, *planTenant.RecoveryClusterTenantMirrorName)
		if err != nil {
			return err
		}

		// 중복 존재
		if isExist {
			updateFlag := true
			reasonCode := constant.RecoveryPlanTenantMirrorNameDuplicated
			planTenant.RecoveryClusterTenantMirrorNameUpdateFlag = &updateFlag
			planTenant.RecoveryClusterTenantMirrorNameUpdateReasonCode = &reasonCode
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(&planTenant).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateTenantRecoveryPlanList] Success to create the cluster tenant(%d) plan(%d).", id, plan.ID)
	}

	// 테넌트 복구 계획 삭제
	for _, planTenant := range planTenants {
		if tenantMap[planTenant.ProtectionClusterTenantID] != nil {
			continue
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Where(&model.PlanTenant{
				RecoveryPlanID:            plan.ID,
				ProtectionClusterTenantID: planTenant.ProtectionClusterTenantID,
			}).Delete(&model.PlanTenant{}).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateTenantRecoveryPlanList] Success to delete the cluster tenant(%d) plan(%d).", planTenant.ProtectionClusterTenantID, plan.ID)
	}

	logger.Infof("[updateTenantRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

func getProtectionGroupAvailabilityZoneMap(pg *drms.ProtectionGroup) map[uint64]*cms.ClusterAvailabilityZone {
	result := make(map[uint64]*cms.ClusterAvailabilityZone)

	for _, instance := range pg.Instances {
		result[instance.AvailabilityZone.Id] = instance.AvailabilityZone
	}

	return result
}

func isExistPlanAvailabilityZone(id uint64, planAvailabilityZones []*model.PlanAvailabilityZone) bool {
	for _, az := range planAvailabilityZones {
		if id == az.ProtectionClusterAvailabilityZoneID {
			return true
		}
	}

	return false
}

func updateAvailabilityZoneRecoveryPlanList(plan *model.Plan, azMap map[uint64]*cms.ClusterAvailabilityZone) (err error) {
	var planAvailabilityZones []*model.PlanAvailabilityZone
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanAvailabilityZone{}.TableName()).
			Where(&model.PlanAvailabilityZone{RecoveryPlanID: plan.ID}).
			Find(&planAvailabilityZones).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	// 가용구역 복구 계획 생성
	for id := range azMap {
		if isExistPlanAvailabilityZone(id, planAvailabilityZones) {
			continue
		}

		updateFlag := true
		reasonCode := constant.RecoveryPlanAvailabilityZoneTargetRequired

		planAvailabilityZone := model.PlanAvailabilityZone{
			RecoveryPlanID:                                  plan.ID,
			ProtectionClusterAvailabilityZoneID:             id,
			RecoveryTypeCode:                                constant.AvailabilityZoneRecoveryPlanTypeMapping,
			RecoveryClusterAvailabilityZoneUpdateFlag:       &updateFlag,
			RecoveryClusterAvailabilityZoneUpdateReasonCode: &reasonCode,
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(&planAvailabilityZone).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateAvailabilityZoneRecoveryPlanList] Success to create the cluster availability zone(%d) plan(%d).", id, plan.ID)
	}

	// 가용구역 복구 계획 삭제
	for _, azPlan := range planAvailabilityZones {
		if azMap[azPlan.ProtectionClusterAvailabilityZoneID] != nil {
			continue
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Where(&model.PlanAvailabilityZone{
				RecoveryPlanID:                      plan.ID,
				ProtectionClusterAvailabilityZoneID: azPlan.ProtectionClusterAvailabilityZoneID,
			}).Delete(&model.PlanAvailabilityZone{}).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateAvailabilityZoneRecoveryPlanList] Success to delete the cluster availability zone(%d) plan(%d).", azPlan.ProtectionClusterAvailabilityZoneID, plan.ID)
	}

	logger.Infof("[updateAvailabilityZoneRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

func isExistPlanStorage(id uint64, planStorages []*model.PlanStorage) bool {
	for _, planStorage := range planStorages {
		if id == planStorage.ProtectionClusterStorageID {
			return true
		}
	}

	return false
}

func isExistPlanVolume(id uint64, planVolumes []*model.PlanVolume) bool {
	for _, planVolume := range planVolumes {
		if id == planVolume.ProtectionClusterVolumeID {
			return true
		}
	}

	return false
}

func updateStorageRecoveryPlanList(plan *model.Plan, storageMap map[uint64]*cms.ClusterStorage) (err error) {
	var planStorages []*model.PlanStorage
	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Table(model.PlanStorage{}.TableName()).
			Where(&model.PlanStorage{RecoveryPlanID: plan.ID}).
			Find(&planStorages).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for id, s := range storageMap {
			if isExistPlanStorage(id, planStorages) {
				continue
			}

			if err = createStorageRecoveryPlan(db, plan.ID, s); err != nil {
				return err
			}
		}

		for _, storagePlan := range planStorages {
			if storageMap[storagePlan.ProtectionClusterStorageID] != nil {
				continue
			}

			if err = deleteStorageRecoveryPlan(db, plan.ID, storagePlan); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[updateStorageRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

func updateVolumeRecoveryPlanList(plan *model.Plan, volumeMap map[uint64]*instanceVolume) (err error) {
	var planVolumes []*model.PlanVolume
	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Table(model.PlanVolume{}.TableName()).
			Where(&model.PlanVolume{RecoveryPlanID: plan.ID}).
			Find(&planVolumes).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for id, v := range volumeMap {
			if isExistPlanVolume(id, planVolumes) {
				continue
			}

			if err = createVolumeRecoveryPlan(db, plan.ID, v.volume); err != nil {
				return err
			}
		}

		for _, volumePlan := range planVolumes {
			if volumeMap[volumePlan.ProtectionClusterVolumeID] != nil {
				continue
			}

			if err = deleteVolumeRecoveryPlan(db, plan.ID, volumePlan); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[updateVolumeRecoveryPlanList] Done: plan(%d).", plan.ID)

	return nil
}

func isExistPlanInstance(id uint64, planInstances []*model.PlanInstance) bool {
	for _, instance := range planInstances {
		if id == instance.ProtectionClusterInstanceID {
			return true
		}
	}

	return false
}

func updateInstanceRecoveryPlanList(ctx context.Context, plan *model.Plan, instanceMap map[uint64]*cms.ClusterInstance) (err error) {
	var planInstances []*model.PlanInstance
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Table(model.PlanInstance{}.TableName()).
			Where(&model.PlanInstance{RecoveryPlanID: plan.ID}).
			Find(&planInstances).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	updateFlag := true
	reasonCode := constant.RecoveryPlanInstanceTargetHypervisorRequired

	// 인스턴스 복구 계획 생성
	for id := range instanceMap {
		if isExistPlanInstance(id, planInstances) {
			continue
		}

		planInstance := model.PlanInstance{
			RecoveryPlanID:                                  plan.ID,
			ProtectionClusterInstanceID:                     id,
			RecoveryTypeCode:                                constant.InstanceRecoveryPlanTypeMirroring,
			RecoveryClusterAvailabilityZoneUpdateFlag:       &updateFlag,
			RecoveryClusterAvailabilityZoneUpdateReasonCode: &reasonCode,
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			return db.Save(&planInstance).Error
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		logger.Infof("[updateInstanceRecoveryPlanList] Success to create the cluster instance(%d) plan(%d).", id, plan.ID)
	}

	// 인스턴스 복구 계획 삭제
	for _, instancePlan := range planInstances {
		if instanceMap[instancePlan.ProtectionClusterInstanceID] != nil {
			continue
		}

		if err = database.GormTransaction(func(db *gorm.DB) error {
			// instance dependency 삭제
			if err := db.Where(&model.PlanInstanceDependency{
				RecoveryPlanID:              plan.ID,
				ProtectionClusterInstanceID: instancePlan.ProtectionClusterInstanceID,
			}).Delete(&model.PlanInstanceDependency{}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			if err := db.Where(&model.PlanInstanceDependency{
				RecoveryPlanID:                    plan.ID,
				DependProtectionClusterInstanceID: instancePlan.ProtectionClusterInstanceID,
			}).Delete(&model.PlanInstanceDependency{}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			// instance plan 삭제
			if err := db.Where(&model.PlanInstance{
				RecoveryPlanID:              plan.ID,
				ProtectionClusterInstanceID: instancePlan.ProtectionClusterInstanceID,
			}).Delete(&model.PlanInstance{}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			return nil
		}); err != nil {
			return err
		}

		logger.Infof("[updateInstanceRecoveryPlanList] Success to delete the cluster instance(%d) plan(%d).", instancePlan.ProtectionClusterInstanceID, plan.ID)
	}

	// TODO: job, plan, pg 자동삭제시 함수 추가후 아래 내용 삭제
	// job 이 존재하지 않는 경우
	if err = database.GormTransaction(func(db *gorm.DB) error {
		// job 이 존재하는 경우: return
		if err = isRecoveryJobExists(db, plan.ProtectionGroupID); errors.Equal(err, internal.ErrRecoveryJobExisted) {
			return nil
		} else if err != nil {
			return errors.UnusableDatabase(err)
		}

		err = db.Table(model.PlanInstance{}.TableName()).
			Where(&model.PlanInstance{RecoveryPlanID: plan.ID}).
			First(&model.PlanInstance{}).Error
		if err != nil && err == gorm.ErrRecordNotFound {
			// instance plan 이 존재하지 않는 경우 해당 plan 도 삭제
			if err = db.Where(&model.Plan{ID: plan.ID}).Delete(&model.Plan{}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			reportEvent(ctx, "cdm-dr.manager.update_instance_recovery_plan_list_for_sync.success-plan_deleted_by_protection_instance_deleted", "", err)
			logger.Infof("[updateInstanceRecoveryPlanList] Success to delete the recovery plan(%d).", plan.ID)

		} else if err != nil {
			return errors.UnusableDatabase(err)
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[updateInstanceRecoveryPlanList] Done: plan(%d).", plan.ID)
	return nil
}

// SyncRecoveryPlanListByProtectionGroupID protection group id로 plan list 조회하여 동기화 진행
func SyncRecoveryPlanListByProtectionGroupID(ctx context.Context, pgID uint64) (err error) {
	var rsp *drms.ProtectionGroup
	if rsp, err = getProtectionGroupInstancesForSync(ctx, pgID); err != nil {
		logger.Errorf("[SyncRecoveryPlanListByProtectionGroupID] Errors occurred during getting the protection group(%d). Cause: %+v", pgID, err)
		return err
	}

	if err = updateRecoveryPlanList(ctx, rsp); err != nil {
		logger.Errorf("[SyncRecoveryPlanListByProtectionGroupID] Could not update the recovery plan of the protection group(%d). Cause: %+v", pgID, err)
		return err
	}

	return nil
}

// SyncRecoveryPlansByRecoveryPlanID recovery plan id로 모든 plan 조회하여 동기화 진행
func SyncRecoveryPlansByRecoveryPlanID(ctx context.Context, pgID, planID uint64) (err error) {
	var pg *drms.ProtectionGroup
	if pg, err = getProtectionGroupInstancesForSync(ctx, pgID); err != nil {
		logger.Errorf("[SyncRecoveryPlansByRecoveryPlanID] Errors occurred during getting the protection group(%d). Cause: %+v", pgID, err)
		return err
	}

	var plan *model.Plan
	if err = database.Execute(func(db *gorm.DB) error {
		if plan, err = getRecoveryPlan(db, pgID, planID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err = updateRecoveryPlan(ctx, plan, pg); err != nil {
		return err
	}

	return nil
}
