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
	"github.com/jinzhu/copier"
)

func (b *TaskBuilder) buildSecurityGroupCreateTask(ctx context.Context, txn store.Txn, c *cms.Cluster, tenant *cms.ClusterTenant, securityGroup *cms.ClusterSecurityGroup) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	securityGroupMap, ok := b.SharedSecurityGroupTaskIDMap[securityGroup.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, securityGroupMap, securityGroup.Id, constant.MigrationTaskTypeCreateSecurityGroup)
		if err != nil {
			logger.Errorf("[buildSecurityGroupCreateTask] Could not build shared task: job(%d) securityGroup(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, securityGroup.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildSecurityGroupCreateTask] Start: job(%d) securityGroup(%d)", b.RecoveryJob.RecoveryJobID, securityGroup.Id)

	if securityGroupMap == nil {
		securityGroupMap = make(map[uint64]string)
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, tenant.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, tenant.Id)
	}

	var dependencies []string
	var input = migrator.SecurityGroupCreateTaskInputData{
		SecurityGroup: new(cms.ClusterSecurityGroup),
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// security group
	if err := copier.CopyWithOption(input.SecurityGroup, securityGroup, copier.Option{DeepCopy: true}); err != nil {
		return "", errors.Unknown(err)
	}
	input.SecurityGroup.Name = fmt.Sprintf("%s.%s (recovered from %s cluster)", securityGroup.Name, b.RecoveryJob.RecoveryJobHash, c.Name)
	isExist, err := cluster.CheckIsExistClusterSecurityGroup(ctx, b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.SecurityGroup.Name)
	if isExist {
		err = internal.SameNameIsAlreadyExisted(b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.SecurityGroup.Name)
		logger.Errorf("[buildSecurityGroupCreateTask] Could not build create security group task. Cause: %+v", err)
		return "", err
	} else if err != nil {
		logger.Warnf("[buildSecurityGroupCreateTask] Error occurred during check security group name(%s) existence. Cause: %+v", input.SecurityGroup.Name, err)
	}

	input.SecurityGroup.Rules = nil

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   securityGroup.Id,
		ResourceName: securityGroup.Name,
		TypeCode:     constant.MigrationTaskTypeCreateSecurityGroup,
		Dependencies: dependencies,
		Input:        &input,
	})
	if err != nil {
		logger.Errorf("[buildSecurityGroupCreateTask] Errors occurred during PutTaskFunc: job(%d) securityGroup(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, securityGroup.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskKey] = task
	b.SecurityGroupTaskIDMap[securityGroup.Id] = task.RecoveryJobTaskID
	securityGroupMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedSecurityGroupTaskIDMap[securityGroup.Id] = securityGroupMap

	logger.Infof("[buildSecurityGroupCreateTask] Success: job(%d) securityGroup(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, securityGroup.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildSecurityGroupRuleCreateTask(ctx context.Context, txn store.Txn, c *cms.Cluster, tenant *cms.ClusterTenant, securityGroup *cms.ClusterSecurityGroup, rule *cms.ClusterSecurityGroupRule) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	securityGroupRuleMap, ok := b.SharedSecurityGroupRuleTaskIDMap[rule.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, securityGroupRuleMap, rule.Id, constant.MigrationTaskTypeCreateSecurityGroupRule)
		if err != nil {
			logger.Errorf("[buildSecurityGroupRuleCreateTask] Could not build shared task: job(%d) securityGroupRule(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, rule.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildSecurityGroupRuleCreateTask] Start: job(%d) securityGroupRule(%d)", b.RecoveryJob.RecoveryJobID, rule.Id)

	if securityGroupRuleMap == nil {
		securityGroupRuleMap = make(map[uint64]string)
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, tenant.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, tenant.Id)
	}

	var dependencies []string
	var input migrator.SecurityGroupRuleCreateTaskInputData

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// security group
	if taskID, err := b.buildSecurityGroupCreateTask(ctx, txn, c, tenant, securityGroup); err == nil {
		input.SecurityGroup = migrator.NewJSONRef(taskID, "/security_group")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// security group rule
	input.SecurityGroupRule = &migrator.SecurityGroupRuleInputData{
		ID:           rule.Id,
		UUID:         rule.Uuid,
		Direction:    rule.Direction,
		Description:  rule.Description,
		EtherType:    rule.EtherType,
		NetworkCidr:  rule.NetworkCidr,
		PortRangeMax: rule.PortRangeMax,
		PortRangeMin: rule.PortRangeMin,
		Protocol:     rule.Protocol,
	}

	if rule.RemoteSecurityGroup.GetId() != 0 {
		// RemoteSecurityGroup 의 Rule 들은 dependency 로 추가하지 않는다.
		// RemoteSecurityGroup 이 Rule 의 SecurityGroup 인 경우,
		// buildSecurityGroupRuleCreateTask recursive 호출이 끝나지 않는다.
		if taskID, ok := b.SecurityGroupTaskIDMap[securityGroup.Id]; ok {
			input.SecurityGroupRule.RemoteSecurityGroup = migrator.NewJSONRef(taskID, "/security_group")
			dependencies = append(dependencies, taskID)

		} else {
			rsg := getRecoverySecurityGroup(b.RecoveryJobDetail.Plan, rule.RemoteSecurityGroup.Id)
			if rsg == nil {
				return "", internal.NotFoundRecoverySecurityGroup(b.RecoveryJob.ProtectionGroupID, rule.RemoteSecurityGroup.Id)
			}

			if taskID, err := b.buildSecurityGroupCreateTask(ctx, txn, c, tenant, rsg); err == nil {
				input.SecurityGroupRule.RemoteSecurityGroup = migrator.NewJSONRef(taskID, "/security_group")
				dependencies = append(dependencies, taskID)
			} else {
				return "", err
			}

			for _, rr := range rsg.Rules {
				if _, err := b.buildSecurityGroupRuleCreateTask(ctx, txn, c, tenant, rsg, rr); err != nil {
					return "", err
				}
			}
		}
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		TypeCode:     constant.MigrationTaskTypeCreateSecurityGroupRule,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildSecurityGroupRuleCreateTask] Errors occurred during PutTaskFunc: job(%d) securityGroupRule(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, rule.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskKey] = task
	b.SecurityGroupRuleTaskIDMap[rule.Id] = task.RecoveryJobTaskID
	securityGroupRuleMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedSecurityGroupRuleTaskIDMap[rule.Id] = securityGroupRuleMap

	logger.Infof("[buildSecurityGroupRuleCreateTask] Success: job(%d) securityGroupRule(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, rule.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildSecurityGroupDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildSecurityGroupDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildSecurityGroupDeleteTask] Could not get tenant create task: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}
	var input = migrator.SecurityGroupDeleteTaskInputData{
		Tenant:        migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		SecurityGroup: migrator.NewJSONRef(task.RecoveryJobTaskID, "/security_group"),
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
		TypeCode:      constant.MigrationTaskTypeDeleteSecurityGroup,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildSecurityGroupDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildSecurityGroupDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
