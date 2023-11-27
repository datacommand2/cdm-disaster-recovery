package recoveryplan

import (
	"context"
	"fmt"
	"github.com/asaskevich/govalidator"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
	"regexp"
)

func validatePlanListRequest(req *drms.RecoveryPlanListRequest) error {
	if req.GetGroupId() == 0 {
		return errors.RequiredParameter("group_id")
	}

	if len(req.Name) > 255 {
		return errors.LengthOverflowParameterValue("name", req.Name, 255)
	}

	return nil
}

func checkValidTenantRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var tenantMap = make(map[uint64]*cms.ClusterTenant)
	for idx, v := range pg.Instances {
		if v.GetTenant().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].tenant.id", idx))
		}

		tenantMap[v.GetTenant().GetId()] = v.Tenant
	}

	for idx, t := range p.Detail.Tenants {
		var matched = false

		if t.GetProtectionClusterTenant().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.tenants[%d].protection_cluster_tenant.id", idx))
		}

		for _, v := range tenantMap {
			matched = matched || (v.Id == t.GetProtectionClusterTenant().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.tenants[%d].protection_cluster_tenant.id", idx), t.ProtectionClusterTenant.Id, "not protected tenant")
		}
	}

	for _, v := range tenantMap {
		var matched = false
		for _, t := range p.Detail.Tenants {
			matched = matched || (v.Id == t.GetProtectionClusterTenant().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue("plan.detail.tenants", nil, "must include tenant("+v.Name+") recovery plan")
		}
	}

	for idx, t := range p.Detail.Tenants {
		if !internal.IsTenantRecoveryPlanTypeCode(t.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.tenants[%d].recovery_type_code", idx), t.RecoveryTypeCode, internal.TenantRecoveryPlanTypeCodes)
		}

		if t.RecoveryClusterTenantMirrorName == "" {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.tenants[%d].recovery_cluster_tenant_mirror_name", idx))
		}

		// tenant name 글자 수 제한 64자 이지만 hash 값(9자) 고려하여 55자로 제한
		if len(t.RecoveryClusterTenantMirrorName) > 55 {
			return errors.LengthOverflowParameterValue(fmt.Sprintf("plan.detail.tenants[%d].recovery_cluster_tenant_mirror_name", idx), t.RecoveryClusterTenantMirrorName, 55)
		}

		if t.GetProtectionClusterTenant().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.tenants[%d].protection_cluster_tenant.id", idx))
		}

		_, err = cluster.GetClusterTenant(ctx, pg.ProtectionCluster.Id, t.ProtectionClusterTenant.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.tenants[%d].protection_cluster_tenant.id", idx), t.ProtectionClusterTenant.Id, "not found protection tenant")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.tenants[%d].protection_cluster_tenant.id", idx), t.ProtectionClusterTenant.Id, "not allowed protection tenant")
			}
			return err
		}

		if t.GetRecoveryClusterTenant().GetId() != 0 {
			_, err = cluster.GetClusterTenant(ctx, p.RecoveryCluster.Id, t.RecoveryClusterTenant.Id)
			if err != nil {
				switch errors.GetIPCStatusCode(err) {
				case errors.IPCStatusNotFound:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.tenants[%d].recovery_cluster_tenant.id", idx), t.RecoveryClusterTenant.Id, "not found recovery tenant")
				case errors.IPCStatusUnauthorized:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.tenants[%d].recovery_cluster_tenant.id", idx), t.RecoveryClusterTenant.Id, "not allowed recovery tenant")
				}
				return err
			}
		}
	}
	return nil
}

func checkValidAvailabilityZoneRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var zoneMap = make(map[uint64]*cms.ClusterAvailabilityZone)
	for idx, v := range pg.Instances {
		if v.GetAvailabilityZone().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].availability_zone.id", idx))
		}

		zoneMap[v.GetAvailabilityZone().GetId()] = v.AvailabilityZone
	}

	for idx, z := range p.Detail.AvailabilityZones {
		var matched = false

		if z.GetProtectionClusterAvailabilityZone().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.availability_zones[%d].protection_cluster_availability_zone.id", idx))
		}

		for _, v := range zoneMap {
			matched = matched || (v.Id == z.GetProtectionClusterAvailabilityZone().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.availability_zones[%d].protection_cluster_availability_zone.id", idx), z.ProtectionClusterAvailabilityZone.Id, "not protected availability zone")
		}
	}

	for _, v := range zoneMap {
		var matched = false
		for _, z := range p.Detail.AvailabilityZones {
			matched = matched || (v.Id == z.GetProtectionClusterAvailabilityZone().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue("plan.detail.availability_zones", nil, "must include availability zone("+v.Name+") recovery plan")
		}
	}

	for idx, z := range p.Detail.AvailabilityZones {
		if !internal.IsAvailabilityZoneRecoveryPlanTypeCode(z.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.availability_zones[%d].recovery_type_code", idx), z.RecoveryTypeCode, internal.AvailabilityZoneRecoveryPlanTypeCodes)
		}

		if z.GetProtectionClusterAvailabilityZone().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.availability_zones[%d].protection_cluster_availability_zone.id", idx))
		}

		_, err = cluster.GetClusterAvailabilityZone(ctx, pg.ProtectionCluster.Id, z.ProtectionClusterAvailabilityZone.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.availability_zones[%d].protection_cluster_availability_zone.id", idx), z.ProtectionClusterAvailabilityZone.Id, "not found protection availability zone")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.availability_zones[%d].protection_cluster_availability_zone.id", idx), z.ProtectionClusterAvailabilityZone.Id, "not allowed protection availability zone")
			}
			return err
		}

		if z.GetRecoveryClusterAvailabilityZone().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.availability_zones[%d].recovery_cluster_availability_zone.id", idx))
		}

		_, err = cluster.GetClusterAvailabilityZone(ctx, p.RecoveryCluster.Id, z.RecoveryClusterAvailabilityZone.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.availability_zones[%d].recovery_cluster_availability_zone.id", idx), z.RecoveryClusterAvailabilityZone.Id, "not found recovery availability zone")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.availability_zones[%d].recovery_cluster_availability_zone.id", idx), z.RecoveryClusterAvailabilityZone.Id, "not allowed recovery availability zone")
			}
			return err
		}
	}
	return nil
}

func checkValidExternalNetworkRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var networkMap = make(map[uint64]*cms.ClusterNetwork)
	for idx1, i := range pg.Instances {
		for idx2, r := range i.Routers {
			if r.GetExternalRoutingInterfaces() == nil {
				return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].routers[%d].external_routing_interfaces", idx1, idx2))
			}

			if len(r.GetExternalRoutingInterfaces()) == 0 || !r.GetExternalRoutingInterfaces()[0].GetNetwork().GetExternalFlag() {
				continue
			}

			network := r.GetExternalRoutingInterfaces()[0].GetNetwork()
			networkMap[network.Id] = network
		}
	}

	for idx, n := range p.Detail.ExternalNetworks {
		var matched = false

		if n.GetProtectionClusterExternalNetwork().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.external_networks[%d].protection_cluster_external_network.id", idx))
		}

		for _, v := range networkMap {
			matched = matched || (v.Id == n.GetProtectionClusterExternalNetwork().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].protection_cluster_external_network.id", idx), n.ProtectionClusterExternalNetwork.Id, "not protected external network")
		}
	}

	for _, v := range networkMap {
		var matched = false
		for _, n := range p.Detail.ExternalNetworks {
			matched = matched || (v.Id == n.GetProtectionClusterExternalNetwork().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue("plan.detail.external_networks", nil, "must include external network("+v.Name+") recovery plan")
		}
	}

	for idx, n := range p.Detail.ExternalNetworks {
		if !internal.IsExternalNetworkRecoveryPlanTypeCode(n.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].recovery_type_code", idx), n.RecoveryTypeCode, internal.ExternalNetworkRecoveryPlanTypeCodes)
		}

		if n.GetProtectionClusterExternalNetwork().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.external_networks[%d].protection_cluster_external_network.id", idx))
		}

		_, err = cluster.GetClusterNetwork(ctx, pg.ProtectionCluster.Id, n.ProtectionClusterExternalNetwork.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].protection_cluster_external_network.id", idx), n.ProtectionClusterExternalNetwork.Id, "not found protection external network")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].protection_cluster_external_network.id", idx), n.ProtectionClusterExternalNetwork.Id, "not allowed protection external network")
			}
			return err
		}

		if n.GetRecoveryClusterExternalNetwork().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.external_networks[%d].recovery_cluster_external_network.id", idx))
		}

		rn, err := cluster.GetClusterNetwork(ctx, p.RecoveryCluster.Id, n.RecoveryClusterExternalNetwork.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].recovery_cluster_external_network.id", idx), n.RecoveryClusterExternalNetwork.Id, "not found recovery external network")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].recovery_cluster_external_network.id", idx), n.RecoveryClusterExternalNetwork.Id, "not allowed recovery external network")
			}
			return err
		}

		if !rn.ExternalFlag {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.external_networks[%d].recovery_cluster_external_network.id", idx), n.RecoveryClusterExternalNetwork.Id, "not valid recovery external network")
		}
	}
	return nil
}

func checkValidRouterRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var routerMap = make(map[uint64]*cms.ClusterRouter)
	for idx1, i := range pg.Instances {
		for idx2, r := range i.Routers {
			if r.GetId() == 0 {
				return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].routers[%d].id", idx1, idx2))
			}

			routerMap[r.GetId()] = r
		}
	}

	for idx, r := range p.Detail.Routers {
		var matched = false

		if r.GetProtectionClusterRouter().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.routers[%d].protection_cluster_router.id", idx))
		}

		for _, v := range routerMap {
			matched = matched || (v.Id == r.GetProtectionClusterRouter().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].protection_cluster_router.id", idx), r.ProtectionClusterRouter.Id, "not protected router")
		}
	}

	for idx, r := range p.Detail.Routers {
		if !internal.IsRouterRecoveryPlanTypeCode(r.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.routers[%d].recovery_type_code", idx), r.RecoveryTypeCode, internal.RouterRecoveryPlanTypeCodes)
		}

		_, err = cluster.GetClusterRouter(ctx, pg.ProtectionCluster.Id, r.ProtectionClusterRouter.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].protection_cluster_router.id", idx), r.ProtectionClusterRouter.Id, "not found protection router")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].protection_cluster_router.id", idx), r.ProtectionClusterRouter.Id, "not allowed protection router")
			}
			return err
		}

		if r.GetRecoveryClusterExternalNetwork().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_network.id", idx))
		}

		_, err = cluster.GetClusterNetwork(ctx, p.RecoveryCluster.Id, r.RecoveryClusterExternalNetwork.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_network.id", idx), r.RecoveryClusterExternalNetwork.Id, "not found recovery external network")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_network.id", idx), r.RecoveryClusterExternalNetwork.Id, "not allowed recovery external network")
			}
			return err
		}

		for idx2, i := range r.RecoveryClusterExternalRoutingInterfaces {
			if i.IpAddress != "" && !govalidator.IsIP(i.IpAddress) {
				return errors.FormatMismatchParameterValue(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_routing_interfaces[%d].ip_address", idx, idx2), i.IpAddress, "IP Address")
			}

			if i.GetSubnet().GetId() == 0 {
				return errors.RequiredParameter(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_routing_interfaces[%d].subnet.id", idx, idx2))
			}

			_, err = cluster.GetClusterSubnet(ctx, p.RecoveryCluster.Id, i.Subnet.Id)
			if err != nil {
				switch errors.GetIPCStatusCode(err) {
				case errors.IPCStatusNotFound:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_routing_interfaces[%d].subnet.id", idx, idx2), i.Subnet.Id, "not found recovery routing interface subnet")
				case errors.IPCStatusUnauthorized:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.routers[%d].recovery_cluster_external_routing_interfaces[%d].subnet.id", idx, idx2), i.Subnet.Id, "not allowed recovery routing interface subnet")
				}
				return err
			}
		}
	}
	return nil
}

func checkValidStorageRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var storageMap = make(map[uint64]*cms.ClusterStorage)
	for idx1, i := range pg.Instances {
		for idx2, v := range i.Volumes {
			if v.GetStorage().GetId() == 0 {
				return errors.RequiredParameter(fmt.Sprintf("group.instances[%d].volumes[%d].storage.id", idx1, idx2))
			}

			storageMap[v.GetStorage().GetId()] = v.Storage
		}
	}

	for idx, s := range p.Detail.Storages {
		var matched = false

		if s.GetProtectionClusterStorage().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.storages[%d].protection_cluster_storage.id", idx))
		}

		for _, v := range storageMap {
			matched = matched || (v.Id == s.GetProtectionClusterStorage().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].protection_cluster_storage.id", idx), s.ProtectionClusterStorage.Id, "not protected storage")
		}
	}

	for _, v := range storageMap {
		var matched = false
		for _, s := range p.Detail.Storages {
			matched = matched || (v.Id == s.GetProtectionClusterStorage().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue("plan.detail.storages", nil, "must include storage("+v.Name+") recovery plan")
		}
	}

	for idx, s := range p.Detail.Storages {
		if !internal.IsStorageRecoveryPlanTypeCode(s.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.storages[%d].recovery_type_code", idx), s.RecoveryTypeCode, internal.StorageRecoveryPlanTypeCodes)
		}

		if s.GetProtectionClusterStorage().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.storages[%d].protection_cluster_storage.id", idx))
		}

		var protectionStorage *cms.ClusterStorage
		protectionStorage, err = cluster.GetClusterStorage(ctx, pg.ProtectionCluster.Id, s.ProtectionClusterStorage.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].protection_cluster_storage.id", idx), s.ProtectionClusterStorage.Id, "not found protection storage")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].protection_cluster_storage.id", idx), s.ProtectionClusterStorage.Id, "not allowed protection storage")
			}
			return err
		}

		// 지원하지 않는 storage type
		if protectionStorage.TypeCode != storage.ClusterStorageTypeCeph {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].protection_cluster_storage.typCode", idx), protectionStorage.TypeCode, "unknown storage type")
		}

		if protectionStorage.TypeCode == storage.ClusterStorageTypeCeph {
			// keyring 이 등록되지 않은 storage
			if !isUsableStorageMetadata(pg.ProtectionCluster.Id, s.ProtectionClusterStorage.Id) {
				return internal.StorageKeyringRegistrationRequired(pg.ProtectionCluster.Id, protectionStorage.Uuid)
			}
		}

		if s.GetRecoveryClusterStorage().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.storages[%d].recovery_cluster_storage.id", idx))
		}

		var recoveryStorage *cms.ClusterStorage
		recoveryStorage, err = cluster.GetClusterStorage(ctx, p.RecoveryCluster.Id, s.RecoveryClusterStorage.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].recovery_cluster_storage.id", idx), s.RecoveryClusterStorage.Id, "not found recovery storage")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].recovery_cluster_storage.id", idx), s.RecoveryClusterStorage.Id, "not allowed recovery storage")
			}
			return err
		}

		if recoveryStorage.TypeCode != storage.ClusterStorageTypeCeph {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.storages[%d].recovery_cluster_storage.typCode", idx), recoveryStorage.TypeCode, "unknown storage type")
		}

		if recoveryStorage.TypeCode == storage.ClusterStorageTypeCeph {
			// keyring 이 등록되지 않은 storage
			if !isUsableStorageMetadata(p.RecoveryCluster.Id, s.RecoveryClusterStorage.Id) {
				return internal.StorageKeyringRegistrationRequired(p.RecoveryCluster.Id, s.RecoveryClusterStorage.Uuid)
			}
		}
	}
	return nil
}

func checkValidInstanceRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var zoneMap = make(map[uint64]uint64)
	for _, v := range p.Detail.AvailabilityZones {
		zoneMap[v.GetProtectionClusterAvailabilityZone().GetId()] = v.GetRecoveryClusterAvailabilityZone().GetId()
	}

	for idx, i := range p.Detail.Instances {
		var matched = false

		if i.GetProtectionClusterInstance().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].protection_cluster_instance.id", idx))
		}

		for _, v := range pg.Instances {
			matched = matched || (v.Id == i.GetProtectionClusterInstance().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].protection_cluster_instance.id", idx), i.ProtectionClusterInstance.Id, "not protected instance")
		}
	}

	for _, v := range pg.Instances {
		var matched = false
		for _, i := range p.Detail.Instances {
			matched = matched || (v.Id == i.GetProtectionClusterInstance().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue("plan.detail.instances", nil, "must include instance("+v.Name+") recovery plan")
		}
	}

	for idx, i := range p.Detail.Instances {
		if !internal.IsInstanceRecoveryPlanTypeCode(i.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_type_code", idx), i.RecoveryTypeCode, internal.InstanceRecoveryPlanTypeCodes)
		}

		if i.DiagnosisFlag {
			if i.DiagnosisTimeout < 60 || i.DiagnosisTimeout > 600 {
				return errors.OutOfRangeParameterValue(fmt.Sprintf("plan.detail.instances[%d].diagnosis_timeout", idx), i.DiagnosisTimeout, 60, 600)
			}

			if i.DiagnosisMethodCode == "" {
				return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].diagnosis_method_code", idx))
			}

			if !internal.IsInstanceDiagnosisMethodCode(i.DiagnosisMethodCode) {
				return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.instances[%d].diagnosis_timeout", idx), i.DiagnosisMethodCode, internal.InstanceDiagnosisMethodCodes)
			}

			if i.DiagnosisMethodData == "" {
				return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].diagnosis_method_data", idx))
			}

			if !govalidator.IsJSON(i.DiagnosisMethodData) {
				return errors.FormatMismatchParameterValue(fmt.Sprintf("plan.detail.instances[%d].diagnosis_timeout", idx), i.DiagnosisMethodData, "JSON")
			}
		}

		if i.GetProtectionClusterInstance().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].protection_cluster_instance.id", idx))
		}

		var ins *cms.ClusterInstance
		ins, err = cluster.GetClusterInstance(ctx, pg.ProtectionCluster.Id, i.ProtectionClusterInstance.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].protection_cluster_instance.id", idx), i.ProtectionClusterInstance.Id, "not found protection instance")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].protection_cluster_instance.id", idx), i.ProtectionClusterInstance.Id, "not allowed protection instance")
			}
			return err
		}

		for _, n := range ins.Networks {
			if n.FloatingIp.GetIpAddress() == "" {
				continue
			}

			isExistFloating, err := cluster.CheckIsExistClusterFloatingIP(ctx, p.RecoveryCluster.Id, n.FloatingIp.IpAddress)
			if err != nil {
				return err
			}

			if isExistFloating {
				return internal.FloatingIPAddressDuplicated(ins.Uuid, n.FloatingIp.IpAddress)
			}

			isExistRouting, err := cluster.CheckIsExistClusterRoutingInterface(ctx, p.RecoveryCluster.Id, n.FloatingIp.IpAddress)
			if err != nil {
				return err
			}

			if isExistRouting {
				return internal.FloatingIPAddressDuplicated(ins.Uuid, n.FloatingIp.IpAddress)
			}
		}

		var zone *cms.ClusterAvailabilityZone
		if i.GetRecoveryClusterAvailabilityZone().GetId() == 0 {
			zone, err = cluster.GetClusterAvailabilityZone(ctx, p.RecoveryCluster.Id, zoneMap[ins.AvailabilityZone.Id])
			if err != nil {
				return err
			}
		} else {
			zone, err = cluster.GetClusterAvailabilityZone(ctx, p.RecoveryCluster.Id, i.RecoveryClusterAvailabilityZone.Id)
			if err != nil {
				switch errors.GetIPCStatusCode(err) {
				case errors.IPCStatusNotFound:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_availability_zone.id", idx), i.RecoveryClusterAvailabilityZone.Id, "not found recovery availability zone")
				case errors.IPCStatusUnauthorized:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_availability_zone.id", idx), i.RecoveryClusterAvailabilityZone.Id, "not allowed recovery availability zone")
				}
				return err
			}
		}

		if i.GetRecoveryClusterHypervisor().GetId() != 0 {
			var matched = false
			for _, h := range zone.Hypervisors {
				matched = matched || (h.Id == i.RecoveryClusterHypervisor.Id)
			}
			if !matched {
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_hypervisor.id", idx), i.RecoveryClusterHypervisor.Id, "not in availability zone("+zone.Name+")")
			}

			hypervisor, err := cluster.GetClusterHypervisor(ctx, p.RecoveryCluster.Id, i.RecoveryClusterHypervisor.Id)
			if err != nil {
				switch errors.GetIPCStatusCode(err) {
				case errors.IPCStatusNotFound:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_hypervisor.id", idx), i.RecoveryClusterHypervisor.Id, "not found recovery hypervisor")
				case errors.IPCStatusUnauthorized:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_hypervisor.id", idx), i.RecoveryClusterHypervisor.Id, "not allowed recovery hypervisor")
				}
				return err
			}

			if hypervisor.State != "up" || hypervisor.Status != "enabled" {
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_hypervisor.id", idx), i.RecoveryClusterHypervisor.Id, "unusable recovery hypervisor")
			}
		} else { // TODO: hypervisor 자동할당 사용시 삭제
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].recovery_cluster_hypervisor_id", idx))
		}

		for idx2, d := range i.Dependencies {
			if d.Id == 0 {
				return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].dependencies[%d].id", idx, idx2))
			}

			var matched = false
			for _, v := range pg.Instances {
				matched = matched || (v.Id == d.Id)
			}
			if !matched {
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.instances[%d].dependencies[%d].id", idx, idx2), nil, "not protected instance")
			}
		}
	}
	return nil
}

