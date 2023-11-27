package protectiongroup

import (
	"context"
	"fmt"
	centerConstant "github.com/datacommand2/cdm-center/cluster-manager/constant"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jinzhu/copier"
	"github.com/jinzhu/gorm"
	"regexp"
)

func getClusterProtectionInstanceList(clusterIDs ...uint64) ([]*model.ProtectionInstance, error) {
	var protectionInstances []*model.ProtectionInstance
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Model(&model.ProtectionInstance{}).
			Joins("join cdm_disaster_recovery_protection_group on cdm_disaster_recovery_protection_group.id = cdm_disaster_recovery_protection_instance.protection_group_id").
			Where("cdm_disaster_recovery_protection_group.protection_cluster_id IN (?)", clusterIDs).
			Find(&protectionInstances).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return protectionInstances, nil
}

func getUnprotectedInstanceList(allInstanceList []*cms.ClusterInstance, protectionInstanceList []*model.ProtectionInstance) []*cms.ClusterInstance {
	var unprotectedInstances []*cms.ClusterInstance

	for _, i := range allInstanceList {
		exists := false
		for _, p := range protectionInstanceList {
			exists = exists || (i.Id == p.ProtectionClusterInstanceID)
		}

		if exists == false {
			unprotectedInstances = append(unprotectedInstances, i)
		}
	}
	return unprotectedInstances
}

