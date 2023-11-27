package builder

import (
	"context"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"path"
)

// task 의 모든 dependencies (recursive) 들을 반환한다.
func (b *TaskBuilder) getDependenciesTaskRecursive(taskID string) []string {
	dependencies := []string{taskID}

	for _, dep := range b.RecoveryJobTaskMap[taskID].Dependencies {
		dependencies = append(dependencies, b.getDependenciesTaskRecursive(dep)...)
	}

	return dependencies
}

// task 가 targetTask 의 선행 task 인지 확인한다.
func isDependencyTask(task, target *migrator.RecoveryJobTask) bool {
	for _, dependency := range target.Dependencies {
		if dependency == task.RecoveryJobTaskID {
			return true
		}
	}

	return false
}

// 실패한 Task 의 Status 와 Result 를 초기화한다.
// 공유 task 인 경우 공유 task 가 실패한 경우 공유 task 의 status 와 result 도 초기화 한다.
// 공유 task 가 다른 작업에 의해 성공 한 경우 공유 task의 status와 result 를 job/task status, result에 업데이트한다.
func (b *TaskBuilder) resetFailedTaskStatus(txn store.Txn, task *migrator.RecoveryJobTask) error {
	logger.Infof("[resetFailedTaskStatus] Run: job(%d) typCode(%s) task(%s)", task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	result, err := task.GetResult()
	switch {
	case errors.Equal(err, migrator.ErrNotFoundTaskResult):
		return nil
	case err != nil:
		return err
	}

	// 성공한 Task 는 초기화하지 않는다.
	if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
		return nil
	}

	reset := func(flag bool) error {
		if err = task.DeleteResult(txn, migrator.WithSharedTask(flag)); err != nil {
			return err
		}

		if err = task.SetStatusWaiting(txn, migrator.WithSharedTask(flag)); err != nil {
			return err
		}

		return nil
	}
	// 공유 task 인지 확인
	if task.SharedTaskKey != "" {
		result, err = task.GetResult(migrator.WithSharedTask(true))
		switch {
		// 공유 task 의 result 값이 없으면(다른 job 에서 재시도 이후 확정 시)
		// job 의 task 만 reset 시킨다
		case errors.Equal(err, migrator.ErrNotFoundTaskResult):
			return reset(false)

		case err != nil:
			return err

		// 공유 task 의 result 값이 성공이면(다른 job 에서 재시도 성공 시)
		// 공유 task 의 result 및 status 를 job task 의 업데이트 시킨다
		case result.ResultCode == constant.MigrationTaskResultCodeSuccess:
			if err = task.SetResult(txn, result); err != nil {
				return err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobTaskResultMonitor, migrator.RecoveryJobTaskResultMessage{
				RecoveryJobTaskResultKey: path.Join(task.RecoveryJobTaskKey, "result"),
				TypeCode:                 task.TypeCode,
				RecoveryJobTaskResult:    result,
			}); err != nil {
				logger.Warnf("[resetFailedTaskStatus] Could not publish task(%s) result(%s). Cause: %+v", task.TypeCode, result.ResultCode, err)
			}

			status, err := task.GetStatus(migrator.WithSharedTask(true))
			if err != nil {
				return nil
			}

			if err = task.SetStatus(txn, status); err != nil {
				return err
			}

			if err = internal.PublishMessage(constant.QueueRecoveryJobTaskStatusMonitor, migrator.RecoveryJobTaskStatusMessage{
				RecoveryJobTaskStatusKey: path.Join(task.RecoveryJobTaskKey, "status"),
				TypeCode:                 task.TypeCode,
				RecoveryJobTaskStatus:    status,
			}); err != nil {
				logger.Warnf("[resetFailedTaskStatus] Could not publish task(%s) status(%s). Cause: %+v", task.TypeCode, status.StateCode, err)
			}

			return nil
		}
	}

	// 공유 task 의 result 값이 ignore 혹은 failed 시
	// 공유 task 의 result 및 status 를 reset 시킨다
	// 공유 task 가 아닌 경우 job task 의 result 및 status 를 reset 시킨다
	return reset(true)
}

