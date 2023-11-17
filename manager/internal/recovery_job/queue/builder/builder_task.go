package builder

import (
	"context"
	"encoding/json"
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
)

const (
	taskBuilderTenantTaskIDMap            = "cdm.dr.manager.recovery_job.builder.tenant_task_id_map"
	taskBuilderInstanceTaskIDMap          = "cdm.dr.manager.recovery_job.builder.instance_task_id_map"
	taskBuilderInstanceSpecTaskIDMap      = "cdm.dr.manager.recovery_job.builder.instance_spec_task_id_map"
	taskBuilderKeypairTaskIDMap           = "cdm.dr.manager.recovery_job.builder.keypair_task_id_map"
	taskBuilderSecurityGroupTaskIDMap     = "cdm.dr.manager.recovery_job.builder.security_group_task_id_map"
	taskBuilderSecurityGroupRuleTaskIDMap = "cdm.dr.manager.recovery_job.builder.security_group_rule_task_id_map"
	taskBuilderNetworkTaskIDMap           = "cdm.dr.manager.recovery_job.builder.network_task_id_map"
	taskBuilderSubnetTaskIDMap            = "cdm.dr.manager.recovery_job.builder.subnet_task_id_map"
	taskBuilderRouterTaskIDMap            = "cdm.dr.manager.recovery_job.builder.router_task_id_map"
	taskBuilderFloatingIPTaskIDMap        = "cdm.dr.manager.recovery_job.builder.floating_ip_task_id_map"
	taskBuilderVolumeImportTaskIDMap      = "cdm.dr.manager.recovery_job.builder.volume_import_task_id_map"
	taskBuilderVolumeCopyTaskIDMap        = "cdm.dr.manager.recovery_job.builder.volume_copy_task_id_map"

	// 재해복구 작업 재시도 이후 rollback 시 재시도에서 성공한 DeleteCopyVolume, UnmanageVolume Task 들이 재시도 방지를 위한 Map
	retryTaskBuilderVolumeCopyTaskMap   = "cdm.dr.manager.recovery_job.retry.builder.volume_copy_task_map"
	retryTaskBuilderVolumeImportTaskMap = "cdm.dr.manager.recovery_job.retry.builder.volume_import_task_map"
)

type TaskBuilder struct {
	RecoveryJob       *migrator.RecoveryJob
	RecoveryJobDetail *drms.RecoveryJob

	// task map
	RecoveryJobTaskMap      map[string]*migrator.RecoveryJobTask
	RecoveryJobClearTaskMap map[string]*migrator.RecoveryJobTask

	// 공유 task map
	SharedTenantTaskIDMap            map[uint64]map[uint64]string
	SharedInstanceSpecTaskIDMap      map[uint64]map[uint64]string
	SharedSecurityGroupTaskIDMap     map[uint64]map[uint64]string
	SharedSecurityGroupRuleTaskIDMap map[uint64]map[uint64]string
	SharedNetworkTaskIDMap           map[uint64]map[uint64]string
	SharedSubnetTaskIDMap            map[uint64]map[uint64]string
	SharedRouterTaskIDMap            map[uint64]map[uint64]string
	SharedKeypairTaskIDMap           map[uint64]map[uint64]string

	TenantTaskIDMap            map[uint64]string
	InstanceSpecTaskIDMap      map[uint64]string
	SecurityGroupTaskIDMap     map[uint64]string
	SecurityGroupRuleTaskIDMap map[uint64]string
	NetworkTaskIDMap           map[uint64]string
	SubnetTaskIDMap            map[uint64]string
	RouterTaskIDMap            map[uint64]string
	FloatingIPTaskIDMap        map[uint64]string
	VolumeImportTaskIDMap      map[uint64]string
	VolumeCopyTaskIDMap        map[uint64]string
	InstanceTaskIDMap          map[uint64]string
	KeypairTaskIDMap           map[uint64]string

	// 재해복구 작업 재시도 이후 rollback 시 재시도에서 성공한 DeleteCopyVolume, UnmanageVolume Task 들이 재시도 방지를 위한 Map
	RetryVolumeImportTaskMap map[string]string
	RetryVolumeCopyTaskMap   map[string]string

	DeletedSharedTaskMap map[string]bool
	// builder options
	Options *taskBuilderOptions
}

func newClearTaskBuilder(ctx context.Context, job *migrator.RecoveryJob, opts ...TaskBuilderOption) (*TaskBuilder, error) {
	return newBuilder(ctx, job, append(opts, PutTaskFunc(job.PutClearTask))...)
}

