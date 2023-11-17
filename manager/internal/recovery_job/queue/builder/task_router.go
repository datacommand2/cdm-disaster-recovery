package builder

import (
	"context"
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
)

func (b *TaskBuilder) buildRouterCreateTask(ctx context.Context, txn store.Txn, router *cms.ClusterRouter) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	routerMap, ok := b.SharedRouterTaskIDMap[router.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, routerMap, router.Id, constant.MigrationTaskTypeCreateRouter)
		if err != nil {
			logger.Errorf("[buildRouterCreateTask] Could not build shared task: job(%d) router(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, router.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildRouterCreateTask] Run: job(%d) router(%d)", b.RecoveryJob.RecoveryJobID, router.Id)

	if routerMap == nil {
		routerMap = make(map[uint64]string)
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, router.Tenant.Id,
	)
	routerPlan := internal.GetRouterRecoveryPlan(
		b.RecoveryJobDetail.Plan, router.Id,
	)
	extNetworkPlan := internal.GetExternalNetworkRecoveryPlan(
		b.RecoveryJobDetail.Plan, router.ExternalRoutingInterfaces[0].Network.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, router.Tenant.Id)
	}
	if extNetworkPlan == nil {
		return "", internal.NotFoundExternalNetworkRecoveryPlan(b.RecoveryJobDetail.Plan.Id, router.ExternalRoutingInterfaces[0].Network.Id)
	}

	var dependencies []string
	var input migrator.RouterCreateTaskInputData

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// router
	var externalInterfaces []*cms.ClusterNetworkRoutingInterface
	if routerPlan != nil {
		externalInterfaces = routerPlan.RecoveryClusterExternalRoutingInterfaces
	}

	if len(externalInterfaces) == 0 {
		var extNetwork *cms.ClusterNetwork
		if routerPlan != nil {
			extNetwork = routerPlan.RecoveryClusterExternalNetwork
		} else {
			extNetwork = extNetworkPlan.RecoveryClusterExternalNetwork
		}

		externalInterfaces = []*cms.ClusterNetworkRoutingInterface{{
			Network:   extNetwork,
			Subnet:    extNetwork.Subnets[0],
			IpAddress: router.ExternalRoutingInterfaces[0].GetIpAddress(),
		}}
	}

	var internalInterfaces []*migrator.InternalRoutingInterfaceInputData
	for _, iface := range router.InternalRoutingInterfaces {
		network := getRecoveryNetwork(b.RecoveryJobDetail.Plan, iface.Network.Id)
		if network == nil {
			// 라우팅 인터페이스가 연결될 내부 네트워크가 복구대상 네트워크가 아닌 경우
			// 해당 라우팅 인터페이스는 복구하지 않고 무시한다.
			continue
		}

		ifaceInput := migrator.InternalRoutingInterfaceInputData{
			IPAddress: iface.IpAddress,
		}

		internalInterfaces = append(internalInterfaces, &ifaceInput)

		if taskID, err := b.buildNetworkCreateTask(ctx, txn, network); err == nil {
			ifaceInput.Network = migrator.NewJSONRef(taskID, "/network")
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}

		for _, subnet := range network.Subnets {
			taskID, err := b.buildSubnetCreateTask(ctx, txn, network, subnet)
			if err == nil {
				dependencies = append(dependencies, taskID)
			} else {
				return "", err
			}

			if subnet.Id == iface.Subnet.Id {
				ifaceInput.Subnet = migrator.NewJSONRef(taskID, "/subnet")
			}
		}
	}

	input.Router = &migrator.RouterInputData{
		ID:                        router.Id,
		Name:                      router.Name,
		Description:               router.Description,
		InternalRoutingInterfaces: internalInterfaces,
		ExternalRoutingInterfaces: externalInterfaces,
		ExtraRoutes:               router.ExtraRoutes,
		State:                     router.State,
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   router.Id,
		ResourceName: router.Name,
		TypeCode:     constant.MigrationTaskTypeCreateRouter,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildRouterCreateTask] Errors occurred during PutTaskFunc: job(%d) router(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, router.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskKey] = task
	b.RouterTaskIDMap[router.Id] = task.RecoveryJobTaskID
	routerMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedRouterTaskIDMap[router.Id] = routerMap

	logger.Infof("[buildRouterCreateTask] Success: job(%d) router(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, router.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildRouterDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildRouterDeleteTask] Run: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildRouterDeleteTask] Could not get tenant create task(%s). Cause: %+v", task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}
	var input = migrator.RouterDeleteTaskInputData{
		Tenant: migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		Router: migrator.NewJSONRef(task.RecoveryJobTaskID, "/router"),
	}

	// task 의 후행 Task 들을 찾아 선행 ClearTask 로 설정한다.
	for _, t := range b.RecoveryJobTaskMap {
		if !isDependencyTask(task, t) {
			continue
		}

		// 후행 Task 의 ClearTask 를 생성하고, 선행으로 설정한다.
		if task, err := b.buildReverseTask(txn, t); task != nil {
			dependencies = append(dependencies, task.RecoveryJobTaskID)

		} else if err == errTaskBuildSkipped {
			return nil, errTaskBuildSkipped

		} else if err != nil {
			return nil, err
		}
	}

	t, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:    task.ResourceID,
		ResourceName:  task.ResourceName,
		SharedTaskKey: task.SharedTaskKey,
		ReverseTaskID: task.RecoveryJobTaskID,
		TypeCode:      constant.MigrationTaskTypeDeleteRouter,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildRouterDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildRouterDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