func checkValidVolumeRecoveryPlans(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	var volumeMap = make(map[uint64]*cms.ClusterVolume)
	for idx1, i := range pg.Instances {
		for idx2, v := range i.Volumes {
			if v.GetVolume().GetId() == 0 {
				return errors.RequiredParameter(fmt.Sprintf("plan.detail.instances[%d].volumes[%d].id", idx1, idx2))
			}

			volumeMap[v.GetVolume().GetId()] = v.Volume
		}
	}

	for idx, v := range p.Detail.Volumes {
		var matched = false

		if v.GetProtectionClusterVolume().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.volumes[%d].protection_cluster_volume.id", idx))
		}

		for _, m := range volumeMap {
			matched = matched || (m.Id == v.GetProtectionClusterVolume().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.volumes[%d].protection_cluster_volume.id", idx), v.ProtectionClusterVolume.Id, "not protected volume")
		}
	}

	for _, m := range volumeMap {
		var matched = false
		for _, v := range p.Detail.Volumes {
			matched = matched || (m.Id == v.GetProtectionClusterVolume().GetId())
		}
		if !matched {
			return errors.InvalidParameterValue("plan.detail.volumes", nil, "must include volume("+m.Name+") recovery plan")
		}
	}

	for idx, v := range p.Detail.Volumes {
		if !internal.IsVolumeRecoveryPlanTypeCode(v.RecoveryTypeCode) {
			return errors.UnavailableParameterValue(fmt.Sprintf("plan.detail.volumes[%d].recovery_type_code", idx), v.RecoveryTypeCode, internal.VolumeRecoveryPlanTypeCodes)
		}

		if v.GetProtectionClusterVolume().GetId() == 0 {
			return errors.RequiredParameter(fmt.Sprintf("plan.detail.volumes[%d].protection_cluster_volume.id", idx))
		}

		_, err = cluster.GetClusterVolume(ctx, pg.ProtectionCluster.Id, v.ProtectionClusterVolume.Id)
		if err != nil {
			switch errors.GetIPCStatusCode(err) {
			case errors.IPCStatusNotFound:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.volumes[%d].protection_cluster_volume.id", idx), v.ProtectionClusterVolume.Id, "not found protection volume")
			case errors.IPCStatusUnauthorized:
				return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.volumes[%d].protection_cluster_volume.id", idx), v.ProtectionClusterVolume.Id, "not allowed protection volume")
			}
			return err
		}

		if v.GetRecoveryClusterStorage().GetId() != 0 {
			_, err = cluster.GetClusterStorage(ctx, p.RecoveryCluster.Id, v.RecoveryClusterStorage.Id)
			if err != nil {
				switch errors.GetIPCStatusCode(err) {
				case errors.IPCStatusNotFound:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.volumes[%d].recovery_cluster_storage.id", idx), v.RecoveryClusterStorage.Id, "not found recovery storage")
				case errors.IPCStatusUnauthorized:
					return errors.InvalidParameterValue(fmt.Sprintf("plan.detail.volumes[%d].recovery_cluster_storage.id", idx), v.RecoveryClusterStorage.Id, "not allowed recovery storage")
				}
				return err
			}
		}
	}

	return nil
}

func isFailbackRecoveryPlanExist(pg *drms.ProtectionGroup) error {
	plan := model.Plan{
		ProtectionGroupID: pg.Id,
		DirectionCode:     constant.RecoveryJobDirectionCodeFailback,
	}

	err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&plan).First(&plan).Error
	})
	if err == nil {
		return internal.FailbackRecoveryPlanExisted(pg.Id, plan.ID)

	} else if err != gorm.ErrRecordNotFound {
		return errors.UnusableDatabase(err)
	}

	return nil
}