func newTaskBuilder(ctx context.Context, job *migrator.RecoveryJob, opts ...TaskBuilderOption) (*TaskBuilder, error) {
	return newBuilder(ctx, job, append(opts, PutTaskFunc(job.PutTask))...)
}

func newBuilder(ctx context.Context, job *migrator.RecoveryJob, opts ...TaskBuilderOption) (*TaskBuilder, error) {
	b := TaskBuilder{
		RecoveryJob:             job,
		RecoveryJobDetail:       new(drms.RecoveryJob),
		RecoveryJobTaskMap:      make(map[string]*migrator.RecoveryJobTask),
		RecoveryJobClearTaskMap: make(map[string]*migrator.RecoveryJobTask),

		SharedTenantTaskIDMap:            make(map[uint64]map[uint64]string),
		SharedInstanceSpecTaskIDMap:      make(map[uint64]map[uint64]string),
		SharedSecurityGroupTaskIDMap:     make(map[uint64]map[uint64]string),
		SharedSecurityGroupRuleTaskIDMap: make(map[uint64]map[uint64]string),
		SharedNetworkTaskIDMap:           make(map[uint64]map[uint64]string),
		SharedSubnetTaskIDMap:            make(map[uint64]map[uint64]string),
		SharedRouterTaskIDMap:            make(map[uint64]map[uint64]string),
		SharedKeypairTaskIDMap:           make(map[uint64]map[uint64]string),

		TenantTaskIDMap:            make(map[uint64]string),
		InstanceSpecTaskIDMap:      make(map[uint64]string),
		SecurityGroupTaskIDMap:     make(map[uint64]string),
		SecurityGroupRuleTaskIDMap: make(map[uint64]string),
		NetworkTaskIDMap:           make(map[uint64]string),
		SubnetTaskIDMap:            make(map[uint64]string),
		RouterTaskIDMap:            make(map[uint64]string),
		FloatingIPTaskIDMap:        make(map[uint64]string),
		VolumeImportTaskIDMap:      make(map[uint64]string),
		VolumeCopyTaskIDMap:        make(map[uint64]string),
		InstanceTaskIDMap:          make(map[uint64]string),
		KeypairTaskIDMap:           make(map[uint64]string),

		RetryVolumeImportTaskMap: make(map[string]string),
		RetryVolumeCopyTaskMap:   make(map[string]string),

		DeletedSharedTaskMap: make(map[string]bool),
	}

	if err := job.GetDetail(b.RecoveryJobDetail); err != nil && !errors.Equal(err, migrator.ErrNotFoundJobDetail) {
		logger.Errorf("[builderTask-newBuilder] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	tasks, err := job.GetTaskList()
	if err != nil {
		logger.Errorf("[builderTask-newBuilder] Could not get task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	clearTasks, err := job.GetClearTaskList()
	if err != nil {
		logger.Errorf("[builderTask-newBuilder] Could not get clear task list: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	for _, t := range tasks {
		b.RecoveryJobTaskMap[t.RecoveryJobTaskID] = t
	}

	for _, t := range clearTasks {
		b.RecoveryJobClearTaskMap[t.ReverseTaskID] = t
	}

	if err = b.loadMetadata(ctx); err != nil {
		return nil, err
	}

	if err = b.initBuilderOptions(ctx, opts...); err != nil {
		logger.Errorf("[builderTask-newBuilder] Could not init builder options: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	return &b, nil
}

func (b *TaskBuilder) initBuilderOptions(ctx context.Context, opts ...TaskBuilderOption) error {
	var options = taskBuilderOptions{
		skipTaskMap: make(map[string]bool),
	}

	for _, o := range opts {
		o(&options)
	}

	// confirm job options
	if options.SkipSuccessInstances {
		var skipTasks []string
		for _, i := range b.RecoveryJobDetail.Plan.Detail.Instances {
			status, err := b.RecoveryJob.GetInstanceStatus(i.ProtectionClusterInstance.Id)
			if err != nil {
				logger.Errorf("[initBuilderOptions] Could not get instance(%d) status. Cause: %+v", i.ProtectionClusterInstance.Id, err)
				return err
			}

			if status.ResultCode != constant.InstanceRecoveryResultCodeSuccess {
				continue
			}
			logger.Infof("[initBuilderOptions] Skip dependency task of success instance: protection group(%d), job(%d), instance(%d) dependency task(%s)",
				b.RecoveryJob.ProtectionGroupID, b.RecoveryJob.RecoveryJobID, i.ProtectionClusterInstance.Id, status.RecoveryJobTaskID)
			skipTasks = append(skipTasks, b.getDependenciesTaskRecursive(status.RecoveryJobTaskID)...)
		}

		for _, task := range skipTasks {
			options.skipTaskMap[task] = true
		}
	}

	b.Options = &options

	return nil
}

func (b *TaskBuilder) loadMetadata(ctx context.Context) error {
	var err error
	if b.SharedTenantTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateTenant); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateTenant, err)
		return err
	}
	if b.SharedSecurityGroupTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateSecurityGroup); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSecurityGroup, err)
		return err
	}
	if b.SharedSecurityGroupRuleTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateSecurityGroupRule); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSecurityGroupRule, err)
		return err
	}

	if b.SharedInstanceSpecTaskIDMap, err = getInstanceSpecSharedTaskIDMap(ctx); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSpec, err)
		return err
	}

	if b.SharedNetworkTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateNetwork); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateNetwork, err)
		return err
	}
	if b.SharedSubnetTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateSubnet); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSubnet, err)
		return err
	}
	if b.SharedRouterTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateRouter); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateRouter, err)
		return err
	}
	if b.SharedKeypairTaskIDMap, err = migrator.GetSharedTaskIDMap(constant.MigrationTaskTypeCreateKeypair); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not set shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateKeypair, err)
		return err
	}

	if err := b.RecoveryJob.GetMetadata(taskBuilderTenantTaskIDMap, &b.TenantTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderTenantTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderInstanceSpecTaskIDMap, &b.InstanceSpecTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderInstanceSpecTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderSecurityGroupTaskIDMap, &b.SecurityGroupTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderSecurityGroupTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderSecurityGroupRuleTaskIDMap, &b.SecurityGroupRuleTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderSecurityGroupRuleTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderNetworkTaskIDMap, &b.NetworkTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderNetworkTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderSubnetTaskIDMap, &b.SubnetTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderSubnetTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderRouterTaskIDMap, &b.RouterTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderRouterTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderFloatingIPTaskIDMap, &b.FloatingIPTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderFloatingIPTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderVolumeImportTaskIDMap, &b.VolumeImportTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderVolumeImportTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderVolumeCopyTaskIDMap, &b.VolumeCopyTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderVolumeCopyTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderInstanceTaskIDMap, &b.InstanceTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderInstanceTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(retryTaskBuilderVolumeCopyTaskMap, &b.RetryVolumeCopyTaskMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, retryTaskBuilderVolumeCopyTaskMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(retryTaskBuilderVolumeImportTaskMap, &b.RetryVolumeImportTaskMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, retryTaskBuilderVolumeImportTaskMap, err)
		return err
	}
	if err := b.RecoveryJob.GetMetadata(taskBuilderKeypairTaskIDMap, &b.KeypairTaskIDMap); err != nil {
		logger.Infof("[builderTask-loadMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderKeypairTaskIDMap, err)
		return err
	}

	return nil
}

func (b *TaskBuilder) putMetadata(txn store.Txn) error {
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateTenant, b.SharedTenantTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateTenant, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateSecurityGroup, b.SharedSecurityGroupTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSecurityGroup, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateSecurityGroupRule, b.SharedSecurityGroupRuleTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSecurityGroupRule, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateSpec, b.SharedInstanceSpecTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSpec, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateNetwork, b.SharedNetworkTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateNetwork, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateSubnet, b.SharedSubnetTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateSubnet, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateRouter, b.SharedRouterTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateRouter, err)
		return err
	}
	if err := migrator.MergeSharedTaskIDMap(txn, constant.MigrationTaskTypeCreateKeypair, b.SharedKeypairTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not merge shared task id map: job(%d) taskType(%s). Cause: %+v",
			b.RecoveryJob.RecoveryJobID, constant.MigrationTaskTypeCreateKeypair, err)
		return err
	}

	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderTenantTaskIDMap, &b.TenantTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderTenantTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderInstanceSpecTaskIDMap, &b.InstanceSpecTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderInstanceSpecTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderKeypairTaskIDMap, &b.KeypairTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderKeypairTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderSecurityGroupTaskIDMap, &b.SecurityGroupTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderSecurityGroupTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderSecurityGroupRuleTaskIDMap, &b.SecurityGroupRuleTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderSecurityGroupRuleTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderNetworkTaskIDMap, &b.NetworkTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderNetworkTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderSubnetTaskIDMap, &b.SubnetTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderSubnetTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderRouterTaskIDMap, &b.RouterTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderRouterTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderFloatingIPTaskIDMap, &b.FloatingIPTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderFloatingIPTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderVolumeCopyTaskIDMap, &b.VolumeCopyTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderVolumeCopyTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderVolumeImportTaskIDMap, &b.VolumeImportTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderVolumeImportTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, taskBuilderInstanceTaskIDMap, &b.InstanceTaskIDMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, taskBuilderInstanceTaskIDMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, retryTaskBuilderVolumeImportTaskMap, &b.RetryVolumeImportTaskMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, retryTaskBuilderVolumeImportTaskMap, err)
		return err
	}
	if err := b.RecoveryJob.PutMetadata(txn, retryTaskBuilderVolumeCopyTaskMap, &b.RetryVolumeCopyTaskMap); err != nil {
		logger.Infof("[builderTask-putMetadata] Could not get metadata: dr.recovery.job/%d/metadata/%s. Cause: %+v",
			b.RecoveryJob.RecoveryJobID, retryTaskBuilderVolumeCopyTaskMap, err)
		return err
	}

	return nil
}

