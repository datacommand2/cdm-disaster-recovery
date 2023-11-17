package builder

import (
	"context"
	"fmt"
	"github.com/datacommand2/cdm-center/services/cluster-manager/storage"
	"github.com/datacommand2/cdm-center/services/cluster-manager/volume"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/snapshot"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
)

var volumeMetadata map[string]string

// TODO 공유 볼륨 처리 추가 필요
func (b *TaskBuilder) buildVolumeCopyTask(ctx context.Context, txn store.Txn, plan *drms.VolumeRecoveryPlan, deps ...[]string) (string, error) {
	var err error
	logger.Infof("[buildVolumeCopyTask] Start: job(%d) volume(%d)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterVolume.Id)

	if taskID, ok := b.VolumeCopyTaskIDMap[plan.ProtectionClusterVolume.Id]; ok {
		return taskID, nil
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, plan.ProtectionClusterVolume.Tenant.Id,
	)

	p := b.RecoveryJobDetail.Plan
	if b.Options.RetryRecoveryPlanSnapshot != nil {
		p = b.Options.RetryRecoveryPlanSnapshot
	}

	storagePlan := internal.GetStorageRecoveryPlan(
		p, plan.ProtectionClusterVolume.Storage.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterVolume.Tenant.Id)
	}
	if storagePlan == nil {
		return "", internal.NotFoundStorageRecoveryPlan(p.Id, plan.ProtectionClusterVolume.Storage.Id)
	}

	var dependencies []string
	var input = migrator.VolumeCopyTaskInputData{
		SourceStorage: plan.ProtectionClusterVolume.Storage,
		Volume:        plan.ProtectionClusterVolume,
		VolumeID:      plan.ProtectionClusterVolume.Id,
		Snapshots:     plan.ProtectionClusterVolume.Snapshots,
	}

	// 별도의 dependency 가 존재하는 경우 추가한다.
	for _, d := range deps {
		dependencies = append(dependencies, d...)
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	// target storage
	if plan.RecoveryClusterStorage.GetId() == 0 {
		input.TargetStorage = storagePlan.RecoveryClusterStorage
	} else {
		input.TargetStorage = plan.RecoveryClusterStorage
	}

	volumeMetadata = make(map[string]string)
	if b.RecoveryJob.RecoveryPointTypeCode == constant.RecoveryPointTypeCodeLatest {
		// 재해복구의 최신 데이터로 복구시에는 snapshot 의 metadata 정보가 아닌 현재 등록된 metadata 를 가져와야한다.
		mv := &mirror.Volume{
			SourceClusterStorage: &storage.ClusterStorage{StorageID: storagePlan.ProtectionClusterStorage.Id},
			TargetClusterStorage: &storage.ClusterStorage{StorageID: storagePlan.RecoveryClusterStorage.Id},
			SourceVolume:         &volume.ClusterVolume{VolumeID: plan.ProtectionClusterVolume.Id},
		}

		metadata, err := mv.GetTargetMetadata()
		if err != nil {
			logger.Errorf("[buildVolumeCopyTask] Could not get volume metadata: cluster(%d) volume(%d) job(%d) plan(%d).",
				plan.ProtectionClusterVolume.Cluster.Id, plan.ProtectionClusterVolume.Id, b.RecoveryJob.RecoveryJobID, b.RecoveryJobDetail.Plan.Id)
			return "", err
		}

		for k, v := range metadata {
			volumeMetadata[k] = fmt.Sprint(v)
		}

	} else { // RecoveryPointTypeCodeLatestSnapshot, RecoveryPointTypeCodeSnapshot 일때
		volumeMetadata, err = snapshot.GetVolumeSnapshotMetadata(ctx, b.RecoveryJobDetail.Plan.Id, b.RecoveryJob.RecoveryPointSnapshot.GetId(), plan.ProtectionClusterVolume.Id)
		if err != nil {
			return "", err
		}
	}

	input.TargetMetadata = volumeMetadata

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		TypeCode:     constant.MigrationTaskTypeCopyVolume,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildVolumeCopyTask] Errors occurred during PutTaskFunc: job(%d) volume(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterVolume.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.VolumeCopyTaskIDMap[plan.ProtectionClusterVolume.Id] = task.RecoveryJobTaskID

	logger.Infof("[buildVolumeCopyTask] Success: job(%d) volume(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterVolume.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

// TODO 공유 볼륨 처리 추가 필요
func (b *TaskBuilder) buildVolumeImportTask(ctx context.Context, txn store.Txn, plan *drms.VolumeRecoveryPlan, deps ...[]string) (string, error) {
	logger.Infof("[buildVolumeImportTask] Start: job(%d) volume(%d)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterVolume.Id)

	if taskID, ok := b.VolumeImportTaskIDMap[plan.ProtectionClusterVolume.Id]; ok {
		return taskID, nil
	}

	tenantPlan := internal.GetTenantRecoveryPlan(
		b.RecoveryJobDetail.Plan, plan.ProtectionClusterVolume.Tenant.Id,
	)

	p := b.RecoveryJobDetail.Plan
	if b.Options.RetryRecoveryPlanSnapshot != nil {
		p = b.Options.RetryRecoveryPlanSnapshot
	}

	storagePlan := internal.GetStorageRecoveryPlan(
		p, plan.ProtectionClusterVolume.Storage.Id,
	)

	if tenantPlan == nil {
		return "", internal.NotFoundTenantRecoveryPlan(b.RecoveryJobDetail.Plan.Id, plan.ProtectionClusterVolume.Tenant.Id)
	}
	if storagePlan == nil {
		return "", internal.NotFoundStorageRecoveryPlan(p.Id, plan.ProtectionClusterVolume.Storage.Id)
	}

	var err error
	var dependencies []string
	var input = migrator.VolumeImportTaskInputData{
		VolumeID: plan.ProtectionClusterVolume.Id,
	}

	// 별도의 dependency 가 존재하는 경우 추가한다.
	for _, d := range deps {
		dependencies = append(dependencies, d...)
	}

	// tenant
	if taskID, err := b.buildTenantCreateTask(ctx, txn, tenantPlan); err == nil {
		input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
		dependencies = append(dependencies, taskID)
	} else {
		return "", err
	}

	var copyTaskID string
	if copyTaskID, err = b.buildVolumeCopyTask(ctx, txn, plan, deps...); err != nil {
		return "", err
	}

	input.Volume = migrator.NewJSONRef(copyTaskID, "/volume")
	input.Snapshots = migrator.NewJSONRef(copyTaskID, "/snapshots")
	input.SourceStorage = migrator.NewJSONRef(copyTaskID, "/source_storage")
	input.TargetStorage = migrator.NewJSONRef(copyTaskID, "/target_storage")
	input.TargetMetadata = volumeMetadata
	dependencies = append(dependencies, copyTaskID)

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   plan.ProtectionClusterVolume.Id,
		ResourceName: plan.ProtectionClusterVolume.Name,
		TypeCode:     constant.MigrationTaskTypeImportVolume,
		Dependencies: dependencies,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildVolumeImportTask] Errors occurred during PutTaskFunc: job(%d) volume(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterVolume.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.VolumeImportTaskIDMap[plan.ProtectionClusterVolume.Id] = task.RecoveryJobTaskID

	logger.Infof("[buildVolumeImportTask] Success: job(%d) volume(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterVolume.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildVolumeCopyDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildVolumeCopyDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	// 재해복구 작업 재시도로 만들어진 DeleteCopyVolume task 이며, result 가 success 일 시 clear task를 만들지 않는다
	if v, ok := b.RetryVolumeCopyTaskMap[task.RecoveryJobTaskID]; ok {
		retryTask, err := b.RecoveryJob.GetTask(v)
		if err != nil {
			logger.Errorf("[buildVolumeCopyDeleteTask] Could not get task: dr.recovery.job/%d/task/%s. Cause: %+v",
				b.RecoveryJob.RecoveryJobID, v, err)
			return nil, err
		}
		result, err := retryTask.GetResult()
		if err != nil {
			return nil, err
		}

		if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			return nil, nil
		}
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildVolumeCopyDeleteTask] Could not get tenant create task: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var in migrator.VolumeCopyTaskInputData
	if err := task.GetInputData(&in); err != nil {
		logger.Errorf("[buildVolumeCopyDeleteTask] Could not get input data: job(%d) task(%s). Cause: %+v", b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}
	var input = migrator.VolumeCopyDeleteTaskInputData{
		Tenant:         migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		SourceStorage:  migrator.NewJSONRef(task.RecoveryJobTaskID, "/source_storage"),
		TargetStorage:  migrator.NewJSONRef(task.RecoveryJobTaskID, "/target_storage"),
		TargetMetadata: in.TargetMetadata,
		Volume:         migrator.NewJSONRef(task.RecoveryJobTaskID, "/volume"),
		Snapshots:      migrator.NewJSONRef(task.RecoveryJobTaskID, "/snapshots"),
		VolumeID:       in.VolumeID,
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
		TypeCode:      constant.MigrationTaskTypeDeleteVolumeCopy,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildVolumeCopyDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildVolumeCopyDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}

func (b *TaskBuilder) buildVolumeUnmanageTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildVolumeUnmanageTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	// 재해복구 작업 재시도로 만들어진 UnmanageVolume task 이며, result 가 success 일 시 clear task를 만들지 않는다
	if v, ok := b.RetryVolumeImportTaskMap[task.RecoveryJobTaskID]; ok {
		retryTask, err := b.RecoveryJob.GetTask(v)
		if err != nil {
			logger.Errorf("[buildVolumeUnmanageTask] Could not get task: dr.recovery.job/%d/task/%s. Cause: %+v",
				b.RecoveryJob.RecoveryJobID, v, err)
			return nil, err
		}
		result, err := retryTask.GetResult()
		if err != nil {
			return nil, err
		}

		if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			return nil, nil
		}
	}

	tenantTask, err := getTenantCreateTask(b.RecoveryJob, task.Dependencies)
	if err != nil {
		logger.Errorf("[buildVolumeUnmanageTask] Could not get tenant create task: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var in migrator.VolumeImportTaskInputData
	if err := task.GetInputData(&in); err != nil {
		logger.Errorf("[buildVolumeUnmanageTask] Could not get input data: job(%d) task(%s). Cause: %+v", b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
		tenantTask.RecoveryJobTaskID,
	}
	var input = migrator.VolumeUnmanageTaskInputData{
		Tenant:         migrator.NewJSONRef(tenantTask.RecoveryJobTaskID, "/tenant"),
		SourceStorage:  migrator.NewJSONRef(task.RecoveryJobTaskID, "/source_storage"),
		TargetStorage:  migrator.NewJSONRef(task.RecoveryJobTaskID, "/target_storage"),
		TargetMetadata: in.TargetMetadata,
		VolumePair:     migrator.NewJSONRef(task.RecoveryJobTaskID, "/volume_pair"),
		SnapshotPairs:  migrator.NewJSONRef(task.RecoveryJobTaskID, "/snapshot_pairs"),
		VolumeID:       in.VolumeID,
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
		TypeCode:      constant.MigrationTaskTypeUnmanageVolume,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildVolumeUnmanageTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	var status *migrator.RecoveryJobVolumeStatus
	if status, err = b.RecoveryJob.GetVolumeStatus(input.VolumeID); err != nil {
		logger.Errorf("[buildVolumeUnmanageTask] Could not get volume status: job(%d) volume(%d). Cause: %+v", b.RecoveryJob.RecoveryJobID, input.VolumeID, err)
		return nil, err
	}

	// 복구 작업 재시도로 인해 만들어진 경우가 아니라면, 볼륨 status 를 업데이트
	if task.RecoveryJobTaskID == status.RecoveryJobTaskID {
		status.RollbackFlag = true
		if err = b.RecoveryJob.SetVolumeStatus(txn, input.VolumeID, status); err != nil {
			logger.Errorf("[buildVolumeUnmanageTask] Could not set volume status: job(%d) volume(%d). Cause: %+v", b.RecoveryJob.RecoveryJobID, input.VolumeID, err)
			return nil, err
		}

		if err = internal.PublishMessage(constant.QueueRecoveryJobVolumeMonitor, migrator.RecoveryJobVolumeMessage{
			RecoveryJobID: b.RecoveryJob.RecoveryJobID,
			VolumeID:      input.VolumeID,
			VolumeStatus:  status,
		}); err != nil {
			logger.Warnf("[buildRetryTasks] Could not publish volume(%d) status. Cause: %+v", input.VolumeID, err)
		}
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildVolumeUnmanageTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
