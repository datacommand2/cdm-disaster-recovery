package recoveryjob

import (
	"context"
	"fmt"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/event"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"time"
)

const (
	// MirrorVolumeGetPauseStateInterval 미러 볼륨 퍼즈 상태 확인 interval
	MirrorVolumeGetPauseStateInterval = 3

	// MirrorVolumeGetPauseStateTimeout 미러 볼륨 퍼즈 상태 확인 time out
	MirrorVolumeGetPauseStateTimeout = 180
)

func reportEvent(ctx context.Context, eventCode, errorCode string, eventContents interface{}) {
	id, err := metadata.GetTenantID(ctx)
	if err != nil {
		logger.Warnf("[RecoveryJob-reportEvent] Could not get the tenant ID. Cause: %v", err)
		return
	}

	err = event.ReportEvent(id, eventCode, errorCode, event.WithContents(eventContents))
	if err != nil {
		logger.Warnf("[RecoveryJob-reportEvent] Could not report the event(%s:%s). Cause: %v", eventCode, errorCode, err)
	}
}

func pauseVolumeMirror(volume *planVolume) error {
	logger.Infof("[pauseVolumeMirror] Start: srcStorage(%d) tagStorage(%d) volume(%d)", volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID)

	mv, err := mirror.GetVolume(volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID)
	if err != nil {
		logger.Errorf("[pauseVolumeMirror] Could not get mirror volume: mirror_volume/storage.%d.%d/volume.%d. Cause: %+v",
			volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID, err)
		return err
	}

	err = internal.PauseVolumeMirroring(mv)
	if err != nil {
		logger.Errorf("[pauseVolumeMirror] Could not pause volume(%d) mirroring. Cause: %+v", volume.volumeID, err)
		return err
	}

	logger.Infof("[pauseVolumeMirror] Success: srcStorage(%d) tagStorage(%d) volume(%d)", volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID)
	return nil
}

func waitVolumeMirrorPaused(volume *planVolume) error {
	timeout := time.After(time.Second * MirrorVolumeGetPauseStateTimeout)

	v, err := mirror.GetVolume(volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID)
	if err != nil {
		logger.Errorf("[waitVolumeMirrorPaused] Could not get mirror volume: mirror_volume/storage.%d.%d/volume.%d. Cause: %+v",
			volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID, err)
		return err
	}

	for {
		s, err := v.GetStatus()
		if err != nil {
			logger.Errorf("[waitVolumeMirrorPaused] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
				volume.protectionStorageID, volume.recoveryStorageID, volume.volumeID, err)
			return err
		}

		if s.StateCode == constant.MirrorVolumeStateCodePaused {
			break
		}

		select {
		case <-timeout:
			return errors.Unknown(errors.New("get mirror pause status is timeout"))

		case <-time.After(time.Second * MirrorVolumeGetPauseStateInterval):
			continue
		}
	}

	return nil
}

type planVolume struct {
	protectionStorageID uint64
	recoveryStorageID   uint64
	volumeID            uint64
}

