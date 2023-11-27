package volume

import (
	"context"
	"path"
	"sync"
	"time"

	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	cloudSync "github.com/datacommand2/cdm-cloud/common/sync"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/internal"
)

const (
	defaultLeaderElectionPath = "mirror_volume_leader"
)

var workerMapLock = sync.Mutex{}
var workerMap = make(map[string]*worker)

type worker struct {
	vol     *mirror.Volume
	storage Storage
}

func newWorker(vol *mirror.Volume) *worker {
	workerMapLock.Lock()
	defer workerMapLock.Unlock()

	if _, ok := workerMap[vol.GetKey()]; ok {
		return nil
	}

	workerMap[vol.GetKey()] = &worker{vol: vol}
	return workerMap[vol.GetKey()]
}

func (w *worker) isUsableMirrorEnvironment() error {
	env, err := w.vol.GetMirrorEnvironment()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundMirrorEnvironment):
		return nil

	case err != nil:
		logger.Errorf("[isUsableMirrorEnvironment] Could not get mirror environment: mirror_environment/storage.%d.%d. Cause: %+v",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, err)
		return err
	}

	var status *mirror.EnvironmentStatus
	if status, err = env.GetStatus(); err != nil {
		logger.Errorf("[isUsableMirrorEnvironment] Could not get environment status: mirror_environment/storage.%d.%d/status. Cause: %+v",
			env.SourceClusterStorage.StorageID, env.TargetClusterStorage.StorageID, err)
		return err
	}

	if status.StateCode != constant.MirrorEnvironmentStateCodeMirroring {
		return ErrIsNotEnvironmentStateCodeMirroring
	}

	return nil
}

func (w *worker) run() {
	go func() {
		defer func() {
			workerMapLock.Lock()
			delete(workerMap, w.vol.GetKey())
			workerMapLock.Unlock()
		}()

		l, err := cloudSync.CampaignLeader(context.Background(), path.Join(defaultLeaderElectionPath, w.vol.GetKey()))
		if err != nil {
			return
		}
		stopCh := make(chan bool, 1)

		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = l.Resign(ctx)
			_ = l.Close()
			cancel()
			close(stopCh)
		}()

		if _, err = mirror.GetVolume(w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID); err != nil {
			return
		}

		if w.storage, err = NewStorage(w.vol); err != nil {
			logger.Errorf("[VolumeWorker-run] Could not create storage volume. Cause: %+v", err)
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_run.failure-create_storage-unknown", CreateStorageFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_run.failure-create_storage", "unknown", CreateStorageFailed(err))
			return
		}

		observe := l.Status()

		logger.Infof("[VolumeWorker-run] Start mirror volume(%s) monitoring.", w.vol.GetKey())
		for {
			select {
			case <-observe:
				return

			case <-stopCh:
				logger.Infof("[VolumeWorker-run] Stop mirror volume(%s) monitoring.", w.vol.GetKey())
				return

			case <-time.After(internal.DefaultMonitorInterval):
				if err = w.operate(stopCh); err != nil {
					return
				}
			}
		}
	}()
}

