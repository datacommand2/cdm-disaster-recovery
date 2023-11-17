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

func (b *TaskBuilder) buildNetworkCreateTask(ctx context.Context, txn store.Txn, network *cms.ClusterNetwork) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	networkMap, ok := b.SharedNetworkTaskIDMap[network.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, networkMap, network.Id, constant.MigrationTaskTypeCreateNetwork)
		if err != nil {
			logger.Errorf("[buildNetworkCreateTask] Could not build shared task: job(%d) network(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, network.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildNetworkCreateTask] Start: job(%d) network(%d)", b.RecoveryJob.RecoveryJobID, network.Id)

	if networkMap == nil {
		networkMap = make(map[uint64]string)
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, network.Tenant.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, network.Tenant.Id)
	}

	var dependencies []string
	var input = migrator.NetworkCreateTaskInputData{
		Network: network,
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   network.Id,
		ResourceName: network.Name,
		TypeCode:     constant.MigrationTaskTypeCreateNetwork,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildNetworkCreateTask] Errors occurred during PutTaskFunc: job(%d) network(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, network.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskKey] = task
	b.NetworkTaskIDMap[network.Id] = task.RecoveryJobTaskID
	networkMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedNetworkTaskIDMap[network.Id] = networkMap

	logger.Infof("[buildNetworkCreateTask] Success: job(%d) network(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, network.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildSubnetCreateTask(ctx context.Context, txn store.Txn, network *cms.ClusterNetwork, subnet *cms.ClusterSubnet) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	subnetMap, ok := b.SharedSubnetTaskIDMap[subnet.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, subnetMap, subnet.Id, constant.MigrationTaskTypeCreateSubnet)
		if err != nil {
			logger.Errorf("[buildSubnetCreateTask] Could not build shared task: job(%d) subnet(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, subnet.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildSubnetCreateTask] Start: job(%d) subnet(%d)", b.RecoveryJob.RecoveryJobID, subnet.Id)

	if subnetMap == nil {
		subnetMap = make(map[uint64]string)
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, network.Tenant.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, network.Tenant.Id)
	}

	var dependencies []string
	var input = migrator.SubnetCreateTaskInputData{
		Subnet: subnet,
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// network
	if taskID, err := b.buildNetworkCreateTask(ctx, txn, network); err == nil {
		input.Network = migrator.NewJSONRef(taskID, "/network")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   subnet.Id,
		ResourceName: subnet.Name,
		TypeCode:     constant.MigrationTaskTypeCreateSubnet,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildSubnetCreateTask] Errors occurred during PutTaskFunc: job(%d) subnet(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, subnet.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.SubnetTaskIDMap[subnet.Id] = task.RecoveryJobTaskID
	subnetMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedSubnetTaskIDMap[subnet.Id] = subnetMap

	logger.Infof("[buildSubnetCreateTask] Success: job(%d) subnet(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, subnet.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildNetworkDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildNetworkDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildNetworkDeleteTask] Could not get tenant create task: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}
	var input = migrator.NetworkDeleteTaskInputData{
		Tenant:  migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		Network: migrator.NewJSONRef(task.RecoveryJobTaskID, "/network"),
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
		TypeCode:      constant.MigrationTaskTypeDeleteNetwork,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildNetworkDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildNetworkDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
