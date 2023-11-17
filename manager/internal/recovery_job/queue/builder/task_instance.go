package builder

import (
	"context"
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
)

func (b *TaskBuilder) buildInstanceCreateTask(ctx context.Context, txn store.Txn, plan *drms.InstanceRecoveryPlan) (string, error) {
	if taskID, ok := b.InstanceTaskIDMap[plan.ProtectionClusterInstance.Id]; ok {
		return taskID, nil
	}

	logger.Infof("[buildInstanceCreateTask] Start: job(%d) instance(%d:%s)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id, plan.ProtectionClusterInstance.Name)

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, plan.ProtectionClusterInstance.Tenant.Id,
	)
	zonePlan := internal.GetAvailabilityZoneRecoveryPlan(
		b.RecoveryJobDetail.Plan, plan.ProtectionClusterInstance.AvailabilityZone.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterInstance.Tenant.Id)
	}
	if zonePlan == nil {
		return "", internal.NotFoundAvailabilityZoneRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterInstance.AvailabilityZone.Id)
	}

	var dependencies []string
	var input = migrator.InstanceCreateTaskInputData{
		Instance:   plan.ProtectionClusterInstance,
		InstanceID: plan.ProtectionClusterInstance.Id,
		Hypervisor: plan.RecoveryClusterHypervisor,
	}

	// availability zone
	if plan.RecoveryClusterAvailabilityZone.GetId() == 0 {
		input.AvailabilityZone = zonePlan.RecoveryClusterAvailabilityZone
	} else {
		input.AvailabilityZone = plan.RecoveryClusterAvailabilityZone
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// security group
	for _, sg := range plan.ProtectionClusterInstance.SecurityGroups {
		taskID, err := b.buildSecurityGroupCreateTask(ctx, txn, plan.ProtectionClusterInstance.Cluster, plan.ProtectionClusterInstance.Tenant, sg)
		if err != nil {
			return "", err
		}

		input.SecurityGroups = append(input.SecurityGroups, migrator.NewJSONRef(taskID, "/security_group"))
		dependencies = append(dependencies, taskID)

		for _, rule := range sg.Rules {
			if taskRuleID, err := b.buildSecurityGroupRuleCreateTask(ctx, txn,
				plan.ProtectionClusterInstance.Cluster,
				plan.ProtectionClusterInstance.Tenant,
				sg, rule,
			); err == nil {
				dependencies = append(dependencies, taskRuleID)
			} else {
				return "", err
			}
		}
	}

	// instance network
	for _, n := range plan.ProtectionClusterInstance.Networks {
		var networkInput = migrator.InstanceNetworkInputData{
			DhcpFlag:  n.DhcpFlag,
			IPAddress: n.IpAddress,
		}

		input.Networks = append(input.Networks, &networkInput)

		// network
		if taskID, err := b.buildNetworkCreateTask(ctx, txn, n.Network); err == nil {
			networkInput.Network = migrator.NewJSONRef(taskID, "/network")
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}

		// subnet
		for _, subnet := range n.Network.Subnets {
			taskID, err := b.buildSubnetCreateTask(ctx, txn, n.Network, subnet)
			if err == nil {
				dependencies = append(dependencies, taskID)
			} else {
				return "", err
			}

			if subnet.Id == n.Subnet.Id {
				networkInput.Subnet = migrator.NewJSONRef(taskID, "/subnet")
			}
		}

		// floating ip
		if n.FloatingIp.GetId() == 0 {
			continue
		}

		ipPlan := internal.GetFloatingIPRecoveryPlan(
			b.RecoveryJobDetail.Plan, n.FloatingIp.Id,
		)

		if ipPlan == nil {
			return "", internal.NotFoundFloatingIPRecoveryPlan(b.RecoveryJobDetail.Plan.Id, n.FloatingIp.Id)
		}

		if taskID, err := b.buildFloatingIPCreateTask(ctx, txn, ipPlan); err == nil {
			networkInput.FloatingIP = migrator.NewJSONRef(taskID, "/floating_ip")
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}
	}

	// router
	for _, r := range plan.ProtectionClusterInstance.Routers {
		if taskID, err := b.buildRouterCreateTask(ctx, txn, r); err == nil {
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}
	}

	// volume
	for _, v := range plan.ProtectionClusterInstance.Volumes {
		volPlan := internal.GetVolumeRecoveryPlan(
			b.RecoveryJobDetail.Plan, v.Volume.Id,
		)

		if volPlan == nil {
			return "", internal.NotFoundVolumeRecoveryPlan(b.RecoveryJobDetail.Plan.Id, v.Volume.Id)
		}

		if taskID, err := b.buildVolumeImportTask(ctx, txn, volPlan); err == nil {
			vs := &migrator.RecoveryJobVolumeStatus{
				RecoveryJobTaskID:     taskID,
				RecoveryPointTypeCode: b.RecoveryJobDetail.RecoveryPointTypeCode,
				RecoveryPoint:         b.RecoveryJob.RecoveryPoint,
				StateCode:             constant.RecoveryJobVolumeStateCodePreparing,
				ResultCode:            constant.VolumeRecoveryResultCodeFailed,
			}

			if err = b.RecoveryJob.SetVolumeStatus(txn, v.Volume.Id, vs); err != nil {
				logger.Errorf("[buildInstanceCreateTask] Could not set volume(%d) status. Cause: %+v", v.Volume.Id, err)
				return "", err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
				RecoveryJobID: b.RecoveryJob.RecoveryJobID,
				VolumeID:      v.Volume.Id,
				VolumeStatus:  vs,
			}); err != nil {
				logger.Warnf("[buildInstanceCreateTask] Could not publish volume(%d) status. Cause: %+v", v.Volume.Id, err)
			}

			in := migrator.InstanceVolumeInputData{
				Volume:     migrator.NewJSONRef(taskID, "/volume_pair/target"),
				DevicePath: v.DevicePath,
				BootIndex:  v.BootIndex,
			}
			input.Volumes = append(input.Volumes, &in)
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}
	}

	// keypair
	if plan.ProtectionClusterInstance.GetKeypair().GetId() != 0 {
		if taskID, err := b.buildKeypairCreateTask(ctx, txn,
			plan.ProtectionClusterInstance.Keypair,
		); err == nil {
			input.Keypair = migrator.NewJSONRef(taskID, "/keypair")
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}
	}

	// instance spec
	if taskID, err := b.buildInstanceSpecCreateTask(ctx, txn,
		plan.ProtectionClusterInstance.Cluster,
		plan.ProtectionClusterInstance.Spec,
	); err == nil {
		input.Spec = migrator.NewJSONRef(taskID, "/spec")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// 이미 task build 시작한 instance 는 우선 instanceTaskIDMap 에 넣어줌
	// 무한 루프 방지를 위함
	b.InstanceTaskIDMap[plan.ProtectionClusterInstance.Id] = ""

	// instance dependencies
	for _, dep := range plan.Dependencies {
		depPlan := internal.GetInstanceRecoveryPlan(
			b.RecoveryJobDetail.Plan, dep.Id,
		)

		if depPlan == nil {
			return "", internal.NotFoundInstanceRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterInstance.Id)
		}

		logger.Infof("[buildInstanceCreateTask] Dependency instance task build: plan(%d) instance(%d) dependency instance(%d)",
			b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterInstance.Id, dep.Id)
		if taskID, err := b.buildInstanceCreateTask(ctx, txn, depPlan); err == nil {
			if taskID != "" {
				dependencies = append(dependencies, taskID)
			}
		} else {
			return "", err
		}
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   plan.ProtectionClusterInstance.Id,
		ResourceName: plan.ProtectionClusterInstance.Name,
		TypeCode:     constant.MigrationTaskTypeCreateAndDiagnosisInstance,
		Dependencies: dependencies,
		Input:        &input,
	})
	if err != nil {
		logger.Errorf("[buildInstanceCreateTask] Errors occurred during PutTaskFunc: job(%d) instance(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.InstanceTaskIDMap[plan.ProtectionClusterInstance.Id] = task.RecoveryJobTaskID

	// 비기동 인스턴스는 인스턴스 stop task 를 추가한다.
	// task 생성에 실패하더라도, 서비스 복구에 직접적인 영향을 끼치지 않으므로 실패 시 warning 처리한다.
	if !plan.AutoStartFlag {
		if err := b.buildInstanceStopTask(ctx, txn, plan.ProtectionClusterInstance.Tenant, plan.ProtectionClusterInstance); err != nil {
			logger.Warnf("[buildInstanceCreateTask] Could not build instance stop task. Cause: %+v", err)
		}
	}

	logger.Infof("[buildInstanceCreateTask] Success: job(%d) instance(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildInstanceDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildInstanceDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	var origInput migrator.InstanceCreateTaskInputData
	if err := task.GetInputData(&origInput); err != nil {
		logger.Errorf("[buildInstanceDeleteTask] Could not get input data. Cause: %+v", err)
		return nil, err
	}

	logger.Infof("[buildInstanceDeleteTask] instance - %d : job(%d) typeCode(%s) task(%s)",
		origInput.InstanceID, b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	status, err := b.RecoveryJob.GetInstanceStatus(origInput.InstanceID)
	if err != nil {
		logger.Errorf("[buildInstanceDeleteTask] Could not get instance(%d) status: job(%d) typeCode(%s) task(%s). Cause: %+v",
			origInput.InstanceID, b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return nil, err
	}

	if status.RecoveryJobTaskID != task.RecoveryJobTaskID {
		return nil, nil
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildInstanceDeleteTask] Could not get tenant create task: job(%d) typeCode(%s) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}
	var input = migrator.InstanceDeleteTaskInputData{
		Tenant:   migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		Instance: migrator.NewJSONRef(task.RecoveryJobTaskID, "/instance"),
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
		TypeCode:      constant.MigrationTaskTypeDeleteInstance,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildInstanceDeleteTask] Errors occurred during PutTaskFunc: job(%d) typeCode(%s) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildInstanceDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}

// 인스턴스 생성의 dependency task 중 copyVolume, manageVolume task 에 대해서만 reverse task 를 만들며,
// 인스턴스 생성이 성공 했다면, 인스턴스 삭제 task 도 만든다.
func (b *TaskBuilder) buildInstanceReverseTask(txn store.Txn, task *migrator.RecoveryJobTask) error {
	logger.Infof("[buildInstanceReverseTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	var err error

	for _, d := range task.Dependencies {
		t := b.RecoveryJobTaskMap[d]
		switch t.TypeCode {
		case constant.MigrationTaskTypeImportVolume:
			if err = b.buildInstanceReverseTask(txn, t); err != nil {
				return err
			}

		case constant.MigrationTaskTypeCopyVolume:
			if _, err = b.buildReverseTask(txn, t); err != nil {
				return err
			}
		}
	}

	logger.Infof("[buildInstanceReverseTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return nil
}

func (b *TaskBuilder) buildInstanceRemoveTask(txn store.Txn, task *migrator.RecoveryJobTask) ([]string, error) {
	logger.Infof("[buildInstanceRemoveTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if err := b.buildInstanceReverseTask(txn, task); err != nil {
		return nil, err
	}

	deleteFunc := func(taskMap map[uint64]string, taskID string) {
		for k, v := range taskMap {
			if v == taskID {
				delete(taskMap, k)
				return
			}
		}
	}

	// 복구 작업 재시도로 만들어진 DeleteCopyVolume, UnmanageVolume task 성공 한 뒤
	// 복구 작업 롤백 시 수행 되지 않도록 설정
	var deps []string
	for k, v := range b.RecoveryJobClearTaskMap {
		var m map[uint64]string
		switch v.TypeCode {
		case constant.MigrationTaskTypeUnmanageVolume:
			m = b.VolumeImportTaskIDMap
			b.RetryVolumeImportTaskMap[v.ReverseTaskID] = v.RecoveryJobTaskID

		case constant.MigrationTaskTypeDeleteVolumeCopy:
			m = b.VolumeCopyTaskIDMap
			b.RetryVolumeCopyTaskMap[v.ReverseTaskID] = v.RecoveryJobTaskID

		case constant.MigrationTaskTypeDeleteInstance:
			m = b.InstanceTaskIDMap
		}

		deps = append(deps, v.RecoveryJobTaskID)
		deleteFunc(m, v.ReverseTaskID)
		delete(b.RecoveryJobTaskMap, v.ReverseTaskID)
		delete(b.RecoveryJobClearTaskMap, k)
	}

	logger.Infof("[buildInstanceRemoveTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return deps, nil
}

func (b *TaskBuilder) rebuildInstanceCreateTask(ctx context.Context, txn store.Txn, origTask *migrator.RecoveryJobTask, plan *drms.InstanceRecoveryPlan) (string, error) {
	logger.Infof("[rebuildInstanceCreateTask] Start: job(%d) instance(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id, origTask.TypeCode)

	// 기존 instance recovery plan 과 create task 의 input 을 가져온다.
	var origPlan *drms.InstanceRecoveryPlan
	var origInput migrator.InstanceCreateTaskInputData
	var forceRetryFlag bool
	var dependencies []string

	if origPlan = internal.GetInstanceRecoveryPlan(b.RecoveryJobDetail.Plan, plan.ProtectionClusterInstance.Id); origPlan == nil {
		return "", internal.NotFoundInstanceRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterInstance.Id)
	}
	if err := origTask.GetInputData(&origInput); err != nil {
		logger.Errorf("[rebuildInstanceCreateTask] Could not get input data. Cause: %+v", err)
		return "", err
	}

	// instance create task 생성
	// 인스턴스 상세 정보와 볼륨만 지정한 snapshot 으로 복구한다.
	var input = migrator.InstanceCreateTaskInputData{
		Instance:   plan.ProtectionClusterInstance,
		InstanceID: plan.ProtectionClusterInstance.Id,

		AvailabilityZone: origInput.AvailabilityZone,
		Hypervisor:       origInput.Hypervisor,
		Keypair:          origInput.Keypair,
	}

	// 기존 인스턴스의 테넌트 생성 Task 를 재활용하며,
	// 테넌트 생성에 실패한 경우 다시 시도할 수 있게 Result 와 Status 를 초기화한다.
	tenantID := origPlan.ProtectionClusterInstance.Tenant.Id
	tenantTask := b.RecoveryJobTaskMap[b.TenantTaskIDMap[tenantID]]

	if tenantTask == nil {
		return "", errors.Unknown(errors.New("not found tenant create task"))
	}

	if err := b.resetFailedTaskStatus(txn, tenantTask); err != nil {
		logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed tenant task status. Cause: %+v", err)
		return "", err
	}

	input.Tenant = migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant")
	dependencies = append(dependencies, tenantTask.RecoveryJobTaskID)

	// 기존 인스턴스의 인스턴스 Spec 생성 Task 를 재활용하며,
	// 인스턴스 Spec 생성에 실패한 경우 다시 시도할 수 있게 Result 와 Status 를 초기화한다.
	specID := origPlan.ProtectionClusterInstance.Spec.Id
	specTask := b.RecoveryJobTaskMap[b.InstanceSpecTaskIDMap[specID]]
	if specTask == nil {
		return "", errors.Unknown(errors.New("not found instance spec create task"))
	}

	if err := b.resetFailedTaskStatus(txn, specTask); err != nil {
		logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed spec task status. Cause: %+v", err)
		return "", err
	}

	input.Spec = migrator.NewJSONRef(specTask.RecoveryJobTaskID, "/spec")
	dependencies = append(dependencies, specTask.RecoveryJobTaskID)

	// 기존 인스턴스의 keypair 생성 Task 를 재활용하며,
	// Keypair 생성에 실패한 경우 다시 시도할 수 있게 Result 와 Status 를 초기화한다.
	if origPlan.ProtectionClusterInstance.GetKeypair().GetId() != 0 {
		keypairID := origPlan.ProtectionClusterInstance.Keypair.Id
		keypairTask := b.RecoveryJobTaskMap[b.KeypairTaskIDMap[keypairID]]
		if keypairTask == nil {
			return "", errors.Unknown(errors.New("not found keypair create task"))
		}

		if err := b.resetFailedTaskStatus(txn, keypairTask); err != nil {
			logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed keypair task status. Cause: %+v", err)
			return "", err
		}

		input.Keypair = migrator.NewJSONRef(keypairTask.RecoveryJobTaskID, "/keypair")
		dependencies = append(dependencies, keypairTask.RecoveryJobTaskID)
	}

	// 재시도할 스냅샷이 작업의 스냅샷과 동일하다면 네트워크, 보안그룹, 선행 인스턴스도 연결한다.
	if b.RecoveryJob.RecoveryPointSnapshot != nil && b.Options.RetryRecoveryPointSnapshot.Id == b.RecoveryJob.RecoveryPointSnapshot.Id {
		// 기존 인스턴스의 보안그룹, 보안그룹 규칙 생성 Task 를 재활용하며,
		// 보안그룹, 보안그룹 규칙 생성에 실패한 경우 다시 시도할 수 있게 Result 와 Status 를 초기화한다.
		for _, sg := range origPlan.ProtectionClusterInstance.SecurityGroups {
			sgTask := b.RecoveryJobTaskMap[b.SecurityGroupTaskIDMap[sg.Id]]
			if sgTask == nil {
				return "", errors.Unknown(errors.New("not found security group create task"))
			}

			if err := b.resetFailedTaskStatus(txn, sgTask); err != nil {
				logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed security group task status. Cause: %+v", err)
				return "", err
			}

			input.SecurityGroups = append(input.SecurityGroups, migrator.NewJSONRef(sgTask.RecoveryJobTaskID, "/security_group"))

			dependencies = append(dependencies, sgTask.RecoveryJobTaskID)

			for _, rule := range sg.Rules {
				ruleTask := b.RecoveryJobTaskMap[b.SecurityGroupRuleTaskIDMap[rule.Id]]
				if ruleTask == nil {
					return "", errors.Unknown(errors.New("not found security group rule create task"))
				}

				if err := b.resetFailedTaskStatus(txn, ruleTask); err != nil {
					logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed security group rule task status. Cause: %+v", err)
					return "", err
				}

				dependencies = append(dependencies, ruleTask.RecoveryJobTaskID)
			}
		}

		// 기존 인스턴스의 네트워크, 서브넷, FloatingIP 생성 Task 를 재활용하며,
		// 네트워크, 서브넷, FloatingIP 생성에 실패한 경우 다시 시도할 수 있게 Result 와 Status 를 초기화한다.
		for _, n := range origPlan.ProtectionClusterInstance.Networks {
			var networkInput = migrator.InstanceNetworkInputData{
				DhcpFlag:  n.DhcpFlag,
				IPAddress: n.IpAddress,
			}

			input.Networks = append(input.Networks, &networkInput)

			// network
			networkTask := b.RecoveryJobTaskMap[b.NetworkTaskIDMap[n.Network.Id]]
			if networkTask == nil {
				return "", errors.Unknown(errors.New("not found network create task"))
			}

			if err := b.resetFailedTaskStatus(txn, networkTask); err != nil {
				logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed network task status. Cause: %+v", err)
				return "", err
			}

			networkInput.Network = migrator.NewJSONRef(networkTask.RecoveryJobTaskID, "/network")

			dependencies = append(dependencies, networkTask.RecoveryJobTaskID)

			// subnet
			for _, subnet := range n.Network.Subnets {
				subnetTask := b.RecoveryJobTaskMap[b.SubnetTaskIDMap[subnet.Id]]
				if subnetTask == nil {
					return "", errors.Unknown(errors.New("not found subnet create task"))
				}

				if err := b.resetFailedTaskStatus(txn, subnetTask); err != nil {
					logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed subnet task status. Cause: %+v", err)
					return "", err
				}

				if subnet.Id == n.Subnet.Id {
					networkInput.Subnet = migrator.NewJSONRef(subnetTask.RecoveryJobTaskID, "/subnet")
				}

				dependencies = append(dependencies, subnetTask.RecoveryJobTaskID)
			}

			// floating ip
			if n.FloatingIp.GetId() == 0 {
				continue
			}

			floatingIPTask := b.RecoveryJobTaskMap[b.FloatingIPTaskIDMap[n.FloatingIp.Id]]
			if floatingIPTask == nil {
				return "", errors.Unknown(errors.New("not found floating ip create task"))
			}

			if err := b.resetFailedTaskStatus(txn, floatingIPTask); err != nil {
				logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed floating IP task status. Cause: %+v", err)
				return "", err
			}

			networkInput.FloatingIP = migrator.NewJSONRef(floatingIPTask.RecoveryJobTaskID, "/floating_ip")

			dependencies = append(dependencies, floatingIPTask.RecoveryJobTaskID)
		}

		// 기존 인스턴스의 라우터 생성 Task 를 재활용하며,
		// 라우터 생성에 실패한 경우 다시 시도할 수 있게 Result 와 Status 를 초기화한다.
		for _, r := range origPlan.ProtectionClusterInstance.Routers {
			routerTask := b.RecoveryJobTaskMap[b.RouterTaskIDMap[r.Id]]
			if routerTask == nil {
				return "", errors.Unknown(errors.New("not found router create task"))
			}

			if err := b.resetFailedTaskStatus(txn, routerTask); err != nil {
				logger.Errorf("[rebuildInstanceCreateTask] Could not reset failed router task status. Cause: %+v", err)
				return "", err
			}

			dependencies = append(dependencies, routerTask.RecoveryJobTaskID)
		}

		// 기존 인스턴스의 선행 인스턴스 생성 Task 를 재활용하며,
		// 선행 인스턴스 생성이 실패한 경우엔 다시 시도하지 않는다.
		for _, dep := range origPlan.Dependencies {
			depTask := b.RecoveryJobTaskMap[b.InstanceTaskIDMap[dep.Id]]
			if depTask == nil {
				return "", errors.Unknown(errors.New("not found depend instance create task"))
			}

			dependencies = append(dependencies, depTask.RecoveryJobTaskID)
		}
	} else {
		// 기존 복구 시점이 아닌 다른 시점의 데이터로 재시도한 작업의 경우, 데이터의 안전을 위해 네트워크, 보안그룹, 선행 인스턴스를 생성하지 않는다.
		forceRetryFlag = true
	}

	// 재시도할 인스턴스 생성 Task 를 만들기 전에 직전에 생성한 인스턴스와 관련 리소스를 제거하는 Task 생성하여,
	// 스냅샷 시점의 volume import task 생성 시, dependency 로 넣어준다.
	retryDeps, err := b.buildInstanceRemoveTask(txn, origTask)
	if err != nil {
		return "", err
	}

	// volume import task 는 스냅샷에 맞게 새로 생성
	for _, v := range plan.ProtectionClusterInstance.Volumes {
		volPlan := internal.GetVolumeRecoveryPlan(
			b.Options.RetryRecoveryPlanSnapshot, v.Volume.Id,
		)

		if volPlan == nil {
			return "", internal.NotFoundVolumeRecoveryPlanSnapshot(b.Options.RetryRecoveryPlanSnapshot.Id, b.Options.RetryRecoveryPointSnapshot.Id, v.Volume.Id)
		}

		if taskID, err := b.buildVolumeImportTask(ctx, txn, volPlan, retryDeps); err == nil {
			vs := &migrator.RecoveryJobVolumeStatus{
				RecoveryJobTaskID:     taskID,
				RecoveryPointTypeCode: b.Options.RetryRecoveryPointTypeCode,
				RecoveryPoint:         b.Options.RetryRecoveryPlanSnapshot.GetCreatedAt(),
				StateCode:             constant.RecoveryJobVolumeStateCodePreparing,
				ResultCode:            constant.VolumeRecoveryResultCodeFailed,
			}

			if err = b.RecoveryJob.SetVolumeStatus(txn, v.Volume.Id, vs); err != nil {
				logger.Errorf("[rebuildInstanceCreateTask] Could not set volume(%d) status. Cause: %+v", v.Volume.Id, err)
				return "", err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
				RecoveryJobID: b.RecoveryJob.RecoveryJobID,
				VolumeID:      volPlan.ProtectionClusterVolume.Id,
				VolumeStatus:  vs,
			}); err != nil {
				logger.Warnf("[rebuildInstanceCreateTask] Could not publish volume(%d) status. Cause: %+v", volPlan.ProtectionClusterVolume.Id, err)
			}

			in := migrator.InstanceVolumeInputData{
				Volume:     migrator.NewJSONRef(taskID, "/volume_pair/target"),
				DevicePath: v.DevicePath,
				BootIndex:  v.BootIndex,
			}

			input.Volumes = append(input.Volumes, &in)
			dependencies = append(dependencies, taskID)
		} else {
			return "", err
		}
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		TypeCode:       constant.MigrationTaskTypeCreateAndDiagnosisInstance,
		Dependencies:   dependencies,
		Input:          &input,
		ForceRetryFlag: forceRetryFlag,
	})
	if err != nil {
		logger.Errorf("[rebuildInstanceCreateTask] Errors occurred during PutTaskFunc: job(%d) instance(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.InstanceTaskIDMap[plan.ProtectionClusterInstance.Id] = task.RecoveryJobTaskID

	// 비기동 인스턴스는 인스턴스 stop task 를 추가한다.
	// task 생성에 실패하더라도, 서비스 복구에 직접적인 영향을 끼치지 않으므로 실패 시 warning 처리한다.
	if !plan.AutoStartFlag {
		if err := b.buildInstanceStopTask(ctx, txn, plan.ProtectionClusterInstance.Tenant, plan.ProtectionClusterInstance); err != nil {
			logger.Warnf("[rebuildInstanceCreateTask] Could not build instance stop task. Cause: %+v", err)
		}
	}

	logger.Infof("[rebuildInstanceCreateTask] Success: job(%d) instance(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildInstanceStopTask(ctx context.Context, txn store.Txn, tenant *cms.ClusterTenant, instance *cms.ClusterInstance) error {
	logger.Infof("[buildInstanceStopTask] Start: job(%d) instance(%d)", b.RecoveryJob.RecoveryJobID, instance.Id)

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, tenant.Id,
	)
	if tenantPlan == nil {
		return internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, tenant.Id)
	}

	instancePlan := internal.GetInstanceRecoveryPlan(
		b.RecoveryJobDetail.Plan, instance.Id,
	)
	if instancePlan == nil {
		return internal.NotFoundInstanceRecoveryPlan(b.RecoveryJobDetail.Plan.Id, instance.Id)
	}

	var dependencies []string
	var input migrator.InstanceStopTaskInputData

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return err
	}

	// instance
	if taskID, err := b.buildInstanceCreateTask(ctx, txn, instancePlan); err == nil {
		input.Instance = migrator.NewJSONRef(taskID, "/instance")
		dependencies = append(dependencies, taskID)
	} else {
		return err
	}

	if _, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		TypeCode:     constant.MigrationTaskTypeStopInstance,
		Dependencies: dependencies,
		Input:        &input,
	}); err != nil {
		logger.Errorf("[buildInstanceStopTask] Errors occurred during PutTaskFunc: job(%d) instance(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, instance.Id, err)
		return err
	}

	logger.Infof("[buildInstanceStopTask] Success: job(%d) instance(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, instance.Id, constant.MigrationTaskTypeStopInstance)
	return nil
}