func (w *worker) operate(stopCh chan bool) error {
	op, err := w.vol.GetOperation()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeOperation):
		return nil

	case err != nil:
		logger.Errorf("[VolumeWorker-operate] Could not get mirror volume operation. Cause: %+v", err)
		return err
	}

	if err = w.isUsableMirrorEnvironment(); err != nil {
		_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-is_usable_mirror_env-unknown", UnusableMirrorEnvironment(err))
		internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-is_usable_mirror_env", "unknown", UnusableMirrorEnvironment(err))
		logger.Errorf("[VolumeWorker-operate] Could not operate mirror volume. Cause: %+v", err)
		logger.Infof("[VolumeWorker-operate] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s}",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError)
		return err
	}

	switch op.Operation {
	case constant.MirrorVolumeOperationStart:
		if err = w.start(); err != nil {
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-work_start-unknown", StartFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-work_start", "unknown", StartFailed(err))
			logger.Errorf("[VolumeWorker-operate] Could not start mirror volume. Cause: %+v", err)
			logger.Infof("[VolumeWorker-operate] set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} operation(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, op.Operation)
			return err
		}

	case constant.MirrorVolumeOperationResume:
		if err = w.resume(); err != nil {
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-work_resume-unknown", ResumeFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-work_resume", "unknown", ResumeFailed(err))
			logger.Errorf("[VolumeWorker-operate] Could not resume mirror volume. Cause: %+v", err)
			logger.Infof("[VolumeWorker-operate] set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} operation(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, op.Operation)
			return err
		}

	case constant.MirrorVolumeOperationStop:
		if err = w.stop(); err != nil {
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-work_stop-unknown", StopFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-work_stop", "unknown", StopFailed(err))
			logger.Errorf("[VolumeWorker-operate] Could not stop mirror volume. Cause: %+v", err)
			logger.Infof("[VolumeWorker-operate] set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} operation(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, op.Operation)
			return err
		}

	case constant.MirrorVolumeOperationStopAndDelete:
		if err = w.stopAndDelete(); err != nil {
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-work_stop_and_delete-unknown", StopAndDeleteFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-work_stop_and_delete", "unknown", StopAndDeleteFailed(err))
			logger.Errorf("[VolumeWorker-operate] Could not stop and delete mirror volume. Cause: %+v", err)
			logger.Infof("[VolumeWorker-operate] set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} operation(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, op.Operation)
			return err
		}

	case constant.MirrorVolumeOperationDestroy:
		if err = w.vol.Delete(); err != nil {
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-work_destroy-unknown", DestroyFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-work_destroy", "unknown", DestroyFailed(err))
			logger.Errorf("[VolumeWorker-operate] Could not destroy mirror volume. Cause: %+v", err)
			logger.Infof("[VolumeWorker-operate] set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} operation(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, op.Operation)
			return err
		}

		stopCh <- true

	case constant.MirrorVolumeOperationPause:
		if err = w.pause(); err != nil {
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_operate.failure-work_pause-unknown", PauseFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_operate.failure-work_pause", "unknown", PauseFailed(err))
			logger.Errorf("[VolumeWorker-operate] Could not pause mirror volume. Cause: %+v", err)
			logger.Infof("[VolumeWorker-operate] set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} operation(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeError, op.Operation)
			return err
		}

	}

	return nil
}

