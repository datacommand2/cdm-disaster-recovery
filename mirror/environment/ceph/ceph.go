package ceph

import (
	"10.1.1.220/cdm/cdm-center/services/cluster-manager/storage"
	"10.1.1.220/cdm/cdm-cloud/common/errors"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/internal/ceph"
	"github.com/micro/go-micro/v2/logger"

	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/environment"
	"github.com/shirou/gopsutil/process"
	"os"
	"os/exec"
)

type cephStorage struct {
	pc *os.Process

	env *mirror.Environment

	sourceStorage *storage.ClusterStorage
	targetStorage *storage.ClusterStorage

	sourceCephMetadata map[string]interface{}
	targetCephMetadata map[string]interface{}
}

func init() {
	environment.RegisterStorageFunc(storage.ClusterStorageTypeCeph, NewCephStorage)
}

// NewCephStorage cephStorage 생성 함수
func NewCephStorage(env *mirror.Environment) (environment.Storage, error) {
	var err error
	if _, err = os.Stat(ceph.DefaultCephPath); os.IsNotExist(err) {
		if err = os.MkdirAll(ceph.DefaultCephPath, 0755); err != nil {
			logger.Errorf("[EnvironmentCeph-NewStorage] Could not make dir: %s. Cause: %+v", ceph.DefaultCephPath, err)
			return nil, errors.Unknown(err)
		}
	}

	c := &cephStorage{env: env}
	if err = c.init(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *cephStorage) init() error {
	logger.Infof("[EnvironmentCeph-init] Start: source(%d:%d) target(%d:%d)",
		c.env.SourceClusterStorage.ClusterID, c.env.SourceClusterStorage.StorageID, c.env.TargetClusterStorage.ClusterID, c.env.TargetClusterStorage.StorageID)

	var err error
	if c.sourceStorage, err = c.env.GetSourceStorage(); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Could not get source storage: clusters/%d/storages/%d. Cause: %+v",
			c.env.SourceClusterStorage.ClusterID, c.env.SourceClusterStorage.StorageID, err)
		return err
	}

	if c.targetStorage, err = c.env.GetTargetStorage(); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Could not get target storage: clusters/%d/storages/%d. Cause: %+v",
			c.env.TargetClusterStorage.ClusterID, c.env.TargetClusterStorage.StorageID, err)
		return err
	}

	if c.sourceCephMetadata, err = c.sourceStorage.GetMetadata(); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Could not get source storage metadata: clusters/%d/storages/%d/metadata. Cause: %+v",
			c.sourceStorage.ClusterID, c.sourceStorage.StorageID, err)
		return err
	}

	if c.targetCephMetadata, err = c.targetStorage.GetMetadata(); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Could not get target storage metadata: clusters/%d/storages/%d/metadata. Cause: %+v",
			c.targetStorage.ClusterID, c.targetStorage.StorageID, err)
		return err
	}

	if err = ceph.CheckValidCephMetadata(c.sourceCephMetadata, c.targetCephMetadata); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Error occurred during validating the metadata: source(%d:%d) target(%d:%d). Cause: %+v",
			c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID, err)
		return err
	}

	var sourceKey = c.sourceStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(sourceKey, c.sourceCephMetadata); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Could not create source storage config file: %s. Cause: %+v", sourceKey, err)
		return err
	}

	var targetKey = c.targetStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(targetKey, c.targetCephMetadata); err != nil {
		logger.Errorf("[EnvironmentWorker-init] Could not create target storage config file: %s. Cause: %+v", targetKey, err)
		return err
	}

	logger.Infof("[EnvironmentCeph-init] Success: source(%d:%d) target(%d:%d)",
		c.env.SourceClusterStorage.ClusterID, c.env.SourceClusterStorage.StorageID, c.env.TargetClusterStorage.ClusterID, c.env.TargetClusterStorage.StorageID)
	return nil
}

// Prepare ceph storage 복제 환경 구성 함수
// config 생성 , mirror pool image enable ,peer 등록 진행
func (c *cephStorage) Prepare() error {
	logger.Infof("[EnvironmentCeph-Prepare] Run >> RunRBDMirrorPeerAddProcess: source(%d:%d) target(%d:%d)",
		c.env.SourceClusterStorage.ClusterID, c.env.SourceClusterStorage.StorageID, c.env.TargetClusterStorage.ClusterID, c.env.TargetClusterStorage.StorageID)

	return ceph.RunRBDMirrorPeerAddProcess(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceCephMetadata, c.targetCephMetadata)
}

// Start ceph storage rbd-mirror process 실행 함수
func (c *cephStorage) Start() error {
	logger.Infof("[EnvironmentCeph-Start] Start: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)

	execCmdString := ceph.RbdMirrorRunCommand(c.targetStorage.GetKey(), c.targetCephMetadata["client"].(string))
	cmd := exec.Command("sh", "-c", execCmdString)
	if err := cmd.Start(); err != nil {
		err = ceph.UnexpectedExecResult(execCmdString, err)
		logger.Errorf("[EnvironmentCeph-Start] Error occurred during exec command. Cause: %+v", err)
		return err
	}

	c.pc = cmd.Process

	go func() {
		_ = cmd.Wait()
	}()

	logger.Infof("[EnvironmentCeph-Start] Success: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return nil
}

// Destroy ceph storage 복제 환경 제거 함수
// peer remove, mirror pool disable 진행
func (c *cephStorage) Destroy() error {
	logger.Infof("[EnvironmentCeph-Destroy] Start: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)

	var err error

	var sourceKey = c.sourceStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(sourceKey, c.sourceCephMetadata); err != nil {
		logger.Errorf("[EnvironmentCeph-Destroy] Could not create source storage config file: %s. Cause: %+v", sourceKey, err)
		return err
	}

	var targetKey = c.targetStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(targetKey, c.targetCephMetadata); err != nil {
		logger.Errorf("[EnvironmentCeph-Destroy] Could not create target storage config file: %s. Cause: %+v", targetKey, err)
		return err
	}

	if err = ceph.RunRBDMirrorPeerRemoveProcess(sourceKey, targetKey, c.sourceCephMetadata, c.targetCephMetadata); err != nil {
		logger.Errorf("[EnvironmentCeph-Destroy] Could not run rbd mirror peer remove process: source(%s) target(%s). Cause: %+v", sourceKey, targetKey, err)
		return err
	}

	if err = ceph.DeleteRBDConfig(sourceKey, c.sourceCephMetadata); err != nil {
		logger.Errorf("[EnvironmentCeph-Destroy] Could not delete source rbd config file: %s. Cause: %+v", sourceKey, err)
		return err
	}

	if err = ceph.DeleteRBDConfig(targetKey, c.targetCephMetadata); err != nil {
		logger.Errorf("[EnvironmentCeph-Destroy] Could not delete target rbd config file: %s. Cause: %+v", targetKey, err)
		return err
	}

	logger.Infof("[EnvironmentCeph-Destroy] Success: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return nil
}

// Stop ceph storage rbd-mirror process 종료 함수
func (c *cephStorage) Stop() error {
	if c.pc == nil {
		return nil
	}

	return c.pc.Kill()
}

// Monitor ceph storage 의 rbd-mirror process 모니터링 함수
func (c *cephStorage) Monitor() error {
	if ok, _ := process.PidExists(int32(c.pc.Pid)); !ok {
		return ceph.NotFoundRBDMirrorProcess(c.env.SourceClusterStorage.ClusterID, c.env.SourceClusterStorage.StorageID,
			c.env.TargetClusterStorage.ClusterID, c.env.TargetClusterStorage.StorageID)
	}

	return nil
}
