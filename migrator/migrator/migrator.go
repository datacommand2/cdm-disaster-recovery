package migrator

import (
	cmsClient "10.1.1.220/cdm/cdm-center/services/cluster-manager/client"
	centerConstant "10.1.1.220/cdm/cdm-center/services/cluster-manager/constant"
	cms "10.1.1.220/cdm/cdm-center/services/cluster-manager/proto"
	"10.1.1.220/cdm/cdm-cloud/common/errors"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/constant"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/migrator"

	"github.com/micro/go-micro/v2/client"
	"github.com/micro/go-micro/v2/client/grpc"

	"context"
)

// DefaultClusterManagerService ClusterManagerService
var DefaultClusterManagerService = cms.NewClusterManagerService(centerConstant.ServiceName, grpc.NewClient())

func getClusterConnectionInfo(cluster *cms.Cluster, tenant *cms.ClusterTenant) *cms.ClusterConnectionInfo {
	conn := cms.ClusterConnectionInfo{
		TypeCode:     cluster.TypeCode,
		ApiServerUrl: cluster.ApiServerUrl,
		Credential:   cluster.Credential,
	}

	if tenant != nil {
		conn.TenantId = tenant.Uuid
	}

	return &conn
}

// createTenantTaskFunc 테넌트 생성 Task 함수
func createTenantTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.TenantCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterTenant(ctx, &cms.CreateClusterTenantRequest{
		Conn:   getClusterConnectionInfo(w.job.RecoveryCluster, nil),
		Tenant: input.Tenant,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.TenantCreateTaskOutput{
		Tenant: rsp.Tenant,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createSecurityGroupTaskFunc 보안 그룹 생성 Task 함수
func createSecurityGroupTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.SecurityGroupCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterSecurityGroup(ctx, &cms.CreateClusterSecurityGroupRequest{
		Conn:          getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:        input.Tenant,
		SecurityGroup: input.SecurityGroup,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.SecurityGroupCreateTaskOutput{
		SecurityGroup: rsp.SecurityGroup,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createSecurityGroupRuleTaskFunc 보안 그룹 규칙 생성 Task 함수
func createSecurityGroupRuleTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.SecurityGroupRuleCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterSecurityGroupRule(ctx, &cms.CreateClusterSecurityGroupRuleRequest{
		Conn:              getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:            input.Tenant,
		SecurityGroup:     input.SecurityGroup,
		SecurityGroupRule: input.SecurityGroupRule,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.SecurityGroupRuleCreateTaskOutput{
		SecurityGroupRule: rsp.SecurityGroupRule,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// copyVolumeTaskFunc 볼륨 copy Task 함수
func copyVolumeTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.VolumeCopyTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	input.Volume.Id = input.VolumeID

	rsp, err := DefaultClusterManagerService.CopyClusterVolume(ctx, &cms.CopyClusterVolumeRequest{
		Conn:           getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:         input.Tenant,
		SourceStorage:  input.SourceStorage,
		TargetStorage:  input.TargetStorage,
		TargetMetadata: input.TargetMetadata,
		Volume:         input.Volume,
		Snapshots:      input.Snapshots,
	}, client.WithRequestTimeout(volumeCopyRequestTimeout), client.WithRetries(0))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	if rsp.GetSnapshots() == nil {
		rsp.Snapshots = []*cms.ClusterVolumeSnapshot{}
	}

	return &migrator.VolumeCopyTaskOutput{
		SourceStorage: rsp.SourceStorage,
		TargetStorage: rsp.TargetStorage,
		Volume:        rsp.Volume,
		Snapshots:     rsp.Snapshots,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// importVolumeTaskFunc 볼륨 import Task 함수
func importVolumeTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.VolumeImportTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	input.Volume.Id = input.VolumeID

	rsp, err := DefaultClusterManagerService.ImportClusterVolume(ctx, &cms.ImportClusterVolumeRequest{
		Conn:           getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:         input.Tenant,
		SourceStorage:  input.SourceStorage,
		TargetStorage:  input.TargetStorage,
		TargetMetadata: input.TargetMetadata,
		Volume:         input.Volume,
		Snapshots:      input.Snapshots,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(0))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	if rsp.GetSnapshotPairs() == nil {
		rsp.SnapshotPairs = []*cms.SnapshotPair{}
	}

	return &migrator.VolumeImportTaskOutput{
		SourceStorage: rsp.SourceStorage,
		TargetStorage: rsp.TargetStorage,
		VolumePair:    rsp.VolumePair,
		SnapshotPairs: rsp.SnapshotPairs,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createSpecTaskFunc 스팩 생성 Task 함수
func createSpecTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.InstanceSpecCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterInstanceSpec(ctx, &cms.CreateClusterInstanceSpecRequest{
		Conn:       getClusterConnectionInfo(w.job.RecoveryCluster, nil),
		Spec:       input.Spec,
		ExtraSpecs: input.ExtraSpecs,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.InstanceSpecCreateTaskOutput{
		Spec: rsp.Spec,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createKeypairTaskFunc keypair 생성 Task 함수
func createKeypairTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.KeypairCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterKeypair(ctx, &cms.CreateClusterKeypairRequest{
		Conn:    getClusterConnectionInfo(w.job.RecoveryCluster, nil),
		Keypair: input.Keypair,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.KeypairCreateTaskOutput{
		Keypair: rsp.Keypair,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createFloatingIPTaskFunc floatingIP 생성 Task 함수
func createFloatingIPTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.FloatingIPCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterFloatingIP(ctx, &cms.CreateClusterFloatingIPRequest{
		Conn:       getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:     input.Tenant,
		Network:    input.Network,
		FloatingIp: input.FloatingIP,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.FloatingIPCreateTaskOutput{
		FloatingIP: rsp.FloatingIp,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createNetworkTaskFunc 네트워크 생성 Task 함수
func createNetworkTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.NetworkCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterNetwork(ctx, &cms.CreateClusterNetworkRequest{
		Conn:    getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:  input.Tenant,
		Network: input.Network,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.NetworkCreateTaskOutput{
		Network: rsp.Network,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createSubnetTaskFunc 서브넷 생성 Task 함수
func createSubnetTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.SubnetCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterSubnet(ctx, &cms.CreateClusterSubnetRequest{
		Conn:    getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:  input.Tenant,
		Network: input.Network,
		Subnet:  input.Subnet,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.SubnetCreateTaskOutput{
		Subnet: rsp.Subnet,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createRouterTaskFunc 라우터 생성 Task 함수
func createRouterTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.RouterCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterRouter(ctx, &cms.CreateClusterRouterRequest{
		Conn:   getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant: input.Tenant,
		Router: input.Router,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(1))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.RouterCreateTaskOutput{
		Router: rsp.Router,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// createAndDiagnosisInstanceTaskFunc 인스턴스 생성 & 정상 동작 여부 확인(기동) Task 함수
func createAndDiagnosisInstanceTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.InstanceCreateTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.CreateClusterInstance(ctx, &cms.CreateClusterInstanceRequest{
		Conn:             getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:           input.Tenant,
		AvailabilityZone: input.AvailabilityZone,
		Hypervisor:       input.Hypervisor,
		Spec:             input.Spec,
		Keypair:          input.Keypair,
		Instance:         input.Instance,
		Networks:         input.Networks,
		SecurityGroups:   input.SecurityGroups,
		Volumes:          input.Volumes,
	}, client.WithRequestTimeout(instanceCreationRequestTimeout), client.WithRetries(0))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.InstanceCreateOutput{
		Instance: rsp.Instance,
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// stopInstanceTaskFunc 인스턴스 중지 Task 함수
func stopInstanceTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.InstanceStopTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.StopClusterInstance(ctx, &cms.StopClusterInstanceRequest{
		Conn:     getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:   input.Tenant,
		Instance: input.Instance,
	}, client.WithRequestTimeout(defaultCreationRequestTimeout), client.WithRetries(0))
	if err != nil {
		return nil, errors.IPCFailed(err)
	}

	return &migrator.InstanceStopTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteInstanceTaskFunc 인스턴스 삭제 Task 함수
func deleteInstanceTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.InstanceDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterInstance(ctx, &cms.DeleteClusterInstanceRequest{
		Conn:     getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:   input.Tenant,
		Instance: input.Instance,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound {
			return &migrator.InstanceDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_instance.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.InstanceDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteSpecTaskFunc 스팩 삭제 Task 함수
func deleteSpecTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.InstanceSpecDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	input.Spec.Cluster = w.job.RecoveryCluster
	rsp, err := DefaultClusterManagerService.DeleteClusterInstanceSpec(ctx, &cms.DeleteClusterInstanceSpecRequest{
		Conn: getClusterConnectionInfo(w.job.RecoveryCluster, nil),
		Spec: input.Spec,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.InstanceSpecDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_instance_spec.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.InstanceSpecDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteKeypairTaskFunc keypair 삭제 Task 함수
func deleteKeypairTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.KeypairDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterKeypair(ctx, &cms.DeleteClusterKeypairRequest{
		Conn:    getClusterConnectionInfo(w.job.RecoveryCluster, nil),
		Keypair: input.Keypair,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.KeypairDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_keypair.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.KeypairDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// unmanageVolumeTaskFunc 볼륨 unmanage Task 함수
func unmanageVolumeTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.VolumeUnmanageTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.UnmanageClusterVolume(ctx, &cms.UnmanageClusterVolumeRequest{
		Conn:           getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:         input.Tenant,
		SourceStorage:  input.SourceStorage,
		TargetStorage:  input.TargetStorage,
		TargetMetadata: input.TargetMetadata,
		VolumeId:       input.VolumeID,
		VolumePair:     input.VolumePair,
		SnapshotPairs:  input.SnapshotPairs,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		// UnmanageClusterVolume 에서 404 를 return 해주지 않음
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.VolumeUnmanageTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.unmanage_cluster_volume.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.VolumeUnmanageTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteVolumeCopyTaskFunc 볼륨 copy 삭제 Task 함수
func deleteVolumeCopyTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.VolumeCopyDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterVolumeCopy(ctx, &cms.DeleteClusterVolumeCopyRequest{
		Conn:           getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:         input.Tenant,
		SourceStorage:  input.SourceStorage,
		TargetStorage:  input.TargetStorage,
		TargetMetadata: input.TargetMetadata,
		Volume:         input.Volume,
		VolumeId:       input.VolumeID,
		Snapshots:      input.Snapshots,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		//if errors.Equal(err, cmsClient.ErrNotFound) {
		//	return &migrator.VolumeCopyDeleteTaskOutput{
		//		Message: &migrator.Message{
		//			Code:     "cdm-center.manager.delete_cluster_volume_copy.success",
		//			Contents: "",
		//		},
		//	}, nil
		//}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.VolumeCopyDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteSecurityGroupTaskFunc 보안 그룹 삭제 Task 함수
func deleteSecurityGroupTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.SecurityGroupDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterSecurityGroup(ctx, &cms.DeleteClusterSecurityGroupRequest{
		Conn:          getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:        input.Tenant,
		SecurityGroup: input.SecurityGroup,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.SecurityGroupDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_security_group.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.SecurityGroupDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteFloatingIPTaskFunc floatingIP 삭제 Task 함수
func deleteFloatingIPTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.FloatingIPDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterFloatingIP(ctx, &cms.DeleteClusterFloatingIPRequest{
		Conn:       getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:     input.Tenant,
		FloatingIp: input.FloatingIP,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.FloatingIPDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_floating_ip.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.FloatingIPDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteRouterTaskFunc 라우터 삭제 Task 함수
func deleteRouterTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.RouterDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterRouter(ctx, &cms.DeleteClusterRouterRequest{
		Conn:   getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant: input.Tenant,
		Router: input.Router,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.RouterDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_router.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.RouterDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteNetworkTaskFunc 네트워크 삭제 Task 함수
func deleteNetworkTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.NetworkDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterNetwork(ctx, &cms.DeleteClusterNetworkRequest{
		Conn:    getClusterConnectionInfo(w.job.RecoveryCluster, input.Tenant),
		Tenant:  input.Tenant,
		Network: input.Network,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.NetworkDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_network.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.NetworkDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

// deleteTenantTaskFunc 테넌트 삭제 Task 함수
func deleteTenantTaskFunc(ctx context.Context, w *Worker) (interface{}, error) {
	var input migrator.TenantDeleteTaskInput
	if err := w.task.ResolveInput(&input); err != nil {
		return nil, err
	}

	rsp, err := DefaultClusterManagerService.DeleteClusterTenant(ctx, &cms.DeleteClusterTenantRequest{
		Conn:   getClusterConnectionInfo(w.job.RecoveryCluster, nil),
		Tenant: input.Tenant,
	}, client.WithRequestTimeout(defaultDeletionRequestTimeout), client.WithRetries(0))
	if err != nil {
		if errors.Equal(err, cmsClient.ErrNotFound) {
			return &migrator.TenantDeleteTaskOutput{
				Message: &migrator.Message{
					Code:     "cdm-center.manager.delete_cluster_tenant.success",
					Contents: "",
				},
			}, nil
		}
		return nil, errors.IPCFailed(err)
	}

	return &migrator.TenantDeleteTaskOutput{
		Message: &migrator.Message{
			Code:     rsp.Message.Code,
			Contents: rsp.Message.Contents,
		},
	}, nil
}

type taskFunc func(ctx context.Context, w *Worker) (interface{}, error)

var taskFuncMap = map[string]taskFunc{
	constant.MigrationTaskTypeCreateTenant:               createTenantTaskFunc,
	constant.MigrationTaskTypeCreateSecurityGroup:        createSecurityGroupTaskFunc,
	constant.MigrationTaskTypeCreateSecurityGroupRule:    createSecurityGroupRuleTaskFunc,
	constant.MigrationTaskTypeCopyVolume:                 copyVolumeTaskFunc,
	constant.MigrationTaskTypeImportVolume:               importVolumeTaskFunc,
	constant.MigrationTaskTypeCreateSpec:                 createSpecTaskFunc,
	constant.MigrationTaskTypeCreateKeypair:              createKeypairTaskFunc,
	constant.MigrationTaskTypeCreateFloatingIP:           createFloatingIPTaskFunc,
	constant.MigrationTaskTypeCreateNetwork:              createNetworkTaskFunc,
	constant.MigrationTaskTypeCreateSubnet:               createSubnetTaskFunc,
	constant.MigrationTaskTypeCreateRouter:               createRouterTaskFunc,
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: createAndDiagnosisInstanceTaskFunc,
	constant.MigrationTaskTypeStopInstance:               stopInstanceTaskFunc,
	constant.MigrationTaskTypeDeleteInstance:             deleteInstanceTaskFunc,
	constant.MigrationTaskTypeDeleteSpec:                 deleteSpecTaskFunc,
	constant.MigrationTaskTypeDeleteKeypair:              deleteKeypairTaskFunc,
	constant.MigrationTaskTypeUnmanageVolume:             unmanageVolumeTaskFunc,
	constant.MigrationTaskTypeDeleteVolumeCopy:           deleteVolumeCopyTaskFunc,
	constant.MigrationTaskTypeDeleteSecurityGroup:        deleteSecurityGroupTaskFunc,
	constant.MigrationTaskTypeDeleteFloatingIP:           deleteFloatingIPTaskFunc,
	constant.MigrationTaskTypeDeleteRouter:               deleteRouterTaskFunc,
	constant.MigrationTaskTypeDeleteNetwork:              deleteNetworkTaskFunc,
	constant.MigrationTaskTypeDeleteTenant:               deleteTenantTaskFunc,
}

var taskIgnoredLogCodeMap = map[string]string{
	constant.MigrationTaskTypeCreateTenant:               "cdm-dr.migrator.create_tenant.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateSecurityGroup:        "cdm-dr.migrator.create_security_group.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateSecurityGroupRule:    "cdm-dr.migrator.create_security_group_rule.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCopyVolume:                 "cdm-dr.migrator.copy_volume.ignored-dependency_task_failed",
	constant.MigrationTaskTypeImportVolume:               "cdm-dr.migrator.import_volume.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateSpec:                 "cdm-dr.migrator.create_spec.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateKeypair:              "cdm-dr.migrator.create_keypair.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateFloatingIP:           "cdm-dr.migrator.create_floating_ip.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateNetwork:              "cdm-dr.migrator.create_network.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateSubnet:               "cdm-dr.migrator.create_subnet.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateRouter:               "cdm-dr.migrator.create_router.ignored-dependency_task_failed",
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: "cdm-dr.migrator.create_and_diagnosis_instance.ignored-dependency_task_failed",
	constant.MigrationTaskTypeStopInstance:               "cdm-dr.migrator.stop_instance.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteInstance:             "cdm-dr.migrator.delete_instance.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteSpec:                 "cdm-dr.migrator.delete_spec.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteKeypair:              "cdm-dr.migrator.delete_keypair.ignored-dependency_task_failed",
	constant.MigrationTaskTypeUnmanageVolume:             "cdm-dr.migrator.unmanage_volume.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteVolumeCopy:           "cdm-dr.migrator.delete_volume_copy.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteSecurityGroup:        "cdm-dr.migrator.delete_security_group.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteFloatingIP:           "cdm-dr.migrator.delete_floating_ip.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteRouter:               "cdm-dr.migrator.delete_router.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteNetwork:              "cdm-dr.migrator.delete_network.ignored-dependency_task_failed",
	constant.MigrationTaskTypeDeleteTenant:               "cdm-dr.migrator.delete_tenant.ignored-dependency_task_failed",
}

var taskCanceledLogCodeMap = map[string]string{
	constant.MigrationTaskTypeCreateTenant:               "cdm-dr.migrator.create_tenant.canceled",
	constant.MigrationTaskTypeCreateSecurityGroup:        "cdm-dr.migrator.create_security_group.canceled",
	constant.MigrationTaskTypeCreateSecurityGroupRule:    "cdm-dr.migrator.create_security_group_rule.canceled",
	constant.MigrationTaskTypeCopyVolume:                 "cdm-dr.migrator.copy_volume.canceled",
	constant.MigrationTaskTypeImportVolume:               "cdm-dr.migrator.import_volume.canceled",
	constant.MigrationTaskTypeCreateSpec:                 "cdm-dr.migrator.create_spec.canceled",
	constant.MigrationTaskTypeCreateKeypair:              "cdm-dr.migrator.create_keypair.canceled",
	constant.MigrationTaskTypeCreateFloatingIP:           "cdm-dr.migrator.create_floating_ip.canceled",
	constant.MigrationTaskTypeCreateNetwork:              "cdm-dr.migrator.create_network.canceled",
	constant.MigrationTaskTypeCreateSubnet:               "cdm-dr.migrator.create_subnet.canceled",
	constant.MigrationTaskTypeCreateRouter:               "cdm-dr.migrator.create_router.canceled",
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: "cdm-dr.migrator.create_and_diagnosis_instance.canceled",
	constant.MigrationTaskTypeStopInstance:               "cdm-dr.migrator.stop_instance.canceled",
	constant.MigrationTaskTypeDeleteInstance:             "cdm-dr.migrator.delete_instance.canceled",
	constant.MigrationTaskTypeDeleteSpec:                 "cdm-dr.migrator.delete_spec.canceled",
	constant.MigrationTaskTypeDeleteKeypair:              "cdm-dr.migrator.delete_keypair.canceled",
	constant.MigrationTaskTypeUnmanageVolume:             "cdm-dr.migrator.unmanage_volume.canceled",
	constant.MigrationTaskTypeDeleteVolumeCopy:           "cdm-dr.migrator.delete_volume_copy.canceled",
	constant.MigrationTaskTypeDeleteSecurityGroup:        "cdm-dr.migrator.delete_security_group.canceled",
	constant.MigrationTaskTypeDeleteFloatingIP:           "cdm-dr.migrator.delete_floating_ip.canceled",
	constant.MigrationTaskTypeDeleteRouter:               "cdm-dr.migrator.delete_router.canceled",
	constant.MigrationTaskTypeDeleteNetwork:              "cdm-dr.migrator.delete_network.canceled",
	constant.MigrationTaskTypeDeleteTenant:               "cdm-dr.migrator.delete_tenant.canceled",
}

var taskFailedLogCodeMap = map[string]string{
	constant.MigrationTaskTypeCreateTenant:               "cdm-dr.migrator.create_tenant.failed",
	constant.MigrationTaskTypeCreateSecurityGroup:        "cdm-dr.migrator.create_security_group.failed",
	constant.MigrationTaskTypeCreateSecurityGroupRule:    "cdm-dr.migrator.create_security_group_rule.failed",
	constant.MigrationTaskTypeCopyVolume:                 "cdm-dr.migrator.copy_volume.failed",
	constant.MigrationTaskTypeImportVolume:               "cdm-dr.migrator.import_volume.failed",
	constant.MigrationTaskTypeCreateSpec:                 "cdm-dr.migrator.create_spec.failed",
	constant.MigrationTaskTypeCreateKeypair:              "cdm-dr.migrator.create_keypair.failed",
	constant.MigrationTaskTypeCreateFloatingIP:           "cdm-dr.migrator.create_floating_ip.failed",
	constant.MigrationTaskTypeCreateNetwork:              "cdm-dr.migrator.create_network.failed",
	constant.MigrationTaskTypeCreateSubnet:               "cdm-dr.migrator.create_subnet.failed",
	constant.MigrationTaskTypeCreateRouter:               "cdm-dr.migrator.create_router.failed",
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: "cdm-dr.migrator.create_and_diagnosis_instance.failed",
	constant.MigrationTaskTypeStopInstance:               "cdm-dr.migrator.stop_instance.failed",
	constant.MigrationTaskTypeDeleteInstance:             "cdm-dr.migrator.delete_instance.failed",
	constant.MigrationTaskTypeDeleteSpec:                 "cdm-dr.migrator.delete_spec.failed",
	constant.MigrationTaskTypeDeleteKeypair:              "cdm-dr.migrator.delete_keypair.failed",
	constant.MigrationTaskTypeUnmanageVolume:             "cdm-dr.migrator.unmanage_volume.failed",
	constant.MigrationTaskTypeDeleteVolumeCopy:           "cdm-dr.migrator.delete_volume_copy.failed",
	constant.MigrationTaskTypeDeleteSecurityGroup:        "cdm-dr.migrator.delete_security_group.failed",
	constant.MigrationTaskTypeDeleteFloatingIP:           "cdm-dr.migrator.delete_floating_ip.failed",
	constant.MigrationTaskTypeDeleteRouter:               "cdm-dr.migrator.delete_router.failed",
	constant.MigrationTaskTypeDeleteNetwork:              "cdm-dr.migrator.delete_network.failed",
	constant.MigrationTaskTypeDeleteTenant:               "cdm-dr.migrator.delete_tenant.failed",
}

var taskSkippedLogCodeMap = map[string]string{
	// instance, volume 은 shared task 대상이 아님
	constant.MigrationTaskTypeDeleteSpec:          "cdm-dr.migrator.delete_spec.skipped-shared_task_in_used",
	constant.MigrationTaskTypeDeleteKeypair:       "cdm-dr.migrator.delete_keypair.skipped-shared_task_in_used",
	constant.MigrationTaskTypeDeleteSecurityGroup: "cdm-dr.migrator.delete_security_group.skipped-shared_task_in_used",
	constant.MigrationTaskTypeDeleteFloatingIP:    "cdm-dr.migrator.delete_floating_ip.skipped-shared_task_in_used",
	constant.MigrationTaskTypeDeleteRouter:        "cdm-dr.migrator.delete_router.skipped-shared_task_in_used",
	constant.MigrationTaskTypeDeleteNetwork:       "cdm-dr.migrator.delete_network.skipped-shared_task_in_used",
	constant.MigrationTaskTypeDeleteTenant:        "cdm-dr.migrator.delete_tenant.skipped-shared_task_in_used",
}