func paginateUnprotectionInstanceList(is []*cms.ClusterInstance, offset, limit uint64) ([]*cms.ClusterInstance, *drms.Pagination) {
	total := uint64(len(is))
	if offset > total {
		offset = total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return is[offset:end], &drms.Pagination{
		Page:       &wrappers.UInt64Value{Value: offset/limit + 1},
		TotalPage:  &wrappers.UInt64Value{Value: (total + limit - 1) / limit},
		TotalItems: &wrappers.UInt64Value{Value: total},
	}
}

// GetUnprotectedInstanceList 비보호 인스턴스 목록 조회 함수
func GetUnprotectedInstanceList(ctx context.Context, req *drms.UnprotectedInstanceListRequest) ([]*cms.ClusterInstance, *drms.Pagination, error) {
	var err error

	if req.GetClusterId() == 0 {
		err = errors.RequiredParameter("cluster_id")
		logger.Errorf("[GetUnprotectedInstanceList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	if err = cluster.IsAccessibleCluster(ctx, req.ClusterId); err != nil {
		logger.Errorf("[GetUnprotectedInstanceList] Errors occurred during checking accessible status of the cluster(%d). Cause: %+v", req.ClusterId, err)
		return nil, nil, err
	}

	var instanceList []*cms.ClusterInstance
	if instanceList, err = cluster.GetClusterInstanceList(ctx, req); err != nil {
		if errors.GetIPCStatusCode(err) != errors.IPCStatusNoContent {
			logger.Errorf("[GetUnprotectedInstanceList] Could not get the instance list. Cause: %+v", err)
		}

		switch errors.GetIPCStatusCode(err) {
		case errors.IPCStatusNoContent:
			return nil, nil, internal.NotFoundClusterInstance(req.GetClusterId())
		case errors.IPCStatusBadRequest:
			return nil, nil, internal.IPCFailedBadRequest(err)
		case errors.IPCStatusNotFound:
			return nil, nil, internal.NotFoundCluster(req.ClusterId)
		case errors.IPCStatusUnauthorized:
			return nil, nil, errors.UnauthorizedRequest(ctx)
		}
		return nil, nil, err
	}

	var protectionInstanceList []*model.ProtectionInstance
	if protectionInstanceList, err = getClusterProtectionInstanceList(req.ClusterId); err != nil {
		logger.Errorf("[GetUnprotectedInstanceList] Could not get the cluster(%d) protection instance list. Cause: %+v", req.ClusterId, err)
		return nil, nil, err
	}

	unprotectedInstances := getUnprotectedInstanceList(instanceList, protectionInstanceList)

	var pagination *drms.Pagination
	if req.GetOffset() != nil && (req.GetLimit() != nil && req.GetLimit().GetValue() != 0) {
		unprotectedInstances, pagination = paginateUnprotectionInstanceList(unprotectedInstances, req.GetOffset().GetValue(), req.GetLimit().GetValue())
	}

	return unprotectedInstances, pagination, nil
}

func getProtectionGroupList(filters ...protectionGroupFilter) ([]*model.ProtectionGroup, error) {
	var err error
	var m []*model.ProtectionGroup

	if err = database.Execute(func(db *gorm.DB) error {
		cond := db
		for _, f := range filters {
			if cond, err = f.Apply(cond); err != nil {
				return err
			}
		}
		return cond.Model(&m).Find(&m).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return m, nil
}

func getProtectionGroupsPagination(filters ...protectionGroupFilter) (*drms.Pagination, error) {
	var err error
	var offset, limit, total uint64

	if err = database.Execute(func(db *gorm.DB) error {
		conditions := db
		for _, f := range filters {
			if _, ok := f.(*paginationFilter); ok {
				offset = f.(*paginationFilter).Offset
				limit = f.(*paginationFilter).Limit
				continue
			}

			if conditions, err = f.Apply(conditions); err != nil {
				return err
			}
		}

		return conditions.Model(&model.ProtectionGroup{}).Count(&total).Error
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

func getProtectionGroupStateCode(ctx context.Context, protectionGroupID uint64) (string, error) {
	plans, _, err := recoveryPlan.GetList(ctx, &drms.RecoveryPlanListRequest{
		GroupId: protectionGroupID,
	})
	if err != nil {
		return "", err
	}

	var (
		isNormal   = true
		isCritical = true
	)

	for _, plan := range plans {
		if plan.StateCode == constant.RecoveryPlanStateEmergency ||
			plan.ProtectionCluster.StateCode == centerConstant.ClusterStateInactive {
			return constant.RecoveryPlanStateEmergency, nil
		}

		isNormal = isNormal && plan.StateCode == constant.RecoveryPlanStateNormal

		isCritical = isCritical && (plan.StateCode == constant.RecoveryPlanStateCritical || plan.RecoveryCluster.StateCode == centerConstant.ClusterStateInactive)
	}

	if isNormal {
		// 모든 재해복구계획의 상태가 Normal 이면 Normal
		return constant.ProtectionGroupStateNormal, nil
	} else if isCritical {
		// 모든 재해복구계획의 상태가 Critical 이면 Critical
		return constant.ProtectionGroupStateCritical, nil
	} else {
		// 일부 재해복구계획의 상태가 Critical 이나 Warning 이라면 Warning
		return constant.ProtectionGroupStateWarning, nil
	}
}

func setProtectionGroupStateCode(ctx context.Context, pg *drms.ProtectionGroup) error {
	stateCode, err := getProtectionGroupStateCode(ctx, pg.Id)
	if err != nil {
		return err
	}

	pg.StateCode = stateCode
	return nil
}

// GetProtectionGroupSummary 보호 그룹 요약 조회
func GetProtectionGroupSummary(ctx context.Context) (*drms.ProtectionGroupSummary, error) {
	groups, _, err := GetList(ctx, &drms.ProtectionGroupListRequest{})
	if err != nil {
		return nil, err
	}

	var ret drms.ProtectionGroupSummary
	for _, group := range groups {
		switch group.StateCode {
		case constant.ProtectionGroupStateNormal:
			ret.NormalGroup++

		case constant.ProtectionGroupStateWarning:
			ret.WarningGroup++

		case constant.ProtectionGroupStateCritical:
			ret.CriticalGroup++

		case constant.ProtectionGroupStateEmergency:
			ret.EmergencyGroup++
		}
	}
	ret.TotalGroup = ret.NormalGroup + ret.WarningGroup + ret.CriticalGroup + ret.EmergencyGroup

	return &ret, nil
}

// GetInstanceSummary 인스턴스 요약 정보 조회
func GetInstanceSummary(ctx context.Context) (*drms.InstanceSummary, error) {
	// get total instance count from cluster-manager
	rsp, err := cluster.GetClusterInstanceNumber(ctx, 0)
	if err != nil {
		return nil, err
	}
	total := rsp.GetInstanceNumber().GetTotal()

	// get protected instance count from database
	clusterMap, err := cluster.GetClusterMap(ctx)
	if err != nil {
		return nil, err
	}
	var cIDs []uint64
	for k := range clusterMap {
		cIDs = append(cIDs, k)
	}

	list, err := getClusterProtectionInstanceList(cIDs...)
	if err != nil {
		return nil, err
	}

	return &drms.InstanceSummary{
		TotalInstance:      total,
		ProtectionInstance: uint64(len(list)),
	}, nil
}

// GetVolumeSummary 볼륨 요약 정보 조회
func GetVolumeSummary(ctx context.Context) (*drms.VolumeSummary, error) {
	clusterMap, err := cluster.GetClusterMap(ctx)
	if err != nil {
		return nil, err
	}

	var sum uint64
	for clusterID := range clusterMap {
		instances, err := getClusterProtectionInstanceList(clusterID)
		if err != nil {
			return nil, err
		}

		for _, instance := range instances {
			volumes, err := cluster.GetClusterVolumeList(ctx, &cms.ClusterVolumeListRequest{
				ClusterId:         clusterID,
				ClusterInstanceId: instance.ProtectionClusterInstanceID,
				Offset: &wrappers.UInt64Value{
					Value: 0,
				},
				Limit: &wrappers.UInt64Value{
					Value: 1,
				},
			})
			if err != nil {
				return nil, err
			}

			sum += volumes.GetPagination().GetTotalItems().GetValue()
		}
	}

	return &drms.VolumeSummary{
		ProtectionVolume: sum,
	}, nil
}

func checkValidProtectionGroupList(name string) error {
	if len(name) > 255 {
		return errors.LengthOverflowParameterValue("protectionGroup.name", name, 255)
	}

	return nil
}

// GetList 보호 그룹 목록 조회
func GetList(ctx context.Context, req *drms.ProtectionGroupListRequest) ([]*drms.ProtectionGroup, *drms.Pagination, error) {
	var clusterMap map[uint64]*cms.Cluster
	var err error

	if err = checkValidProtectionGroupList(req.Name); err != nil {
		logger.Errorf("[ProtectionGroup-GetList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	if clusterMap, err = cluster.GetClusterMap(ctx,
		cluster.TypeCode(req.ProtectionClusterTypeCode),
		cluster.OwnerGroupID(req.OwnerGroupId)); err != nil {
		logger.Errorf("[ProtectionGroup-GetList] Could not get protection group list. Cause: %+v", err)
		switch errors.GetIPCStatusCode(err) {
		case errors.IPCStatusBadRequest:
			return nil, nil, internal.IPCFailedBadRequest(err)
		}
		return nil, nil, err
	}

	if len(clusterMap) == 0 {
		err = internal.IPCNoContent()
		logger.Errorf("[ProtectionGroup-GetList] Not found cluster. Cause: %+v", err)
		return nil, nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)
	filters := makeProtectionGroupFilter(req, tid, clusterMap)

	var protectionGroups []*drms.ProtectionGroup
	var pgs []*model.ProtectionGroup
	if pgs, err = getProtectionGroupList(filters...); err != nil {
		logger.Errorf("[ProtectionGroup-GetList] Could not get the protection group list. Cause: %+v", err)
		return nil, nil, err
	}

	for _, pg := range pgs {
		var protectionGroup drms.ProtectionGroup
		if err = protectionGroup.SetFromModel(pg); err != nil {
			return nil, nil, err
		}

		protectionGroup.ProtectionCluster = clusterMap[pg.ProtectionClusterID]
		protectionGroup.OwnerGroup = protectionGroup.ProtectionCluster.OwnerGroup

		if err = setProtectionGroupStateCode(ctx, &protectionGroup); err != nil {
			logger.Errorf("[ProtectionGroup-GetList] Could not set the protection group(%d) state code. Cause: %+v", pg.ID, err)
			return nil, nil, err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return isRecoveryJobExists(db, pg.ID)
		}); err == nil {
			protectionGroup.Updatable = true
		} else if !errors.Equal(err, internal.ErrRecoveryJobExisted) {
			logger.Errorf("[ProtectionGroup-GetList] Errors occurred during checking existence of the recovery job(pgID:%d). Cause: %+v", pg.ID, err)
			return nil, nil, err
		}

		protectionGroups = append(protectionGroups, &protectionGroup)
	}

	pagination, err := getProtectionGroupsPagination(filters...)
	if err != nil {
		logger.Errorf("[ProtectionGroup-GetList] Could not get the protection groups pagination. Cause: %+v", err)
		return nil, nil, err
	}

	return protectionGroups, pagination, nil
}

func getProtectionGroup(tid, id uint64) (*model.ProtectionGroup, error) {
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

func getRecoveryPlan(db *gorm.DB, pgID, pID uint64) (*model.Plan, error) {
	var p model.Plan
	err := db.First(&p, &model.Plan{ID: pID, ProtectionGroupID: pgID}).Error
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil, internal.NotFoundRecoveryPlan(pgID, pID)

	case err != nil:
		return nil, errors.UnusableDatabase(err)
	}
	return &p, nil
}

func getProtectionGroupInstances(ctx context.Context, pg *model.ProtectionGroup) ([]*cms.ClusterInstance, error) {
	var list []model.ProtectionInstance
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Find(&list, &model.ProtectionInstance{ProtectionGroupID: pg.ID}).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	var instances []*cms.ClusterInstance
	for _, i := range list {
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

// GetSimpleInfo 보호 그룹의 간단한 정보를 조회 하는 함수
func GetSimpleInfo(ctx context.Context, req *drms.ProtectionGroupRequest) (*drms.ProtectionGroup, error) {
	var pg *model.ProtectionGroup
	var err error
	tid, _ := metadata.GetTenantID(ctx)
	if pg, err = getProtectionGroup(tid, req.GroupId); err != nil {
		return nil, err
	}

	// 권한이 없는 보호그룹 조회시 UnauthorizedRequest error
	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, pg.OwnerGroupID) {
		return nil, errors.UnauthorizedRequest(ctx)
	}

	c, err := cluster.GetCluster(ctx, pg.ProtectionClusterID)
	if err != nil {
		return nil, err
	}

	var protectionGroup drms.ProtectionGroup
	if err = protectionGroup.SetFromModel(pg); err != nil {
		return nil, err
	}

	protectionGroup.ProtectionCluster = c
	protectionGroup.OwnerGroup = c.OwnerGroup

	return &protectionGroup, nil
}

// Get 보호 그룹 조회
func Get(ctx context.Context, req *drms.ProtectionGroupRequest) (*drms.ProtectionGroup, error) {
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[ProtectionGroup-Get] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)

	var pg *model.ProtectionGroup
	if pg, err = getProtectionGroup(tid, req.GroupId); err != nil {
		logger.Errorf("[ProtectionGroup-Get] Could not get the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	// 권한이 없는 보호그룹 조회시 UnauthorizedRequest error
	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, pg.OwnerGroupID) {
		logger.Errorf("[ProtectionGroup-Get] Errors occurred during checking the authentication of the user.")
		return nil, errors.UnauthorizedRequest(ctx)
	}

	c, err := cluster.GetCluster(ctx, pg.ProtectionClusterID)
	if err != nil {
		logger.Errorf("[ProtectionGroup-Get] Could not get the protection group(%d) cluster(%d). Cause: %+v", req.GroupId, pg.ProtectionClusterID, err)
		return nil, err
	}

	var protectionGroup drms.ProtectionGroup
	if err = protectionGroup.SetFromModel(pg); err != nil {
		return nil, err
	}

	protectionGroup.ProtectionCluster = c
	protectionGroup.OwnerGroup = c.OwnerGroup

	if err = setProtectionGroupStateCode(ctx, &protectionGroup); err != nil {
		logger.Errorf("[ProtectionGroup-Get] Could not set the protection group(%d) state code. Cause: %+v", pg.ID, err)
		return nil, err
	}

	if protectionGroup.Instances, err = getProtectionGroupInstances(ctx, pg); err != nil {
		logger.Errorf("[ProtectionGroup-Get] Could not get the protection group(%d) instances. Cause: %+v", pg.ID, err)
		return nil, err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return isRecoveryJobExists(db, pg.ID)
	}); err == nil {
		protectionGroup.Updatable = true
	} else if err != nil && !errors.Equal(err, internal.ErrRecoveryJobExisted) {
		logger.Errorf("[ProtectionGroup-Get] Errors occurred during checking existence of the recovery job(pgID:%d). Cause: %+v", pg.ID, err)
		return nil, err
	}

	return &protectionGroup, nil
}

func checkValidAddProtectionGroup(ctx context.Context, pg *drms.ProtectionGroup) error {
	tid, _ := metadata.GetTenantID(ctx)
	err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.ProtectionGroup{TenantID: tid, Name: pg.Name}).First(&model.ProtectionGroup{}).Error
	})
	switch {
	case err == nil:
		return errors.ConflictParameterValue("protection_group.name", pg.Name)
	case err != gorm.ErrRecordNotFound:
		return errors.UnusableDatabase(err)
	}
	return nil
}

func checkValidUpdateProtectionGroup(ctx context.Context, pg *drms.ProtectionGroup) error {
	tid, _ := metadata.GetTenantID(ctx)
	err := database.Execute(func(db *gorm.DB) error {
		return db.Not(&model.ProtectionGroup{ID: pg.Id}).
			Where(&model.ProtectionGroup{TenantID: tid, Name: pg.Name}).
			First(&model.ProtectionGroup{}).Error
	})
	switch {
	case err == nil:
		return errors.ConflictParameterValue("protection_group.name", pg.Name)
	case err != gorm.ErrRecordNotFound:
		return errors.UnusableDatabase(err)
	}
	return nil
}

func checkValidProtectionGroup(ctx context.Context, pg *drms.ProtectionGroup) error {
	if pg == nil {
		return errors.RequiredParameter("group")
	}

	if pg.GetProtectionCluster().GetId() == 0 {
		return errors.RequiredParameter("group.protection_cluster.id")
	}

	if _, err := cluster.GetCluster(ctx, pg.ProtectionCluster.Id); err != nil {
		switch errors.GetIPCStatusCode(err) {
		case errors.IPCStatusNotFound:
			return errors.InvalidParameterValue("group.protection_cluster.id", pg.ProtectionCluster.Id, "not found protection cluster")
		case errors.IPCStatusUnauthorized:
			return errors.UnauthorizedRequest(ctx)
		}
		return err
	}

	// RPO validate
	if pg.RecoveryPointObjectiveType == "" {
		return errors.RequiredParameter("group.recovery_point_objective_type")
	}

	switch pg.RecoveryPointObjectiveType {
	case constant.RecoveryPointObjectiveTypeMinute:
		if pg.RecoveryPointObjective < 10 || pg.RecoveryPointObjective > 59 {
			return errors.OutOfRangeParameterValue("group.recovery_point_objective", pg.RecoveryPointObjective, 10, 59)
		}

	case constant.RecoveryPointObjectiveTypeHour:
		if pg.RecoveryPointObjective < 1 || pg.RecoveryPointObjective > 23 {
			return errors.OutOfRangeParameterValue("group.recovery_point_objective", pg.RecoveryPointObjective, 1, 23)
		}

	case constant.RecoveryPointObjectiveTypeDay:
		if pg.RecoveryPointObjective < 1 || pg.RecoveryPointObjective > 30 {
			return errors.OutOfRangeParameterValue("group.recovery_point_objective", pg.RecoveryPointObjective, 1, 30)
		}

	default:
		return errors.UnavailableParameterValue("group.recovery_point_objective_type", pg.RecoveryPointObjectiveType, []interface{}{
			constant.RecoveryPointObjectiveTypeMinute,
			constant.RecoveryPointObjectiveTypeHour,
			constant.RecoveryPointObjectiveTypeDay})
	}

	// RTO validate
	if pg.RecoveryTimeObjective < 15 || pg.RecoveryTimeObjective > 60 {
		return errors.OutOfRangeParameterValue("group.recovery_time_objective", pg.RecoveryTimeObjective, 15, 60)
	}

	if len(pg.Name) == 0 {
		return errors.RequiredParameter("group.name")
	}

	if len(pg.Name) > 255 {
		return errors.LengthOverflowParameterValue("group.name", pg.Name, 255)
	}

	matched, _ := regexp.MatchString("^[a-zA-Z0-9가-힣\\.\\#\\-\\_]*$", pg.Name)
	if !matched {
		return errors.InvalidParameterValue("group.name", pg.Name, "Only Hangul, Alphabet, Number(0-9) and special characters (-, _, #, .) are allowed.")
	}

	if len(pg.Remarks) > 300 {
		return errors.LengthOverflowParameterValue("group.remarks", pg.Remarks, 300)
	}

	return nil
}

func makeGroupedVolumeMap(ctx context.Context, clusterID uint64) (map[string]string, error) {
	volumeGroupList, err := cluster.GetClusterVolumeGroupList(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	volMap := make(map[string]string)

	if volumeGroupList == nil {
		return volMap, nil
	}

	for _, vg := range volumeGroupList {
		for _, vol := range vg.Volumes {
			volMap[vol.Uuid] = vg.Uuid
		}
	}

	return volMap, nil
}

func checkValidInstanceAddProtectionGroup(ctx context.Context, pg *drms.ProtectionGroup) error {
	volMap, err := makeGroupedVolumeMap(ctx, pg.ProtectionCluster.Id)
	if err != nil {
		return err
	}

	for idx, i := range pg.Instances {
		if i.Id == 0 {
			return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].id", idx))
		}

		if i.Uuid == "" {
			return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].uuid", idx))
		}

		isExist, err := cluster.CheckIsExistClusterInstance(ctx, pg.ProtectionCluster.Id, i.Uuid)
		if err != nil {
			logger.Warnf("[checkValidInstanceAddProtectionGroup] Could not get cluster(%d) instance(%d:%s). Cause: %+v", pg.ProtectionCluster.Id, i.Id, i.Uuid, err)
		} else if !isExist {
			return errors.InvalidParameterValue(fmt.Sprintf("group.instances[%d].uuid", idx), i.Uuid, "not found instance")
		}

		// 보호대상 인스턴스 목록에는 원본 클러스터의 인스턴스만 추가할 수 있다.
		instance, err := cluster.GetClusterInstance(ctx, pg.ProtectionCluster.Id, i.Id)
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			return errors.InvalidParameterValue(fmt.Sprintf("group.instances[%d].id", idx), i.Id, "not found instance")
		} else if err != nil {
			return err
		}

		// 다른 보호그룹에 포함된 인스턴스는 보호대상으로 추가할 수 없다.
		err = database.Execute(func(db *gorm.DB) error {
			return db.Model(&model.ProtectionInstance{}).
				Where("protection_group_id <> ?", pg.Id).
				Where("protection_cluster_instance_id = ?", i.Id).
				First(&model.ProtectionInstance{}).Error
		})
		switch {
		case err == nil:
			return errors.ConflictParameterValue(fmt.Sprintf("group.instances[%d].id", idx), i.Id)
		case err != gorm.ErrRecordNotFound:
			return errors.UnusableDatabase(err)
		}

		for _, v := range instance.Volumes {
			// storage 의 type code 가 ceph 이 아닌 인스턴스는 보호대상으로 추가할 수 없다.
			if v.Storage.TypeCode != storage.ClusterStorageTypeCeph {
				return errors.InvalidParameterValue(fmt.Sprintf("group.instances[%d].id", idx), i.Id, "unknown storage type")
			}

			if _, ok := volMap[v.Volume.Uuid]; ok {
				return internal.VolumeGroupIsExisted(pg.ProtectionCluster.Id, i.Id, v.Volume.Uuid)
			}
		}
	}

	return nil
}

func checkValidInstanceUpdateProtectionGroup(ctx context.Context, pg *drms.ProtectionGroup, origInstances []*cms.ClusterInstance) error {
	var err error
	instanceMap := make(map[uint64]*cms.ClusterInstance)
	for _, i := range origInstances {
		instanceMap[i.Id] = i
	}

	volMap, err := makeGroupedVolumeMap(ctx, pg.ProtectionCluster.Id)
	if err != nil {
		return err
	}

	for idx, i := range pg.Instances {
		if i.Id == 0 {
			return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].id", idx))
		}

		if i.Uuid == "" {
			return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].uuid", idx))
		}

		var (
			rsp   *cms.ClusterInstance
			isNew bool
		)
		if rsp = instanceMap[i.Id]; rsp == nil {
			isExist, err := cluster.CheckIsExistClusterInstance(ctx, pg.ProtectionCluster.Id, i.Uuid)
			if err != nil {
				logger.Warnf("[checkValidInstanceUpdateProtectionGroup] Could not get cluster(%d) instance(%d:%s). Cause: %+v", pg.ProtectionCluster.Id, i.Id, i.Uuid, err)
			} else if !isExist {
				return errors.InvalidParameterValue(fmt.Sprintf("group.instances[%d].uuid", idx), i.Uuid, "not found instance")
			}

			// 보호대상 인스턴스 목록에는 원본 클러스터의 인스턴스만 추가할 수 있다.
			rsp, err = cluster.GetClusterInstance(ctx, pg.ProtectionCluster.Id, i.Id)
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				return errors.InvalidParameterValue(fmt.Sprintf("group.instances[%d].id", idx), i.Id, "not found instance")
			} else if err != nil {
				return err
			}

			isNew = true
		}

		// 다른 보호그룹에 포함된 인스턴스는 보호대상으로 추가할 수 없다.
		err = database.Execute(func(db *gorm.DB) error {
			return db.Model(&model.ProtectionInstance{}).
				Where("protection_group_id <> ?", pg.Id).
				Where("protection_cluster_instance_id = ?", i.Id).
				First(&model.ProtectionInstance{}).Error
		})
		switch {
		case err == nil:
			return errors.ConflictParameterValue(fmt.Sprintf("group.instances[%d].id", idx), i.Id)
		case err != gorm.ErrRecordNotFound:
			return errors.UnusableDatabase(err)
		}

		for _, v := range rsp.Volumes {
			// storage 의 type code 가 ceph 이 아닌 인스턴스는 보호대상으로 추가할 수 없다.
			if v.Storage.TypeCode != storage.ClusterStorageTypeCeph {
				return errors.InvalidParameterValue(fmt.Sprintf("group.instances[%d].id", idx), i.Id, "unknown storage type")
			}

			if _, ok := volMap[v.Volume.Uuid]; ok && isNew {
				return internal.VolumeGroupIsExisted(pg.ProtectionCluster.Id, i.Id, v.Volume.Uuid)
			}
		}
	}

	return nil
}

