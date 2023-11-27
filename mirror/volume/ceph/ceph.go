package ceph

import (
	"os"
	"strings"

	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/internal"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/internal/ceph"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/volume"
)

type cephStorage struct {
	vol *mirror.Volume

	sourceStorage *storage.ClusterStorage
	targetStorage *storage.ClusterStorage

	sourceCephMetadata map[string]interface{}
	targetCephMetadata map[string]interface{}

	sourceVolumeName string
}

func init() {
	volume.RegisterStorageFunc(storage.ClusterStorageTypeCeph, NewCephStorage)
}

// NewCephStorage cephStorage 생성 함수
func NewCephStorage(vol *mirror.Volume) (volume.Storage, error) {
	var err error
	if _, err = os.Stat(ceph.DefaultCephPath); os.IsNotExist(err) {
		if err = os.MkdirAll(ceph.DefaultCephPath, 0755); err != nil {
			logger.Errorf("[VolumeCeph-NewStorage] Could not make dir: %s. Cause: %+v", ceph.DefaultCephPath, err)
			return nil, errors.Unknown(err)
		}
	}

	s := &cephStorage{vol: vol, sourceVolumeName: internal.GetVolumeName(vol.SourceVolume.VolumeUUID)}
	if err = s.init(); err != nil {
		return nil, err
	}

	return s, nil
}

func (c *cephStorage) init() error {
	logger.Infof("[VolumeCeph-init] Start: source(%d:%d) target(%d:%d)",
		c.vol.SourceClusterStorage.ClusterID, c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.ClusterID, c.vol.TargetClusterStorage.StorageID)

	var err error
	if c.sourceStorage, err = c.vol.GetSourceStorage(); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not get source storage: clusters/%d/storages/%d. Cause: %+v",
			c.vol.SourceClusterStorage.ClusterID, c.vol.SourceClusterStorage.StorageID, err)
		return err
	}

	if c.targetStorage, err = c.vol.GetTargetStorage(); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not get target storage: clusters/%d/storages/%d. Cause: %+v",
			c.vol.TargetClusterStorage.ClusterID, c.vol.TargetClusterStorage.StorageID, err)
		return err
	}

	if c.sourceCephMetadata, err = c.sourceStorage.GetMetadata(); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not get source storage metadata: clusters/%d/storages/%d/metadata. Cause: %+v",
			c.sourceStorage.ClusterID, c.targetStorage.StorageID, err)
		return err
	}

	if c.targetCephMetadata, err = c.targetStorage.GetMetadata(); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not get target storage metadata: clusters/%d/storages/%d/metadata. Cause: %+v",
			c.sourceStorage.ClusterID, c.targetStorage.StorageID, err)
		return err
	}

	if err := c.vol.SetTargetMetadata(c.targetCephMetadata); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not set source target metadata: clusters/%d/storages/%d/metadata. Cause: %+v",
			c.targetStorage.ClusterID, c.targetStorage.StorageID, err)
		return err
	}
	logger.Infof("[VolumeCeph-init] Done - set source target metadata: clusters/%d/storages/%d/metadata.",
		c.targetStorage.ClusterID, c.targetStorage.StorageID)

	if err = ceph.CheckValidCephMetadata(c.sourceCephMetadata, c.targetCephMetadata); err != nil {
		logger.Errorf("[VolumeCeph-init] Error occurred during validating the metadata: source(%d:%d) target(%d:%d). Cause: %+v",
			c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID, err)
		return err
	}

	var sourceKey = c.sourceStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(sourceKey, c.sourceCephMetadata); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not create source storage config file: %s. Cause: %+v", sourceKey, err)
		return err
	}

	var targetKey = c.targetStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(targetKey, c.targetCephMetadata); err != nil {
		logger.Errorf("[VolumeCeph-init] Could not create target storage config file:  %s. Cause: %+v", targetKey, err)
		return err
	}

	logger.Infof("[VolumeCeph-init] Success: source(%d:%d) target(%d:%d)",
		c.vol.SourceClusterStorage.ClusterID, c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.ClusterID, c.vol.TargetClusterStorage.StorageID)
	return nil
}

// Start 볼륨 복제 시작 함수
// target image 정보와 source image 정보 확인 및 mirror enable 설정
func (c *cephStorage) Start() error {
	logger.Infof("[VolumeCeph-Start] Run >> PrepareMirroringRBDImage: source(%d:%d) target(%d:%d)",
		c.vol.SourceClusterStorage.ClusterID, c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.ClusterID, c.vol.TargetClusterStorage.StorageID)

	return ceph.PrepareMirroringRBDImage(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata)
}

