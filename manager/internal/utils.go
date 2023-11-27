package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-cloud/common/store"
	identity "github.com/datacommand2/cdm-cloud/services/identity/proto"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
)

// IsAdminUser admin 역할의 사용자인지 확인
func IsAdminUser(user *identity.User) bool {
	for _, r := range user.Roles {
		if r.Role == "admin" {
			return true
		}
	}
	return false
}

// IsGroupUser 그룹에 속한 사용자인지 확인
func IsGroupUser(user *identity.User, groupID uint64) bool {
	for _, g := range user.Groups {
		if groupID == g.Id {
			return true
		}
	}
	return false
}

// GetTenantRecoveryPlan 재해복구계획에서 테넌트 복구계획을 찾아 반환한다.
func GetTenantRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.TenantRecoveryPlan {
	for _, p := range plan.Detail.Tenants {
		if p.ProtectionClusterTenant.Id == id {
			return p
		}
	}

	return nil
}

// GetAvailabilityZoneRecoveryPlan 재해복구계획에서 가용구역 복구계획을 찾아 반환한다.
func GetAvailabilityZoneRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.AvailabilityZoneRecoveryPlan {
	for _, p := range plan.Detail.AvailabilityZones {
		if p.ProtectionClusterAvailabilityZone.Id == id {
			return p
		}
	}

	return nil
}

// GetExternalNetworkRecoveryPlan 재해복구계획에서 외부 네트워크 복구계획을 찾아 반환한다.
func GetExternalNetworkRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.ExternalNetworkRecoveryPlan {
	for _, p := range plan.Detail.ExternalNetworks {
		if p.ProtectionClusterExternalNetwork.Id == id {
			return p
		}
	}

	return nil
}

// GetRouterRecoveryPlan 재해복구계획에서 라우터 복구계획을 찾아 반환한다.
func GetRouterRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.RouterRecoveryPlan {
	for _, p := range plan.Detail.Routers {
		if p.ProtectionClusterRouter.Id == id {
			return p
		}
	}

	return nil
}

// GetStorageRecoveryPlan 재해복구계획에서 스토리지 복구계획을 찾아 반환한다.
func GetStorageRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.StorageRecoveryPlan {
	for _, p := range plan.Detail.Storages {
		if p.ProtectionClusterStorage.Id == id {
			return p
		}
	}

	return nil
}

// GetFloatingIPRecoveryPlan 재해복구계획에서 Floating IP 복구계획을 찾아 반환한다.
func GetFloatingIPRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.FloatingIPRecoveryPlan {
	for _, p := range plan.Detail.FloatingIps {
		if p.ProtectionClusterFloatingIp.Id == id {
			return p
		}
	}

	return nil
}

// GetInstanceRecoveryPlan 재해복구계획에서 인스턴스의 복구계획을 찾아 반환한다.
func GetInstanceRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.InstanceRecoveryPlan {
	for _, p := range plan.Detail.Instances {
		if p.ProtectionClusterInstance.Id == id {
			return p
		}
	}

	return nil
}

// GetInstanceRecoveryPlanUsingVolume 재해복구계획에서 해당 볼륨을 사용하는 인스턴스의 복구계획을 찾아 반환한다.
func GetInstanceRecoveryPlanUsingVolume(plan *drms.RecoveryPlan, volID uint64) *drms.InstanceRecoveryPlan {
	for _, p := range plan.Detail.Instances {
		for _, v := range p.ProtectionClusterInstance.Volumes {
			if v.Volume.Id == volID {
				return p
			}
		}
	}

	return nil
}

// GetVolumeRecoveryPlan 재해복구계획에서 볼륨 복구계획을 찾아 반환한다.
func GetVolumeRecoveryPlan(plan *drms.RecoveryPlan, id uint64) *drms.VolumeRecoveryPlan {
	for _, p := range plan.Detail.Volumes {
		if p.ProtectionClusterVolume.Id == id {
			return p
		}
	}

	return nil
}