// dependencies 중 create tenant task 를 찾는다.
func getTenantCreateTask(job *migrator.RecoveryJob, dependencies []string) (*migrator.RecoveryJobTask, error) {
	for _, dependency := range dependencies {
		t, err := job.GetTask(dependency)
		if err != nil {
			return nil, err
		}

		if t.TypeCode == constant.MigrationTaskTypeCreateTenant {
			return t, nil
		}
	}

	return nil, errors.Unknown(errors.New("not found create tenant task"))
}

// plan 에서 network 을 찾아 반환한다.
func getRecoveryNetwork(plan *drms.RecoveryPlan, id uint64) *cms.ClusterNetwork {
	for _, i := range plan.Detail.Instances {
		for _, n := range i.ProtectionClusterInstance.Networks {
			if n.Network.Id == id {
				return n.Network
			}
		}
	}

	return nil
}

// plan 에서 security group 을 찾아 반환한다.
func getRecoverySecurityGroup(plan *drms.RecoveryPlan, id uint64) *cms.ClusterSecurityGroup {
	for _, i := range plan.Detail.Instances {
		for _, sg := range i.ProtectionClusterInstance.SecurityGroups {
			if sg.Id == id {
				return sg
			}
		}
	}

	for _, sg := range plan.Detail.ExtraRemoteSecurityGroups {
		if sg.Id == id {
			return sg
		}
	}

	return nil
}

// task type 별로 builder 의 taskID map 정보를 가져온다.
func (b *TaskBuilder) getTaskIDMap(typeCode string) map[uint64]string {
	switch typeCode {
	case constant.MigrationTaskTypeCreateTenant:
		return b.TenantTaskIDMap

	case constant.MigrationTaskTypeCreateSecurityGroup:
		return b.SecurityGroupTaskIDMap

	case constant.MigrationTaskTypeCreateSecurityGroupRule:
		return b.SecurityGroupRuleTaskIDMap

	case constant.MigrationTaskTypeCreateSpec:
		return b.InstanceSpecTaskIDMap

	case constant.MigrationTaskTypeCreateKeypair:
		return b.KeypairTaskIDMap

	case constant.MigrationTaskTypeCreateNetwork:
		return b.NetworkTaskIDMap

	case constant.MigrationTaskTypeCreateSubnet:
		return b.SubnetTaskIDMap

	case constant.MigrationTaskTypeCreateRouter:
		return b.RouterTaskIDMap
	}

	return nil
}

// etcd 에 있는 shared task 정보로 task 를 build 한다.
func (b *TaskBuilder) buildSharedTask(txn store.Txn, sharedMap map[uint64]string, id uint64, typeCode string) (string, error) {
	var err error

	taskIDMap := b.getTaskIDMap(typeCode)
	if taskIDMap == nil {
		err = errors.Unknown(errors.New("not shared task type"))
		logger.Errorf("[buildSharedTask] Could not get task id map: typeCode(%s). Cause: %+v", typeCode, err)
		return "", err
	}

	isShared := true
	taskID, sharedTaskOk := sharedMap[b.RecoveryJob.RecoveryCluster.Id]
	_, ok := taskIDMap[id]
	if sharedTaskOk && !ok {
		logger.Infof("[buildSharedTask] Start to put Shared Task: job(%d) typeCode(%s-id:%d)", b.RecoveryJob.RecoveryJobID, typeCode, id)
		err = b.putSharedTask(txn, typeCode, taskID)
		switch {
		case errors.Equal(err, migrator.ErrNotFoundSharedTask):
			isShared = false

		case err != nil:
			logger.Errorf("[buildSharedTask] Could not put shared task: typeCode(%s) task(%s). Cause: %+v", typeCode, taskID, err)
			return "", err
		}

		if isShared {
			taskIDMap[id] = taskID
			logger.Infof("[buildSharedTask] Success: job(%d) typeCode(%s-id:%d) task(%s)", b.RecoveryJob.RecoveryJobID, typeCode, id, taskID)
			return taskID, nil
		}

	} else if sharedTaskOk && ok {
		return taskID, nil
	}

	logger.Warnf("[buildSharedTask] Empty task id return: job(%d) typeCode(%s-id:%d)", b.RecoveryJob.RecoveryJobID, typeCode, id)
	return "", nil
}

