package builder

import (
	"context"
	"fmt"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/copier"
)

func (b *TaskBuilder) buildTenantCreateTask(ctx context.Context, txn store.Txn, plan *drms.TenantRecoveryPlan) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	tenantMap, ok := b.SharedTenantTaskIDMap[plan.ProtectionClusterTenant.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, tenantMap, plan.ProtectionClusterTenant.Id, constant.MigrationTaskTypeCreateTenant)
		if err != nil {
			logger.Errorf("[buildTenantCreateTask] Could not build shared task: job(%d) tenant(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterTenant.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}
	if tenantMap == nil {
		tenantMap = make(map[uint64]string)
	}

	logger.Infof("[buildTenantCreateTask] Start: job(%d) tenant(%d)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterTenant.Id)

	var input = migrator.TenantCreateTaskInputData{
		Tenant: new(cms.ClusterTenant),
	}
	if err := copier.CopyWithOption(input.Tenant, plan.ProtectionClusterTenant, copier.Option{DeepCopy: true}); err != nil {
		return "", errors.Unknown(err)
	}

	input.Tenant.Name = fmt.Sprintf("%s.%s", plan.RecoveryClusterTenantMirrorName, b.RecoveryJob.RecoveryJobHash)
	isExist, err := cluster.CheckIsExistClusterTenant(ctx, b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.Tenant.Name)
	if isExist {
		err = internal.SameNameIsAlreadyExisted(b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.Tenant.Name)
		logger.Errorf("[buildTenantCreateTask] Could not build create tenant task. Cause: %+v", err)
		return "", err
	} else if err != nil {
		logger.Warnf("[buildTenantCreateTask] Error occurred during check tenant name(%s) existence. Cause: %+v", input.Tenant.Name, err)
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   plan.ProtectionClusterTenant.Id,
		ResourceName: plan.ProtectionClusterTenant.Name,
		TypeCode:     constant.MigrationTaskTypeCreateTenant,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildTenantCreateTask] Errors occurred during PutTaskFunc: job(%d) tenant(%d). Cause: %+v", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterTenant.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.TenantTaskIDMap[plan.ProtectionClusterTenant.Id] = task.RecoveryJobTaskID
	tenantMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedTenantTaskIDMap[plan.ProtectionClusterTenant.Id] = tenantMap

	logger.Infof("[buildTenantCreateTask] Success: job(%d) tenant(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterTenant.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildTenantDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildTenantDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
	}
	var input = migrator.TenantDeleteTaskInputData{
		Tenant: migrator.NewJSONRef(task.RecoveryJobTaskID, "/tenant"),
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
		TypeCode:      constant.MigrationTaskTypeDeleteTenant,
		ResourceID:    task.ResourceID,
		ResourceName:  task.ResourceName,
		SharedTaskKey: task.SharedTaskKey,
		Dependencies:  dependencies,
		ReverseTaskID: task.RecoveryJobTaskID,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildTenantDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildTenantDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