// Stop ceph 볼륨 복제 중지 함수
// Stop 함수에 경우 target 쪽의 volume 및 스냅샷을 삭제 하지 않는다
func (c *cephStorage) Stop() error {
	if err := c.vol.SetStatus(constant.MirrorVolumeStateCodeStopping, "", nil); err != nil {
		logger.Errorf("[VolumeWorker-pause] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s}",
			c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID, constant.MirrorVolumeStateCodeStopping)
		return err
	}
	logger.Infof("[VolumeWorker-Stop] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s}",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID, constant.MirrorVolumeStateCodeStopping)

	logger.Infof("[VolumeCeph-Stop] Run >> StopRBDMirrorImage: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return ceph.StopRBDMirrorImage(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata, c.vol)
}

// StopAndDelete 볼륨 복제 중지 및 삭제 함수
// StopAndDelete 함수에 경우 target 쪽의 volume 및 스냅샷을 삭제 한다.
func (c *cephStorage) StopAndDelete() error {
	if err := c.vol.SetStatus(constant.MirrorVolumeStateCodeDeleting, "", nil); err != nil {
		logger.Errorf("[VolumeWorker-StopAndDelete] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s}",
			c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID, constant.MirrorVolumeStateCodeDeleting)
		return err
	}
	logger.Infof("[VolumeWorker-StopAndDelete] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s}",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID, constant.MirrorVolumeStateCodeDeleting)

	logger.Infof("[VolumeCeph-StopAndDelete] Run >> StopAndDeleteRBDImage: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return ceph.StopAndDeleteRBDImage(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata, c.vol)
}

// Pause 볼륨 복제 일시 중지 함수
// target image 를 promote 로 변경 한다.
func (c *cephStorage) Pause() error {
	logger.Infof("[VolumeCeph-Pause] Run >> SetRBDMirrorImagePromote: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return ceph.SetRBDMirrorImagePromote(c.targetStorage.GetKey(), c.sourceVolumeName, c.targetCephMetadata)
}

// Resume 볼륨 복제 재개 함수
func (c *cephStorage) Resume() error {
	logger.Infof("[VolumeCeph-Resume] Start: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)

	err := ceph.SetTargetRBDImageDemote(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata)
	switch {
	case err != nil && (errors.Equal(err, ceph.ErrNotFoundMirrorImage) || errors.Equal(err, ceph.ErrNotSetMirrorEnable)):
		logger.Infof("[VolumeCeph-Resume] Run >> mirror start: source(%d:%d) target(%d:%d)",
			c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
		return c.Start()

	case err != nil:
		return err
	}

	logger.Infof("[VolumeCeph-Resume] Success: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return nil
}

// MirrorEnable 볼륨 복제 시작을 지속 적으로 활성화 시키는 함수
func (c *cephStorage) MirrorEnable(status string) error {
	logger.Infof("[VolumeCeph-MirrorEnable] Start: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)

	var rbdErr, err error

	state, desc, rbdErr := ceph.GetRBDMirrorImageStatus(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata)
	switch {
	case errors.Equal(rbdErr, ceph.ErrNotFoundMirrorImage):
		return nil

	// ceph storage 에 정상적으로 접근할 수 없는 경우
	case errors.Equal(rbdErr, ceph.ErrTimeoutExecCommand):
		// protection cluster ceph storage 접근이 안될 때는 재해복구가 가능하도록 mirroring image 를 promote 상태로 바꾼다.
		// recovery cluster ceph storage 접근이 안될 때는 error 를 return 한다.
		if err := c.Pause(); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not mirror pause. Cause: %+v", err)
			return err
		}

	case rbdErr != nil:
		return rbdErr
	}

	if state == "up+replaying" && status == constant.MirrorVolumeStateCodeMirroring {
		return nil
	}

	if (state == "up+starting_replay" || state == "up+syncing") && status == constant.MirrorVolumeStateCodeInitializing {
		return nil
	}

	// down 상태: rbd-mirror daemon 이 종료 되었을 경우
	// up 상태: rbd-mirror daemon 이 실행 중인 경우
	switch {
	case state == "down+error":
		// split-brain detected
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-down_error", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "down+stopped":
		//description: force promoted
		//description: stopped
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-down_stopped", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "down+unknown":
		//description:status not found
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-down_unknown", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "up+stopped":
		// description: force promoted
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-up_stopped", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

		if strings.Contains(desc, "local image is primary") || strings.Contains(desc, "force promoted") {
			rbdErr = ceph.SetTargetRBDImageDemote(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata)
		}

	case state == "up+error":
		// description: split-brain detected
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-up_error", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

		switch {
		case strings.Contains(desc, "failed to commit journal event"):
			rbdErr = ceph.RunRBDMirrorImageDisable(c.sourceStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata)

		case strings.Contains(desc, "split-brain detected"):
			rbdErr = ceph.ReSyncTargetRBDImage(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata)

		case strings.Contains(desc, "remote image does not exist"):
			rbdErr = ceph.SetRBDMirrorImageEnable(c.sourceStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata)
			// ceph storage 에 정상적으로 접근할 수 없는 경우
			if errors.Equal(rbdErr, ceph.ErrTimeoutExecCommand) {
				// protection cluster ceph storage 접근이 안될 때는 재해복구가 가능하도록 mirroring image 를 promote 상태로 바꾼다.
				// recovery cluster ceph storage 접근이 안될 때는 error 를 return 한다.
				if err := c.Pause(); err != nil {
					return err
				}
			}

			rbdErr = ceph.DeleteRBDImage(c.targetStorage.GetKey(), c.sourceVolumeName, c.targetCephMetadata)
		}

	case state == "down+replaying":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-down_replaying", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "down+starting_replay":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-down_starting_replay", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "up+replaying":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeMirroring, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-up_replaying", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeMirroring, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeMirroring, state)

		rbdErr = ceph.SetRBDMirrorImageEnable(c.sourceStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata)

	case state == "up+starting_replay", state == "up+syncing":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeInitializing, "cdm-dr.mirror.volume_ceph_mirror_enable.success-set_status-up_starting_replay", nil); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, state)
			return err
		}
		logger.Infof("[VolumeCeph-MirrorEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, state)

		rbdErr = ceph.SetRBDMirrorImageEnable(c.sourceStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata)
	}

	// rbd 명령어 실패 시 통합 에러 처리
	switch {
	// ceph storage 에 정상적으로 접근할 수 없는 경우
	case errors.Equal(rbdErr, ceph.ErrTimeoutExecCommand):
		// protection cluster ceph storage 접근이 안될 때는 재해복구가 가능하도록 mirroring image 를 promote 상태로 바꾼다.
		// recovery cluster ceph storage 접근이 안될 때는 error 를 return 한다.
		if err := c.Pause(); err != nil {
			logger.Errorf("[VolumeCeph-MirrorEnable] Could not mirror pause. Cause: %+v", err)
			return err
		}

	case rbdErr != nil:
		return rbdErr
	}

	logger.Infof("[VolumeCeph-MirrorEnable] Success: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return nil
}

// PauseEnable 볼륨 복제 일시 중지를 지속 적으로 활성화 시키는 함수
func (c *cephStorage) PauseEnable(status string) error {
	logger.Infof("[VolumeCeph-PauseEnable] Start: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)

	state, desc, err := ceph.GetRBDMirrorImageStatus(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceVolumeName, c.sourceCephMetadata, c.targetCephMetadata)
	switch {
	case errors.Equal(err, ceph.ErrNotFoundMirrorImage):
		return nil

	case err != nil:
		return err
	}

	switch {
	case state == "down+error":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-down_error", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "down+stopped":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodePaused, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-down_stopped", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, state)

	case state == "down+unknown":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-down_unknown", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "up+stopped":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodePaused, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-up_stopped", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, state)

	case state == "up+error":
		switch {
		case strings.Contains(desc, "split-brain detected") || strings.Contains(desc, "remote image does not exist"):
			if err = c.vol.SetStatus(constant.MirrorVolumeStateCodePaused, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-up_error", nil); err != nil {
				logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
					c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, state)
				return err
			}
			logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, state)

		default:
			if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-up_error", nil); err != nil {
				logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
					c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
				return err
			}
			logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
		}

	case state == "down+replaying":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-down_replaying", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "down+starting_replay":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-down_starting_replay", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, state)

	case state == "up+replaying":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeMirroring, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-up_replaying", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeMirroring, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeMirroring, state)

		if err = c.Pause(); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not mirror pause. Cause: %+v", err)
			return err
		}

	case state == "up+starting_replay" || state == "up+syncing":
		if err = c.vol.SetStatus(constant.MirrorVolumeStateCodeInitializing, "cdm-dr.mirror.volume_ceph_pause_enable.success-set_status-up_starting_replay", nil); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, state)
			return err
		}
		logger.Infof("[VolumeCeph-PauseEnable] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			c.vol.SourceClusterStorage.StorageID, c.vol.TargetClusterStorage.StorageID, c.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, state)

		if err = c.Pause(); err != nil {
			logger.Errorf("[VolumeCeph-PauseEnable] Could not mirror pause. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[VolumeCeph-PauseEnable] Success: source(%d:%d) target(%d:%d)",
		c.sourceStorage.ClusterID, c.sourceStorage.StorageID, c.targetStorage.ClusterID, c.targetStorage.StorageID)
	return nil
}
