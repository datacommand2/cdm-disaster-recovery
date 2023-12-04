package recoveryplan

import (
	"context"
	"fmt"
	centerConstant "github.com/datacommand2/cdm-center/cluster-manager/constant"
	cmsModel "github.com/datacommand2/cdm-center/cluster-manager/database/model"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-center/cluster-manager/queue"
	cmsStorage "github.com/datacommand2/cdm-center/cluster-manager/storage"
	cmsVolume "github.com/datacommand2/cdm-center/cluster-manager/volume"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jinzhu/copier"
	"github.com/jinzhu/gorm"
	"sync"
	"time"
)

// StopMirrorVolumeOptions 복구 계획 삭제 시 볼륨 복제 중지 대한 operation 옵션
type StopMirrorVolumeOptions struct {
	Operation string
	Stop      bool
	Delete    bool
}

// Option StopMirrorVolumeOptions 설정 함수
type Option func(*StopMirrorVolumeOptions)

// StopOperation 볼륨 복제 중지 operation 옵션 설정 함수
func StopOperation(op string) Option {
	return func(o *StopMirrorVolumeOptions) {
		o.Operation = op
	}
}

type RecoveryPointTypeCode struct {
	TypeCode string
}

type TypeCode func(*RecoveryPointTypeCode)

func SetRecoveryPointTypeCode(typeCode string) TypeCode {
	return func(o *RecoveryPointTypeCode) {
		o.TypeCode = typeCode
	}
}

func getProtectionGroup(gid, tid uint64) (*model.ProtectionGroup, error) {
	var m model.ProtectionGroup
	err := database.Execute(func(db *gorm.DB) error {
		return db.First(&m, &model.ProtectionGroup{ID: gid, TenantID: tid}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil, internal.NotFoundProtectionGroup(gid, tid)

	case err != nil:
		return nil, errors.UnusableDatabase(err)
	}

	return &m, nil
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
		if err != nil {
			return nil, err
		}

		instances = append(instances, rsp)
	}
	return instances, nil
}

func getProtectionGroupMessage(ctx context.Context, gid uint64) (*drms.ProtectionGroup, error) {
	var pg *model.ProtectionGroup
	var err error
	tid, _ := metadata.GetTenantID(ctx)
	if pg, err = getProtectionGroup(gid, tid); err != nil {
		return nil, err
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

	if protectionGroup.Instances, err = getProtectionGroupInstances(ctx, pg); err != nil {
		return nil, err
	}

	return &protectionGroup, nil
}

func getPlan(gid, pid uint64) (*model.Plan, error) {
	var m model.Plan
	err := database.Execute(func(db *gorm.DB) error {
		return db.First(&m, &model.Plan{ID: pid, ProtectionGroupID: gid}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil, internal.NotFoundRecoveryPlan(gid, pid)
	case err != nil:
		return nil, errors.UnusableDatabase(err)
	}

	return &m, nil
}

func appendTenantRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, tenants []model.PlanTenant) (err error) {
	for _, t := range tenants {
		var tenant drms.TenantRecoveryPlan
		recoveryPlan.Detail.Tenants = append(recoveryPlan.Detail.Tenants, &tenant)

		if err = tenant.SetFromModel(&t); err != nil {
			return err
		}

		if tenant.ProtectionClusterTenant, err = cluster.GetClusterTenant(
			ctx, recoveryPlan.ProtectionCluster.Id, t.ProtectionClusterTenantID,
		); err != nil {
			return err
		}

		if t.RecoveryClusterTenantID == nil || *t.RecoveryClusterTenantID == 0 {
			continue
		}

		if tenant.RecoveryClusterTenant, err = cluster.GetClusterTenant(
			ctx, recoveryPlan.RecoveryCluster.Id, *t.RecoveryClusterTenantID,
		); err != nil {
			return err
		}
	}

	return nil
}

func appendAvailabilityZoneRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, zones []model.PlanAvailabilityZone) (err error) {
	for _, z := range zones {
		var zone drms.AvailabilityZoneRecoveryPlan
		recoveryPlan.Detail.AvailabilityZones = append(recoveryPlan.Detail.AvailabilityZones, &zone)

		if err = zone.SetFromModel(&z); err != nil {
			return err
		}

		if zone.ProtectionClusterAvailabilityZone, err = cluster.GetClusterAvailabilityZone(
			ctx, recoveryPlan.ProtectionCluster.Id, z.ProtectionClusterAvailabilityZoneID,
		); err != nil {
			return err
		}

		if z.RecoveryClusterAvailabilityZoneID == nil || *z.RecoveryClusterAvailabilityZoneID == 0 {
			continue
		}

		if zone.RecoveryClusterAvailabilityZone, err = cluster.GetClusterAvailabilityZone(
			ctx, recoveryPlan.RecoveryCluster.Id, *z.RecoveryClusterAvailabilityZoneID,
		); err != nil {
			logger.Warnf("[appendAvailabilityZoneRecoveryPlans] Could not get protection cluster(%d) availability zone(%d). Cause: %+v",
				recoveryPlan.RecoveryCluster.Id, *z.RecoveryClusterAvailabilityZoneID, err)
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				if err := internal.PublishMessage(centerConstant.QueueNoticeClusterAvailabilityZoneDeleted, &queue.DeleteClusterAvailabilityZone{
					Cluster:              &cmsModel.Cluster{ID: recoveryPlan.ProtectionCluster.Id},
					OrigAvailabilityZone: &cmsModel.ClusterAvailabilityZone{ID: *z.RecoveryClusterAvailabilityZoneID},
				}); err != nil {
					logger.Warnf("[appendAvailabilityZoneRecoveryPlans] Could not publish availability zone deleted plan sync message: cluster(%d) availability zone(%d). Cause: %+v",
						recoveryPlan.RecoveryCluster.Id, *z.RecoveryClusterAvailabilityZoneID, err)
				} else {
					logger.Infof("[appendAvailabilityZoneRecoveryPlans] Publish availability zone deleted plan sync: cluster(%d) availability zone(%d).",
						recoveryPlan.RecoveryCluster.Id, *z.RecoveryClusterAvailabilityZoneID)
				}
			}
			return err
		}
	}
	return nil
}

func appendExternalNetworkRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, networks []model.PlanExternalNetwork) (err error) {
	for _, n := range networks {
		var network drms.ExternalNetworkRecoveryPlan
		recoveryPlan.Detail.ExternalNetworks = append(recoveryPlan.Detail.ExternalNetworks, &network)

		if err = network.SetFromModel(&n); err != nil {
			return err
		}

		if network.ProtectionClusterExternalNetwork, err = cluster.GetClusterNetwork(
			ctx, recoveryPlan.ProtectionCluster.Id, n.ProtectionClusterExternalNetworkID,
		); err != nil {
			return err
		}

		if n.RecoveryClusterExternalNetworkID == nil || *n.RecoveryClusterExternalNetworkID == 0 {
			continue
		}

		if network.RecoveryClusterExternalNetwork, err = cluster.GetClusterNetwork(
			ctx, recoveryPlan.RecoveryCluster.Id, *n.RecoveryClusterExternalNetworkID,
		); err != nil {
			logger.Warnf("[appendExternalNetworkRecoveryPlans] Could not get protection cluster(%d) network(%d). Cause: %+v",
				recoveryPlan.ProtectionCluster.Id, *n.RecoveryClusterExternalNetworkID, err)
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				if err := internal.PublishMessage(centerConstant.QueueNoticeClusterNetworkDeleted, &queue.DeleteClusterNetwork{
					Cluster:     &cmsModel.Cluster{ID: recoveryPlan.ProtectionCluster.Id},
					OrigNetwork: &cmsModel.ClusterNetwork{ID: *n.RecoveryClusterExternalNetworkID},
				}); err != nil {
					logger.Warnf("[appendExternalNetworkRecoveryPlans] Could not publish network deleted plan sync message: cluster(%d) network(%d). Cause: %+v",
						recoveryPlan.ProtectionCluster.Id, *n.RecoveryClusterExternalNetworkID, err)
				} else {
					logger.Infof("[appendExternalNetworkRecoveryPlans] Publish network deleted plan sync: cluster(%d) network(%d).",
						recoveryPlan.ProtectionCluster.Id, *n.RecoveryClusterExternalNetworkID)
				}
			}
			return err
		}
	}
	return nil
}

func appendRouterRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, routers []model.PlanRouter) (err error) {
	for _, r := range routers {
		var router drms.RouterRecoveryPlan
		recoveryPlan.Detail.Routers = append(recoveryPlan.Detail.Routers, &router)

		if err = router.SetFromModel(&r); err != nil {
			return err
		}

		if router.ProtectionClusterRouter, err = cluster.GetClusterRouter(
			ctx, recoveryPlan.ProtectionCluster.Id, r.ProtectionClusterRouterID,
		); err != nil {
			return err
		}

		if r.RecoveryClusterExternalNetworkID == nil || *r.RecoveryClusterExternalNetworkID == 0 {
			continue
		}

		if router.RecoveryClusterExternalNetwork, err = cluster.GetClusterNetwork(
			ctx, recoveryPlan.RecoveryCluster.Id, *r.RecoveryClusterExternalNetworkID,
		); err != nil {
			return err
		}

		var ifaceList []model.PlanExternalRoutingInterface
		if err = database.Execute(func(db *gorm.DB) error {
			if ifaceList, err = r.GetPlanExternalRoutingInterfaces(db); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, i := range ifaceList {
			var iface = cms.ClusterNetworkRoutingInterface{
				Network: router.RecoveryClusterExternalNetwork,
			}

			if i.IPAddress != nil && *i.IPAddress != "" {
				iface.IpAddress = *i.IPAddress
			}

			for _, s := range router.RecoveryClusterExternalNetwork.Subnets {
				if i.RecoveryClusterExternalSubnetID == s.Id {
					iface.Subnet = s
					break
				}
			}

			if iface.Subnet != nil {
				router.RecoveryClusterExternalRoutingInterfaces = append(
					router.RecoveryClusterExternalRoutingInterfaces, &iface,
				)
			}
		}
	}
	return nil
}

func appendStorageRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, storages []model.PlanStorage) (err error) {
	for _, s := range storages {
		var storage drms.StorageRecoveryPlan
		recoveryPlan.Detail.Storages = append(recoveryPlan.Detail.Storages, &storage)

		if err = storage.SetFromModel(&s); err != nil {
			return err
		}

		if storage.ProtectionClusterStorage, err = cluster.GetClusterStorage(
			ctx, recoveryPlan.ProtectionCluster.Id, s.ProtectionClusterStorageID,
		); err != nil {
			return err
		}

		if s.RecoveryClusterStorageID == nil || *s.RecoveryClusterStorageID == 0 {
			continue
		}

		if storage.RecoveryClusterStorage, err = cluster.GetClusterStorage(
			ctx, recoveryPlan.RecoveryCluster.Id, *s.RecoveryClusterStorageID,
		); err != nil {
			logger.Warnf("[appendStorageRecoveryPlans] Could not get protection cluster(%d) storage(%d). Cause: %+v",
				recoveryPlan.RecoveryCluster.Id, *s.RecoveryClusterStorageID, err)
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				if err := internal.PublishMessage(centerConstant.QueueNoticeClusterStorageDeleted, &queue.DeleteClusterStorage{
					Cluster:     &cmsModel.Cluster{ID: recoveryPlan.ProtectionCluster.Id},
					OrigStorage: &cmsModel.ClusterStorage{ID: *s.RecoveryClusterStorageID},
				}); err != nil {
					logger.Warnf("[appendStorageRecoveryPlans] Could not publish storage deleted plan sync message: cluster(%d) storage(%d). Cause: %+v",
						recoveryPlan.RecoveryCluster.Id, *s.RecoveryClusterStorageID, err)
				} else {
					logger.Infof("[appendStorageRecoveryPlans] Publish storage deleted plan sync: cluster(%d) storage(%d).",
						recoveryPlan.RecoveryCluster.Id, *s.RecoveryClusterStorageID)
				}
			}
			return err
		}
	}
	return nil
}

func getRemoteSecurityGroupRecursive(ctx context.Context, c *cms.Cluster, sg *cms.ClusterSecurityGroup, isExists func(id uint64) bool) ([]*cms.ClusterSecurityGroup, error) {
	if isExists(sg.Id) {
		return nil, nil
	}

	rsp, err := cluster.GetClusterSecurityGroup(ctx, c.Id, sg.Id)
	if err != nil {
		return nil, err
	}

	var l []*cms.ClusterSecurityGroup

	for _, r := range sg.Rules {
		if r.RemoteSecurityGroup.GetId() == 0 {
			continue
		}

		if l, err = getRemoteSecurityGroupRecursive(ctx, c, r.RemoteSecurityGroup, isExists); err != nil {
			return nil, err
		}
	}

	l = append(l, rsp)

	return l, nil
}