// DeleteJobSharedTasks job 의 모든 task 의 shared task 중 reference count 가 0인 shared task 삭제
func DeleteJobSharedTasks(job *migrator.RecoveryJob) error {
	taskList, err := job.GetTaskList()
	if err != nil {
		logger.Errorf("[DeleteJobSharedTasks] Could not get task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	for _, task := range taskList {
		if c, err := task.GetConfirm(); c != nil || err != nil {
			// confirm 된 작업은 count 와 상관없이 삭제하지 않는다.
			if err != nil {
				logger.Warnf("[DeleteJobSharedTasks] Could not get confirm info: job(%d) typeCode(%s) task(%s). Cause: %+v",
					job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			}
			continue
		}

		if err = deleteSharedTask(task, job.RecoveryCluster.Id); err != nil {
			logger.Warnf("[DeleteJobSharedTasks] Could not delete shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		}
	}

	return nil
}

// deleteSharedTask shared task 삭제
// clusterID: recovery cluster ID
func deleteSharedTask(task *migrator.RecoveryJobTask, clusterID uint64) error {
	var count uint64
	sharedTask, err := migrator.GetSharedTask(task.TypeCode, task.RecoveryJobTaskID)
	switch {
	case errors.Equal(err, migrator.ErrNotSharedTaskType):
		return nil

	case errors.Equal(err, migrator.ErrNotFoundSharedTask):
		break

	case err != nil:
		logger.Errorf("[DeleteSharedTask] Could not get shared task: typeCode(%s) task(%s). Cause: %+v",
			task.TypeCode, task.RecoveryJobTaskID, err)
		return err

	default:
		// err == nil
		count, err = sharedTask.GetRefCount()
		if err != nil {
			logger.Errorf("[DeleteSharedTask] Could not get shared task reference count: typeCode(%s) task(%s). Cause: %+v",
				task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}
	}

	if count > 0 {
		return nil
	}

	if err = store.Transaction(func(txn store.Txn) error {
		return migrator.DeleteSharedTask(txn, task.TypeCode, task.RecoveryJobTaskID, clusterID)
	}); err != nil {
		logger.Errorf("[DeleteSharedTask] Could not delete shared task: typeCode(%s) task(%s) recovery cluster(%d). Cause: %+v",
			task.TypeCode, task.RecoveryJobTaskID, clusterID, err)
		return err
	}

	logger.Infof("[DeleteSharedTask] Deleted: cluster(%d) typeCode(%s)", clusterID, task.TypeCode)

	return nil
}

// RunningJobDecreaseReferenceCount job 의 모든 task 의 reference count 를 decrease 하고 그 count 가 0이면 해당 shared task 는 삭제
func RunningJobDecreaseReferenceCount(job *migrator.RecoveryJob) error {
	logger.Infof("[RunningJobDecreaseReferenceCount] Start: job(%d).", job.RecoveryJobID)

	taskList, err := job.GetTaskList()
	if err != nil {
		logger.Errorf("[RunningJobDecreaseReferenceCount] Could not get task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	for _, task := range taskList {
		// result 가 success 인 경우 decrease
		result, err := task.GetResult()
		if err != nil {
			logger.Warnf("[RunningJobDecreaseReferenceCount] Could not get task result: job(%d) task(%s:%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue
		}

		if result.ResultCode != constant.MigrationTaskResultCodeSuccess {
			continue
		}

		// result.ResultCode == constant.MigrationTaskResultCodeSuccess
		sharedTask, err := migrator.GetSharedTask(task.TypeCode, task.RecoveryJobTaskID)
		switch {
		case errors.Equal(err, migrator.ErrNotSharedTaskType), errors.Equal(err, migrator.ErrNotFoundSharedTask):
			logger.Warnf("[RunningJobDecreaseReferenceCount] Could not get shared task: job(%d) task(%s:%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue

		case err != nil:
			logger.Errorf("[RunningJobDecreaseReferenceCount] Could not get shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		// err == nil
		var count uint64
		if err = store.Transaction(func(txn store.Txn) error {
			count, err = sharedTask.DecreaseRefCount(txn)
			return err
		}); err != nil {
			logger.Warnf("[RunningJobDecreaseReferenceCount] Could not decrease the reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue
		}
		logger.Infof("[RunningJobDecreaseReferenceCount] Decrease the shared task reference count: job(%d) typeCode(%s) task(%s).",
			job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

		if count > 0 {
			continue
		}

		if c, err := task.GetConfirm(); c != nil || err != nil {
			// confirm 된 작업은 count 와 상관없이 삭제하지 않는다.
			if err != nil {
				logger.Warnf("[RunningJobDecreaseReferenceCount] Could not get confirm info: job(%d) typeCode(%s) task(%s). Cause: %+v",
					job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			}
		}

		if err = store.Transaction(func(txn store.Txn) error {
			return migrator.DeleteSharedTask(txn, task.TypeCode, task.RecoveryJobTaskID, job.RecoveryCluster.Id)
		}); err != nil {
			logger.Warnf("[RunningJobDecreaseReferenceCount] Could not delete shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue
		}
		logger.Infof("[RunningJobDecreaseReferenceCount] Delete the shared task: job(%d) typeCode(%s) task(%s).",
			job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	}

	logger.Infof("[RunningJobDecreaseReferenceCount] Done: job(%d).", job.RecoveryJobID)
	return nil
}

// ClearingJobDecreaseReferenceCount job 의 task 중 clearing 이 진행되지 않은 task 의 reference count 를 decrease 하고 그 count 가 0이면 해당 shared task 는 삭제
func ClearingJobDecreaseReferenceCount(job *migrator.RecoveryJob) error {
	logger.Infof("[ClearingJobDecreaseReferenceCount] Start: job(%d).", job.RecoveryJobID)

	taskList, err := job.GetClearTaskList()
	if err != nil {
		logger.Errorf("[ClearingJobDecreaseReferenceCount] Could not get clear task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return err
	}

	for _, task := range taskList {
		// result 가 success 가 아니거나 result 가 없는 경우 decrease
		result, err := task.GetResult()
		if err != nil && !errors.Equal(err, migrator.ErrNotFoundTaskResult) {
			logger.Warnf("[ClearingJobDecreaseReferenceCount] Could not get task result: job(%d) task(%s:%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue
		} else if err == nil && result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			continue
		}

		// errors.Equal(err, migrator.ErrNotFoundTaskResult) || err == nil && result.ResultCode != constant.MigrationTaskResultCodeSuccess
		sharedTask, err := migrator.GetReverseSharedTask(task.TypeCode, task.RecoveryJobTaskID)
		switch {
		case errors.Equal(err, migrator.ErrNotSharedTaskType), errors.Equal(err, migrator.ErrNotFoundSharedTask):
			logger.Warnf("[ClearingJobDecreaseReferenceCount] Could not get reverse shared task: job(%d) task(%s:%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue

		case err != nil:
			logger.Errorf("[ClearingJobDecreaseReferenceCount] Could not get reverse shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		// err == nil
		var count uint64
		if err = store.Transaction(func(txn store.Txn) error {
			count, err = sharedTask.DecreaseRefCount(txn)
			return err
		}); err != nil {
			logger.Warnf("[ClearingJobDecreaseReferenceCount] Could not decrease the reference count: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue
		}
		logger.Infof("[ClearingJobDecreaseReferenceCount] Decrease the reverse shared task reference count: job(%d) typeCode(%s) task(%s).",
			job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

		if count > 0 {
			continue
		}

		if err = store.Transaction(func(txn store.Txn) error {
			return migrator.DeleteReverseSharedTask(txn, task.TypeCode, task.RecoveryJobTaskID, job.RecoveryCluster.Id)
		}); err != nil {
			logger.Warnf("[ClearingJobDecreaseReferenceCount] Could not delete reverse shared task: job(%d) typeCode(%s) task(%s). Cause: %+v",
				job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			continue
		}
		logger.Infof("[ClearingJobDecreaseReferenceCount] Delete the reverse shared task: job(%d) typeCode(%s) task(%s).",
			job.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	}

	logger.Infof("[ClearingJobDecreaseReferenceCount] Done: job(%d).", job.RecoveryJobID)
	return nil
}

func isExistInstanceSpec(ctx context.Context, cid uint64, taskID string) (bool, error) {
	task, err := migrator.GetSharedTask(constant.MigrationTaskTypeCreateSpec, taskID)
	if err != nil {
		logger.Warnf("[isExistInstanceSpec] Could not get shared task(%s:%s) for check instance spec existence. Cause: %+v",
			constant.MigrationTaskTypeCreateSpec, taskID, err)
		return false, err
	}

	result, err := task.GetResult(migrator.WithSharedTask(true))
	if err != nil {
		logger.Warnf("[isExistInstanceSpec] Could not get shared task(%s:%s) result for check instance spec existence. Cause: %+v",
			constant.MigrationTaskTypeCreateSpec, taskID, err)
		return false, err
	}

	if result.ResultCode != constant.MigrationTaskResultCodeSuccess {
		logger.Warnf("[isExistInstanceSpec] Shared task(%s:%s) result is not success(%s). Cause: %+v",
			constant.MigrationTaskTypeCreateSpec, taskID, result.ResultCode, err)
		if result.FailedReason != nil {
			logger.Warnf("[isExistInstanceSpec] Shared task(%s:%s) result failed reason: %+v", constant.MigrationTaskTypeCreateSpec, taskID, result.FailedReason)
		}
		return false, nil
	}

	var out migrator.InstanceSpecCreateTaskOutput
	if err = task.GetOutput(&out, migrator.WithSharedTask(true)); err != nil {
		logger.Warnf("[isExistInstanceSpec] Could not get shared task(%s:%s) output for check instance spec existence. Cause: %+v",
			constant.MigrationTaskTypeCreateSpec, taskID, err)
		return false, err
	}

	if out.Spec == nil || out.Spec.Uuid == "" {
		logger.Warnf("[isExistInstanceSpec] Shared task(%s:%s) result out put information is not existed.", constant.MigrationTaskTypeCreateSpec, taskID)
		return false, nil
	}

	_, err = cluster.GetClusterInstanceSpecByUUID(ctx, cid, out.Spec.Uuid)
	switch {
	case err == nil:
		return true, nil

	case errors.GetIPCStatusCode(err) == errors.IPCStatusNotFound:
		store.Transaction(func(txn store.Txn) error {
			err = migrator.DeleteSharedTask(txn, constant.MigrationTaskTypeCreateSpec, taskID, cid)
			if err == nil {
				logger.Infof("[isExistInstanceSpec] Done - deleted shared task(%s:%s) cluster(%d) uuid(%s)",
					constant.MigrationTaskTypeCreateSpec, taskID, cid, out.Spec.Uuid)
			}
			return nil
		})
		return false, nil

	default:
		logger.Warnf("[isExistInstanceSpec] Could not get instance spec(%s). Cause: %+v", out.Spec.Uuid, err)
		return false, err
	}
}

func getInstanceSpecSharedTaskIDMap(ctx context.Context) (map[uint64]map[uint64]string, error) {
	specMap := make(map[uint64]map[uint64]string)
	specMap, err := migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateSpec)
	if err != nil {
		return nil, err
	}
	for specID, s := range specMap {
		for cid, taskID := range s {
			if isExist, err := isExistInstanceSpec(ctx, cid, taskID); isExist || err != nil {
				continue
			}
			delete(specMap[specID], cid)
		}
		if len(specMap[specID]) == 0 {
			delete(specMap, specID)
		}
	}

	return specMap, nil
}
