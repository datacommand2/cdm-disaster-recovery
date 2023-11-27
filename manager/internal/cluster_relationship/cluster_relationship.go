package clusterrelationship

import (
	"context"
	centerConstant "github.com/datacommand2/cdm-center/cluster-manager/constant"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jinzhu/gorm"
)

type clusterRelation struct {
	ProtectionGroup model.ProtectionGroup `gorm:"embedded"`
	RecoveryPlan    model.Plan            `gorm:"embedded"`
}

func worstState(plans []*drms.RelationshipPlan) (string, error) {
	m := make(map[string]interface{})
	for _, plan := range plans {
		m[plan.StateCode] = true
	}

	_, ok := m[constant.RecoveryPlanStateEmergency]
	if ok {
		return constant.ClusterRelationshipStateEmergency, nil
	}

	_, ok = m[constant.RecoveryPlanStateCritical]
	if ok {
		return constant.ClusterRelationshipStateCritical, nil
	}

	_, ok = m[constant.RecoveryPlanStateWarning]
	if ok {
		return constant.ClusterRelationshipStateWarning, nil
	}

	_, ok = m[constant.RecoveryPlanStateNormal]
	if ok {
		return constant.ClusterRelationshipStateNormal, nil
	}

	return "", errors.Unknown(errors.New("not found plans state"))
}

func (c *clusterRelation) drmsRelationshipPlan(ctx context.Context) (*drms.RelationshipPlan, error) {
	rsp, err := recoveryPlan.Get(ctx, &drms.RecoveryPlanRequest{
		GroupId: c.ProtectionGroup.ID,
		PlanId:  c.RecoveryPlan.ID,
	})
	if err != nil {
		return nil, err
	}

	return &drms.RelationshipPlan{
		Id:   rsp.GetId(),
		Name: rsp.GetName(),
		Group: &drms.ProtectionGroup{
			Id:   c.ProtectionGroup.ID,
			Name: c.ProtectionGroup.Name,
		},
		StateCode: rsp.GetStateCode(),
	}, nil
}

