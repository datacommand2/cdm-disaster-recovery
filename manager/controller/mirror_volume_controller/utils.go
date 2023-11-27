package mirrorvolumecontroller

import (
	"context"
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-center/cluster-manager/volume"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
	"reflect"
)

func getDesiredMirrorEnvironmentMap(ctx context.Context) (map[string]*mirrorEnvironment, error) {
	var envMap = make(map[string]*mirrorEnvironment)

	var records []struct {
		Group model.ProtectionGroup `gorm:"embedded"`
		Plan  model.Plan            `gorm:"embedded"`
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Table(model.ProtectionGroup{}.TableName()).
			Select("*").
			Joins("join cdm_disaster_recovery_plan on cdm_disaster_recovery_plan.protection_group_id = cdm_disaster_recovery_protection_group.id").
			Find(&records).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, r := range records {
		plan, err := recoveryPlan.Get(ctx, &drms.RecoveryPlanRequest{
			GroupId: r.Group.ID,
			PlanId:  r.Plan.ID,
		})
		if err != nil {
			return nil, err
		}

		storageMap := make(map[uint64]uint64)
		for _, s := range plan.Detail.Storages {
			// 복구대상 클러스터 스토리지 변경이 필요한 경우, 복제환경을 구성하지 않는다.
			if s.RecoveryClusterStorageUpdateFlag || s.RecoveryClusterStorage.GetId() == 0 {
				continue
			}

			storageMap[s.ProtectionClusterStorage.Id] = s.RecoveryClusterStorage.GetId()
		}

		for _, v := range plan.Detail.Volumes {
			// 볼륨을 복제할 복구대상 클러스터 스토리지 변경이 필요한 경우, 복제하지 않는다.
			if v.RecoveryClusterStorageUpdateFlag {
				continue
			}

			// 볼륨을 사용하는 인스턴스가 없는 경우, 복제하지 않는다.
			i := internal.GetInstanceRecoveryPlanUsingVolume(plan, v.ProtectionClusterVolume.Id)
			if i == nil {
				continue
			}

			// 볼륨을 사용하는 인스턴스가 하이퍼바이저에 배치되어있지 않은 경우, 볼륨을 복제하지 않는다.
			if i.ProtectionClusterInstance.Hypervisor == nil {
				continue
			}

			// 볼륨을 복제할 복구대상 스토리지가 없다면, 복제하지 않는다.
			if v.RecoveryClusterStorage.GetId() == 0 && storageMap[v.ProtectionClusterVolume.Storage.Id] == 0 {
				continue
			}

			mv := mirror.Volume{
				SourceClusterStorage: &storage.ClusterStorage{
					ClusterID: plan.ProtectionCluster.Id,
					StorageID: v.ProtectionClusterVolume.Storage.Id,
				},
				TargetClusterStorage: &storage.ClusterStorage{
					ClusterID: plan.RecoveryCluster.Id,
					StorageID: v.RecoveryClusterStorage.GetId(),
				},
				SourceVolume: &volume.ClusterVolume{
					ClusterID:  plan.ProtectionCluster.Id,
					VolumeID:   v.ProtectionClusterVolume.Id,
					VolumeUUID: v.ProtectionClusterVolume.Uuid,
				},
				SourceAgent: &cdmReplicator.Agent{
					IP:   i.ProtectionClusterInstance.Hypervisor.IpAddress,
					Port: uint(i.ProtectionClusterInstance.Hypervisor.AgentPort),
				},
			}

			if mv.TargetClusterStorage.StorageID == 0 {
				mv.TargetClusterStorage.StorageID = storageMap[v.ProtectionClusterVolume.Storage.Id]
			}

			// db 에서 해당 값이 등록되어 있지 않는 경우 mirror daemon 에서 61001 로 사용하기 때문에
			// 사용자가 61001 로 db 에 등록시에 변경감지 하여 stop mirroring 을 하지 않기 위해 임시 default 값 지정
			// TODO: 추후 해당 port 값을 필수 등록값으로 하여 kv store 또는 db 값을 가져다 쓸경우에는 해당 부분 삭제 필요함
			if mv.SourceAgent.Port == 0 {
				mv.SourceAgent.Port = 61001
			}

			env := mirror.Environment{
				SourceClusterStorage: mv.SourceClusterStorage,
				TargetClusterStorage: mv.TargetClusterStorage,
			}

			if envMap[env.GetKey()] == nil {
				envMap[env.GetKey()] = &mirrorEnvironment{
					env:              &env,
					volumes:          make(map[string]volumeInfo),
					disasterRecovery: !r.Plan.ActivationFlag, //재해복구 flag = false
				}
			}

			envMap[env.GetKey()].volumes[mv.GetKey()] = volumeInfo{volume: &mv, planID: plan.Id}
		}
	}

	return envMap, nil
}

// 현재 복제 환경의 볼륨 목록을 가져올 때, stopped 상태인 볼륨 정보는 제거하고, 볼륨 목록에 포함시키지 않는다.
func getCurrentMirrorEnvironmentMap() (map[string]*mirrorEnvironment, error) {
	var envMap = make(map[string]*mirrorEnvironment)

	// 현재 존재하는 복제환경을 조회한다.
	envList, err := mirror.GetEnvironmentList()
	if err != nil {
		return nil, err
	}

	for _, env := range envList {
		envMap[env.GetKey()] = &mirrorEnvironment{
			env:     env,
			volumes: make(map[string]volumeInfo),
		}
	}

	// 현재 복제 중인 볼륨을 조회한다.
	volList, err := mirror.GetVolumeList()
	if err != nil {
		return nil, err
	}

	for _, vol := range volList {
		// 복제 중인 볼륨의 상태 값을 확인한다.
		status, err := vol.GetStatus()
		switch {
		case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeStatus):
			status = &mirror.VolumeStatus{StateCode: constant.MirrorVolumeStateCodeWaiting}

		case err != nil && !errors.Equal(err, mirror.ErrNotFoundMirrorVolumeStatus):
			logger.Errorf("[getCurrentMirrorEnvironmentMap] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
				vol.SourceClusterStorage.StorageID, vol.TargetClusterStorage.StorageID, vol.SourceVolume.VolumeID, err)
			return nil, err
		}

		// mirror volume status 가 stopped 상태인 볼륨을 destroy 한다.(etcd 에서 mirror volume 정보 제거)
		if status.StateCode == constant.MirrorVolumeStateCodeStopped {
			logger.Infof("[getCurrentMirrorEnvironmentMap] Volume status: dr.volume.mirror.state.stopped >> Start DestroyVolumeMirroring: volume(%d)", vol.SourceVolume.VolumeID)
			if err := internal.DestroyVolumeMirroring(vol); err != nil {
				logger.Warnf("[getCurrentMirrorEnvironmentMap] Could not destroy mirror volume. Cause: %+v", err)
			}

			// 제거된 볼륨은 current 볼륨에서 제외된다.
			continue
		}

		env := mirror.Environment{
			SourceClusterStorage: vol.SourceClusterStorage,
			TargetClusterStorage: vol.TargetClusterStorage,
		}

		if envMap[env.GetKey()] == nil {
			envMap[env.GetKey()] = &mirrorEnvironment{
				env:     &env,
				volumes: make(map[string]volumeInfo),
			}
		}

		envMap[env.GetKey()].volumes[vol.GetKey()] = volumeInfo{volume: vol}
	}

	return envMap, nil
}