func (w *worker) start() error {
	status, err := w.vol.GetStatus()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeStatus):
		status = &mirror.VolumeStatus{StateCode: constant.MirrorVolumeStateCodeWaiting}

	case err != nil && !errors.Equal(err, mirror.ErrNotFoundMirrorVolumeStatus):
		logger.Errorf("[VolumeWorker-start] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, err)
		return err
	}

	switch status.StateCode {
	case constant.MirrorVolumeStateCodeInitializing, constant.MirrorVolumeStateCodeMirroring:
		if err = w.storage.MirrorEnable(status.StateCode); err != nil {
			logger.Errorf("[VolumeWorker-start] Could not mirror enable: state(%s). Cause: %+v", status.StateCode, err)
			return err
		}

		return nil

	case constant.MirrorVolumeStateCodeWaiting, constant.MirrorVolumeStateCodeError:
		if err = w.storage.Start(); err != nil {
			logger.Errorf("[VolumeWorker-start] Could not mirror start: state(%s). Cause: %+v", status.StateCode, err)
			return err
		}

		if err = w.vol.SetStatus(constant.MirrorVolumeStateCodeInitializing, "", nil); err != nil {
			logger.Errorf("[VolumeWorker-start] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, status.StateCode)
			return err
		}
		logger.Infof("[VolumeWorker-start] Done -set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, status.StateCode)

	default:
		return errors.InvalidParameterValue("status", status.StateCode, "unexpected mirror volume state")
	}

	return nil
}

func (w *worker) resume() error {
	status, err := w.vol.GetStatus()
	if err != nil {
		logger.Errorf("[VolumeWorker-resume] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, err)
		return err
	}

	switch status.StateCode {
	case constant.MirrorVolumeStateCodeInitializing, constant.MirrorVolumeStateCodeMirroring:
		return w.storage.MirrorEnable(status.StateCode)

	case constant.MirrorVolumeStateCodePaused, constant.MirrorVolumeStateCodeError:
		if err = w.storage.Resume(); err != nil {
			logger.Errorf("[VolumeWorker-resume] Could not mirror resume: state(%s). Cause: %+v", status.StateCode, err)
			return err
		}

		if err = w.vol.SetStatus(constant.MirrorVolumeStateCodeInitializing, "", nil); err != nil {
			logger.Errorf("[VolumeWorker-resume] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, status.StateCode)
			return err
		}
		logger.Infof("[VolumeWorker-resume] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeInitializing, status.StateCode)

	default:
		return errors.InvalidParameterValue("status", status.StateCode, "unexpected mirror volume state")
	}

	return nil
}

func (w *worker) pause() error {
	status, err := w.vol.GetStatus()
	if err != nil {
		logger.Errorf("[VolumeWorker-pause] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, err)
		return err
	}

	switch status.StateCode {
	case constant.MirrorVolumeStateCodePaused:
		return w.storage.PauseEnable(status.StateCode)

	case constant.MirrorVolumeStateCodeInitializing, constant.MirrorVolumeStateCodeMirroring, constant.MirrorVolumeStateCodeError:
		if err = w.storage.Pause(); err != nil {
			logger.Errorf("[VolumeWorker-pause] Could not mirror pause: state(%s). Cause: %+v", status.StateCode, err)
			return err
		}

		if err = w.vol.SetStatus(constant.MirrorVolumeStateCodePaused, "", nil); err != nil {
			logger.Errorf("[VolumeWorker-pause] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, status.StateCode)
			return err
		}
		logger.Infof("[VolumeWorker-pause] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodePaused, status.StateCode)

	default:
		return errors.InvalidParameterValue("status", status.StateCode, "unexpected mirror volume state")
	}

	return nil
}

func (w *worker) stop() error {
	status, err := w.vol.GetStatus()
	if err != nil {
		logger.Errorf("[VolumeWorker-stop] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, err)
		return err
	}

	switch status.StateCode {
	case constant.MirrorVolumeStateCodeStopped:
		return nil

	case constant.MirrorVolumeStateCodeStopping, constant.MirrorVolumeStateCodePaused,
		constant.MirrorVolumeStateCodeInitializing, constant.MirrorVolumeStateCodeMirroring, constant.MirrorVolumeStateCodeError:
		if err := w.storage.Stop(); err != nil {
			logger.Errorf("[VolumeWorker-stop] Could not mirror stop: state(%s). Cause: %+v", status.StateCode, err)
			return err
		}

		if err := w.vol.SetStatus(constant.MirrorVolumeStateCodeStopped, "", nil); err != nil {
			logger.Errorf("[VolumeWorker-stop] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeStopped, status.StateCode)
			return err
		}
		logger.Infof("[VolumeWorker-stop] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeStopped, status.StateCode)

	default:
		return errors.InvalidParameterValue("status", status.StateCode, "unexpected mirror volume state")
	}

	return nil
}

func (w *worker) stopAndDelete() error {
	status, err := w.vol.GetStatus()
	if err != nil {
		logger.Errorf("[VolumeWorker-stopAndDelete] Could not get volume status: mirror_volume/storage.%d.%d/volume.%d/status. Cause: %+v",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, err)
		return err
	}

	switch status.StateCode {
	case constant.MirrorVolumeStateCodeStopped:
		return nil

	case constant.MirrorVolumeStateCodeDeleting, constant.MirrorVolumeStateCodePaused, constant.MirrorVolumeStateCodeInitializing,
		constant.MirrorVolumeStateCodeMirroring, constant.MirrorVolumeStateCodeError:
		if err := w.storage.StopAndDelete(); err != nil {
			logger.Errorf("[VolumeWorker-stopAndDelete] Could not mirror stop and delete: state(%s). Cause: %+v", status.StateCode, err)
			return err
		}

		if err := w.vol.SetStatus(constant.MirrorVolumeStateCodeStopped, "", nil); err != nil {
			logger.Errorf("[VolumeWorker-stopAndDelete] Could not set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
				w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeStopped, status.StateCode)
			return err
		}
		logger.Infof("[VolumeWorker-stopAndDelete] Done - set status: mirror_volume/storage.%d.%d/volume.%d/status {%s} state(%s)",
			w.vol.SourceClusterStorage.StorageID, w.vol.TargetClusterStorage.StorageID, w.vol.SourceVolume.VolumeID, constant.MirrorVolumeStateCodeStopped, status.StateCode)

	default:
		return errors.InvalidParameterValue("status", status.StateCode, "unexpected mirror volume state")
	}

	return nil
}