func createSnapshotSchedule(ctx context.Context, pgid uint64, intervalType string, interval uint32) (uint64, error) {
	// TODO : Not implemented
	return 0, nil
}

func updateSnapshotSchedule(ctx context.Context, sid, pgid uint64, intervalType string, interval uint32) error {
	// TODO : Not implemented
	return nil
}

// Add 보호그룹 등록
func Add(ctx context.Context, req *drms.AddProtectionGroupRequest) (*drms.ProtectionGroup, error) {
	logger.Info("[ProtectionGroup-Add] Start")

	var err error
	if err = checkValidProtectionGroup(ctx, req.Group); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkValidInstanceAddProtectionGroup(ctx, req.Group); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkValidAddProtectionGroup(ctx, req.Group); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	var pg *model.ProtectionGroup
	if pg, err = req.Group.Model(); err != nil {
		return nil, err
	}

	var c *cms.Cluster
	if c, err = cluster.GetCluster(ctx, req.Group.ProtectionCluster.Id); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Could not get the cluster(%d). Cause: %+v", req.Group.ProtectionCluster.Id, err)
		return nil, err
	}

	pg.ProtectionClusterID = c.Id
	pg.OwnerGroupID = c.OwnerGroup.Id
	pg.TenantID, _ = metadata.GetTenantID(ctx)

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Create(pg).Error; err != nil {
			logger.Errorf("[ProtectionGroup-Add] Could not create the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
			return err
		}

		for _, i := range req.Group.Instances {
			if err = db.Save(&model.ProtectionInstance{
				ProtectionGroupID:           pg.ID,
				ProtectionClusterInstanceID: i.Id,
			}).Error; err != nil {
				logger.Errorf("[ProtectionGroup-Add] Could not add the protection instance(%d). Cause: %+v", i.Id, err)
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	defer func() {
		if err == nil {
			return
		}
		if err = database.GormTransaction(func(db *gorm.DB) error {
			if err = db.Where(&model.ProtectionInstance{ProtectionGroupID: pg.ID}).Delete(&model.ProtectionInstance{}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			return db.Delete(pg).Error
		}); err != nil {
			logger.Errorf("[ProtectionGroup-Add] Could not delete protection group(%d:%s) during rollback. Cause: %+v", pg.ID, pg.Name, err)
		}
	}()

	if pg.SnapshotScheduleID, err = createSnapshotSchedule(ctx, pg.ID, req.Group.SnapshotIntervalType, req.Group.SnapshotInterval); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Could not create the snapshot schedule of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
		return nil, err
	}

	defer func() {
		if err == nil {
			return
		}
		if e := deleteSnapshotSchedule(ctx, pg.SnapshotScheduleID); e != nil {
			logger.Errorf("[ProtectionGroup-Add] Could not delete the snapshot schedule(%d) of the protection group(%d:%s). Cause: %+v", pg.SnapshotScheduleID, pg.ID, pg.Name, e)
		}
	}()

	if err = database.GormTransaction(func(db *gorm.DB) error {
		return db.Save(pg).Error
	}); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Could not add protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
		return nil, errors.UnusableDatabase(err)
	}

	var rsp *drms.ProtectionGroup
	if rsp, err = Get(ctx, &drms.ProtectionGroupRequest{GroupId: pg.ID}); err != nil {
		logger.Errorf("[ProtectionGroup-Add] Could not get the protection group(%d). Cause: %+v", pg.ID, err)
		return nil, err
	}

	logger.Infof("[ProtectionGroup-Add] Success: group(%d:%s)", pg.ID, pg.Name)
	return rsp, nil
}

func isRecoveryJobExists(db *gorm.DB, pgid uint64) error {
	var plans []*model.Plan
	if err := db.Where(&model.Plan{ProtectionGroupID: pgid}).Find(&plans).Error; err != nil {
		return err
	}

	for _, plan := range plans {
		var n int
		if err := db.Model(&model.Job{}).Where(&model.Job{RecoveryPlanID: plan.ID}).Count(&n).Error; err != nil {
			return err
		}

		if n > 0 {
			return internal.RecoveryJobExisted(pgid, plan.ID)
		}
	}

	return nil
}

func checkUpdatableProtectionGroup(orig *model.ProtectionGroup, req *drms.UpdateProtectionGroupRequest) error {
	if req.GroupId != req.Group.Id {
		return errors.UnchangeableParameter("group.id")
	}

	if req.Group != nil && req.Group.OwnerGroup != nil && req.Group.OwnerGroup.Id != orig.OwnerGroupID {
		return errors.UnchangeableParameter("group.owner_group.id")
	}

	if req.Group != nil && req.Group.ProtectionCluster != nil && req.Group.ProtectionCluster.Id != orig.ProtectionClusterID {
		return errors.UnchangeableParameter("group.protection_cluster.id")
	}

	// job 이 존재하는 경우 protection group 을 수정할 수 없다.
	if err := database.Execute(func(db *gorm.DB) error {
		return isRecoveryJobExists(db, req.GroupId)
	}); err != nil {
		return err
	}
	return nil
}

func updateProtectionInstanceList(db *gorm.DB, pg *model.ProtectionGroup, instances []*cms.ClusterInstance) error {
	var origList []*model.ProtectionInstance

	// 기존 보호대상 인스턴스 목록 조회
	if err := db.Where("protection_group_id = ?", pg.ID).Find(&origList).Error; err != nil {
		return errors.UnusableDatabase(err)
	}

	// 추가된 인스턴스
	for _, i := range instances {
		var exists bool
		for _, orig := range origList {
			exists = exists || orig.ProtectionClusterInstanceID == i.Id
		}
		if exists {
			continue
		}

		if err := db.Save(&model.ProtectionInstance{
			ProtectionGroupID:           pg.ID,
			ProtectionClusterInstanceID: i.Id,
		}).Error; err != nil {
			return errors.UnusableDatabase(err)
		}
		logger.Infof("[updateProtectionInstanceList] Success to add the protection group(%d) instance(%d).", pg.ID, i.Id)
	}

	// 제거된 인스턴스
	for _, orig := range origList {
		var exists bool
		for _, i := range instances {
			exists = exists || orig.ProtectionClusterInstanceID == i.Id
		}
		if exists {
			continue
		}

		if err := db.Where(&model.ProtectionInstance{
			ProtectionGroupID:           orig.ProtectionGroupID,
			ProtectionClusterInstanceID: orig.ProtectionClusterInstanceID,
		}).Delete(&model.ProtectionInstance{}).Error; err != nil {
			return errors.UnusableDatabase(err)
		}
		logger.Infof("[updateProtectionInstanceList] Success to delete the protection group(%d) instance(%d).", orig.ProtectionGroupID, orig.ProtectionClusterInstanceID)
	}

	return nil
}

// Update 보호그룹 수정
func Update(ctx context.Context, req *drms.UpdateProtectionGroupRequest) (*drms.ProtectionGroup, error) {
	logger.Info("[ProtectionGroup-Update] Start")

	var err error
	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetGroup() == nil {
		err = errors.RequiredParameter("group")
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetGroup().GetId() == 0 {
		err = errors.RequiredParameter("group.id")
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetGroup().GetId() != req.GetGroupId() {
		err = errors.UnchangeableParameter("group.id")
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)

	var orig *model.ProtectionGroup
	if orig, err = getProtectionGroup(tid, req.GroupId); err != nil {
		logger.Errorf("[ProtectionGroup-Update] Could not get the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, orig.OwnerGroupID) {
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during checking the authentication of the user. Cause: %+v", err)
		return nil, errors.UnauthorizedRequest(ctx)
	}

	if err = checkValidProtectionGroup(ctx, req.Group); err != nil {
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	origPG := new(model.ProtectionGroup)
	if err = copier.CopyWithOption(origPG, orig, copier.Option{DeepCopy: true}); err != nil {
		return nil, errors.Unknown(err)
	}

	var instances []*cms.ClusterInstance
	if instances, err = getProtectionGroupInstances(ctx, origPG); err != nil {
		return nil, err
	}

	if err = checkValidInstanceUpdateProtectionGroup(ctx, req.Group, instances); err != nil {
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during checking updatable status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	if err = checkValidUpdateProtectionGroup(ctx, req.Group); err != nil {
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkUpdatableProtectionGroup(orig, req); err != nil {
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during checking updatable status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	var pg *model.ProtectionGroup
	if pg, err = req.Group.Model(); err != nil {
		return nil, err
	}

	orig.Name = pg.Name
	orig.Remarks = pg.Remarks
	orig.RecoveryPointObjectiveType = pg.RecoveryPointObjectiveType
	orig.RecoveryPointObjective = pg.RecoveryPointObjective
	orig.RecoveryTimeObjective = pg.RecoveryTimeObjective

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Save(&orig).Error; err != nil {
			logger.Errorf("[ProtectionGroup-Update] Could not update the protection group(%d:%s) db. Cause: %+v", pg.ID, pg.Name, err)
			return err
		}

		if err = updateProtectionInstanceList(db, orig, req.Group.Instances); err != nil {
			logger.Errorf("[ProtectionGroup-Update] Could not update the instance list of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
			return err
		}

		return nil
	}); err != nil {
		logger.Errorf("[ProtectionGroup-Update] Could not update the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
		return nil, errors.UnusableDatabase(err)
	}

	defer func() {
		if err == nil {
			return
		}
		database.GormTransaction(func(db *gorm.DB) error {
			if err = db.Save(&origPG).Error; err != nil {
				logger.Errorf("[ProtectionGroup-Update] Could not update the protection group(%d:%s) db. Cause: %+v", pg.ID, pg.Name, err)
				return err
			}

			if err = updateProtectionInstanceList(db, origPG, instances); err != nil {
				logger.Errorf("[ProtectionGroup-Update] Could not update the instance list of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
			}
			return nil
		})
	}()

	var rsp *drms.ProtectionGroup
	rsp, err = Get(ctx, &drms.ProtectionGroupRequest{GroupId: pg.ID})
	if err != nil {
		logger.Errorf("[ProtectionGroup-Update] Errors occurred during getting the protection group(%d). Cause: %+v", pg.ID, err)
		return nil, err
	}

	if err := internal.PublishMessage(constant.QueueSyncPlanListByProtectionGroup, pg.ID); err != nil {
		logger.Warnf("[ProtectionGroup-Update] Could not publish sync all plans message: protection group(%d). Cause: %+v", pg.ID, err)
	} else {
		logger.Infof("[ProtectionGroup-Update] Publish sync all plans: protection group(%d).", pg.ID)
	}

	logger.Infof("[ProtectionGroup-Update] Success: group(%d:%s)", pg.ID, pg.Name)
	return rsp, err
}

func deleteSnapshotSchedule(ctx context.Context, id uint64) error {
	// TODO: Not Implemented
	return nil
}

func checkDeletableProtectionGroup(pg *model.ProtectionGroup) error {
	if err := database.Execute(func(db *gorm.DB) error {
		if plans, err := pg.GetPlans(db); err != nil {
			return errors.UnusableDatabase(err)
		} else if len(plans) > 0 {
			return internal.RecoveryPlanExisted(pg.ProtectionClusterID, pg.ID)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// Delete 보호그룹 제거
func Delete(ctx context.Context, req *drms.DeleteProtectionGroupRequest) error {
	logger.Info("[ProtectionGroup-Delete] Start")

	var err error
	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[ProtectionGroup-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	tid, _ := metadata.GetTenantID(ctx)

	var pg *model.ProtectionGroup
	if pg, err = getProtectionGroup(tid, req.GroupId); err != nil {
		logger.Errorf("[ProtectionGroup-Delete] Could not get the protection group(%d). Cause: %+v", req.GroupId, err)
		return err
	}

	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, pg.OwnerGroupID) {
		logger.Errorf("[ProtectionGroup-Delete] Errors occurred during checking the authentication of the user. Cause: %+v", err)
		return errors.UnauthorizedRequest(ctx)
	}

	if err = checkDeletableProtectionGroup(pg); err != nil {
		logger.Errorf("[ProtectionGroup-Delete] Errors occurred during checking deletable status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return err
	}

	if err = deleteSnapshotSchedule(ctx, pg.SnapshotScheduleID); err != nil {
		logger.Warnf("[ProtectionGroup-Delete] Could not delete the snapshot schedule(%d) of the protection group(%d:%s). Cause: %+v", pg.SnapshotScheduleID, pg.ID, pg.Name, err)
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Where(&model.ProtectionInstance{ProtectionGroupID: pg.ID}).Delete(&model.ProtectionInstance{}).Error; err != nil {
			logger.Errorf("[ProtectionGroup-Delete] Could not delete the protection instance of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
			return err
		}

		if err = db.Where(&model.ProtectionGroup{ID: req.GroupId, TenantID: tid}).Delete(&model.ProtectionGroup{}).Error; err != nil {
			logger.Errorf("[ProtectionGroup-Delete] Could not delete the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
			return err
		}
		return nil
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	if err = deleteProtectionGroupSnapshot(pg.ID); err != nil {
		logger.Errorf("[ProtectionGroup-Delete] Could not delete the all snapshots of the protection group(%d:%s). Cause: %+v", pg.ID, pg.Name, err)
		return err
	}

	if err = migrator.DeleteAllFailbackProtectionGroup(req.GroupId); err != nil {
		logger.Warnf("[ProtectionGroup-Delete] Could not delete failback protection group info: protection group(%d). Cause: %+v", req.GroupId, err)
	}

	logger.Infof("[ProtectionGroup-Delete] Success: group(%d:%s)", pg.ID, pg.Name)
	return nil
}

// GetSnapshotList 보호 그룹 스냅샷 목록 조회
func GetSnapshotList(ctx context.Context, req *drms.ProtectionGroupSnapshotListRequest) ([]*drms.ProtectionGroupSnapshot, error) {
	return nil, nil
}