func equalMirrorEnvironment(a, b *mirror.Environment) (bool, error) {
	if a.GetKey() != b.GetKey() {
		return false, nil
	}

	storageA, err := storage.GetStorage(a.SourceClusterStorage.ClusterID, a.SourceClusterStorage.StorageID)
	if err != nil {
		return false, err
	}

	storageB, err := storage.GetStorage(b.SourceClusterStorage.ClusterID, b.SourceClusterStorage.StorageID)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(storageA, storageB), nil
}

func equalMirrorVolume(a, b *mirror.Volume) bool {
	return reflect.DeepEqual(a, b)
}

func startMirrorEnvironment(e *mirrorEnvironment) error {
	if err := e.env.Put(); err != nil {
		return err
	}

	if err := e.env.SetOperation(constant.MirrorEnvironmentOperationStart); err != nil {
		return err
	}

	for _, v := range e.volumes {
		if err := internal.StartVolumeMirroring(v.volume, v.planID); err != nil {
			return err
		}

		// 복제를 시작한 볼륨의 plan 이 init 필요로 인한 update flag 가 설정 되어있다면 설정 해제
		if err := recoveryPlan.UnsetVolumeRecoveryPlanUnavailableFlag(v.planID, v.volume.SourceVolume.VolumeID, v.volume.TargetClusterStorage.StorageID); err != nil {
			logger.Warnf("[startMirrorEnvironment] Could not update the volume(%d) recovery plan. Cause: %+v", v.volume.SourceVolume.VolumeID, err)
			return err
		}
	}

	logger.Infof("[startMirrorEnvironment] Start mirror environment(%s).", e.env.GetKey())
	return nil
}

func stopMirrorEnvironment(e *mirrorEnvironment) error {
	for _, v := range e.volumes {
		logger.Info("[stopMirrorEnvironment] - MirrorVolumeOperationStopAndDelete")
		if err := internal.StopVolumeMirroring(v.volume, constant.MirrorVolumeOperationStopAndDelete); err != nil {
			return err
		}
	}

	err := e.env.IsMirrorVolumeExisted()

	if errors.Equal(err, mirror.ErrVolumeExisted) {
		return nil
	} else if err != nil {
		logger.Errorf("[stopMirrorEnvironment] Could not get mirror_volume/storage.%d.%d. Cause: %+v", e.env.SourceClusterStorage.StorageID, e.env.TargetClusterStorage.StorageID, err)
		return err
	}

	if err = e.env.SetOperation(constant.MirrorEnvironmentOperationStop); err != nil {
		logger.Errorf("[stopMirrorEnvironment] Could not set mirror_environment/storage.%d.%d/operation. Cause: %+v", e.env.SourceClusterStorage.StorageID, e.env.TargetClusterStorage.StorageID, err)
		return err
	}

	logger.Infof("[stopMirrorEnvironment] Stop mirror environment(%s).", e.env.GetKey())
	return nil
}
