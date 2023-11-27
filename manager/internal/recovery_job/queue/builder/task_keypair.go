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

func (b *TaskBuilder) buildKeypairCreateTask(ctx context.Context, txn store.Txn, keypair *cms.ClusterKeypair) (string, error) {
	// 공유 task 존재 할 경우, 공유 task 의 input 을 현재 job/task 의 input 으로 설정
	keypairMap, ok := b.SharedKeypairTaskIDMap[keypair.Id]
	if ok {
		// shared task 가 존재하면 shared task ID 값을 return 한다.
		taskID, err := b.buildSharedTask(txn, keypairMap, keypair.Id, constant.MigrationTaskTypeCreateKeypair)
		if err != nil {
			logger.Errorf("[buildKeypairCreateTask] Could not build shared task: job(%d) keypair(%d). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, keypair.Id, err)
			return "", err
		}

		if taskID != "" {
			return taskID, nil
		}
	}

	logger.Infof("[buildKeypairCreateTask] Start: job(%d) keypair(%d)", b.RecoveryJob.RecoveryJobID, keypair.Id)

	if keypairMap == nil {
		keypairMap = make(map[uint64]string)
	}

	var input = migrator.KeypairCreateTaskInputData{
		Keypair: new(cms.ClusterKeypair),
	}

	// keypair
	if err := copier.CopyWithOption(input.Keypair, keypair, copier.Option{DeepCopy: true}); err != nil {
		return "", errors.Unknown(err)
	}

	// keypair name 에 특수 문자는 [@_- ] 만 허용된다.
	input.Keypair.Name = fmt.Sprintf("%s-%s", keypair.Name, b.RecoveryJob.RecoveryJobHash)
	isExist, err := cluster.CheckIsExistClusterKeypair(ctx, b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.Keypair.Name)
	if isExist {
		err = internal.SameNameIsAlreadyExisted(b.RecoveryJobDetail.Plan.RecoveryCluster.Id, input.Keypair.Name)
		logger.Errorf("[buildKeypairCreateTask] Could not build create keypair task. Cause: %+v", err)
		return "", err
	} else if err != nil {
		logger.Warnf("[buildKeypairCreateTask] Error occurred during check keypair name(%s) existence. Cause: %+v", input.Keypair.Name, err)
	}

	task, err := b.Options.PutTaskFunc(txn, &migrator.RecoveryJobTask{
		ResourceID:   keypair.Id,
		ResourceName: keypair.Name,
		TypeCode:     constant.MigrationTaskTypeCreateKeypair,
		Input:        &input,
	})

	if err != nil {
		logger.Errorf("[buildKeypairCreateTask] Errors occurred during PutTaskFunc: job(%d) keypair(%d). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, keypair.Id, err)
		return "", err
	}

	b.RecoveryJobTaskMap[task.RecoveryJobTaskID] = task
	b.KeypairTaskIDMap[keypair.Id] = task.RecoveryJobTaskID
	keypairMap[b.RecoveryJob.RecoveryCluster.Id] = task.RecoveryJobTaskID
	b.SharedKeypairTaskIDMap[keypair.Id] = keypairMap

	logger.Infof("[buildKeypairCreateTask] Success: job(%d) keypair(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, keypair.Id, task.TypeCode, task.RecoveryJobTaskID)
	return task.RecoveryJobTaskID, nil
}

func (b *TaskBuilder) buildKeypairDeleteTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildKeypairDeleteTask] Start: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)

	if t, ok := b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID]; ok {
		return t, nil
	}

	var dependencies = []string{
		task.RecoveryJobTaskID,
	}
	var input = migrator.KeypairDeleteTaskInputData{
		Keypair: migrator.NewJSONRef(task.RecoveryJobTaskID, "/keypair"),
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
		TypeCode:      constant.MigrationTaskTypeDeleteKeypair,
		Dependencies:  dependencies,
		Input:         &input,
	})

	if err != nil {
		logger.Errorf("[buildKeypairDeleteTask] Errors occurred during PutTaskFunc: job(%d) task(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, err)
		return nil, err
	}

	b.RecoveryJobClearTaskMap[task.RecoveryJobTaskID] = t

	logger.Infof("[buildKeypairDeleteTask] Success: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
	return t, nil
}