func checkValidCluster(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) error {
	var err error

	// 계획 생성시 protection cluter 의 기동 여부, 보호그룹의 보호 클러스터와 일치 여부 및 owner group 확인을 위해 필수 항목으로 수정
	if p.GetProtectionCluster().GetId() == 0 {
		return errors.RequiredParameter("plan.protection_cluster.id")
	}

	// recovery cluster id 는 필수 항목
	if p.GetRecoveryCluster().GetId() == 0 {
		return errors.RequiredParameter("plan.recovery_cluster.id")
	}

	// protection cluster 와 recovery cluster 가 같을 수 없음
	if pg.GetProtectionCluster().GetId() == p.RecoveryCluster.Id {
		return errors.InvalidParameterValue("plan.recovery_cluster.id", p.RecoveryCluster.Id, "protection cluster is selected as recovery cluster")
	}

	// 보호그룹에 저장된 보호 클러스터와 사용자가 입력한 계획의 보호 클러스터 id가 일치해야함
	if pg.GetProtectionCluster().GetId() != p.GetProtectionCluster().GetId() {
		return internal.MismatchParameter("group.protection_cluster.id", pg.GetProtectionCluster().GetId(), "plan.protection_cluster.id", p.GetProtectionCluster().GetId())
	}

	// 같은 보호 그룹 내 plan 의 recovery cluster 는 중복될 수 없음
	var n int
	if err = database.Execute(func(db *gorm.DB) error {
		return db.Model(&model.Plan{}).Not(&model.Plan{ID: p.GetId()}).
			Where(&model.Plan{ProtectionGroupID: pg.Id, RecoveryClusterID: p.RecoveryCluster.Id}).
			Count(&n).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	if n > 0 {
		return internal.RecoveryPlanExisted(p.RecoveryCluster.Id, pg.Id)
	}

	// 사용가능한 cluster 만 등록 가능.
	protectionCluster, err := cluster.GetCluster(ctx, p.ProtectionCluster.Id)
	if err != nil {
		switch errors.GetIPCStatusCode(err) {
		case errors.IPCStatusNotFound:
			return errors.InvalidParameterValue("plan.protection_cluster.id", p.ProtectionCluster.Id, "not found protection cluster")
		case errors.IPCStatusUnauthorized:
			return errors.InvalidParameterValue("plan.protection_cluster.id", p.ProtectionCluster.Id, "not allowed protection cluster")
		}
		return err
	}

	recoveryCluster, err := cluster.GetCluster(ctx, p.RecoveryCluster.Id)
	if err != nil {
		switch errors.GetIPCStatusCode(err) {
		case errors.IPCStatusNotFound:
			return errors.InvalidParameterValue("plan.recovery_cluster.id", p.RecoveryCluster.Id, "not found recovery cluster")
		case errors.IPCStatusUnauthorized:
			return errors.InvalidParameterValue("plan.recovery_cluster.id", p.RecoveryCluster.Id, "not allowed recovery cluster")
		}
		return err
	}

	if protectionCluster.GetOwnerGroup().GetId() != recoveryCluster.GetOwnerGroup().GetId() {
		return internal.MismatchParameter("protectin_cluster.owner_group.id", protectionCluster.GetOwnerGroup().GetId(), "recovery_cluster.owner_group.id", recoveryCluster.GetOwnerGroup().GetId())
	}

	// protection cluster 와 recovery cluster 의 owner group 은 동일해야함
	if p.GetProtectionCluster().GetOwnerGroup().GetId() != p.GetRecoveryCluster().GetOwnerGroup().GetId() {
		return internal.MismatchParameter("plan.protection_cluster.owner_group.id", p.ProtectionCluster.OwnerGroup.Id, "plan.recovery_cluster.owner_group.id", p.RecoveryCluster.OwnerGroup.Id)
	}

	return nil
}

func checkValidAddRecoveryPlan(pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	err = database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.Plan{ProtectionGroupID: pg.Id, Name: p.Name}).First(&model.Plan{}).Error
	})
	switch {
	case err == nil:
		return errors.ConflictParameterValue("plan.name", pg.Name)
	case err != gorm.ErrRecordNotFound:
		// 동일한 이름의 plan 이 없어야 생성 가능
		return errors.UnusableDatabase(err)
	}
	return nil
}