// 공유 task 의 input 을 가져와 현재 task 의 input 으로 설정 한다.
func (b *TaskBuilder) putSharedTask(txn store.Txn, typeCode, taskID string) error {
	logger.Infof("[putSharedTask] Run: job(%d) typeCode(%s) task(%s)", b.RecoveryJob.RecoveryJobID, typeCode, taskID)

	// etcd 에서 task type 에 따라 해당 공유 task 의 정보를 가져온다.
	// 예) cdm.dr.manager.recovery_job.builder.shared_tenant_task_map/1881d1f4-192a-48be-8bc7-d45126c072b1 -> 공유 tenant 에 대한 정보
	sharedTask, err := migrator.GetSharedTask(typeCode, taskID)
	if err != nil {
		logger.Errorf("[putSharedTask] Could not get shared task: job(%d) typeCode(%s) task(%s). Cause: %+v", b.RecoveryJob.RecoveryJobID, typeCode, taskID, err)
		return err
	}

	// 가져온 공유 task 의 recovery_job_id 정보를 채운다.
	sharedTask.RecoveryJobID = b.RecoveryJob.RecoveryJobID

	logger.Infof("[putSharedTask] set recovery task key: dr.recovery.job/%d/task/%s >> %s", sharedTask.RecoveryJobID, taskID, typeCode)
	// 가져온 공유 task 의 recovery_job_task_key 정보를 채운다.
	sharedTask.SetRecoveryTaskKey()

	logger.Infof("[putSharedTask] put store: job(%d) typeCode(%s) task(%s)", sharedTask.RecoveryJobID, typeCode, taskID)
	// 공유 task 값을 현재 task 정보로 etcd 에 저장한다.
	if err = sharedTask.Put(txn); err != nil {
		return err
	}

	t, err := json.Marshal(migrator.RecoveryJobTask{
		RecoveryJobTaskKey: sharedTask.RecoveryJobTaskKey,
		RecoveryJobID:      sharedTask.RecoveryJobID,
		RecoveryJobTaskID:  sharedTask.RecoveryJobTaskID,
		ReverseTaskID:      sharedTask.ReverseTaskID,
		TypeCode:           sharedTask.TypeCode,
		ResourceID:         sharedTask.ResourceID,
		ResourceName:       sharedTask.ResourceName,
		Input:              sharedTask.Input,
	})
	if err != nil {
		return errors.Unknown(err)
	}

	if err = broker.Publish(constant.QueueRecoveryJobTaskMonitor, &broker.Message{Body: t}); err != nil {
		logger.Warnf("[putSharedTask] Could not publish shared task. Cause: %+v", err)
	}

	return nil
}

