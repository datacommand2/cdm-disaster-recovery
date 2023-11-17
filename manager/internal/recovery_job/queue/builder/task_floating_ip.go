package builder

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
)

func (b *TaskBuilder) buildFloatingIPCreateTask(ctx context.Context, txn store.Txn, plan *drms.FloatingIPRecoveryPlan) (string, error) {
	logger.Infof("[buildFloatingIPCreateTask] Start: job(%d) floatingIP(%d)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterFloatingIp.Id)

	if taskID, ok := b.FloatingIPTaskIDMap[plan.ProtectionClusterFloatingIp.Id]; ok {
		return taskID, nil
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, plan.ProtectionClusterFloatingIp.Tenant.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterFloatingIp.Tenant.Id)
	}

	extNetworkPlan := internal.GetExternalNetworkRecoveryPlan(
		b.RecoveryJobDetail.Plan, plan.ProtectionClusterFloatingIp.Network.Id,
	)

	if extNetworkPlan == nil {
		return "", internal.NotFoundExternalNetworkRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterFloatingIp.Network.Id)
	}

	var dependencies []string
	var input = migrator.FloatingIPCreateTaskInputData{
		Network:    extNetworkPlan.RecoveryClusterExternalNetwork,
		FloatingIP: plan.ProtectionClusterFloatingIp,
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   plan.ProtectionClusterFloatingIp.Id,
		ResourceName: plan.ProtectionClusterFloatingIp.IpAddress,
		TypeCode:     constant.MigrationTaskTypeCreateFloatingIP,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildFloatingIPCreateTask] Errors occurred during PutTaskFunc: job(%d) floatingIP(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterFloatingIp.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskKey] = task
	b.FloatingIPTaskIDMap[plan.ProtectionClusterFloatingIp.Id] = task.RecoveryJobTaskID

	logger.Infof("[buildFloatingIPCreateTask] Success: job(%d) floating IP(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterFloatingIp.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildFloatingIPDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildFloatingIPDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildFloatingIPDeleteTask] Could not get tenant create task: job(%d) typeCode(%s) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}

	var input = migrator.FloatingIPDeleteTaskInputData{
		Tenant:     migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		FloatingIP: migrator.NewJSONRef(task.RecoveryJobTaskID, "/floating_ip"),
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
		ReverseTaskID: task.RecoveryJobTaskID,
		TypeCode:      constant.MigrationTaskTypeDeleteFloatingIP,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildFloatingIPDeleteTask] Errors occurred during PutTaskFunc: job(%d) typeCode(%s) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildFloatingIPDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