func checkValidUpdateRecoveryPlan(pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	err = database.Execute(func(db *gorm.DB) error {
		return db.Not(&model.Plan{ID: p.Id}).
			Where(&model.Plan{ProtectionGroupID: pg.Id, Name: p.Name}).
			First(&model.Plan{}).Error
	})
	switch {
	case err == nil:
		return errors.ConflictParameterValue("plan.name", pg.Name)
	case err != gorm.ErrRecordNotFound:
		// 동일한 이름의 plan 이 없어야 수정 가능
		return errors.UnusableDatabase(err)
	}
	return nil
}

func checkValidRecoveryPlan(ctx context.Context, pg *drms.ProtectionGroup, p *drms.RecoveryPlan) (err error) {
	if err = checkValidCluster(ctx, pg, p); err != nil {
		return err
	}

	if len(p.Name) == 0 {
		return errors.RequiredParameter("plan.name")
	}

	if len(p.Name) > 255 {
		return errors.LengthOverflowParameterValue("plan.name", p.Name, 255)
	}

	matched, _ := regexp.MatchString("^[a-zA-Z\\d가-힣.#\\-_]*$", p.Name)
	if !matched {
		return errors.InvalidParameterValue("group.name", pg.Name, "Only Hangul, Alphabet, Number(0-9) and special characters (-, _, #, .) are allowed.")
	}

	if len(p.Remarks) > 300 {
		return errors.LengthOverflowParameterValue("plan.remarks", p.Remarks, 300)
	}

	if len(p.DirectionCode) == 0 {
		return errors.RequiredParameter("plan.direction_code")
	}

	if p.DirectionCode != constant.RecoveryJobDirectionCodeFailover && p.DirectionCode != constant.RecoveryJobDirectionCodeFailback {
		return errors.UnavailableParameterValue("plan.direction_code", p.DirectionCode, []interface{}{constant.RecoveryJobDirectionCodeFailover, constant.RecoveryJobDirectionCodeFailback})
	}

	if p.Detail == nil {
		return errors.RequiredParameter("plan.detail")
	}

	if err = checkValidTenantRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	if err = checkValidAvailabilityZoneRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	if err = checkValidExternalNetworkRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	if err = checkValidRouterRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	if err = checkValidStorageRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	if err = checkValidInstanceRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	if err = checkValidVolumeRecoveryPlans(ctx, pg, p); err != nil {
		return err
	}

	return nil
}

func isRecoveryJobExists(pgid, pid uint64) error {
	var n int
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Model(&model.Job{}).Where(&model.Job{RecoveryPlanID: pid}).Count(&n).Error
	}); err != nil {
		return err
	}

	if n > 0 {
		return internal.RecoveryJobExisted(pgid, pid)
	}

	return nil
}

func checkUpdatableRecoveryPlan(pg *drms.ProtectionGroup, orig *model.Plan, p *drms.RecoveryPlan) error {
	if p.ProtectionCluster != nil && p.ProtectionCluster.Id != pg.ProtectionCluster.Id {
		return errors.UnchangeableParameter("plan.protection_cluster.id")
	}

	if p.RecoveryCluster != nil && p.RecoveryCluster.Id != orig.RecoveryClusterID {
		return errors.UnchangeableParameter("plan.recovery_cluster.id")
	}

	// job 이 존재하는 경우 plan 을 수정할 수 없다.
	if err := isRecoveryJobExists(pg.Id, orig.ID); err != nil {
		return err
	}

	return nil
}

func checkDeletableRecoveryPlan(req *drms.RecoveryPlanRequest) error {
	if err := isRecoveryJobExists(req.GroupId, req.PlanId); err != nil {
		return err
	}

	return nil
}

// isUsableStorageMetadata metadata 사용가능 여부 확인
// ceph 을 사용하는 경우 사용자가 keyring 을 직접 입력해줘야
// 추후에 plan 삭제시 mirroring 해제가 가능함
// true: 사용가능 false: 사용불가능
func isUsableStorageMetadata(cid, sid uint64) bool {
	cs := &storage.ClusterStorage{
		ClusterID: cid,
		StorageID: sid,
	}
	md, err := cs.GetMetadata()
	if err != nil {
		logger.Errorf("[isUsableStorageMetadata] Could not get storage metadata: clusters/%d/storages/%d/metadata. Cause: %+v", cid, sid, err)
		return false
	}

	if key, ok := md["admin_keyring"].(string); !ok || key == "" {
		return false
	}

	return true
}