func appendExtraRemoteSecurityGroups(ctx context.Context, recoveryPlan *drms.RecoveryPlan) error {
	var m = make(map[uint64]bool)

	var isExists = func(id uint64) bool {
		if m[id] {
			return true
		}
		m[id] = true
		return false
	}

	// 인스턴스와 연결된 Security Group 은 제외한다.
	for _, i := range recoveryPlan.Detail.Instances {
		for _, sg := range i.ProtectionClusterInstance.SecurityGroups {
			m[sg.Id] = true
		}
	}

	for _, i := range recoveryPlan.Detail.Instances {
		for _, sg := range i.ProtectionClusterInstance.SecurityGroups {
			for _, r := range sg.Rules {
				if r.RemoteSecurityGroup.GetId() == 0 {
					continue
				}

				tmp, err := getRemoteSecurityGroupRecursive(ctx, i.ProtectionClusterInstance.Cluster, r.RemoteSecurityGroup, isExists)
				if err != nil {
					return err
				}

				if tmp != nil {
					recoveryPlan.Detail.ExtraRemoteSecurityGroups = append(
						recoveryPlan.Detail.ExtraRemoteSecurityGroups,
						tmp...,
					)
				}
			}
		}
	}

	return nil
}

func appendInstanceRecoveryPlans(ctx context.Context, pgID uint64, recoveryPlan *drms.RecoveryPlan, instances []model.PlanInstance) (err error) {
	for _, i := range instances {
		var instance drms.InstanceRecoveryPlan
		recoveryPlan.Detail.Instances = append(recoveryPlan.Detail.Instances, &instance)

		if err = instance.SetFromModel(&i); err != nil {
			return err
		}

		if instance.ProtectionClusterInstance, err = cluster.GetClusterInstance(
			ctx, recoveryPlan.ProtectionCluster.Id, i.ProtectionClusterInstanceID,
		); err != nil {
			logger.Warnf("[appendInstanceRecoveryPlans] Could not get protection cluster(%d) instance(%d). Cause: %+v",
				recoveryPlan.ProtectionCluster.Id, i.ProtectionClusterInstanceID, err)
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				if err := internal.PublishMessage(constant.QueueSyncAllPlansByRecoveryPlan, &internal.SyncPlanMessage{
					ProtectionGroupID: pgID,
					RecoveryPlanID:    recoveryPlan.Id,
				}); err != nil {
					logger.Warnf("[appendInstanceRecoveryPlans] Could not publish instance deleted plan sync message: pg(%d) plan(%d). Cause: %+v",
						pgID, recoveryPlan.Id, err)
				} else {
					logger.Infof("[appendInstanceRecoveryPlans] Publish all plan sync: pg(%d) plan(%d).",
						pgID, recoveryPlan.Id)
				}
			}
			return err
		}

		if i.RecoveryClusterAvailabilityZoneID != nil && *i.RecoveryClusterAvailabilityZoneID != 0 {
			if instance.RecoveryClusterAvailabilityZone, err = cluster.GetClusterAvailabilityZone(
				ctx, recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterAvailabilityZoneID,
			); err != nil {
				logger.Warnf("[appendInstanceRecoveryPlans] Could not get protection cluster(%d) availability zone(%d). Cause: %+v",
					recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterAvailabilityZoneID, err)
				if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
					if err := internal.PublishMessage(centerConstant.QueueNoticeClusterAvailabilityZoneDeleted, &queue.DeleteClusterAvailabilityZone{
						Cluster:              &cmsModel.Cluster{ID: recoveryPlan.ProtectionCluster.Id},
						OrigAvailabilityZone: &cmsModel.ClusterAvailabilityZone{ID: *i.RecoveryClusterAvailabilityZoneID},
					}); err != nil {
						logger.Warnf("[appendInstanceRecoveryPlans] Could not publish availability zone deleted plan sync message: cluster(%d) availability zone(%d). Cause: %+v",
							recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterAvailabilityZoneID, err)
					} else {
						logger.Infof("[appendInstanceRecoveryPlans] Publish availability zone deleted plan sync: cluster(%d) availability zone(%d).",
							recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterAvailabilityZoneID)
					}
				}
				return err
			}
		}

		if i.RecoveryClusterHypervisorID != nil && *i.RecoveryClusterHypervisorID != 0 {
			if instance.RecoveryClusterHypervisor, err = cluster.GetClusterHypervisor(
				ctx, recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterHypervisorID,
			); err != nil {
				logger.Warnf("[appendInstanceRecoveryPlans] Could not get protection cluster(%d) hypervisor(%d). Cause: %+v",
					recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterHypervisorID, err)
				if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
					if err := internal.PublishMessage(centerConstant.QueueNoticeClusterHypervisorDeleted, &queue.DeleteClusterHypervisor{
						Cluster:        &cmsModel.Cluster{ID: recoveryPlan.ProtectionCluster.Id},
						OrigHypervisor: &cmsModel.ClusterHypervisor{ID: *i.RecoveryClusterHypervisorID},
					}); err != nil {
						logger.Warnf("[appendInstanceRecoveryPlans] Could not publish hypervisor deleted plan sync message: cluster(%d) hypervisor(%d). Cause: %+v",
							recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterHypervisorID, err)
					} else {
						logger.Infof("[appendInstanceRecoveryPlans] Publish hypervisor deleted plan sync: cluster(%d) hypervisor(%d).",
							recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterHypervisorID)
					}
				}
				return err
			}
		}

		var dependencies []uint64
		if err = database.Execute(func(db *gorm.DB) error {
			if dependencies, err = i.GetDependProtectionInstanceIDs(db); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, d := range dependencies {
			instance.Dependencies = append(instance.Dependencies, &cms.ClusterInstance{Id: d})
		}
	}

	if err := appendExtraRemoteSecurityGroups(ctx, recoveryPlan); err != nil {
		return err
	}

	return nil
}

func appendVolumeRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, volumes []model.PlanVolume) (err error) {
	for _, v := range volumes {
		var volume drms.VolumeRecoveryPlan
		recoveryPlan.Detail.Volumes = append(recoveryPlan.Detail.Volumes, &volume)

		if err = volume.SetFromModel(&v); err != nil {
			return err
		}

		if volume.ProtectionClusterVolume, err = cluster.GetClusterVolume(
			ctx, recoveryPlan.ProtectionCluster.Id, v.ProtectionClusterVolumeID,
		); err != nil {
			return err
		}

		if v.RecoveryClusterStorageID == nil || *v.RecoveryClusterStorageID == 0 {
			continue
		}

		if volume.RecoveryClusterStorage, err = cluster.GetClusterStorage(
			ctx, recoveryPlan.RecoveryCluster.Id, *v.RecoveryClusterStorageID,
		); err != nil {
			logger.Warnf("[appendVolumeRecoveryPlans] Could not get protection cluster(%d) storage(%d). Cause: %+v",
				recoveryPlan.RecoveryCluster.Id, *v.RecoveryClusterStorageID, err)
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				if err := internal.PublishMessage(centerConstant.QueueNoticeClusterStorageDeleted, &queue.DeleteClusterStorage{
					Cluster:     &cmsModel.Cluster{ID: recoveryPlan.ProtectionCluster.Id},
					OrigStorage: &cmsModel.ClusterStorage{ID: *v.RecoveryClusterStorageID},
				}); err != nil {
					logger.Warnf("[appendVolumeRecoveryPlans] Could not publish storage deleted plan sync message: cluster(%d) storage(%d). Cause: %+v",
						recoveryPlan.RecoveryCluster.Id, *v.RecoveryClusterStorageID, err)
				} else {
					logger.Infof("[appendVolumeRecoveryPlans] Publish storage deleted plan sync: cluster(%d) storage(%d).",
						recoveryPlan.RecoveryCluster.Id, *v.RecoveryClusterStorageID)
				}
			}
			return err
		}
	}
	return nil
}

func appendFloatingIPRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, floatingIPs []model.PlanInstanceFloatingIP) (err error) {
	for _, f := range floatingIPs {
		var floatingIP drms.FloatingIPRecoveryPlan
		recoveryPlan.Detail.FloatingIps = append(recoveryPlan.Detail.FloatingIps, &floatingIP)

		if err = floatingIP.SetFromModel(&f); err != nil {
			return err
		}

		if floatingIP.ProtectionClusterFloatingIp, err = cluster.GetClusterFloatingIP(
			ctx, recoveryPlan.ProtectionCluster.Id, f.ProtectionClusterInstanceFloatingIPID,
		); err != nil && errors.GetIPCStatusCode(err) != errors.IPCStatusNotFound {
			return err
		}

	}
	return nil
}

// appendAvailabilityZoneRecoveryPlansWithDeletedInfo plan 동기화를 위해 삭제된 항목 포함
func appendAvailabilityZoneRecoveryPlansWithDeletedInfo(ctx context.Context, recoveryPlan *drms.RecoveryPlan, zones []model.PlanAvailabilityZone) (err error) {
	for _, z := range zones {
		var zone drms.AvailabilityZoneRecoveryPlan
		recoveryPlan.Detail.AvailabilityZones = append(recoveryPlan.Detail.AvailabilityZones, &zone)

		if err = zone.SetFromModel(&z); err != nil {
			return err
		}

		zone.ProtectionClusterAvailabilityZone, err = cluster.GetClusterAvailabilityZone(ctx, recoveryPlan.ProtectionCluster.Id, z.ProtectionClusterAvailabilityZoneID)
		// 삭제된 availability zone 에 대한 plan 동기화를 위한 처리
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			logger.Warnf("[appendAvailabilityZoneRecoveryPlansWithDeletedInfo] Deleted protection cluster availability zone(%d).", z.ProtectionClusterAvailabilityZoneID)
			zone.ProtectionClusterAvailabilityZone = &cms.ClusterAvailabilityZone{Id: z.ProtectionClusterAvailabilityZoneID}
		} else if err != nil {
			return err
		}

		if z.RecoveryClusterAvailabilityZoneID == nil || *z.RecoveryClusterAvailabilityZoneID == 0 {
			continue
		}

		zone.RecoveryClusterAvailabilityZone, err = cluster.GetClusterAvailabilityZone(ctx, recoveryPlan.RecoveryCluster.Id, *z.RecoveryClusterAvailabilityZoneID)
		// 삭제된 availability zone 에 대한 plan 동기화를 위한 처리
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			logger.Warnf("[appendAvailabilityZoneRecoveryPlansWithDeletedInfo] Deleted recovery cluster availability zone(%d).", *z.RecoveryClusterAvailabilityZoneID)
			zone.RecoveryClusterAvailabilityZone = &cms.ClusterAvailabilityZone{Id: *z.RecoveryClusterAvailabilityZoneID}
		} else if err != nil {
			return err
		}

	}
	return nil
}

// appendInstanceRecoveryPlansWithDeletedInfo plan 동기화를 위해 삭제된 항목 포함
func appendInstanceRecoveryPlansWithDeletedInfo(ctx context.Context, recoveryPlan *drms.RecoveryPlan, instances []model.PlanInstance) (err error) {
	for _, i := range instances {
		var instance drms.InstanceRecoveryPlan
		recoveryPlan.Detail.Instances = append(recoveryPlan.Detail.Instances, &instance)

		if err = instance.SetFromModel(&i); err != nil {
			return err
		}

		instance.ProtectionClusterInstance, err = cluster.GetClusterInstance(ctx, recoveryPlan.ProtectionCluster.Id, i.ProtectionClusterInstanceID)
		// 삭제된 instance 에 대한 plan 동기화를 위한 처리
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			logger.Warnf("[appendInstanceRecoveryPlansWithDeletedInfo] Deleted protection cluster instance(%d).", i.ProtectionClusterInstanceID)
			instance.ProtectionClusterInstance = &cms.ClusterInstance{Id: i.ProtectionClusterInstanceID}
		} else if err != nil {
			return err
		}

		if i.RecoveryClusterAvailabilityZoneID != nil && *i.RecoveryClusterAvailabilityZoneID != 0 {
			instance.RecoveryClusterAvailabilityZone, err = cluster.GetClusterAvailabilityZone(ctx, recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterAvailabilityZoneID)
			// 삭제된 az에 대한 plan 동기화를 위한 처리
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				logger.Warnf("[appendInstanceRecoveryPlansWithDeletedInfo] Deleted recovery cluster availability zone(%d).", *i.RecoveryClusterAvailabilityZoneID)
				instance.RecoveryClusterAvailabilityZone = &cms.ClusterAvailabilityZone{Id: *i.RecoveryClusterAvailabilityZoneID}
			} else if err != nil {
				return err
			}
		}

		if i.RecoveryClusterHypervisorID != nil && *i.RecoveryClusterHypervisorID != 0 {
			instance.RecoveryClusterHypervisor, err = cluster.GetClusterHypervisor(ctx, recoveryPlan.RecoveryCluster.Id, *i.RecoveryClusterHypervisorID)
			// 삭제된 hypervisor 에 대한 plan 동기화를 위한 처리
			if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
				logger.Warnf("[appendInstanceRecoveryPlansWithDeletedInfo] Deleted recovery cluster hypervisor(%d).", *i.RecoveryClusterHypervisorID)
				instance.RecoveryClusterHypervisor = &cms.ClusterHypervisor{Id: *i.RecoveryClusterHypervisorID}
			} else if err != nil {
				return err
			}
		}

		var dependencies []uint64
		if err = database.Execute(func(db *gorm.DB) error {
			if dependencies, err = i.GetDependProtectionInstanceIDs(db); err != nil {
				return errors.UnusableDatabase(err)
			}
			return nil
		}); err != nil {
			return err
		}

		for _, d := range dependencies {
			instance.Dependencies = append(instance.Dependencies, &cms.ClusterInstance{Id: d})
		}
	}

	return nil
}