// IsAccessibleProtectionGroup 은 접근 가능한 보호그룹인지 확인하는 함수이다.
func IsAccessibleProtectionGroup(ctx context.Context, id uint64) error {
	var err error
	user, _ := metadata.GetAuthenticatedUser(ctx)
	tid, _ := metadata.GetTenantID(ctx)

	var pg model.ProtectionGroup
	err = database.Execute(func(db *gorm.DB) error {
		return db.First(&pg, &model.ProtectionGroup{ID: id, TenantID: tid}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return NotFoundProtectionGroup(id, tid)

	case err != nil:
		return errors.UnusableDatabase(err)
	}

	// admin 의 경우 모든 보호그룹에 대한 권한이 있다.
	if !IsAdminUser(user) && !IsGroupUser(user, pg.OwnerGroupID) {
		return errors.UnauthorizedRequest(ctx)
	}

	return nil
}

// EncodeProtectionGroupName 보호그룹 이름을 이용해 해시값 계산
func EncodeProtectionGroupName(ctx context.Context, id uint64) (string, error) {
	tid, _ := metadata.GetTenantID(ctx)

	var pg model.ProtectionGroup
	err := database.Execute(func(db *gorm.DB) error {
		return db.First(&pg, &model.ProtectionGroup{ID: id, TenantID: tid}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return "", NotFoundProtectionGroup(id, tid)

	case err != nil:
		return "", errors.UnusableDatabase(err)
	}

	b := sha256.Sum256([]byte(pg.Name))
	return hex.EncodeToString(b[:4]), nil
}

// IsAccessibleRecoveryReport 접근 가능한 복구결과 보고서 인지 확인하는 함수이다.
func IsAccessibleRecoveryReport(ctx context.Context, ogID uint64) error {
	user, _ := metadata.GetAuthenticatedUser(ctx)

	// admin 의 경우 모든 보호그룹에 대한 권한이 있다.
	if !IsAdminUser(user) && !IsGroupUser(user, ogID) {
		return errors.UnauthorizedRequest(ctx)
	}

	return nil
}

// StartVolumeMirroring etcd 에 volume mirroring 을 시작하는 정보와 operation 을 추가한다.
func StartVolumeMirroring(v *mirror.Volume, pid uint64) error {
	var err error

	// db 에서 해당 값이 등록되어 있지 않는 경우 mirror daemon 에서 61001 로 사용하기 때문에
	// 사용자가 61001 로 db 에 등록시에 변경감지 하여 stop mirroring 을 하지 않기 위해 임시 default 값 지정
	// TODO: 추후 해당 port 값을 필수 등록값으로 하여 kv store 또는 db 값을 가져다 쓸경우에는 해당 부분 삭제 필요함
	if v.SourceAgent.Port == 0 {
		v.SourceAgent.Port = 61001
	}

	// source volume 을 mirroring 하고 있는 target reference count 를 증가시킨다.
	if err = store.Transaction(func(txn store.Txn) error {
		if err = v.IncreaseRefCount(txn); err != nil {
			logger.Errorf("[StartVolumeMirroring] Could not increase the reference count(volumeID:%d). Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		// volume mirror 정보를 etcd 에 저장한다.
		if err = v.Put(); err != nil {
			logger.Errorf("[StartVolumeMirroring] Could not put the mirror volume(%d) info. Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		// mirror volume 의 대상이 되는 plan 의 id 값을 etcd 에 저장한다.
		if err = v.SetPlanOfMirrorVolume(pid); err != nil {
			logger.Errorf("[StartVolumeMirroring] Could not set the mirror volume(%d) plan id. Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		// volume mirroring operation 을 etcd 에 저장한다.
		if err = v.SetOperation(constant.MirrorVolumeOperationStart); err != nil {
			logger.Errorf("[StartVolumeMirroring] Could not set the volume(%d) mirroring operation info. Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		// volume mirroring status 를 waiting 으로 초기화한다.
		if err := v.SetStatus(constant.MirrorVolumeStateCodeWaiting, "", nil); err != nil {
			logger.Errorf("[StartVolumeMirroring] Could not set the volume(%d) mirroring status. Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		return nil

	}); err != nil {
		return err
	}

	logger.Infof("[StartVolumeMirroring] Start volume(%d) mirroring.", v.SourceVolume.VolumeID)

	return nil
}

// StopVolumeMirroring volume mirroring 중지하는 operation 을 추가한다.
// option 에 따라 stop 혹은 stop & delete operation 중에 하나의 값을 operation 으로 지정한다.
func StopVolumeMirroring(v *mirror.Volume, op string) error {
	status, err := v.GetStatus()
	if err != nil {
		logger.Errorf("[StopVolumeMirroring] Could not get the volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			v.SourceClusterStorage.StorageID, v.TargetClusterStorage.StorageID, v.SourceVolume.VolumeID, err)
		return err
	}

	// volume mirroring 이 이미 정지된 상태이다.
	if status.StateCode == constant.MirrorVolumeStateCodeStopped {
		logger.Infof("[StopVolumeMirroring] Volume(%d) is already stopped.", v.SourceVolume.VolumeID)
		return nil
	}

	if err := store.Transaction(func(txn store.Txn) error {
		// source volume 을 mirroring 하고 있는 target reference count 를 감소시킨다.
		if err := v.DecreaseRefCount(txn); err != nil {
			logger.Errorf("[StopVolumeMirroring] Could not decrease the reference count(volumeID:%d). Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		if err := v.SetOperation(op); err != nil {
			logger.Errorf("[StopVolumeMirroring] Could not set the volume(%d) mirroring operation info. Cause: %+v", v.SourceVolume.VolumeID, err)
			return err
		}

		return nil

	}); err != nil {
		return err
	}

	logger.Infof("[StopVolumeMirroring] Stop volume(%d) mirroring.", v.SourceVolume.VolumeID)

	return nil
}

// PauseVolumeMirroring etcd 에 volume mirroring 을 일시정지하는 operation 을 추가한다.
func PauseVolumeMirroring(v *mirror.Volume) error {
	status, err := v.GetStatus()
	if err != nil {
		logger.Errorf("[PauseVolumeMirroring] Could not get the volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			v.SourceClusterStorage.StorageID, v.TargetClusterStorage.StorageID, v.SourceVolume.VolumeID, err)
		return err
	}

	if status.StateCode == constant.MirrorVolumeStateCodePaused {
		logger.Infof("[PauseVolumeMirroring] Mirror volume(%d) state is already paused: %s", v.SourceVolume.VolumeID)
		return nil
	} else if status.StateCode != constant.MirrorVolumeStateCodeMirroring {
		// 현재 상태가 mirroring 이 아닌 mirror volume 은 pause 할 수 없다.
		logger.Warnf("[PauseVolumeMirroring] Mirror volume(%d) state is not mirroring: %s", v.SourceVolume.VolumeID, status.StateCode)
		return VolumeIsNotMirroring(v.SourceVolume.VolumeID, status.StateCode)
	}

	if err := v.SetOperation(constant.MirrorVolumeOperationPause); err != nil {
		logger.Errorf("[PauseVolumeMirroring] Could not set the volume(%d) mirroring operation info. Cause: %+v", v.SourceVolume.VolumeID, err)
		return err
	}

	logger.Infof("[PauseVolumeMirroring] Pause volume(%d) mirroring.", v.SourceVolume.VolumeID)

	return nil
}

// DestroyVolumeMirroring etcd 에서 volume mirroring 정보를 삭제하는 operation 을 추가한다.
func DestroyVolumeMirroring(v *mirror.Volume) error {
	if err := v.SetOperation(constant.MirrorVolumeOperationDestroy); err != nil {
		logger.Errorf("[DestroyVolumeMirroring] Could not set the volume(%d) mirroring operation info. Cause: %+v", v.SourceVolume.VolumeID, err)
		return err
	}

	logger.Infof("[DestroyVolumeMirroring] Destroy volume(%d) mirroring.", v.SourceVolume.VolumeID)

	return nil
}

// PublishMessage message marshal 하여 publish
func PublishMessage(topic string, obj interface{}) error {
	if topic == "" {
		return nil
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	return broker.Publish(topic, &broker.Message{Body: b})
}
