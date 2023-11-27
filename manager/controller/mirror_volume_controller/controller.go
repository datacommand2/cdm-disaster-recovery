package mirrorvolumecontroller

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/manager/controller"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	recoveryplan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	"github.com/jinzhu/gorm"
	"time"
)

const (
	defaultInterval = time.Minute * 5
)

var (
	defaultController mirrorVolumeController // singleton
	closeCh           chan interface{}
)

type mirrorVolumeController struct {
}

type volumeInfo struct {
	volume *mirror.Volume
	planID uint64
}

type mirrorEnvironment struct {
	env              *mirror.Environment
	volumes          map[string]volumeInfo
	disasterRecovery bool
}

func init() {
	controller.RegisterController(&defaultController)
}

func (c *mirrorVolumeController) syncMirrorVolumes() error {
	var (
		err           error
		ctx           context.Context
		desiredEnvMap map[string]*mirrorEnvironment
		currentEnvMap map[string]*mirrorEnvironment
	)
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	// 복제가 필요한 복제환경과 볼륨을 조회
	if desiredEnvMap, err = getDesiredMirrorEnvironmentMap(ctx); err != nil {
		logger.Errorf("[syncMirrorVolumes] Could not get desired mirror environment map. Cause: %+v", err)
		return err
	}

	// 현재 복제 중인 복제환경과 볼륨을 조회
	if currentEnvMap, err = getCurrentMirrorEnvironmentMap(); err != nil {
		logger.Errorf("[syncMirrorVolumes] Could not get current mirror environment map. Cause: %+v", err)
		return err
	}

	// 현재 복제 중인 복제환경들 중 더 이상 필요하지 않거나 구성이 변경된 복제환경을 제거하고,
	// 나머지 복제환경으로 복제중인 볼륨들 중 복제가 필요없거나 구성이 변경된 볼륨의 복제를 중지한다.
	for key, currentEnv := range currentEnvMap {
		desiredEnv := desiredEnvMap[key]

		// 더 이상 필요하지 않은 복제환경을 제거한다.
		if desiredEnv == nil {
			logger.Infof("[syncMirrorVolumes] Desired mirror environment is not existed >> Start stopMirrorEnvironment(current): %s", currentEnv.env.GetKey())
			if err := stopMirrorEnvironment(currentEnv); err != nil {
				logger.Warnf("[syncMirrorVolumes] Could not stop mirror environment. Cause: %+v", err)
			}
			continue
		}

		// 구성이 변경된 복제환경을 제거한다.
		if eq, err := equalMirrorEnvironment(currentEnv.env, desiredEnv.env); !eq {
			logger.Infof("[syncMirrorVolumes] Current and desired mirror environment is not same >> Start stopMirrorEnvironment(current): current(%s), desired(%s)", currentEnv.env.GetKey(), desiredEnv.env.GetKey())
			if err := stopMirrorEnvironment(currentEnv); err != nil {
				logger.Warnf("[syncMirrorVolumes] Could not stop mirror environment. Cause: %+v", err)
			}
			continue

		} else if err != nil {
			logger.Errorf("[syncMirrorVolumes] Could not check equal mirror environment. Cause: %+v", err)
			return err
		}

		// 복제중인 볼륨들 중 복제가 필요없거나 구성이 변경된 볼륨의 복제를 중지한다.
		for key, currentVol := range currentEnv.volumes {
			if desiredEnv.volumes[key].volume != nil && equalMirrorVolume(currentVol.volume, desiredEnv.volumes[key].volume) {
				continue
			}

			logger.Info("[stopMirrorEnvironment] - MirrorVolumeOperationStopAndDelete")
			if err := internal.StopVolumeMirroring(currentVol.volume, constant.MirrorVolumeOperationStopAndDelete); err != nil {
				logger.Warnf("[syncMirrorVolumes] Could not stop mirror volume. Cause: %+v", err)
			}
		}
	}

	// 현재 존재하지 않는 복제환경을 생성하고, 복제 중이 아닌 볼륨의 복제를 시작한다.
	for key, desiredEnv := range desiredEnvMap {
		currentEnv := currentEnvMap[key]

		// 현재 존재하지 않는 복제환경이라면 생성한다.
		if currentEnv == nil {
			logger.Infof("[syncMirrorVolumes] Current mirror environment is not existed >> Start startMirrorEnvironment: %s", desiredEnv.env.GetKey())
			if err := startMirrorEnvironment(desiredEnv); err != nil {
				logger.Warnf("[syncMirrorVolumes] Could not start mirror environment. Cause: %+v", err)
			}
			continue
		}

		// 현재 복제 중이 아닌 볼륨의 복제를 시작한다.
		for key, desiredVol := range desiredEnv.volumes {
			if currentEnv.volumes[key].volume != nil {
				continue
			}

			//재해복구 시 Skip
			if desiredEnv.disasterRecovery {
				logger.Infof("[syncMirrorVolumes] Desired mirror environment is not mirroring >> status DisasterRecovery : %t", desiredEnv.disasterRecovery)
				continue
			}

			logger.Infof("[syncMirrorVolumes] Desired mirror environment is not mirroring >> Start StartVolumeMirroring: key(%s)", key)
			if err := internal.StartVolumeMirroring(desiredVol.volume, desiredVol.planID); err != nil {
				logger.Warnf("[syncMirrorVolumes] Could not start mirror volume. Cause: %+v", err)
			}

			// 복제를 시작한 볼륨의 plan 이 init 필요로 인한 update flag 가 설정 되어있다면 설정 해제
			if recoveryplan.UnsetVolumeRecoveryPlanUnavailableFlag(desiredVol.planID, desiredVol.volume.SourceVolume.VolumeID, desiredVol.volume.TargetClusterStorage.StorageID); err != nil {
				logger.Warnf("[syncMirrorVolumes] Could not update the volume(%d) recovery plan. Cause: %+v", desiredVol.volume.SourceVolume.VolumeID, err)
			}
		}
	}

	return nil
}

func (c *mirrorVolumeController) run() {
	for {
		if err := c.syncMirrorVolumes(); err != nil {
			logger.Warnf("[MirrorVolumeController-run] Error occurred on mirror volume controller. Cause: %+v", err)
		}

		select {
		case <-time.After(defaultInterval):
		case <-closeCh:
			return
		}
	}
}

func (c *mirrorVolumeController) Run() {
	if closeCh != nil {
		logger.Warn("[MirrorVolumeController-Run] Already mirror volume controller started")
		return
	}

	logger.Info("[MirrorVolumeController-Run] Starting mirror volume controller.")
	closeCh = make(chan interface{})

	go c.run()
}

func (c *mirrorVolumeController) Close() {
	if closeCh == nil {
		logger.Warn("[MirrorVolumeController-Close] Mirror volume controller is not started")
		return
	}

	logger.Info("[MirrorVolumeController-Close] Stopping mirror volume controller")
	close(closeCh)

	closeCh = nil
}