// getVolumeRecoveryPlans mirror volume 정보 조회를 위한 volume plan 조회
func getVolumeRecoveryPlans(ctx context.Context, recoveryPlan *drms.RecoveryPlan, volumes []model.PlanVolume) (err error) {
	for _, v := range volumes {
		var volume drms.VolumeRecoveryPlan
		recoveryPlan.Detail.Volumes = append(recoveryPlan.Detail.Volumes, &volume)

		if err = volume.SetFromModel(&v); err != nil {
			return err
		}

		volume.ProtectionClusterVolume, err = cluster.GetClusterVolume(ctx, recoveryPlan.ProtectionCluster.Id, v.ProtectionClusterVolumeID)
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			logger.Warnf("[getVolumeRecoveryPlans] Deleted protection cluster volume(%d).", v.ProtectionClusterVolumeID)
		} else if err != nil {
			return err
		}

		if v.RecoveryClusterStorageID == nil || *v.RecoveryClusterStorageID == 0 {
			continue
		}

		volume.RecoveryClusterStorage, err = cluster.GetClusterStorage(ctx, recoveryPlan.RecoveryCluster.Id, *v.RecoveryClusterStorageID)
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			logger.Warnf("[getVolumeRecoveryPlans] Deleted recovery cluster storage(%d).", *v.RecoveryClusterStorageID)
		} else if err != nil {
			return err
		}
	}
	return nil
}

func getPlanList(filters ...recoveryPlanFilter) ([]model.Plan, error) {
	var err error
	var plans []model.Plan

	if err = database.Execute(func(db *gorm.DB) error {
		cond := db
		for _, f := range filters {
			if cond, err = f.Apply(cond); err != nil {
				return err
			}
		}
		return cond.Find(&plans).Error
	}); err != nil {
		return nil, err
	}

	return plans, nil
}

