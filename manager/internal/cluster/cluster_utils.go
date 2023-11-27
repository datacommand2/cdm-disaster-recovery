package cluster

import (
	"context"
	centerConstant "github.com/datacommand2/cdm-center/cluster-manager/constant"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
	"github.com/micro/go-micro/v2/client"
	"github.com/micro/go-micro/v2/client/grpc"
	"time"
)

// DefaultClusterManagerService ClusterManagerService
var DefaultClusterManagerService = cms.NewClusterManagerService(centerConstant.ServiceName, grpc.NewClient())

// Option 클러스터를 조회하기위한 옵션이다.
type Option func(*cms.ClusterListRequest)

// TypeCode Cluster 의 TypeCode 를 옵션으로 지정하기위한 함수이다.
func TypeCode(typeCode string) Option {
	return func(req *cms.ClusterListRequest) {
		req.TypeCode = typeCode
	}
}

// OwnerGroupID 클러스터의 Owner Group 을 옵션으로 지정하기위한 함수이다.
func OwnerGroupID(id uint64) Option {
	return func(req *cms.ClusterListRequest) {
		req.OwnerGroupId = id
	}
}

// GetClusterList 는 클러스터 목록을 조회하는 함수이다.
func GetClusterList(ctx context.Context, opts ...Option) ([]*cms.Cluster, error) {
	var req cms.ClusterListRequest
	for _, o := range opts {
		o(&req)
	}

	rsp, err := DefaultClusterManagerService.GetClusterList(ctx, &req)
	if errors.GetIPCStatusCode(err) == errors.IPCStatusNoContent {
		return nil, internal.IPCNoContent()
	} else if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetClusters(), nil
}

// GetClusterMap 는 클러스터 목록을 조회하는 함수이다.
func GetClusterMap(ctx context.Context, opts ...Option) (map[uint64]*cms.Cluster, error) {
	list, err := GetClusterList(ctx, opts...)
	if err != nil {
		return nil, err
	}

	clusterMap := make(map[uint64]*cms.Cluster)
	for _, c := range list {
		clusterMap[c.Id] = c
	}
	return clusterMap, nil
}