// buildTasks 재해복구작업의 Task 들을 생성한다.
func (b *TaskBuilder) buildTasks(ctx context.Context, txn store.Txn) (err error) {

	defer func() {
		if err != nil {
			logger.Errorf("[buildTasks] Could not build job (%d) task. Cause: %+v", b.RecoveryJob.RecoveryJobID, err)
		}
	}()

	for _, plan := range b.RecoveryJobDetail.Plan.Detail.Instances {
		logger.Infof("[buildTasks] Building instance (%s) creation task", plan.ProtectionClusterInstance.Name)
		taskID, err := b.buildInstanceCreateTask(ctx, txn, plan)
		if err != nil {
			return err
		}

		is := &migrator.RecoveryJobInstanceStatus{
			RecoveryJobTaskID:     taskID,
			RecoveryPointTypeCode: b.RecoveryJobDetail.RecoveryPointTypeCode,
			RecoveryPoint:         b.RecoveryJob.RecoveryPoint,
			StateCode:             constant.RecoveryJobInstanceStateCodePreparing,
			ResultCode:            constant.InstanceRecoveryResultCodeFailed,
		}

		if err = b.RecoveryJob.SetInstanceStatus(txn, plan.ProtectionClusterInstance.Id, is); err != nil {
			logger.Errorf("[buildTasks] Could not set instance(%d) status. Cause: %+v", plan.ProtectionClusterInstance.Id, err)
			return err
		}

		if err = internal.PublishMessage(constant.QueueRecoveryJobInstanceMonitor, migrator.RecoveryJobInstanceMessage{
			RecoveryJobID:  b.RecoveryJob.RecoveryJobID,
			InstanceID:     plan.ProtectionClusterInstance.Id,
			InstanceStatus: is,
		}); err != nil {
			logger.Warnf("[buildTasks] Could not publish instance(%d) status. Cause: %+v", plan.ProtectionClusterInstance.Id, err)
		}

		logger.Infof("[buildTasks] Done - set instance status: dr.recovery.job/%d/result/instance/%d.",
			b.RecoveryJob.RecoveryJobID, plan.ProtectionClusterInstance.Id)
	}

	logger.Infof("[buildTasks] Success: job(%d)", b.RecoveryJob.RecoveryJobID)
	return nil
}

