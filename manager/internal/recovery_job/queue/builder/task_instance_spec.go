package builder

import (
	"context"
	"fmt"
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/cluster"
	"github.com/jinzhu/copier"
)

func (b *TaskBuilder) buildInstanceSpecCreateTask(ctx context.Context, txn store.Txn, c *cms.Cluster, spec *cms.ClusterInstanceSpec) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	specMap, ok := b.SharedInstanceSpecTaskIDMap[spec.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, specMap, spec.Id, constant.MigrationTaskTypeCreateSpec)
		if err != nil {
			logger.Errorf("[buildInstanceSpecCreateTask] Could not build shared task: job(%d) spec(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, spec.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildInstanceSpecCreateTask] Start: job(%d) sepc(%d)", b.RecoveryJob.RecoveryJobID, spec.Id)

	if specMap == nil {
		specMap = make(map[uint64]string)
	}

	var input = migrator.InstanceSpecCreateTaskInputData{
		Spec:       new(cms.ClusterInstanceSpec),
		ExtraSpecs: spec.ExtraSpecs,
	}

	// spec
	if err := copier.CopyWithOption(input.Spec, spec, copier.Option{DeepCopy: true}); err != nil {
		return "", errors.Unknown(err)
	}
	input.Spec.Name = fmt.Sprintf("%s.%s (recovered from %s cluster)", spec.Name, b.RecoveryJob.RecoveryJobHash, c.Name)
	isExist, err := cluster.CheckIsExistClusterInstanceSpec(ctx, b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.Spec.Name)
	if isExist {
		err = internal.SameNameIsAlreadyExisted(b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.Spec.Name)
		logger.Errorf("[buildInstanceSpecCreateTask] Could not build create instance spec task. Cause: %+v", err)
		return "", err
	} else if err != nil {
		logger.Warnf("[buildInstanceSpecCreateTask] Error occurred during check instance spec name(%s) existence. Cause: %+v", input.Spec.Name, err)
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   spec.Id,
		ResourceName: spec.Name,
		TypeCode:     constant.MigrationTaskTypeCreateSpec,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildInstanceSpecCreateTask] Errors occurred during PutTaskFunc: job(%d) sepc(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, spec.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.InstanceSpecTaskIDMap[spec.Id] = task.RecoveryJobTaskID
	specMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedInstanceSpecTaskIDMap[spec.Id] = specMap

	logger.Infof("[buildInstanceSpecCreateTask] Success: job(%d) sepc(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, spec.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildInstanceSpecDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildInstanceSpecDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
	}
	var input = migrator.InstanceSpecDeleteTaskInputData{
		Spec: migrator.NewJSONRef(task.RecoveryJobTaskID, "/spec"),
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
		TypeCode:      constant.MigrationTaskTypeDeleteSpec,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildInstanceSpecDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildInstanceSpecDeleteTask] Success: job(%d) typeCode(%s) task(%s)",
		b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