// GetCluster 클러스터를 조회하기위한 함수이다.
func GetCluster(ctx context.Context, id uint64) (*cms.Cluster, error) {
	rsp, err := DefaultClusterManagerService.GetCluster(ctx, &cms.ClusterRequest{ClusterId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetCluster(), nil
}

// GetClusterSummary 클러스터 요약 정보를 조회하기위한 함수이다.
func GetClusterSummary(ctx context.Context) (*drms.ClusterSummary, error) {
	summary := &drms.ClusterSummary{}
	rsp, err := DefaultClusterManagerService.GetClusterList(ctx, &cms.ClusterListRequest{})
	if errors.GetIPCStatusCode(err) == errors.IPCStatusNoContent {
		return nil, internal.IPCNoContent()
	} else if err != nil {
		return nil, errors.IPCFailed(err)
	}

	for _, cluster := range rsp.Clusters {
		if cluster.GetStateCode() != centerConstant.ClusterStateInactive {
			summary.ActiveCluster++
		} else {
			summary.InactiveCluster++
		}
	}
	summary.TotalCluster = summary.ActiveCluster + summary.InactiveCluster

	return summary, nil
}

// GetClusterTenant 클러스터 테넌트를 조회하기위한 함수이다.
func GetClusterTenant(ctx context.Context, cid, id uint64) (*cms.ClusterTenant, error) {
	rsp, err := DefaultClusterManagerService.GetClusterTenant(ctx, &cms.ClusterTenantRequest{
		ClusterId: cid, ClusterTenantId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetTenant(), nil
}

// CheckIsExistClusterTenant 테넌트 이름으로 클러스터 테넌트 존재 여부 확인
func CheckIsExistClusterTenant(ctx context.Context, cid uint64, name string) (bool, error) {
	rsp, err := DefaultClusterManagerService.CheckIsExistClusterTenant(ctx, &cms.CheckIsExistByNameRequest{
		ClusterId: cid,
		Name:      name,
	})
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}

// GetClusterAvailabilityZone 클러스터 가용구역을 조회하기위한 함수이다.
func GetClusterAvailabilityZone(ctx context.Context, cid, id uint64) (*cms.ClusterAvailabilityZone, error) {
	rsp, err := DefaultClusterManagerService.GetClusterAvailabilityZone(ctx, &cms.ClusterAvailabilityZoneRequest{
		ClusterId: cid, ClusterAvailabilityZoneId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetAvailabilityZone(), nil
}

// GetClusterHypervisorList 클러스터 하이퍼바이저를 조회하기위한 함수이다.
func GetClusterHypervisorList(ctx context.Context, req *cms.ClusterHypervisorListRequest) ([]*cms.ClusterHypervisor, error) {
	rsp, err := DefaultClusterManagerService.GetClusterHypervisorList(ctx, req)
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetHypervisors(), nil
}

// GetClusterHypervisor 클러스터 하이퍼바이저를 조회하기위한 함수이다.
func GetClusterHypervisor(ctx context.Context, cid, id uint64) (*cms.ClusterHypervisor, error) {
	rsp, err := DefaultClusterManagerService.GetClusterHypervisor(ctx, &cms.ClusterHypervisorRequest{
		ClusterId: cid, ClusterHypervisorId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetHypervisor(), nil
}

// GetClusterHypervisorWithSync 클러스터 하이퍼바이저를 동기화 후 조회하기위한 함수이다.
func GetClusterHypervisorWithSync(ctx context.Context, cid, id uint64) (*cms.ClusterHypervisor, error) {
	rsp, err := DefaultClusterManagerService.GetClusterHypervisor(ctx, &cms.ClusterHypervisorRequest{
		ClusterId:           cid,
		ClusterHypervisorId: id,
		Sync:                true,
	})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetHypervisor(), nil
}

// GetClusterNetwork 클러스터 네트워크를 조회하기위한 함수이다.
func GetClusterNetwork(ctx context.Context, cid, id uint64) (*cms.ClusterNetwork, error) {
	rsp, err := DefaultClusterManagerService.GetClusterNetwork(ctx, &cms.ClusterNetworkRequest{
		ClusterId: cid, ClusterNetworkId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetNetwork(), nil
}

// GetClusterNetworkByUUID UUID 로 클러스터 네트워크를 조회
func GetClusterNetworkByUUID(ctx context.Context, cid uint64, uuid string) (*cms.ClusterNetwork, error) {
	rsp, err := DefaultClusterManagerService.GetClusterNetworkByUUID(ctx, &cms.ClusterNetworkByUUIDRequest{
		ClusterId: cid,
		Sync:      true,
		Uuid:      uuid,
	})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetNetwork(), nil
}

// GetClusterSubnet 클러스터 네트워크 서브넷을 조회하기위한 함수이다.
func GetClusterSubnet(ctx context.Context, cid, id uint64) (*cms.ClusterSubnet, error) {
	rsp, err := DefaultClusterManagerService.GetClusterSubnet(ctx, &cms.ClusterSubnetRequest{
		ClusterId: cid, ClusterSubnetId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetSubnet(), nil
}

// GetClusterRouter 클러스터 라우터를 조회하기위한 함수이다.
func GetClusterRouter(ctx context.Context, cid, id uint64) (*cms.ClusterRouter, error) {
	rsp, err := DefaultClusterManagerService.GetClusterRouter(ctx, &cms.ClusterRouterRequest{
		ClusterId: cid, ClusterRouterId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetRouter(), nil
}

// CheckIsExistClusterRoutingInterface ip address 를 통한 클러스터 Routing Interface 존재유무 확인
func CheckIsExistClusterRoutingInterface(ctx context.Context, clusterID uint64, ipAddress string) (bool, error) {
	rsp, err := DefaultClusterManagerService.CheckIsExistClusterRoutingInterface(ctx, &cms.CheckIsExistClusterRoutingInterfaceRequest{
		ClusterId:                        clusterID,
		ClusterRoutingInterfaceIpAddress: ipAddress,
	})
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}

// GetClusterFloatingIP 클러스터 FloatingIP 를 조회하기위한 함수이다.
func GetClusterFloatingIP(ctx context.Context, cid, id uint64) (*cms.ClusterFloatingIP, error) {
	rsp, err := DefaultClusterManagerService.GetClusterFloatingIP(ctx, &cms.ClusterFloatingIPRequest{
		ClusterId: cid, ClusterFloatingIpId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetFloatingIp(), nil
}

// CheckIsExistClusterFloatingIP ip address 를 통한 클러스터 FloatingIP 존재유무 확인
func CheckIsExistClusterFloatingIP(ctx context.Context, clusterID uint64, ipAddress string) (bool, error) {
	rsp, err := DefaultClusterManagerService.CheckIsExistClusterFloatingIP(ctx, &cms.CheckIsExistClusterFloatingIPRequest{
		ClusterId:                  clusterID,
		ClusterFloatingIpIpAddress: ipAddress,
	})
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}

// GetClusterSecurityGroup 클러스터 보안그룹을 조회하기위한 함수이다.
func GetClusterSecurityGroup(ctx context.Context, cid, id uint64) (*cms.ClusterSecurityGroup, error) {
	rsp, err := DefaultClusterManagerService.GetClusterSecurityGroup(ctx, &cms.ClusterSecurityGroupRequest{
		ClusterId: cid, ClusterSecurityGroupId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetSecurityGroup(), nil
}

// CheckIsExistClusterSecurityGroup 보안그룹 이름으로 클러스터 보안그룹 존재 여부 확인
func CheckIsExistClusterSecurityGroup(ctx context.Context, tid uint64, name string) (bool, error) {
	rsp, err := DefaultClusterManagerService.CheckIsExistClusterSecurityGroup(ctx, &cms.CheckIsExistByNameRequest{
		ClusterId: tid,
		Name:      name,
	})
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}

// GetClusterInstanceList 인스턴스 목록 조회
func GetClusterInstanceList(ctx context.Context, req *drms.UnprotectedInstanceListRequest) ([]*cms.ClusterInstance, error) {
	rsp, err := DefaultClusterManagerService.GetClusterInstanceList(ctx,
		&cms.ClusterInstanceListRequest{
			ClusterId:                 req.ClusterId,
			ClusterTenantId:           req.ClusterTenantId,
			ClusterAvailabilityZoneId: req.ClusterAvailabilityZoneId,
			ClusterHypervisorId:       req.ClusterHypervisorId,
			Name:                      req.Name,
		},
	)

	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetInstances(), nil
}

// GetClusterInstance 클러스터 인스턴스를 조회하기위한 함수이다.
func GetClusterInstance(ctx context.Context, cid, id uint64) (*cms.ClusterInstance, error) {
	rsp, err := DefaultClusterManagerService.GetClusterInstance(ctx, &cms.ClusterInstanceRequest{
		ClusterId: cid, ClusterInstanceId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetInstance(), nil
}

// GetClusterInstanceByUUID UUID 로 클러스터 인스턴스를 조회
func GetClusterInstanceByUUID(ctx context.Context, cid uint64, uuid string, sync bool) (*cms.ClusterInstance, error) {
	rsp, err := DefaultClusterManagerService.GetClusterInstanceByUUID(ctx, &cms.ClusterInstanceByUUIDRequest{
		ClusterId: cid,
		Sync:      sync,
		Uuid:      uuid,
	})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetInstance(), nil
}

// GetClusterInstanceSpecByUUID UUID 로 클러스터 spec 을 조회
func GetClusterInstanceSpecByUUID(ctx context.Context, cid uint64, uuid string) (*cms.ClusterInstanceSpec, error) {
	rsp, err := DefaultClusterManagerService.GetClusterInstanceSpecByUUID(ctx, &cms.ClusterInstanceSpecByUUIDRequest{
		ClusterId: cid,
		Uuid:      uuid,
		Sync:      true,
	})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetSpec(), nil
}

func GetClusterInstanceNumber(ctx context.Context, clusterID uint64) (*cms.ClusterInstanceNumberResponse, error) {
	req := &cms.ClusterInstanceNumberRequest{}
	if clusterID != 0 {
		req.ClusterId = clusterID
	}

	rsp, err := DefaultClusterManagerService.GetClusterInstanceNumber(ctx, req)
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp, nil
}

func CheckIsExistClusterInstance(ctx context.Context, clusterID uint64, uuid string) (bool, error) {
	req := &cms.CheckIsExistClusterInstanceRequest{}
	if clusterID != 0 {
		req.ClusterId = clusterID
	}

	if uuid != "" {
		req.Uuid = uuid
	}

	rsp, err := DefaultClusterManagerService.CheckIsExistClusterInstance(ctx, req)
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}

// CheckIsExistClusterInstanceSpec 인스턴스 Spec 이름으로 클러스터 인스턴스 Spec 존재 여부 확인
func CheckIsExistClusterInstanceSpec(ctx context.Context, cid uint64, name string) (bool, error) {
	rsp, err := DefaultClusterManagerService.CheckIsExistClusterInstanceSpec(ctx, &cms.CheckIsExistByNameRequest{
		ClusterId: cid,
		Name:      name,
	})
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}

// GetClusterStorage 클러스터 스토리지를 조회하기위한 함수이다.
func GetClusterStorage(ctx context.Context, cid, id uint64) (*cms.ClusterStorage, error) {
	rsp, err := DefaultClusterManagerService.GetClusterStorage(ctx, &cms.ClusterStorageRequest{
		ClusterId: cid, ClusterStorageId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetStorage(), nil
}

// GetClusterVolumeList 클러스터 볼륨 목록을 조회하기위한 함수이다.
func GetClusterVolumeList(ctx context.Context, req *cms.ClusterVolumeListRequest) (*cms.ClusterVolumeListResponse, error) {
	rsp, err := DefaultClusterManagerService.GetClusterVolumeList(ctx, req)
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp, nil
}

// GetClusterVolume 클러스터 볼륨을 조회하기위한 함수이다.
func GetClusterVolume(ctx context.Context, cid, id uint64) (*cms.ClusterVolume, error) {
	rsp, err := DefaultClusterManagerService.GetClusterVolume(ctx, &cms.ClusterVolumeRequest{
		ClusterId: cid, ClusterVolumeId: id})
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetVolume(), nil
}

// SyncClusterVolumeSnapshotList 클러스터 볼륨 스냅샷을 동기화하기위한 함수이다.
func SyncClusterVolumeSnapshotList(ctx context.Context, cid uint64) error {
	// TODO: client.WithRequestTimeout(time.Minute)는 디버깅을 위해 임시로 추가 된 코드. 스냅샷이 생성되기 이전에 작업에서 발생하는 지연들은 최소화되야하므로 디버깅후 지워야함.
	_, err := DefaultClusterManagerService.SyncClusterVolumeSnapshotList(ctx, &cms.SyncClusterVolumeSnapshotListRequest{
		Cluster: &cms.Cluster{Id: cid},
	}, client.WithRequestTimeout(time.Minute))
	if err != nil {
		return errors.IPCFailed(err)
	}

	return nil
}

// IsAccessibleCluster 는 접근 가능한 클러스터인지 확인하는 함수이다.
func IsAccessibleCluster(ctx context.Context, id uint64) error {
	_, err := GetCluster(ctx, id)
	if err != nil {
		switch errors.GetIPCStatusCode(err) {

		case errors.IPCStatusNotFound:
			return internal.NotFoundCluster(id)

		case errors.IPCStatusUnauthorized:
			return errors.UnauthorizedRequest(ctx)
		}

		return err
	}

	return nil
}

// CheckDeletableCluster 삭제 가능 클러스터 확인
func CheckDeletableCluster(req *drms.CheckDeletableClusterRequest) error {
	var err error

	err = database.Execute(func(db *gorm.DB) error {
		return db.
			Where(&model.ProtectionGroup{ProtectionClusterID: req.ClusterId}).
			First(&model.ProtectionGroup{}).Error
	})
	switch {
	case err == nil:
		return internal.ProtectionGroupExisted(req.ClusterId)

	case err != gorm.ErrRecordNotFound:
		return errors.UnusableDatabase(err)
	}

	var plan model.Plan
	err = database.Execute(func(db *gorm.DB) error {
		return db.
			Where(&model.Plan{RecoveryClusterID: req.ClusterId}).
			First(&plan).Error
	})
	switch {
	case err == nil:
		return internal.RecoveryPlanExisted(req.ClusterId, plan.ProtectionGroupID)

	case err != gorm.ErrRecordNotFound:
		return errors.UnusableDatabase(err)
	}

	return nil
}

// GetClusterVolumeGroupList 클러스터 볼륨 그룹 목록 조회
func GetClusterVolumeGroupList(ctx context.Context, cid uint64) ([]*cms.ClusterVolumeGroup, error) {
	rsp, err := DefaultClusterManagerService.GetClusterVolumeGroupList(ctx, &cms.GetClusterVolumeGroupListRequest{
		ClusterId: cid,
	})
	switch {
	case errors.GetIPCStatusCode(err) == errors.IPCStatusNoContent:
		return nil, nil
	case err != nil:
		return nil, errors.IPCFailed(err)
	}

	return rsp.GetVolumeGroups(), nil
}

// CheckIsExistClusterKeypair Keypair 이름으로 클러스터 Keypair 존재 여부 확인
func CheckIsExistClusterKeypair(ctx context.Context, cid uint64, name string) (bool, error) {
	rsp, err := DefaultClusterManagerService.CheckIsExistClusterKeypair(ctx, &cms.CheckIsExistByNameRequest{
		ClusterId: cid,
		Name:      name,
	})
	if err != nil {
		return false, errors.IPCFailed(err)
	}

	return rsp.GetIsExist(), nil
}