func (b *TaskBuilder) buildReverseTask(txn store.Txn, task *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error) {
	logger.Infof("[buildReverseTask] Start: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)

	var err error

	// 옵션으로 지정된 Task 에 대해서는 clear task 를 생성하지 않는다.
	if b.Options.skipTaskMap[task.RecoveryJobTaskID] {
		err = errTaskBuildSkipped
		logger.Errorf("[buildReverseTask] skip task build: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)
		return nil, err
	}

	result, err := task.GetResult()
	if err != nil {
		logger.Errorf("[buildReverseTask] Could not get task result: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)
		return nil, err
	}

	// 성공한 task 에 대한 clear task 만 생성한다.
	// 공유 task 일 경우, 공유 task 의 status 와 result 를 job 하위 task 의 status, result 로 설정한다.
	if result.ResultCode != constant.MigrationTaskResultCodeSuccess {
		logger.Infof("[buildReverseTask] result code - %s: job(%d) typeCode(%s)", result.ResultCode, b.RecoveryJob.RecoveryJobID, task.TypeCode)

		if task.SharedTaskKey == "" {
			logger.Infof("[buildReverseTask] task.SharedTaskKey is empty: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)
			return nil, nil
		}

		result, err = task.GetResult(migrator.WithSharedTask(true))
		switch {
		case errors.Equal(err, migrator.ErrNotFoundTaskResult):
			logger.Infof("[buildReverseTask] sharedTask - ErrNotFoundTaskResult: job(%d) typeCode(%s) sharedTaskKey(%s)",
				b.RecoveryJob.RecoveryJobID, task.TypeCode, task.SharedTaskKey)
			return nil, nil

		case err != nil:
			logger.Errorf("[buildReverseTask] Could not get shared task result: job(%d) typeCode(%s) sharedTaskKey(%s). Cause: %+v",
				b.RecoveryJob.RecoveryJobID, task.TypeCode, task.SharedTaskKey, err)
			return nil, err
		}

		logger.Infof("[buildReverseTask] shared task result code - %s: job(%d) typeCode(%s) sharedTaskKey(%s)",
			result.ResultCode, b.RecoveryJob.RecoveryJobID, task.TypeCode, task.SharedTaskKey)

		// 다른 작업 에서 공유 task 수행 성공 시,
		// 공유 task status, result 를 job/task status, result 로 설정
		if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
			status, err := task.GetStatus(migrator.WithSharedTask(true))
			if err != nil {
				logger.Errorf("[buildReverseTask] Could not get shared task status: job(%d) typeCode(%s) sharedTaskKey(%s) result(success). Cause: %+v",
					b.RecoveryJob.RecoveryJobID, task.TypeCode, task.SharedTaskKey, err)
				return nil, err
			}

			if err = store.Transaction(func(txn store.Txn) error {
				if err = task.SetStatus(txn, status); err != nil {
					return err
				}
				logger.Infof("[buildTasks] Done - set task status: dr.recovery.job/%d/task/%s/status {%s}.",
					b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, status.StateCode)

				return task.SetResult(txn, result)
			}); err != nil {
				logger.Errorf("Could not set task status(%s) and result(%s): job(%d) typeCode(%s) sharedTaskKey(%s). Cause: %+v",
					status.StateCode, result.ResultCode, b.RecoveryJob.RecoveryJobID, task.TypeCode, task.SharedTaskKey)
				return nil, err
			}
			logger.Infof("[buildTasks] Done - set task result: dr.recovery.job/%d/task/%s/result {%s}.",
				b.RecoveryJob.RecoveryJobID, task.RecoveryJobTaskID, result.ResultCode)

		} else {
			if c, err := task.GetConfirm(); c != nil || err != nil {
				// confirm 된 작업은 count 와 상관없이 삭제하지 않는다.
				if err != nil {
					logger.Warnf("[buildReverseTask] Could not get confirm info: job(%d) typeCode(%s) task(%s). Cause: %+v",
						b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				}
				logger.Infof("[buildReverseTask] Skipped confirmed task: job(%d) typeCode(%s) task(%s).", b.RecoveryJob.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
			} else {
				// 공유 task 수행 실패 시,
				// not found shared task 이거나 (= ref count 0)
				// ref count 1 일 경우(단일 복구 작업에서 수행 후 실패일 경우) 공유 task 를 지운다.
				var count uint64
				sharedTask, err := migrator.GetSharedTask(task.TypeCode, task.RecoveryJobTaskID)
				switch {
				case errors.Equal(err, migrator.ErrNotFoundSharedTask):
					break

				case err != nil:
					// not found shared task 가 아닌 error 는 return
					return nil, err

				default:
					// err == nil
					count, err = sharedTask.GetRefCount()
					if err != nil {
						return nil, err
					}
					logger.Infof("[buildReverseTask] ref count - %d: job(%d) typeCode(%s)", count, b.RecoveryJob.RecoveryJobID, task.TypeCode)
				}

				if count <= 1 {
					if err = store.Transaction(func(txn store.Txn) error {
						return migrator.DeleteSharedTask(txn, task.TypeCode, task.RecoveryJobTaskID, b.RecoveryJob.RecoveryCluster.Id)
					}); err != nil {
						return nil, err
					}

					logger.Infof("[buildReverseTask] DeleteSharedTask: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)

					b.DeletedSharedTaskMap[task.RecoveryJobTaskID] = true

					logger.Infof("[buildReverseTask] Success: job(%d) resultCode(%s) typeCode(%s)", b.RecoveryJob.RecoveryJobID, result.ResultCode, task.TypeCode)
					return nil, nil
				}
			}
		}
	}

	logger.Infof("[buildReverseTask] Run next - delete task: job(%d) typeCode(%s)", b.RecoveryJob.RecoveryJobID, task.TypeCode)
	switch task.TypeCode {
	case constant.MigrationTaskTypeCreateTenant:
		return b.buildTenantDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateSecurityGroup:
		return b.buildSecurityGroupDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateSecurityGroupRule:
		// Rule 은 SecurityGroup 삭제 Task 에서 같이 처리함
		return nil, nil

	case constant.MigrationTaskTypeImportVolume:
		return b.buildVolumeUnmanageTask(txn, task)

	case constant.MigrationTaskTypeCopyVolume:
		return b.buildVolumeCopyDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateSpec:
		return b.buildInstanceSpecDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateKeypair:
		return b.buildKeypairDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateFloatingIP:
		return b.buildFloatingIPDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateNetwork:
		return b.buildNetworkDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateSubnet:
		// Subnet 은 Network 삭제 Task 에서 같이 처리함
		return nil, nil

	case constant.MigrationTaskTypeCreateRouter:
		return b.buildRouterDeleteTask(txn, task)

	case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
		return b.buildInstanceDeleteTask(txn, task)

	}

	// 재해복구 작업 재시도 인 경우 ,DeleteCopyVolume, UnmanageVolume Task 들이 task 에 들어감
	return nil, nil
}