func pausePlanVolumes(plan *drms.RecoveryPlan) error {
	logger.Infof("[pausePlanVolumes] Start: plan(%d:%s)", plan.Id, plan.GetName())
	var volumes []*planVolume
	var err error

	// rollback
	defer func() {
		if err == nil {
			return
		}

		// protection cluster 가 재해 상황이므로 이므로 resume 할 수 없음
		//for _, volume := range volumes {
		//	if err := resumeVolumeMirror(volume); err != nil {
		//		logger.Warnf("Could not set volume mirror operation(%s). Cause: +v", constant.MirrorVolumeOperationResume, err)
		//	}
		//}
	}()

	// pause volumes
	for _, vol := range plan.GetDetail().GetVolumes() {
		protectionStorageID := vol.GetProtectionClusterVolume().GetStorage().GetId()
		recoveryStorageID := vol.GetRecoveryClusterStorage().GetId()
		if recoveryStorageID == 0 {
			storagePlan := internal.GetStorageRecoveryPlan(plan, protectionStorageID)
			if storagePlan == nil {
				return internal.NotFoundStorageRecoveryPlan(plan.Id, protectionStorageID)
			}
			recoveryStorageID = storagePlan.RecoveryClusterStorage.Id
		}

		volume := &planVolume{
			protectionStorageID: protectionStorageID,
			recoveryStorageID:   recoveryStorageID,
			volumeID:            vol.GetProtectionClusterVolume().GetId(),
		}

		// TODO : 추후 resume 기능 활성화 시, 현재 시점 볼륨 mirroring 만 다시 시작해야 함
		// 재해 복구 시점의 plan 이 스냅샷 시점일 경우, 현재 시점과 볼륨 mirroring 상태가 다를 수 있기 때문에
		// mirror volume 정보가 not found 일 경우 continue 처리 한다.
		err = pauseVolumeMirror(volume)
		switch {
		case errors.Equal(err, mirror.ErrNotFoundMirrorVolume), errors.Equal(err, internal.ErrVolumeIsNotMirroring):
			continue

		case err != nil:
			return err
		}

		volumes = append(volumes, volume)
	}

	// wait volumes paused
	for _, volume := range volumes {
		if err = waitVolumeMirrorPaused(volume); err != nil {
			return err
		}
	}

	logger.Infof("[pausePlanVolumes] Success: plan(%d:%s)", plan.Id, plan.GetName())
	return nil
}

// checkRecoveryClusterHypervisorResource 복구대상으로 지정된 하이퍼바이저의 리소스가 모두 충분한지 확인
func checkRecoveryClusterHypervisorResource(ctx context.Context, job *drms.RecoveryJob) error {
	hypervisorMap := make(map[uint64]*drms.RecoveryHypervisorResource)

	for _, i := range job.Plan.Detail.Instances {
		if i.GetRecoveryClusterHypervisor() == nil {
			continue
		}

		if _, ok := hypervisorMap[i.RecoveryClusterHypervisor.Id]; !ok {
			hypervisor, err := cluster.GetClusterHypervisorWithSync(ctx, job.Plan.RecoveryCluster.Id, i.RecoveryClusterHypervisor.Id)
			if err != nil {
				return err
			}

			hypervisorMap[hypervisor.Id] = &drms.RecoveryHypervisorResource{
				VcpuTotalCnt:      hypervisor.VcpuTotalCnt,
				VcpuExpectedUsage: hypervisor.VcpuUsedCnt,
				MemTotalBytes:     hypervisor.MemTotalBytes,
				MemExpectedUsage:  hypervisor.MemUsedBytes,
			}
		}

		hypervisorMap[i.RecoveryClusterHypervisor.Id].VcpuExpectedUsage += i.ProtectionClusterInstance.Spec.VcpuTotalCnt
		hypervisorMap[i.RecoveryClusterHypervisor.Id].MemExpectedUsage += i.ProtectionClusterInstance.Spec.MemTotalBytes
	}

	var (
		code []string
		flag bool
	)
	for k, v := range hypervisorMap {
		var (
			requiredVcpu uint32
			requiredMem  uint64
		)

		if v.VcpuExpectedUsage > v.VcpuTotalCnt {
			flag = true
			requiredVcpu = v.VcpuExpectedUsage - v.VcpuTotalCnt
		}

		if v.MemExpectedUsage > v.MemTotalBytes {
			flag = true
			requiredMem = v.MemExpectedUsage - v.MemTotalBytes
		}

		if requiredVcpu > 0 || requiredMem > 0 {
			code = append(code, fmt.Sprintf("recovery_cluster_hypervisor.%d.insufficient_vcpu.%d.insufficient_mem.%d", k, requiredVcpu, requiredMem))
		}
	}

	if flag {
		return internal.InsufficientRecoveryHypervisorResource(job.Plan.RecoveryCluster.Id, code)
	}

	return nil
}

func timeUnixToString(t int64) string {
	return time.Unix(t, 0).Format("2006/01/02 15:04")
}