func queryRelations(offset, limit *wrappers.UInt64Value, clusterIDs []uint64) ([]*clusterRelation, error) {
	var relations []*clusterRelation
	if err := database.Execute(func(db *gorm.DB) error {
		cond := db
		if offset != nil {
			cond = cond.Offset(offset.GetValue())
		}
		if limit != nil {
			cond = cond.Limit(limit.GetValue())
		}

		// gorm v1 에서 Distinct 를 지원하지 않음
		return cond.
			Raw("SELECT DISTINCT protection_cluster_id,recovery_cluster_id "+
				"FROM cdm_disaster_recovery_protection_group JOIN cdm_disaster_recovery_plan ON cdm_disaster_recovery_plan.protection_group_id = cdm_disaster_recovery_protection_group.id "+
				"WHERE protection_cluster_id IN (?) "+
				"ORDER BY protection_cluster_id,recovery_cluster_id", clusterIDs).
			Scan(&relations).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return relations, nil
}

func countRelations(clusterIDs []uint64) (uint64, error) {
	var relations []*clusterRelation
	if err := database.Execute(func(db *gorm.DB) error {
		return db.
			Raw("SELECT DISTINCT protection_cluster_id,recovery_cluster_id "+
				"FROM cdm_disaster_recovery_protection_group "+
				"JOIN cdm_disaster_recovery_plan "+
				"ON cdm_disaster_recovery_plan.protection_group_id = cdm_disaster_recovery_protection_group.id "+
				"WHERE protection_cluster_id IN (?) "+
				"ORDER BY protection_cluster_id,recovery_cluster_id", clusterIDs).
			Scan(&relations).Error
	}); err != nil {
		return 0, errors.UnusableDatabase(err)
	}

	return uint64(len(relations)), nil
}

func queryPlans(protectionClusterID, recoveryClusterID uint64) ([]*clusterRelation, error) {
	var plans []*clusterRelation
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Model(&model.ProtectionGroup{}).
			Select("*").
			Joins("JOIN cdm_disaster_recovery_plan " +
				"ON cdm_disaster_recovery_plan.protection_group_id = cdm_disaster_recovery_protection_group.id").
			Where(model.ProtectionGroup{
				ProtectionClusterID: protectionClusterID,
			}).
			Where(model.Plan{
				RecoveryClusterID: recoveryClusterID,
			}).
			Scan(&plans).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return plans, nil
}

// GetClusterRelationList 클러스터 관계도 목록 조회
func GetClusterRelationList(ctx context.Context, req *drms.ClusterRelationshipListRequest) ([]*drms.ClusterRelationship, *drms.Pagination, error) {
	var err error
	var clusterMap map[uint64]*cms.Cluster

	if clusterMap, err = cluster.GetClusterMap(ctx); err != nil {
		logger.Errorf("[GetClusterRelationList] Could not get the cluster map. Cause: %+v", err)
		return nil, nil, err
	}
	var cIDs []uint64
	for cID := range clusterMap {
		cIDs = append(cIDs, cID)
	}

	relations, err := queryRelations(req.GetOffset(), req.GetLimit(), cIDs)
	if err != nil {
		logger.Errorf("[GetClusterRelationList] Errors occurred during getting query relations. Cause: %+v", err)
		return nil, nil, err
	}

	var ret []*drms.ClusterRelationship
	for _, relation := range relations {
		protection, err := cluster.GetCluster(ctx, relation.ProtectionGroup.ProtectionClusterID)
		if err != nil {
			logger.Errorf("[GetClusterRelationList] Could not get the protection cluster(%d). Cause: %+v", relation.ProtectionGroup.ProtectionClusterID, err)
			return nil, nil, err
		}

		recovery, err := cluster.GetCluster(ctx, relation.RecoveryPlan.RecoveryClusterID)
		if err != nil {
			logger.Errorf("[GetClusterRelationList] Could not get the recovery cluster(%d). Cause: %+v", relation.RecoveryPlan.RecoveryClusterID, err)
			return nil, nil, err
		}
		d := drms.ClusterRelationship{
			ProtectionCluster: protection,
			RecoveryCluster:   recovery,
			Plans:             []*drms.RelationshipPlan{},
		}

		if recovery.StateCode == centerConstant.ClusterStateInactive {
			d.StateCode = constant.ClusterRelationshipStateCritical
		} else if protection.StateCode == centerConstant.ClusterStateInactive {
			d.StateCode = constant.ClusterRelationshipStateEmergency
		} else {
			plans, err := queryPlans(relation.ProtectionGroup.ProtectionClusterID, relation.RecoveryPlan.RecoveryClusterID)
			if err != nil {
				logger.Errorf("[GetClusterRelationList] Could not get the plans. Cause: %+v", err)
				return nil, nil, err
			}

			for _, plan := range plans {
				drmsPlan, err := plan.drmsRelationshipPlan(ctx)
				if err != nil {
					logger.Errorf("[GetClusterRelationList] Could not get the relationship plan. Cause: %+v", err)
					return nil, nil, err
				}
				d.Plans = append(d.Plans, drmsPlan)
			}

			state, err := worstState(d.Plans)
			if err != nil {
				logger.Errorf("[GetClusterRelationList] Could not get the worst state. Cause: %+v", err)
				return nil, nil, err
			}
			d.StateCode = state
		}

		ret = append(ret, &d)
	}

	pagination, err := getClusterRelationListPagination(cIDs, req.GetOffset().GetValue(), req.GetLimit().GetValue())
	if err != nil {
		logger.Errorf("[GetClusterRelationList] Could not get the cluster relation list pagination. Cause: %+v", err)
		return nil, nil, err
	}

	return ret, pagination, nil
}

func getClusterRelationListPagination(clusterIDs []uint64, offset, limit uint64) (*drms.Pagination, error) {
	total, err := countRelations(clusterIDs)
	if err != nil {
		return nil, err
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