func getPlanListPagination(filters ...recoveryPlanFilter) (*drms.Pagination, error) {
	var err error
	var offset, limit uint64
	var total uint64

	if err = database.Execute(func(db *gorm.DB) error {
		cond := db
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

		return cond.Model(&model.Plan{}).Count(&total).Error
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

func mergeTenantRecoveryPlan(db *gorm.DB, pid uint64, tenants []*drms.TenantRecoveryPlan) (err error) {
	for _, t := range tenants {
		var tenant *model.PlanTenant
		if tenant, err = t.Model(); err != nil {
			return err
		}

		tenant.RecoveryPlanID = pid

		if err = db.Save(tenant).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if tenant.RecoveryClusterTenantID == nil || *tenant.RecoveryClusterTenantID == 0 {
			cols["recovery_cluster_tenant_id"] = nil
		}
		if tenant.RecoveryClusterTenantMirrorName == nil || *tenant.RecoveryClusterTenantMirrorName == "" {
			cols["recovery_cluster_tenant_mirror_name"] = nil
		}
		if tenant.RecoveryClusterTenantMirrorNameUpdateFlag == nil {
			cols["recovery_cluster_tenant_mirror_name_update_flag"] = nil
		}
		if tenant.RecoveryClusterTenantMirrorNameUpdateReasonCode == nil || *tenant.RecoveryClusterTenantMirrorNameUpdateReasonCode == "" {
			cols["recovery_cluster_tenant_mirror_name_update_reason_code"] = nil
		}
		if tenant.RecoveryClusterTenantMirrorNameUpdateReasonContents == nil || *tenant.RecoveryClusterTenantMirrorNameUpdateReasonContents == "" {
			cols["recovery_cluster_tenant_mirror_name_update_reason_contents"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&tenant).Where(&tenant).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}
	}
	return nil
}

func mergeAvailabilityZoneRecoveryPlan(db *gorm.DB, pid uint64, zones []*drms.AvailabilityZoneRecoveryPlan) (err error) {
	for _, z := range zones {
		var zone *model.PlanAvailabilityZone
		if zone, err = z.Model(); err != nil {
			return err
		}

		zone.RecoveryPlanID = pid

		if err = db.Save(zone).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if zone.RecoveryClusterAvailabilityZoneID == nil || *zone.RecoveryClusterAvailabilityZoneID == 0 {
			cols["recovery_cluster_availability_zone_id"] = nil
		}
		if zone.RecoveryClusterAvailabilityZoneUpdateFlag == nil {
			cols["recovery_cluster_availability_zone_update_flag"] = nil
		}
		if zone.RecoveryClusterAvailabilityZoneUpdateReasonCode == nil || *zone.RecoveryClusterAvailabilityZoneUpdateReasonCode == "" {
			cols["recovery_cluster_availability_zone_update_reason_code"] = nil
		}
		if zone.RecoveryClusterAvailabilityZoneUpdateReasonContents == nil || *zone.RecoveryClusterAvailabilityZoneUpdateReasonContents == "" {
			cols["recovery_cluster_availability_zone_update_reason_contents"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&zone).Where(&zone).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}
	}
	return nil
}

func mergeExternalNetworkRecoveryPlan(db *gorm.DB, pid uint64, networks []*drms.ExternalNetworkRecoveryPlan) (err error) {
	for _, n := range networks {
		var network *model.PlanExternalNetwork
		if network, err = n.Model(); err != nil {
			return err
		}

		network.RecoveryPlanID = pid

		if err = db.Save(network).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if network.RecoveryClusterExternalNetworkID == nil || *network.RecoveryClusterExternalNetworkID == 0 {
			cols["recovery_cluster_external_network_id"] = nil
		}
		if network.RecoveryClusterExternalNetworkUpdateFlag == nil {
			cols["recovery_cluster_external_network_update_flag"] = nil
		}
		if network.RecoveryClusterExternalNetworkUpdateReasonCode == nil || *network.RecoveryClusterExternalNetworkUpdateReasonCode == "" {
			cols["recovery_cluster_external_network_update_reason_code"] = nil
		}
		if network.RecoveryClusterExternalNetworkUpdateReasonContents == nil || *network.RecoveryClusterExternalNetworkUpdateReasonContents == "" {
			cols["recovery_cluster_external_network_update_reason_contents"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&network).Where(&network).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}
	}
	return nil
}

func mergeRouterRecoveryPlan(db *gorm.DB, pid uint64, routers []*drms.RouterRecoveryPlan) (err error) {
	for _, r := range routers {
		var router *model.PlanRouter
		if router, err = r.Model(); err != nil {
			return err
		}

		router.RecoveryPlanID = pid

		if err = db.Save(router).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if router.RecoveryClusterExternalNetworkID == nil || *router.RecoveryClusterExternalNetworkID == 0 {
			cols["recovery_cluster_external_network_id"] = nil
		}
		if router.RecoveryClusterExternalNetworkUpdateFlag == nil {
			cols["recovery_cluster_external_network_update_flag"] = nil
		}
		if router.RecoveryClusterExternalNetworkUpdateReasonCode == nil || *router.RecoveryClusterExternalNetworkUpdateReasonCode == "" {
			cols["recovery_cluster_external_network_update_reason_code"] = nil
		}
		if router.RecoveryClusterExternalNetworkUpdateReasonContents == nil || *router.RecoveryClusterExternalNetworkUpdateReasonContents == "" {
			cols["recovery_cluster_external_network_update_reason_contents"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&router).Where(&router).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}

		for _, i := range r.RecoveryClusterExternalRoutingInterfaces {
			var iface = model.PlanExternalRoutingInterface{
				RecoveryPlanID:                  pid,
				ProtectionClusterRouterID:       router.ProtectionClusterRouterID,
				RecoveryClusterExternalSubnetID: i.Subnet.Id,
				IPAddress:                       &i.IpAddress,
			}

			if err = db.Save(&iface).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			if i.IpAddress == "" {
				if err = db.Model(&iface).Where(&iface).Updates(map[string]interface{}{
					"ip_address": nil,
				}).Error; err != nil {
					return errors.UnusableDatabase(err)
				}
			}
		}
	}
	return nil
}

func mergeStorageRecoveryPlan(db *gorm.DB, pid uint64, storages []*drms.StorageRecoveryPlan) (err error) {
	for _, s := range storages {
		var storage *model.PlanStorage
		if storage, err = s.Model(); err != nil {
			return err
		}

		storage.RecoveryPlanID = pid

		if err = db.Save(storage).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if storage.RecoveryClusterStorageID == nil || *storage.RecoveryClusterStorageID == 0 {
			cols["recovery_cluster_storage_id"] = nil
		}
		if storage.RecoveryClusterStorageUpdateFlag == nil {
			cols["recovery_cluster_storage_update_flag"] = nil
		}
		if storage.RecoveryClusterStorageUpdateReasonCode == nil || *storage.RecoveryClusterStorageUpdateReasonCode == "" {
			cols["recovery_cluster_storage_update_reason_code"] = nil
		}
		if storage.RecoveryClusterStorageUpdateReasonContents == nil || *storage.RecoveryClusterStorageUpdateReasonContents == "" {
			cols["recovery_cluster_storage_update_reason_contents"] = nil
		}
		if storage.UnavailableFlag == nil {
			cols["unavailable_flag"] = nil
		}
		if storage.UnavailableReasonCode == nil || *storage.UnavailableReasonCode == "" {
			cols["unavailable_reason_code"] = nil
		}
		if storage.UnavailableReasonContents == nil || *storage.UnavailableReasonContents == "" {
			cols["unavailable_reason_contents"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&storage).Where(&storage).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}
	}
	return nil
}

func mergeInstanceRecoveryPlan(db *gorm.DB, pid uint64, instances []*drms.InstanceRecoveryPlan) (err error) {
	for _, i := range instances {
		var instance *model.PlanInstance
		if instance, err = i.Model(); err != nil {
			return err
		}

		instance.RecoveryPlanID = pid

		if err = db.Save(instance).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if instance.RecoveryClusterAvailabilityZoneID == nil || *instance.RecoveryClusterAvailabilityZoneID == 0 {
			cols["recovery_cluster_availability_zone_id"] = nil
		}
		if instance.RecoveryClusterAvailabilityZoneUpdateFlag == nil {
			cols["recovery_cluster_availability_zone_update_flag"] = nil
		}
		if instance.RecoveryClusterAvailabilityZoneUpdateReasonCode == nil || *instance.RecoveryClusterAvailabilityZoneUpdateReasonCode == "" {
			cols["recovery_cluster_availability_zone_update_reason_code"] = nil
		}
		if instance.RecoveryClusterAvailabilityZoneUpdateReasonContents == nil || *instance.RecoveryClusterAvailabilityZoneUpdateReasonContents == "" {
			cols["recovery_cluster_availability_zone_update_reason_contents"] = nil
		}
		if instance.RecoveryClusterHypervisorID == nil || *instance.RecoveryClusterHypervisorID == 0 {
			cols["recovery_cluster_hypervisor_id"] = nil
		}
		if instance.AutoStartFlag == nil {
			cols["auto_start_flag"] = nil
		}
		if instance.DiagnosisFlag == nil {
			cols["diagnosis_flag"] = nil
		}
		if instance.DiagnosisMethodCode == nil || *instance.DiagnosisMethodCode == "" {
			cols["diagnosis_method_code"] = nil
		}
		if instance.DiagnosisMethodData == nil || *instance.DiagnosisMethodData == "" {
			cols["diagnosis_method_data"] = nil
		}
		if instance.DiagnosisTimeout == nil || *instance.DiagnosisTimeout == 0 {
			cols["diagnosis_timeout"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&instance).Where(&instance).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}

		// 기존 instance dependency 관계를 제거한다.
		if err := db.Where(&model.PlanInstanceDependency{
			RecoveryPlanID:              pid,
			ProtectionClusterInstanceID: instance.ProtectionClusterInstanceID,
		}).Delete(&model.PlanInstanceDependency{}).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, d := range i.Dependencies {
			if err = db.Save(&model.PlanInstanceDependency{
				RecoveryPlanID:                    pid,
				ProtectionClusterInstanceID:       instance.ProtectionClusterInstanceID,
				DependProtectionClusterInstanceID: d.Id,
			}).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}

		var instanceModel = cmsModel.ClusterInstance{ID: instance.ProtectionClusterInstanceID}
		var networkModels []cmsModel.ClusterInstanceNetwork

		if networkModels, err = instanceModel.GetInstanceNetworks(db); err != nil {
			return errors.UnusableDatabase(err)
		}

		for _, n := range networkModels {
			if n.ClusterFloatingIPID == nil || *n.ClusterFloatingIPID == 0 {
				continue
			}

			var floatingIP = model.PlanInstanceFloatingIP{
				RecoveryPlanID:                        pid,
				ProtectionClusterInstanceID:           instance.ProtectionClusterInstanceID,
				ProtectionClusterInstanceFloatingIPID: *n.ClusterFloatingIPID,
			}

			if err = db.Save(&floatingIP).Error; err != nil {
				return errors.UnusableDatabase(err)
			}

			cols = make(map[string]interface{})

			if floatingIP.UnavailableFlag == nil {
				cols["unavailable_flag"] = nil
			}
			if floatingIP.UnavailableReasonCode == nil || *floatingIP.UnavailableReasonCode == "" {
				cols["unavailable_reason_code"] = nil
			}
			if floatingIP.UnavailableReasonContents == nil || *floatingIP.UnavailableReasonContents == "" {
				cols["unavailable_reason_contents"] = nil
			}

			if len(cols) > 0 {
				if err = db.Model(&floatingIP).Where(&floatingIP).Updates(cols).Error; err != nil {
					return errors.UnusableDatabase(err)
				}
			}
		}
	}
	return nil
}

func mergeVolumeRecoveryPlan(db *gorm.DB, pid uint64, volumes []*drms.VolumeRecoveryPlan) (err error) {
	for _, v := range volumes {
		var volume *model.PlanVolume
		if volume, err = v.Model(); err != nil {
			return err
		}

		volume.RecoveryPlanID = pid

		if err = db.Save(volume).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		var cols = make(map[string]interface{})

		if volume.RecoveryClusterStorageID == nil || *volume.RecoveryClusterStorageID == 0 {
			cols["recovery_cluster_storage_id"] = nil
		}
		if volume.RecoveryClusterStorageUpdateFlag == nil {
			cols["recovery_cluster_storage_update_flag"] = nil
		}
		if volume.RecoveryClusterStorageUpdateReasonCode == nil || *volume.RecoveryClusterStorageUpdateReasonCode == "" {
			cols["recovery_cluster_storage_update_reason_code"] = nil
		}
		if volume.RecoveryClusterStorageUpdateReasonContents == nil || *volume.RecoveryClusterStorageUpdateReasonContents == "" {
			cols["recovery_cluster_storage_update_reason_contents"] = nil
		}
		if volume.UnavailableFlag == nil {
			cols["unavailable_flag"] = nil
		}
		if volume.UnavailableReasonCode == nil || *volume.UnavailableReasonCode == "" {
			cols["unavailable_reason_code"] = nil
		}
		if volume.UnavailableReasonContents == nil || *volume.UnavailableReasonContents == "" {
			cols["unavailable_reason_contents"] = nil
		}

		if len(cols) > 0 {
			if err = db.Model(&volume).Where(&volume).Updates(cols).Error; err != nil {
				return errors.UnusableDatabase(err)
			}
		}
	}
	return nil
}

func isFlagged(flag *bool) bool {
	return flag != nil && *flag
}

func GetVolumeMirrorStateCode(volumes []*drms.VolumeRecoveryPlan) (string, []*drms.Message, error) {
	var mirroring []*drms.Message
	mirrorMap := make(map[string]int) // mirrorMap[상태코드] = 해당상태 volume 개수
	stateCode := constant.RecoveryPlanMirroringStateWarning

	// volume mirroring 상태에 따른 prepare 처리
	for _, v := range volumes {
		contents := fmt.Sprintf("SourceStorage.%d.TargetStorage.%d.SourceVolume.%d",
			v.ProtectionClusterVolume.Storage.Id, v.GetRecoveryClusterStorage().GetId(), v.ProtectionClusterVolume.Id)

		mv, err := mirror.GetVolume(v.ProtectionClusterVolume.Storage.Id, v.GetRecoveryClusterStorage().GetId(), v.ProtectionClusterVolume.Id)
		switch {
		// mirror volume 정보가 없는 경우(최초 plan 생성 시에는 아직 volume mirroring 을 실행하지 않음)
		// protection group 수정으로 instance 를 추가한 경우(volume mirroring 을 실행하지 않음)
		case errors.Equal(err, mirror.ErrNotFoundMirrorVolume):
			mirroring = append(mirroring, &drms.Message{Code: "cdm-dr.manager.get_plan_mirror_status.not_found_mirror_volume", Contents: contents})
			continue

		case err != nil:
			err = errors.UnusableStore(err)
			logger.Errorf("[GetVolumeMirrorStateCode] Could not get the volume: mirror_volume/storage.%d.%d/volume.%d. Cause: %+v",
				v.GetProtectionClusterVolume().GetStorage().GetId(), v.GetRecoveryClusterStorage().GetId(), v.GetProtectionClusterVolume().GetId(), err)
			return "", nil, err
		}

		status, err := mv.GetStatus()
		switch {
		// mirror volume status 가 없는 경우
		case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeStatus):
			mirroring = append(mirroring, &drms.Message{Code: "cdm-dr.manager.get_plan_mirror_status.not_found_mirror_volume_status", Contents: contents})
			continue

		case err != nil:
			err = errors.UnusableStore(err)
			logger.Errorf("[GetVolumeMirrorStateCode] Could not get the mirror volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
				v.ProtectionClusterVolume.Storage.Id, v.GetRecoveryClusterStorage().GetId(), v.ProtectionClusterVolume.Id, err)
			return "", nil, err
		}

		switch status.StateCode {
		// mirroring 인 경우에만 normal 상태로 봄
		case constant.MirrorVolumeStateCodeMirroring:
			mirrorMap[constant.RecoveryPlanMirroringStateMirroring]++

		// mirroring prepare 상태
		case constant.MirrorVolumeStateCodeWaiting, constant.MirrorVolumeStateCodeInitializing:
			mirrorMap[constant.RecoveryPlanMirroringStatePrepare]++
			mirroring = append(mirroring, &drms.Message{Code: status.StateCode, Contents: contents})

		case constant.MirrorVolumeStateCodePaused:
			mirrorMap[constant.RecoveryPlanMirroringStatePaused]++
			mirroring = append(mirroring, &drms.Message{Code: status.StateCode, Contents: contents})

		case constant.MirrorVolumeStateCodeStopped:
			mirrorMap[constant.RecoveryPlanMirroringStateStopped]++
			mirroring = append(mirroring, &drms.Message{Code: status.StateCode, Contents: contents})

		default:
			mirroring = append(mirroring, &drms.Message{Code: status.StateCode, Contents: contents})
		}
	}

	for state, volumeCount := range mirrorMap {
		// 모든 volume 의 상태가 같을때만 해당 상태로 표시 (stateCode 기본값은 warning)
		if len(volumes) == volumeCount {
			stateCode = state
			break
		}
	}

	return stateCode, mirroring, nil
}

// GetList 재해 복구 계획 목록을 조회 하는 함수
func GetList(ctx context.Context, req *drms.RecoveryPlanListRequest) ([]*drms.RecoveryPlan, *drms.Pagination, error) {
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryPlan-GetList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	if err = validatePlanListRequest(req); err != nil {
		logger.Errorf("[RecoveryPlan-GetList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-GetList] Errors occurred during checking accessible status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)
	var pg *model.ProtectionGroup
	if pg, err = getProtectionGroup(req.GroupId, tid); err != nil {
		logger.Errorf("[RecoveryPlan-GetList] Could not get the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, nil, err
	}

	var pc *cms.Cluster
	if pc, err = cluster.GetCluster(ctx, pg.ProtectionClusterID); err != nil {
		logger.Errorf("[RecoveryPlan-GetList] Could not get the protection cluster(%d). Cause: %+v", pg.ProtectionClusterID, err)
		return nil, nil, err
	}

	var plans []model.Plan
	var filters = makeRecoveryPlanFilters(req)

	if plans, err = getPlanList(filters...); err != nil {
		logger.Errorf("[RecoveryPlan-GetList] Could not get the plan list(pgID:%d). Cause: %+v", req.GroupId, err)
		return nil, nil, errors.UnusableDatabase(err)
	}

	var recoveryPlans []*drms.RecoveryPlan
	for _, p := range plans {
		var warning, mirroring []*drms.Message
		var recoveryPlan = drms.RecoveryPlan{
			ProtectionCluster:    pc,
			StateCode:            constant.RecoveryPlanStateNormal,
			MirrorStateCode:      constant.MirrorVolumeStateCodeMirroring,
			AbnormalStateReasons: new(drms.RecoveryPlanAbnormalStateReason),
			Detail:               new(drms.RecoveryPlanDetail),
		}

		if err = recoveryPlan.SetFromModel(&p); err != nil {
			return nil, nil, err
		}
		if recoveryPlan.RecoveryCluster, err = cluster.GetCluster(ctx, p.RecoveryClusterID); err != nil {
			logger.Errorf("[RecoveryPlan-GetList] Could not get the recovery cluster(%d). Cause: %+v", p.RecoveryClusterID, err)
			return nil, nil, err
		}

		if recoveryPlan.ProtectionCluster.StateCode == centerConstant.ClusterStateInactive {
			recoveryPlan.AbnormalStateReasons.Emergency = append(recoveryPlan.AbnormalStateReasons.Emergency,
				&drms.Message{Code: "cdm-dr.manager.get_plan_check_protection_cluster_connection.failure-ipc_failed"})
			//recoveryPlan.StateCode = constant.RecoveryPlanStateEmergency
		}

		if recoveryPlan.RecoveryCluster.StateCode == centerConstant.ClusterStateInactive {
			recoveryPlan.AbnormalStateReasons.Critical = append(recoveryPlan.AbnormalStateReasons.Critical,
				&drms.Message{Code: "cdm-dr.manager.get_plan_check_recovery_cluster_connection.failure-ipc_failed"})
			//recoveryPlan.StateCode = constant.RecoveryPlanStateCritical
		}

		var tenants []model.PlanTenant
		var zones []model.PlanAvailabilityZone
		var networks []model.PlanExternalNetwork
		var routers []model.PlanRouter
		var storages []model.PlanStorage
		var instances []model.PlanInstance
		var volumes []model.PlanVolume
		var floatingIPs []model.PlanInstanceFloatingIP

		if err = database.Execute(func(db *gorm.DB) error {
			if tenants, err = p.GetPlanTenants(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the tenant recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if zones, err = p.GetPlanAvailabilityZones(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the availability zone recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if networks, err = p.GetPlanExternalNetworks(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the external network recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if routers, err = p.GetPlanRouters(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the router recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if storages, err = p.GetPlanStorages(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the storage recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if instances, err = p.GetPlanInstances(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the instance recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if volumes, err = p.GetPlanVolumes(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the volume recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			if floatingIPs, err = p.GetPlanInstanceFloatingIPs(db); err != nil {
				logger.Errorf("[RecoveryPlan-GetList] Could not get the instance floating ip recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
				return err
			}
			return nil
		}); err != nil {
			return nil, nil, errors.UnusableDatabase(err)
		}

		// volume mirroring 상태를 확인하기 위한 detail 에 volume 정보를 가져온다.
		if err = getVolumeRecoveryPlans(ctx, &recoveryPlan, volumes); err != nil {
			logger.Errorf("[RecoveryPlan-GetList] Could not append the volume recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return nil, nil, err
		}

		for _, t := range tenants {
			if isFlagged(t.RecoveryClusterTenantMirrorNameUpdateFlag) {
				code := constant.RecoveryPlanTenantUpdateFlag
				if t.RecoveryClusterTenantMirrorNameUpdateReasonCode != nil {
					code = *t.RecoveryClusterTenantMirrorNameUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, z := range zones {
			if isFlagged(z.RecoveryClusterAvailabilityZoneUpdateFlag) {
				code := constant.RecoveryPlanAvailabilityZoneUpdateFlag
				if z.RecoveryClusterAvailabilityZoneUpdateReasonCode != nil {
					code = *z.RecoveryClusterAvailabilityZoneUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, n := range networks {
			if isFlagged(n.RecoveryClusterExternalNetworkUpdateFlag) {
				code := constant.RecoveryPlanNetworkUpdateFlag
				if n.RecoveryClusterExternalNetworkUpdateReasonCode != nil {
					code = *n.RecoveryClusterExternalNetworkUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, r := range routers {
			if isFlagged(r.RecoveryClusterExternalNetworkUpdateFlag) {
				code := constant.RecoveryPlanRouterUpdateFlag
				if r.RecoveryClusterExternalNetworkUpdateReasonCode != nil {
					code = *r.RecoveryClusterExternalNetworkUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, s := range storages {
			if isFlagged(s.RecoveryClusterStorageUpdateFlag) {
				code := constant.RecoveryPlanStorageUpdateFlag
				if s.RecoveryClusterStorageUpdateReasonCode != nil {
					code = *s.RecoveryClusterStorageUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
			if isFlagged(s.UnavailableFlag) {
				code := constant.RecoveryPlanStorageUnavailableFlag
				if s.UnavailableReasonCode != nil {
					code = *s.UnavailableReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, i := range instances {
			if isFlagged(i.RecoveryClusterAvailabilityZoneUpdateFlag) {
				code := constant.RecoveryPlanInstanceUpdateFlag
				if i.RecoveryClusterAvailabilityZoneUpdateReasonCode != nil {
					code = *i.RecoveryClusterAvailabilityZoneUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, v := range volumes {
			if isFlagged(v.RecoveryClusterStorageUpdateFlag) {
				code := constant.RecoveryPlanVolumeUpdateFlag
				if v.RecoveryClusterStorageUpdateReasonCode != nil {
					code = *v.RecoveryClusterStorageUpdateReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
			if isFlagged(v.UnavailableFlag) {
				code := constant.RecoveryPlanVolumeUnavailableFlag
				if v.UnavailableReasonCode != nil {
					code = *v.UnavailableReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}
		for _, f := range floatingIPs {
			if isFlagged(f.UnavailableFlag) {
				code := constant.RecoveryPlanFloatingIPUpdateFlag
				if f.UnavailableReasonCode != nil {
					code = *f.UnavailableReasonCode
				}
				warning = append(warning, &drms.Message{Code: code})
			}
		}

		if len(warning) > 0 {
			recoveryPlan.AbnormalStateReasons.Warning = warning
			recoveryPlan.StateCode = constant.RecoveryPlanStateWarning
			if err := internal.PublishMessage(constant.QueueSyncAllPlansByRecoveryPlan, &internal.SyncPlanMessage{
				ProtectionGroupID: pg.ID,
				RecoveryPlanID:    p.ID,
			}); err != nil {
				logger.Warnf("[RecoveryPlan-GetList] Could not publish sync all plans message: cluster(%d) protection group(%d) recovery plan(%d). Cause: %+v",
					pg.ProtectionClusterID, pg.ID, p.ID, err)
			} else {
				logger.Infof("[RecoveryPlan-GetList] Publish sync all plans: cluster(%d) protection group(%d) recovery plan(%d).",
					pg.ProtectionClusterID, pg.ID, p.ID)
			}
		}

		recoveryPlan.MirrorStateCode, mirroring, err = GetVolumeMirrorStateCode(recoveryPlan.Detail.Volumes)
		if err != nil {
			return nil, nil, err
		}

		if len(mirroring) > 0 {
			recoveryPlan.AbnormalStateReasons.Mirroring = mirroring
		}

		if err = isRecoveryJobExists(pg.ID, p.ID); err == nil {
			recoveryPlan.Updatable = true
		} else if err != nil && !errors.Equal(err, internal.ErrRecoveryJobExisted) {
			logger.Errorf("[RecoveryPlan-GetList] Errors occurred during checking existence of the recovery job(pgID:%d,pID:%d). Cause: %+v", pg.ID, p.ID, err)
			return nil, nil, err
		}

		recoveryPlans = append(recoveryPlans, &recoveryPlan)
	}

	pagination, err := getPlanListPagination(filters...)
	if err != nil {
		logger.Errorf("[RecoveryPlan-GetList] Could not get the plan list pagination(pgID:%d). Cause: %+v", pg.ID, err)
		return nil, nil, err
	}

	return recoveryPlans, pagination, nil
}

// GetSimpleInfo 재해복구계획의 간단한 정보를 조회 하는 함수
func GetSimpleInfo(ctx context.Context, req *drms.RecoveryPlanRequest) (*drms.RecoveryPlan, error) {
	var p *model.Plan
	var pg *model.ProtectionGroup
	var recoveryPlan drms.RecoveryPlan
	var err error

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

	if err = recoveryPlan.SetFromModel(p); err != nil {
		return nil, err
	}

	if recoveryPlan.ProtectionCluster, err = cluster.GetCluster(ctx, pg.ProtectionClusterID); err != nil {
		return nil, err
	}

	if recoveryPlan.RecoveryCluster, err = cluster.GetCluster(ctx, p.RecoveryClusterID); err != nil {
		return nil, err
	}

	return &recoveryPlan, nil
}

// GetSimpleInfoWithStorage 는 계획에 스토리지 정보만 포함해서 조회하는 함수이다.
// 스냅샷 생성과정에서 계획과 스토리지 정보만 필요한데 Get() 함수는 많은 리소스 조회를 포함하고 있어 별도로 분리하였다.
func GetSimpleInfoWithStorage(ctx context.Context, req *drms.RecoveryPlanRequest) (*drms.RecoveryPlan, error) {
	var p *model.Plan
	var pg *model.ProtectionGroup
	var err error

	//TODO: 필요 여부 확인해야함
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
	var recoveryPlan = drms.RecoveryPlan{
		StateCode: constant.RecoveryPlanStateNormal,
		Detail:    new(drms.RecoveryPlanDetail),
	}

	if err = recoveryPlan.SetFromModel(p); err != nil {
		return nil, err
	}

	if recoveryPlan.ProtectionCluster, err = cluster.GetCluster(ctx, pg.ProtectionClusterID); err != nil {
		logger.Errorf("[RecoveryPlan-GetSimpleInfoWithStorage] Could not get the protection cluster(%d). Cause: %+v", pg.ProtectionClusterID, err)
		return nil, err
	}
	if recoveryPlan.RecoveryCluster, err = cluster.GetCluster(ctx, p.RecoveryClusterID); err != nil {
		logger.Errorf("[RecoveryPlan-GetSimpleInfoWithStorage] Could not get the recovery cluster(%d). Cause: %+v", p.RecoveryClusterID, err)
		return nil, err
	}

	var storages []model.PlanStorage
	if err = database.Execute(func(db *gorm.DB) error {
		storages, err = p.GetPlanStorages(db)
		if err != nil {
			logger.Errorf("[RecoveryPlan-GetSimpleInfoWithStorage] Could not get the storage recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	if err = appendStorageRecoveryPlans(ctx, &recoveryPlan, storages); err != nil {
		logger.Errorf("[RecoveryPlan-GetSimpleInfoWithStorage] Could not append the storage recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}

	return &recoveryPlan, nil
}

// Get 재해 복구 계획 상세정보를 조회 하는 함수
func Get(ctx context.Context, req *drms.RecoveryPlanRequest) (*drms.RecoveryPlan, error) {
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryPlan-Get] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetPlanId() == 0 {
		err = errors.RequiredParameter("plan_id")
		logger.Errorf("[RecoveryPlan-Get] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Errors occurred during checking accessible status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	tid, _ := metadata.GetTenantID(ctx)

	var pg *model.ProtectionGroup
	if pg, err = getProtectionGroup(req.GroupId, tid); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not get the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	p, err := getPlan(req.GroupId, req.PlanId)
	if err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not get the recovery plan(%d). Cause: %+v", req.PlanId, err)
		return nil, err
	}

	var warning, mirroring []*drms.Message
	var recoveryPlan = drms.RecoveryPlan{
		StateCode:            constant.RecoveryPlanStateNormal,
		MirrorStateCode:      constant.MirrorVolumeStateCodeMirroring,
		AbnormalStateReasons: new(drms.RecoveryPlanAbnormalStateReason),
		Detail:               new(drms.RecoveryPlanDetail),
	}

	if err = recoveryPlan.SetFromModel(p); err != nil {
		return nil, err
	}
	if recoveryPlan.ProtectionCluster, err = cluster.GetCluster(ctx, pg.ProtectionClusterID); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not get the protection cluster(%d). Cause: %+v", pg.ProtectionClusterID, err)
		return nil, err
	}
	if recoveryPlan.RecoveryCluster, err = cluster.GetCluster(ctx, p.RecoveryClusterID); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not get the recovery cluster(%d). Cause: %+v", p.RecoveryClusterID, err)
		return nil, err
	}

	if recoveryPlan.ProtectionCluster.StateCode == centerConstant.ClusterStateInactive {
		recoveryPlan.AbnormalStateReasons.Emergency = append(recoveryPlan.AbnormalStateReasons.Emergency,
			&drms.Message{Code: "cdm-dr.manager.check_protection_cluster_connection.failure-ipc_failed"})
		// recoveryPlan.StateCode = constant.RecoveryPlanStateEmergency
	}

	if recoveryPlan.RecoveryCluster.StateCode == centerConstant.ClusterStateInactive {
		recoveryPlan.AbnormalStateReasons.Critical = append(recoveryPlan.AbnormalStateReasons.Critical,
			&drms.Message{Code: "cdm-dr.manager.check_recovery_cluster_connection.failure-ipc_failed"})
		// recoveryPlan.StateCode = constant.RecoveryPlanStateCritical
	}

	var tenants []model.PlanTenant
	var zones []model.PlanAvailabilityZone
	var networks []model.PlanExternalNetwork
	var routers []model.PlanRouter
	var storages []model.PlanStorage
	var instances []model.PlanInstance
	var volumes []model.PlanVolume
	var floatingIPs []model.PlanInstanceFloatingIP

	if err = database.Execute(func(db *gorm.DB) error {
		if tenants, err = p.GetPlanTenants(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the tenant recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if zones, err = p.GetPlanAvailabilityZones(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the availability zone recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if networks, err = p.GetPlanExternalNetworks(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the external network recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if routers, err = p.GetPlanRouters(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the router recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if storages, err = p.GetPlanStorages(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the storage recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if instances, err = p.GetPlanInstances(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the instance recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if volumes, err = p.GetPlanVolumes(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the volume recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if floatingIPs, err = p.GetPlanInstanceFloatingIPs(db); err != nil {
			logger.Errorf("[RecoveryPlan-Get] Could not get the instance floating ip recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		return nil
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	if err = appendTenantRecoveryPlans(ctx, &recoveryPlan, tenants); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the tenant recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendAvailabilityZoneRecoveryPlans(ctx, &recoveryPlan, zones); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the availability zone recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendExternalNetworkRecoveryPlans(ctx, &recoveryPlan, networks); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the external network recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendRouterRecoveryPlans(ctx, &recoveryPlan, routers); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the router recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendStorageRecoveryPlans(ctx, &recoveryPlan, storages); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the storage recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendInstanceRecoveryPlans(ctx, req.GroupId, &recoveryPlan, instances); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the instance recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendVolumeRecoveryPlans(ctx, &recoveryPlan, volumes); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the volume recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}
	if err = appendFloatingIPRecoveryPlans(ctx, &recoveryPlan, floatingIPs); err != nil {
		logger.Errorf("[RecoveryPlan-Get] Could not append the instance floating ip recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
		return nil, err
	}

	for _, t := range tenants {
		if isFlagged(t.RecoveryClusterTenantMirrorNameUpdateFlag) {
			code := constant.RecoveryPlanTenantUpdateFlag
			if t.RecoveryClusterTenantMirrorNameUpdateReasonCode != nil {
				code = *t.RecoveryClusterTenantMirrorNameUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, z := range zones {
		if isFlagged(z.RecoveryClusterAvailabilityZoneUpdateFlag) {
			code := constant.RecoveryPlanAvailabilityZoneUpdateFlag
			if z.RecoveryClusterAvailabilityZoneUpdateReasonCode != nil {
				code = *z.RecoveryClusterAvailabilityZoneUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, n := range networks {
		if isFlagged(n.RecoveryClusterExternalNetworkUpdateFlag) {
			code := constant.RecoveryPlanNetworkUpdateFlag
			if n.RecoveryClusterExternalNetworkUpdateReasonCode != nil {
				code = *n.RecoveryClusterExternalNetworkUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, r := range routers {
		if isFlagged(r.RecoveryClusterExternalNetworkUpdateFlag) {
			code := constant.RecoveryPlanRouterUpdateFlag
			if r.RecoveryClusterExternalNetworkUpdateReasonCode != nil {
				code = *r.RecoveryClusterExternalNetworkUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, s := range storages {
		if isFlagged(s.RecoveryClusterStorageUpdateFlag) {
			code := constant.RecoveryPlanStorageUpdateFlag
			if s.RecoveryClusterStorageUpdateReasonCode != nil {
				code = *s.RecoveryClusterStorageUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
		if isFlagged(s.UnavailableFlag) {
			code := constant.RecoveryPlanStorageUnavailableFlag
			if s.UnavailableReasonCode != nil {
				code = *s.UnavailableReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, i := range instances {
		if isFlagged(i.RecoveryClusterAvailabilityZoneUpdateFlag) {
			code := constant.RecoveryPlanInstanceUpdateFlag
			if i.RecoveryClusterAvailabilityZoneUpdateReasonCode != nil {
				code = *i.RecoveryClusterAvailabilityZoneUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, v := range volumes {
		if isFlagged(v.RecoveryClusterStorageUpdateFlag) {
			code := constant.RecoveryPlanVolumeUpdateFlag
			if v.RecoveryClusterStorageUpdateReasonCode != nil {
				code = *v.RecoveryClusterStorageUpdateReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
		if isFlagged(v.UnavailableFlag) {
			code := constant.RecoveryPlanVolumeUnavailableFlag
			if v.UnavailableReasonCode != nil {
				code = *v.UnavailableReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}
	for _, f := range floatingIPs {
		if isFlagged(f.UnavailableFlag) {
			code := constant.RecoveryPlanFloatingIPUpdateFlag
			if f.UnavailableReasonCode != nil {
				code = *f.UnavailableReasonCode
			}
			warning = append(warning, &drms.Message{Code: code})
		}
	}

	if len(warning) > 0 {
		recoveryPlan.AbnormalStateReasons.Warning = warning
		recoveryPlan.StateCode = constant.RecoveryPlanStateWarning
		if err := internal.PublishMessage(constant.QueueSyncAllPlansByRecoveryPlan, &internal.SyncPlanMessage{
			ProtectionGroupID: pg.ID,
			RecoveryPlanID:    p.ID,
		}); err != nil {
			logger.Warnf("[RecoveryPlan-Get] Could not publish sync all plans message: cluster(%d) protection group(%d) recovery plan(%d). Cause: %+v",
				pg.ProtectionClusterID, pg.ID, p.ID, err)
		} else {
			logger.Infof("[RecoveryPlan-Get] Publish sync all plans: cluster(%d) protection group(%d) recovery plan(%d).",
				pg.ProtectionClusterID, pg.ID, p.ID)
		}
	}

	recoveryPlan.MirrorStateCode, mirroring, err = GetVolumeMirrorStateCode(recoveryPlan.Detail.Volumes)
	if err != nil {
		return nil, err
	}

	if len(mirroring) > 0 {
		recoveryPlan.AbnormalStateReasons.Mirroring = mirroring
	}

	if err = isRecoveryJobExists(pg.ID, p.ID); err == nil {
		recoveryPlan.Updatable = true
	} else if err != nil && !errors.Equal(err, internal.ErrRecoveryJobExisted) {
		return nil, err
	}

	return &recoveryPlan, nil
}

// 복제환경을 구성한다.
func putMirrorEnvironment(source, target *cmsStorage.ClusterStorage) error {
	timeout := time.After(30 * time.Second)
	for {
		// 복제환경이 이미 구성되어있는지 확인한다.
		env, err := mirror.GetEnvironment(source.StorageID, target.StorageID)
		if errors.Equal(err, mirror.ErrNotFoundMirrorEnvironment) {
			break
		} else if err != nil {
			logger.Errorf("[putMirrorEnvironment] Could not get the mirror environment: mirror_environment/storage.%d.%d. Cause: %+v",
				source.StorageID, target.StorageID, err)
			return err
		}

		// 복제환경의 구성을 제거하고 있지는 않은지 확인한다.
		if op, err := env.GetOperation(); err != nil {
			logger.Errorf("[putMirrorEnvironment] Could not get the mirror environment: mirror_environment/storage.%d.%d/operation. Cause: %+v",
				source.StorageID, target.StorageID, err)
			return err
		} else if op.Operation == constant.MirrorEnvironmentOperationStart {
			return nil
		}

		// 기존의 복제환경의 구성을 제거하는 중이라면 제거가 완료될 때까지 대기한다.
		// 일정시간 동안 복제환경 구성 제거가 완료되지 않으면 실패로 처리한다.
		select {
		case <-time.After(2 * time.Second):
			continue

		case <-timeout:
			err = internal.StoppingMirrorEnvironmentExisted(source.StorageID, target.StorageID)
			logger.Errorf("[putMirrorEnvironment] Timeout. Cause: %+v", err)
			return err
		}
	}

	// 복제환경 추가
	env := mirror.Environment{
		SourceClusterStorage: source,
		TargetClusterStorage: target,
	}

	if err := env.Put(); err != nil {
		logger.Errorf("[putMirrorEnvironment] Could not put environment: mirror_environment/storage.%d.%d. Cause: %+v",
			env.SourceClusterStorage.StorageID, env.TargetClusterStorage.StorageID, err)
		return err
	}

	if err := env.SetOperation(constant.MirrorEnvironmentOperationStart); err != nil {
		logger.Errorf("[putMirrorEnvironment] Could not set environment operation: mirror_environment/storage.%d.%d/operation. Cause: %+v",
			env.SourceClusterStorage.StorageID, env.TargetClusterStorage.StorageID, err)
		return err
	}

	logger.Infof("[putMirrorEnvironment] Done - mirror_volume/storage.%d.%d", source.StorageID, target.StorageID)
	return nil
}

// 재해복구계획에서 필요한 모든 복제환경들을 구성한다.
func putMirrorEnvironments(plan *drms.RecoveryPlan) error {
	// storage recovery plan
	for _, ps := range plan.Detail.Storages {
		if ps.RecoveryClusterStorage == nil || ps.RecoveryClusterStorage.Id == 0 {
			continue
		}
		if ps.RecoveryClusterStorageUpdateFlag {
			continue
		}

		if err := putMirrorEnvironment(
			&cmsStorage.ClusterStorage{ClusterID: ps.ProtectionClusterStorage.Cluster.Id, StorageID: ps.ProtectionClusterStorage.Id},
			&cmsStorage.ClusterStorage{ClusterID: ps.RecoveryClusterStorage.Cluster.Id, StorageID: ps.RecoveryClusterStorage.Id},
		); err != nil {
			return err
		}
	}

	// volume recovery plan
	for _, pv := range plan.Detail.Volumes {
		if pv.RecoveryClusterStorage == nil || pv.RecoveryClusterStorage.Id == 0 {
			continue
		}

		if err := putMirrorEnvironment(
			&cmsStorage.ClusterStorage{ClusterID: pv.ProtectionClusterVolume.Cluster.Id, StorageID: pv.ProtectionClusterVolume.Storage.Id},
			&cmsStorage.ClusterStorage{ClusterID: pv.RecoveryClusterStorage.Cluster.Id, StorageID: pv.RecoveryClusterStorage.Id},
		); err != nil {
			return err
		}
	}

	return nil
}

func startVolumeMirroring(plan *drms.RecoveryPlan, volID uint64) error {
	var instancePlan = internal.GetInstanceRecoveryPlanUsingVolume(plan, volID)
	var volumePlan = internal.GetVolumeRecoveryPlan(plan, volID)
	var storagePlan = internal.GetStorageRecoveryPlan(plan, volumePlan.ProtectionClusterVolume.Storage.Id)

	var recoveryClusterStorage *cms.ClusterStorage
	if volumePlan.RecoveryClusterStorage == nil || volumePlan.RecoveryClusterStorage.Id == 0 {
		recoveryClusterStorage = storagePlan.RecoveryClusterStorage
	} else {
		recoveryClusterStorage = volumePlan.RecoveryClusterStorage
	}

	timeout := time.After(30 * time.Second)
	for {
		// 볼륨 복제가 이미 시작되었는지 확인한다.
		vol, err := mirror.GetVolume(
			volumePlan.ProtectionClusterVolume.Storage.Id,
			recoveryClusterStorage.Id,
			volID,
		)
		if errors.Equal(err, mirror.ErrNotFoundMirrorVolume) {
			break
		} else if err != nil {
			logger.Errorf("[RecoveryPlan-startVolumeMirroring] Could not get the volume: mirror_volume/storage.%d.%d/volume.%d. Cause: %+v",
				volumePlan.ProtectionClusterVolume.Storage.Id, recoveryClusterStorage.Id, volID, err)
			return err
		}

		// 볼륨 복제를 중지하고 있지는 않은지 확인한다.
		if op, err := vol.GetOperation(); err != nil {
			logger.Errorf("[RecoveryPlan-startVolumeMirroring] Could not get the volume operation: mirror_volume/storage.%d.%d/volume.%d/operation. Cause: %+v",
				volumePlan.ProtectionClusterVolume.Storage.Id, recoveryClusterStorage.Id, volID, err)
			return err
		} else if op.Operation == constant.MirrorVolumeOperationStart ||
			op.Operation == constant.MirrorVolumeOperationPause ||
			op.Operation == constant.MirrorVolumeOperationResume {
			return nil
		}

		// 기존의 볼륨 복제를 중지하는 중이라면 중지가 완료될 때까지 대기한다.
		// 일정시간 동안 볼륨 복제가 중지되지 않으면 실패로 처리한다.
		select {
		case <-time.After(2 * time.Second):
			continue

		case <-timeout:
			err = internal.StoppingMirrorVolumeExisted(volumePlan.ProtectionClusterVolume.Storage.Id, recoveryClusterStorage.Id, volID)
			logger.Errorf("[RecoveryPlan-startVolumeMirroring] Timeout. Cause: %+v", err)
			return err
		}
	}

	mv := &mirror.Volume{
		SourceClusterStorage: &cmsStorage.ClusterStorage{
			ClusterID: plan.ProtectionCluster.Id,
			StorageID: volumePlan.ProtectionClusterVolume.Storage.Id,
		},
		TargetClusterStorage: &cmsStorage.ClusterStorage{
			ClusterID: plan.RecoveryCluster.Id,
			StorageID: recoveryClusterStorage.Id,
		},
		SourceVolume: &cmsVolume.ClusterVolume{
			ClusterID:  plan.ProtectionCluster.Id,
			VolumeID:   volumePlan.ProtectionClusterVolume.Id,
			VolumeUUID: volumePlan.ProtectionClusterVolume.Uuid,
		},
		SourceAgent: &mirror.Agent{
			IP:   instancePlan.ProtectionClusterInstance.Hypervisor.IpAddress,
			Port: uint(instancePlan.ProtectionClusterInstance.Hypervisor.AgentPort),
		},
	}

	if err := internal.StartVolumeMirroring(mv, plan.Id); err != nil {
		return err
	}

	return nil
}

func startVolumesMirroring(plan *drms.RecoveryPlan) error {
	for _, pv := range plan.Detail.Volumes {
		if err := startVolumeMirroring(plan, pv.ProtectionClusterVolume.Id); err != nil {
			return err
		}
	}

	return nil
}

// Add 재해 복구 계획을 추가하는 함수
func Add(ctx context.Context, req *drms.AddRecoveryPlanRequest) (*drms.RecoveryPlan, error) {
	logger.Infof("[RecoveryPlan-Add] Start")

	var err error
	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryPlan-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetPlan() == nil {
		err = errors.RequiredParameter("plan")
		logger.Errorf("[RecoveryPlan-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Errors occurred during checking accessible status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	var pg *drms.ProtectionGroup
	if pg, err = getProtectionGroupMessage(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Could not get the protection group(%d) message. Cause: %+v", req.GroupId, err)
		return nil, err
	}

	if err := isFailbackRecoveryPlanExist(pg); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Errors occurred during checking the failback existence status of the recovery plan. Cause: %+v", err)
		return nil, err
	}

	if err = checkValidRecoveryPlan(ctx, pg, req.Plan); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkValidAddRecoveryPlan(pg, req.Plan); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Errors occurred while validating the request of the recovery plan(%d:%s). Cause: %+v", req.Plan.Id, req.Plan.Name, err)
		return nil, err
	}

	var p *model.Plan
	if p, err = req.Plan.Model(); err != nil {
		return nil, err
	}

	p.ActivationFlag = true
	p.ProtectionGroupID = pg.Id

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Create(p).Error; err != nil {
			logger.Errorf("[RecoveryPlan-Add] Could not create the recovery plan(%s). Cause: %+v", req.Plan.Name, err)
			return errors.UnusableDatabase(err)
		}

		// FIXME: 업데이트 할 수 있는 컬럼만 업데이트해야 한다.
		if err = mergeTenantRecoveryPlan(db, p.ID, req.Plan.Detail.Tenants); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the tenant recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if err = mergeAvailabilityZoneRecoveryPlan(db, p.ID, req.Plan.Detail.AvailabilityZones); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the availability zone recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if err = mergeExternalNetworkRecoveryPlan(db, p.ID, req.Plan.Detail.ExternalNetworks); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the external network recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if err = mergeRouterRecoveryPlan(db, p.ID, req.Plan.Detail.Routers); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the router recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if err = mergeStorageRecoveryPlan(db, p.ID, req.Plan.Detail.Storages); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the storage recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if err = mergeInstanceRecoveryPlan(db, p.ID, req.Plan.Detail.Instances); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the instance recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}
		if err = mergeVolumeRecoveryPlan(db, p.ID, req.Plan.Detail.Volumes); err != nil {
			logger.Errorf("[RecoveryPlan-Add] Errors occurred during adding the volume recovery plan(%d:%s). Cause: %+v", p.ID, p.Name, err)
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	defer func() {
		if err == nil {
			return
		}
		_ = deletePlan(p.ID)
	}()

	var plan *drms.RecoveryPlan
	if plan, err = Get(ctx, &drms.RecoveryPlanRequest{GroupId: p.ProtectionGroupID, PlanId: p.ID}); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Could not get the recovery plan(%d). Cause: %+v", p.ID, err)
		return nil, err
	}

	// 재해복구계획에서 필요한 모든 복제환경들을 구성한다.
	if err = putMirrorEnvironments(plan); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Could not put mirror environments of the recovery plan(%d:%s). Cause: %+v", plan.Id, plan.Name, err)
		return nil, err
	}

	// 재해복구계획에서 필요한 모든 volume mirroring 을 시작한다.
	if err = startVolumesMirroring(plan); err != nil {
		logger.Errorf("[RecoveryPlan-Add] Could not start mirror volumes of the recovery plan(%d:%s). Cause: %+v", plan.Id, plan.Name, err)
		return nil, err
	}

	logger.Infof("[RecoveryPlan-Add] Success: plan(%d:%s)", plan.Id, plan.Name)
	return plan, nil
}

// compared 와 비교하여 standard 에만 존재하는 storage 복구 계획 목록을 가져오는 함수
func diffStoragePlanList(standard, compared []*drms.StorageRecoveryPlan) []*drms.StorageRecoveryPlan {
	var diff []*drms.StorageRecoveryPlan
	comparedMap := make(map[string]bool)

	// compared storage 복구 계획에 있는 storage mapping 정보를 map 에 저장한다.
	for _, c := range compared {
		comparedMap[fmt.Sprintf("%d/%d", c.ProtectionClusterStorage.GetId(), c.RecoveryClusterStorage.GetId())] = true
	}

	// standard storage 복구 계획 중 compared storage 복구 계획에 존재하지 않는 storage 복구 계획의 목록을 구한다.
	for _, s := range standard {
		// 새로 mapping 된 storage 를 스킵한다.
		if s.RecoveryClusterStorageUpdateFlag {
			continue
		}

		// 기존 storage 중 recovery cluster storage 가 변경된 storage 를 검사한다.
		if !comparedMap[fmt.Sprintf("%d/%d", s.ProtectionClusterStorage.GetId(), s.RecoveryClusterStorage.GetId())] {
			diff = append(diff, s)
		}
	}

	return diff
}

// compared 와 비교하여 standard 에만 존재하는 volume 복구 계획 목록을 가져오는 함수
func diffVolumePlanList(standard, compared []*drms.VolumeRecoveryPlan) []*drms.VolumeRecoveryPlan {
	var diff []*drms.VolumeRecoveryPlan
	comparedMap := make(map[string]bool)

	// compared volume 복구 계획에 있는 volume mapping 정보를 map 에 저장한다.
	for _, c := range compared {
		comparedMap[fmt.Sprintf("%d/%d", c.ProtectionClusterVolume.GetId(), c.RecoveryClusterStorage.GetId())] = true
	}

	// standard volume 복구 계획 중 compared volume 복구 계획에 존재하지 않는 volume 복구 계획의 목록을 구한다.
	for _, s := range standard {
		// 새로운 attached volume 은 스킵한다.
		if s.UnavailableFlag {
			continue
		}

		// 기존 volume 중 recovery cluster storage 가 변경된 volume 를 검사한다.
		if !comparedMap[fmt.Sprintf("%d/%d", s.ProtectionClusterVolume.GetId(), s.RecoveryClusterStorage.GetId())] {
			diff = append(diff, s)
		}
	}

	return diff
}

// Update 재해 복구 계획을 수정하는 함수
func Update(ctx context.Context, req *drms.UpdateRecoveryPlanRequest) (*drms.RecoveryPlan, error) {
	logger.Info("[RecoveryPlan-Update] Start")

	var orig *model.Plan
	var pg *drms.ProtectionGroup
	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetPlanId() == 0 {
		err = errors.RequiredParameter("plan_id")
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetPlan() == nil {
		err = errors.RequiredParameter("plan")
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetPlan().GetId() == 0 {
		err = errors.RequiredParameter("plan.id")
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if req.GetPlanId() != req.GetPlan().GetId() {
		err = errors.UnchangeableParameter("plan.id")
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during checking accessible status of the protection group(%d). Cause: %+v", req.GroupId, err)
		return nil, err
	}

	if orig, err = getPlan(req.GroupId, req.PlanId); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Errors occurred while getting the recovery plan(%d). Cause: %+v", req.PlanId, err)
		return nil, err
	}

	if pg, err = getProtectionGroupMessage(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Could not get the protection group(%d) message. Cause: %+v", req.GroupId, err)
		return nil, err
	}

	// FIXME: Add 에서는 recovery cluster object 가 nil 일 수 없으나, Update 에서는 nil 일 수 있음.
	if err = checkValidRecoveryPlan(ctx, pg, req.Plan); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkValidUpdateRecoveryPlan(pg, req.Plan); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkUpdatableRecoveryPlan(pg, orig, req.Plan); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Errors occurred during checking updatable status of the recovery plan(%d). Cause: %+v", req.PlanId, err)
		return nil, err
	}

	var recoveryPlan *drms.RecoveryPlan
	if recoveryPlan, err = Get(ctx, &drms.RecoveryPlanRequest{GroupId: req.GroupId, PlanId: req.PlanId}); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Could not get the recovery plan(%d). Cause: %+v", req.PlanId, err)
		return nil, err
	}

	origPlan := new(model.Plan)
	if err = copier.CopyWithOption(origPlan, orig, copier.Option{DeepCopy: true}); err != nil {
		return nil, errors.Unknown(err)
	}

	origDetail := new(drms.RecoveryPlanDetail)
	if err = copier.CopyWithOption(origDetail, recoveryPlan.Detail, copier.Option{DeepCopy: true}); err != nil {
		return nil, errors.Unknown(err)
	}

	var p *model.Plan
	if p, err = req.Plan.Model(); err != nil {
		return nil, err
	}

	orig.Name = p.Name
	orig.Remarks = p.Remarks

	if err = updatePlan(orig, req.Plan.Detail); err != nil {
		return nil, err
	}

	defer func() {
		if err == nil {
			return
		}
		_ = updatePlan(origPlan, origDetail)
	}()

	// 롤백을 위해 임시 저장
	var updatedPlan *drms.RecoveryPlan
	if updatedPlan, err = Get(ctx, &drms.RecoveryPlanRequest{GroupId: orig.ProtectionGroupID, PlanId: orig.ID}); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Could not get the recovery plan(%d). Cause: %+v", orig.ID, err)
		return nil, err
	}

	// 실패 시 추가된 복제환경과 볼륨을 삭제(롤백)한다.
	defer func() {
		if err == nil {
			return
		}

		// 롤백 시 추가된 볼륨의 복제를 중지하고, 복제환경을 제거한다.
		addedStoragePlanList := diffStoragePlanList(updatedPlan.Detail.Storages, recoveryPlan.Detail.Storages)
		addedVolumePlanList := diffVolumePlanList(updatedPlan.Detail.Volumes, recoveryPlan.Detail.Volumes)

		for _, pv := range addedVolumePlanList {
			mv, err := getMirrorVolume(recoveryPlan, pv.ProtectionClusterVolume.Id)
			switch {
			case errors.Equal(err, mirror.ErrNotFoundMirrorVolume):
				continue

			case err != nil:
				logger.Warnf("[RecoveryPlan-Update] Could not get volume mirroring: plan(%d) volume(%d). Cause: %v", updatedPlan.Id, pv.ProtectionClusterVolume.Id, err)
				return
			}

			if err := operateStopMirrorVolume(mv); err != nil {
				logger.Warnf("[RecoveryPlan-Update] Could not stop(rollback) volume mirroring: plan(%d) volume(%d). Cause: %+v", updatedPlan.Id, pv.ProtectionClusterVolume.Id, err)
			}
		}
		for _, pv := range addedVolumePlanList {
			if err := deleteVolumeEnvironment(pv); err != nil {
				logger.Warnf("[RecoveryPlan-Update] Could not delete mirror(rollback) environment mirroring: protection(%d) recovery(%d) storage. Cause: %+v",
					pv.ProtectionClusterVolume.Storage.Id, pv.RecoveryClusterStorage.Id, err)
			}
		}
		for _, ps := range addedStoragePlanList {
			if err := deleteStorageEnvironment(ps); err != nil {
				logger.Warnf("[RecoveryPlan-Update] Could not delete(rollback) mirror environment mirroring: protection(%d) recovery(%d) storage. Cause: %+v",
					ps.ProtectionClusterStorage.Id, ps.RecoveryClusterStorage.Id, err)
			}
		}
	}()

	// TODO: 모든 항목에 대해 동기화

	// 업데이트 된 plan 의 모든 복제 환경 추가
	if err = putMirrorEnvironments(updatedPlan); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Could not put mirror environments of the recovery plan(%d:%s). Cause: %+v", updatedPlan.Id, updatedPlan.Name, err)
		return nil, err
	}

	// 업데이트 된 plan 의 모든 복제 실행
	if err = startVolumesMirroring(updatedPlan); err != nil {
		logger.Errorf("[RecoveryPlan-Update] Could not start mirror volumes of the recovery plan(%d:%s). Cause: %+v", updatedPlan.Id, updatedPlan.Name, err)
		return nil, err
	}

	// 삭제된 볼륨의 복제를 중지하고, 복제환경을 제거한다.
	deletedStoragePlanList := diffStoragePlanList(recoveryPlan.Detail.Storages, updatedPlan.Detail.Storages)
	deletedVolumePlanList := diffVolumePlanList(recoveryPlan.Detail.Volumes, updatedPlan.Detail.Volumes)

	for _, pv := range deletedVolumePlanList {
		mv, err := getMirrorVolume(updatedPlan, pv.ProtectionClusterVolume.Id)
		switch {
		case errors.Equal(err, mirror.ErrNotFoundMirrorVolume):
			continue

		case err != nil:
			logger.Warnf("[RecoveryPlan-Update] Could not get volume mirroring: plan(%d) volume(%d). Cause: %v", recoveryPlan.Id, pv.ProtectionClusterVolume.Id, err)
			return nil, err
		}

		if err := operateStopMirrorVolume(mv); err != nil {
			logger.Warnf("[RecoveryPlan-Update] Could not stop volume mirroring: plan(%d) volume(%d). Cause: %+v", recoveryPlan.Id, pv.ProtectionClusterVolume.Id, err)
		}
	}

	for _, pv := range deletedVolumePlanList {
		if err := deleteVolumeEnvironment(pv); err != nil {
			logger.Warnf("[RecoveryPlan-Update] Could not delete mirror environment mirroring: protection(%d) recovery(%d) storage. Cause: %+v", pv.ProtectionClusterVolume.Storage.Id, pv.RecoveryClusterStorage.Id, err)
		}
	}

	for _, ps := range deletedStoragePlanList {
		if err := deleteStorageEnvironment(ps); err != nil {
			logger.Warnf("[RecoveryPlan-Update] Could not delete mirror environment mirroring: protection(%d) recovery(%d) storage. Cause: %+v", ps.ProtectionClusterStorage.Id, ps.RecoveryClusterStorage.Id, err)
		}
	}

	logger.Infof("[RecoveryPlan-Update] Success: plan(%d:%s)", updatedPlan.Id, updatedPlan.Name)
	return updatedPlan, nil
}

func deleteMirrorEnvironment(source, target *cmsStorage.ClusterStorage) error {
	env, err := mirror.GetEnvironment(source.StorageID, target.StorageID)
	if errors.Equal(err, mirror.ErrNotFoundMirrorEnvironment) {
		return nil
	} else if err != nil {
		logger.Errorf("[RecoveryPlan-deleteMirrorEnvironment] Could not get the mirror environment: mirror_environment/storage.%d.%d. Cause: %+v",
			source.StorageID, target.StorageID, err)
		return err
	}

	err = env.IsMirrorVolumeExisted()
	if errors.Equal(err, mirror.ErrVolumeExisted) {
		return nil
	} else if err != nil {
		logger.Errorf("[RecoveryPlan-deleteMirrorEnvironment] Could not get mirror volume: mirror_volume/storage.%d.%d. Cause: %+v",
			env.SourceClusterStorage.StorageID, env.TargetClusterStorage.StorageID, err)
		return err
	}

	if err = env.SetOperation(constant.MirrorEnvironmentOperationStop); err != nil {
		logger.Errorf("[RecoveryPlan-deleteMirrorEnvironment] Could not set environment operation: mirror_environment/storage.%d.%d/operation. Cause: %+v",
			env.SourceClusterStorage.StorageID, env.TargetClusterStorage.StorageID, err)
		return err
	}

	logger.Infof("[deleteMirrorEnvironment] Done - mirror_volume/storage.%d.%d", source.StorageID, target.StorageID)
	return nil
}

func deleteStorageEnvironment(ps *drms.StorageRecoveryPlan) error {
	if ps.RecoveryClusterStorage == nil || ps.RecoveryClusterStorage.Id == 0 {
		return nil
	}

	if err := deleteMirrorEnvironment(
		&cmsStorage.ClusterStorage{ClusterID: ps.ProtectionClusterStorage.Cluster.Id, StorageID: ps.ProtectionClusterStorage.Id},
		&cmsStorage.ClusterStorage{ClusterID: ps.RecoveryClusterStorage.Cluster.Id, StorageID: ps.RecoveryClusterStorage.Id},
	); err != nil {
		return err
	}

	return nil
}

func deleteVolumeEnvironment(pv *drms.VolumeRecoveryPlan) error {
	if pv.RecoveryClusterStorage == nil || pv.RecoveryClusterStorage.Id == 0 {
		return nil
	}

	if err := deleteMirrorEnvironment(
		&cmsStorage.ClusterStorage{ClusterID: pv.ProtectionClusterVolume.Cluster.Id, StorageID: pv.ProtectionClusterVolume.Storage.Id},
		&cmsStorage.ClusterStorage{ClusterID: pv.RecoveryClusterStorage.Cluster.Id, StorageID: pv.RecoveryClusterStorage.Id},
	); err != nil {
		return err
	}

	return nil
}

func deleteMirrorEnvironments(plan *drms.RecoveryPlan) {
	// storage recovery plan
	for _, ps := range plan.Detail.Storages {
		if err := deleteStorageEnvironment(ps); err != nil {
			logger.Warnf("[RecoveryPlan-deleteMirrorEnvironments] Could not delete mirror environment mirroring: protection(%d) recovery(%d) storage. Cause: %+v",
				ps.ProtectionClusterStorage.Id, ps.RecoveryClusterStorage.Id, err)
		}
	}

	// volume recovery plan
	for _, pv := range plan.Detail.Volumes {
		if err := deleteVolumeEnvironment(pv); err != nil {
			logger.Warnf("[RecoveryPlan-deleteMirrorEnvironments] Could not delete mirror environment mirroring: protection(%d) recovery(%d) storage. Cause: %+v",
				pv.ProtectionClusterVolume.Storage.Id, pv.RecoveryClusterStorage.Id, err)
		}
	}
}

// volume mirroring 정보를 가져온다.
func getMirrorVolume(plan *drms.RecoveryPlan, volID uint64) (*mirror.Volume, error) {
	var volumePlan = internal.GetVolumeRecoveryPlan(plan, volID)
	var storagePlan = internal.GetStorageRecoveryPlan(plan, volumePlan.ProtectionClusterVolume.Storage.Id)

	var recoveryClusterStorage *cms.ClusterStorage
	if volumePlan.RecoveryClusterStorage == nil || volumePlan.RecoveryClusterStorage.Id == 0 {
		recoveryClusterStorage = storagePlan.RecoveryClusterStorage
	} else {
		recoveryClusterStorage = volumePlan.RecoveryClusterStorage
	}

	return mirror.GetVolume(
		volumePlan.ProtectionClusterVolume.Storage.Id,
		recoveryClusterStorage.Id,
		volID,
	)
}

// 해당 plan 의 volume mirroring 을 중지하고, mirror volume 정보를 삭제한다.
func stopAndDestroyVolumesMirroring(plan *drms.RecoveryPlan, typeCodes ...string) {
	wg := sync.WaitGroup{}
	// volume mirroring 이 정지 완료 될 때까지 기다렸다가 완료되면 etcd 에서 해당 mirror volume 정보를 삭제하는 operation 을 추가한다.
	waitStoppedAndDestroy := func(v *mirror.Volume) {
		defer wg.Done()

		timeout := time.After(15 * time.Second)
		for {
			select {
			case <-time.After(1 * time.Second):
			case <-timeout:
				logger.Warnf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not stop volume mirroring: plan(%d) volume(%d). Cause: timeout occurred", plan.Id, v.SourceVolume.VolumeID)
				return
			}

			status, err := v.GetStatus()
			if err != nil {
				logger.Errorf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not get the volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
					v.SourceClusterStorage.StorageID, v.TargetClusterStorage.StorageID, v.SourceVolume.VolumeID, err)
				return
			}

			if status.StateCode != constant.MirrorVolumeStateCodeStopped {
				continue
			}

			if err := internal.DestroyVolumeMirroring(v); err != nil {
				logger.Warnf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not destroy volume mirroring: plan(%d) volume(%d). Cause: %+v", plan.Id, v.SourceVolume.VolumeID, err)
			}

			return
		}
	}

	// mirror volume 전체 목록을 조회한다.
	volumeList, err := mirror.GetVolumeList()
	if err != nil {
		logger.Warnf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not get volume mirroring list. Cause: %v", err)
		return
	}

	// mirror volume 전체 목록에서 현재 plan 에 해당 하는 것만 삭제한다.
	for _, vol := range volumeList {
		pid, err := vol.GetPlanOfMirrorVolume()
		if err != nil {
			logger.Warnf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not stop volume mirroring: plan(%d) volume(%d). Cause: %v", plan.Id, vol.SourceVolume.VolumeID, err)
			continue
		}

		// 현재 plan 에 해당하는 mirror volume 이 아니면 넘어간다.
		if plan.Id != pid {
			continue
		}

		var typeCode string
		for _, t := range typeCodes {
			typeCode = t
		}

		if typeCode != "" {
			// 각 볼륨의 mirroring 을 중지하고 삭제한다.
			// 최신 데이터 복구 시, 원본 볼륨을 삭제하지 않고 미러링만 해제한다.
			if err := operateStopMirrorVolume(vol, StopOperation(constant.MirrorVolumeOperationStop)); err != nil {
				logger.Warnf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not stop volume mirroring: plan(%d) volume(%d). Cause: %v", plan.Id, vol.SourceVolume.VolumeID, err)
				continue
			}
		} else {
			// 각 볼륨의 mirroring 을 중지하고 삭제한다.
			if err := operateStopMirrorVolume(vol); err != nil {
				logger.Warnf("[RecoveryPlan-stopAndDestroyVolumesMirroring] Could not stop volume mirroring: plan(%d) volume(%d). Cause: %v", plan.Id, vol.SourceVolume.VolumeID, err)
				continue
			}
		}

		wg.Add(1)
		go waitStoppedAndDestroy(vol)
	}

	// 모든 volume mirroring 이 중지되거나 timeout 이 발생할 때까지 기다린다.
	wg.Wait()
}

// volume mirroring 을 중지하는 operation 을 설정한다.
func operateStopMirrorVolume(mv *mirror.Volume, opts ...Option) error {
	// option 이 따로 지정되어 있지 않을 경우 stop & delete 를 수행한다.
	var option = StopMirrorVolumeOptions{
		Operation: constant.MirrorVolumeOperationStopAndDelete,
	}

	// 볼륨 복제 중지 오퍼레이션 옵션 추가
	for _, o := range opts {
		o(&option)
	}

	logger.Infof("[operateStopMirrorVolume] - %v", option)
	if err := internal.StopVolumeMirroring(mv, option.Operation); err != nil {
		return err
	}

	return nil
}

// Delete 재해 복구 계획을 삭제하는 함수
func Delete(ctx context.Context, req *drms.RecoveryPlanRequest, typeCodes ...string) error {
	logger.Info("[RecoveryPlan-Delete] Start")

	var err error
	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryPlan-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetPlanId() == 0 {
		err = errors.RequiredParameter("plan_id")
		logger.Errorf("[RecoveryPlan-Delete] Errors occurred while validating the request. Cause: %+v", err)
		return err
	}

	if err = internal.IsAccessibleProtectionGroup(ctx, req.GroupId); err != nil {
		logger.Errorf("[RecoveryPlan-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = checkDeletableRecoveryPlan(req); err != nil {
		logger.Errorf("[RecoveryPlan-Delete] Errors occurred during checking deletable status of the recovery plan(%d). Cause: %+v", req.PlanId, err)
		return err
	}

	var plan *drms.RecoveryPlan
	if plan, err = Get(ctx, &drms.RecoveryPlanRequest{GroupId: req.GroupId, PlanId: req.PlanId}); err != nil {
		return err
	}

	if err = deletePlan(req.PlanId); err != nil {
		return err
	}

	// 재해복구계획의 복제대상 볼륨들의 복제를 중지한다.
	stopAndDestroyVolumesMirroring(plan, typeCodes...)

	// 복제를 위해 구성한 볼륨타입간 복제환경을 제거한다.
	// 만약 해당 복제환경을 통해 복제중인 볼륨이 존재한다면 복제환경을 제거하지 않는다.
	deleteMirrorEnvironments(plan)

	logger.Infof("[RecoveryPlan-Delete] Success: plan(%d:%s)", plan.Id, plan.Name)
	return nil
}

func updatePlan(plan *model.Plan, detail *drms.RecoveryPlanDetail) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Save(&plan).Error; err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Could not update the recovery plan(%d). Cause: %+v", plan.ID, err)
			return errors.UnusableDatabase(err)
		}

		if err = mergeTenantRecoveryPlan(db, plan.ID, detail.Tenants); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the tenant recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}
		if err = mergeAvailabilityZoneRecoveryPlan(db, plan.ID, detail.AvailabilityZones); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the availability zone recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}
		if err = mergeExternalNetworkRecoveryPlan(db, plan.ID, detail.ExternalNetworks); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the external network recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}
		if err = mergeRouterRecoveryPlan(db, plan.ID, detail.Routers); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the router recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}
		if err = mergeStorageRecoveryPlan(db, plan.ID, detail.Storages); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the storage recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}
		if err = mergeInstanceRecoveryPlan(db, plan.ID, detail.Instances); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the instance recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}
		if err = mergeVolumeRecoveryPlan(db, plan.ID, detail.Volumes); err != nil {
			logger.Errorf("[RecoveryPlan-updatePlan] Errors occurred during updating the volume recovery plan(%d:%s). Cause: %+v", plan.ID, plan.Name, err)
			return err
		}

		return nil
	})
}

func deletePlan(planID uint64) (err error) {
	return database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Where(&model.PlanTenant{RecoveryPlanID: planID}).Delete(&model.PlanTenant{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the tenant recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanAvailabilityZone{RecoveryPlanID: planID}).Delete(&model.PlanAvailabilityZone{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the availability zone recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanExternalNetwork{RecoveryPlanID: planID}).Delete(&model.PlanExternalNetwork{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the external network recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanExternalRoutingInterface{RecoveryPlanID: planID}).Delete(&model.PlanExternalRoutingInterface{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the external routing interface recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanRouter{RecoveryPlanID: planID}).Delete(&model.PlanRouter{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the router recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanStorage{RecoveryPlanID: planID}).Delete(&model.PlanStorage{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the storage recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanInstanceFloatingIP{RecoveryPlanID: planID}).Delete(&model.PlanInstanceFloatingIP{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the floating ip recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanInstanceDependency{RecoveryPlanID: planID}).Delete(&model.PlanInstanceDependency{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the instance dependency recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanInstance{RecoveryPlanID: planID}).Delete(&model.PlanInstance{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the instance recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.PlanVolume{RecoveryPlanID: planID}).Delete(&model.PlanVolume{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the volume recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		if err = db.Where(&model.Plan{ID: planID}).Delete(&model.Plan{}).Error; err != nil {
			logger.Errorf("[RecoveryPlan-deletePlan] Could not delete the recovery plan(%d). Cause: %+v", planID, err)
			return err
		}
		return nil
	})
}
