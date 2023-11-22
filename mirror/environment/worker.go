package environment

import (
	"sync"
	"time"

	"10.1.1.220/cdm/cdm-cloud/common/errors"
	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/constant"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/internal"
)

var workerMapLock = sync.Mutex{}
var workerMap = make(map[string]*worker)

// worker 복제 환경 구성, 삭제 를 처리 하기 위한 구조체
type worker struct {
	env     *mirror.Environment
	storage Storage
}

func newWorker(env *mirror.Environment) *worker {
	workerMapLock.Lock()
	defer workerMapLock.Unlock()

	if _, ok := workerMap[env.GetKey()]; ok {
		return nil
	}

	workerMap[env.GetKey()] = &worker{env: env}
	return workerMap[env.GetKey()]
}

func (w *worker) run() {
	go func() {
		var err error
		stopCh := make(chan bool, 1)

		defer func() {
			workerMapLock.Lock()
			delete(workerMap, w.env.GetKey())
			workerMapLock.Unlock()
			close(stopCh)
		}()

		if _, err := mirror.GetEnvironment(w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID); err != nil {
			logger.Warnf("[EnvironmentWorker-run] Could not get mirror environment: mirror_environment/storage.%d.%d. Cause: %+v",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, err)
			return
		}

		if w.storage, err = NewStorage(w.env); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_run.failure-create_storage-unknown", CreateStorageFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_run.failure-create_storage", "unknown", CreateStorageFailed(err))
			logger.Errorf("[EnvironmentWorker-run] Could not create storage. Cause: %+v", err)
			logger.Infof("[EnvironmentWorker-run] set status: mirror_environment/storage.%d.%d/status {%s}",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError)
			return
		}

		// 전처리: 복제 환경 구성 or 복제 환경 제거
		if err = w.preprocess(); err != nil {
			return
		}

		// 모니터링
		logger.Infof("[EnvironmentWorker-run]Start mirror environment(%s) monitoring.", w.env.GetKey())
		for {
			select {
			case <-stopCh:
				logger.Infof("[EnvironmentWorker-run]Stop mirror environment(%s) monitoring.", w.env.GetKey())
				return

			case <-time.After(internal.DefaultMonitorInterval):
				if err = w.monitor(stopCh); err != nil {
					return
				}
			}
		}
	}()
}

// preprocess environment 의 오퍼 레이션 전처리 함수
func (w *worker) preprocess() error {
	op, err := w.env.GetOperation()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundMirrorEnvironmentOperation):
		return nil

	case err != nil:
		logger.Errorf("[EnvironmentWorker-preprocess] Could not get mirror environment operation: mirror_environment/storage.%d.%d. Cause: %+v",
			w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, err)
		return err
	}

	switch op.Operation {
	case constant.MirrorEnvironmentOperationStart:
		if err = w.storage.Prepare(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-storage_prepare-unknown", InitializeFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-storage_prepare", "unknown", InitializeFailed(err))
			logger.Errorf("[EnvironmentWorker-preprocess] Could not initialize mirror environment. Cause: %+v", err)
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation)
			return err
		}

		if err = w.storage.Start(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-storage_start-unknown", StartFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-storage_start", "unknown", StartFailed(err))
			logger.Errorf("[EnvironmentWorker-preprocess] Could not start mirror environment. Cause: %+v", err)
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation)
			return err
		}

		if err = w.env.SetStatus(constant.MirrorEnvironmentStateCodeMirroring, "", nil); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-env_set_status-unknown", StartFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-env_set_status", "unknown", StartFailed(err))
			logger.Errorf("[EnvironmentWorker-preprocess] Could not update mirror environment state. Cause: %+v", err)
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation)
		} else {
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeMirroring, op.Operation)
		}

	case constant.MirrorEnvironmentOperationStop:
		if err = w.env.IsMirrorVolumeExisted(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-env_is_mirror_volume_existed-unknown", StopFailed(err))
			logger.Warnf("[EnvironmentWorker-preprocess] Could not stop mirror environment. Cause: %+v", err)
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation)
			return nil
		}

		st, err := w.env.GetStatus()
		if err == nil &&
			(st.StateCode == constant.MirrorEnvironmentStateCodeMirroring || st.StateCode == constant.MirrorVolumeStateCodeError) {
			if err = w.env.SetStatus(constant.MirrorEnvironmentStateCodeStopping, "", nil); err != nil {
				_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-env_set_status-unknown", StopFailed(err))
				internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-env_set_status", "unknown", StopFailed(err))
				logger.Errorf("[EnvironmentWorker-preprocess] Could not update mirror environment state. Cause: %+v", err)
				logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s) status(%s)",
					w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation, st.StateCode)
			} else {
				logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s) status(%s)",
					w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeStopping, op.Operation, st.StateCode)
			}

			defer func() {
				if err = w.env.Delete(); err != nil {
					_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-env_delete-unknown", DeletedFail(err))
					internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-env_delete", "unknown", DeletedFail(err))
					logger.Errorf("[EnvironmentWorker-preprocess] Could not delete mirror environment. Cause: %+v", err)
					logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s) status(%s)",
						w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation, st.StateCode)
				} else {
					logger.Infof("[EnvironmentWorker-preprocess] delete all: mirror_environment/storage.%d.%d operation(%s) status(%s)",
						w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, op.Operation, st.StateCode)
				}
			}()
		}

		if err = w.storage.Stop(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-storage_stop-unknown", StopFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-storage_stop", "unknown", StopFailed(err))
			logger.Errorf("[EnvironmentWorker-preprocess] Could not stop mirror environment. cause: %+v", err)
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation)
		}

		if err = w.storage.Destroy(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_preprocess.failure-storage_destroy-unknown", DestroyFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_preprocess.failure-storage_destroy", "unknown", DeletedFail(err))
			logger.Errorf("[EnvironmentWorker-preprocess] Could not destroy mirror environment. Cause: %+v", err)
			logger.Infof("[EnvironmentWorker-preprocess] set status: mirror_environment/storage.%d.%d/status {%s} operation(%s)",
				w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, constant.MirrorEnvironmentStateCodeError, op.Operation)
			return err
		}
	}

	return nil
}

// monitor 복제 환경 구성 이후, 복제 환경이 정상적 인지 확인 하는 함수
// 스토리지 타입 별로 처리가 다르다.
// ceph 의 경우 rbd-mirror process 실행 여부 확인
func (w *worker) monitor(stopCh chan bool) error {
	op, err := w.env.GetOperation()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundMirrorEnvironmentOperation):
		stopCh <- true
		return nil

	case err != nil:
		logger.Errorf("[EnvironmentWorker-monitor] Could not get mirror environment operation: mirror_environment/storage.%d.%d. Cause: %+v",
			w.env.SourceClusterStorage.StorageID, w.env.TargetClusterStorage.StorageID, err)
		return err
	}

	switch op.Operation {
	case constant.MirrorEnvironmentOperationStart:
		if err = w.storage.Monitor(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_monitor.failure-storage_monitor-unknown", MonitorFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_monitor.failure-storage_monitor", "unknown", MonitorFailed(err))
			logger.Errorf("[EnvironmentWorker-monitor] Could not monitor mirror environment. Cause: %+v", err)
			return err
		}

	case constant.MirrorEnvironmentOperationStop:
		if err = w.env.IsMirrorVolumeExisted(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_monitor.failure-env_is_mirror_volume_existed-unknown", StopFailed(err))
			logger.Warnf("[EnvironmentWorker-monitor] Could not stop mirror environment. Cause: %+v", err)
			return nil
		}

		st, err := w.env.GetStatus()
		if err == nil &&
			(st.StateCode == constant.MirrorEnvironmentStateCodeMirroring || st.StateCode == constant.MirrorVolumeStateCodeError) {
			if err = w.env.SetStatus(constant.MirrorEnvironmentStateCodeStopping, "", nil); err != nil {
				_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_monitor.failure-env_set_status-unknown", StopFailed(err))
				internal.ReportEvent("cdm-dr.mirror.env_worker_monitor.failure-env_set_status", "unknown", StopFailed(err))
				logger.Errorf("[EnvironmentWorker-monitor] Could not update mirror environment state. Cause: %+v", err)
			}

			defer func() {
				if err = w.env.Delete(); err != nil {
					_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_monitor.failure-env_delete-unknown", DeletedFail(err))
					internal.ReportEvent("cdm-dr.mirror.env_worker_monitor.failure-env_delete", "unknown", DeletedFail(err))
					logger.Errorf("[EnvironmentWorker-monitor] Could not delete mirror environment. Cause: %+v", err)
				}
			}()
		}

		if err = w.storage.Stop(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_monitor.failure-storage_stop-unknown", StopFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_monitor.failure-storage_stop", "unknown", StopFailed(err))
			logger.Errorf("[EnvironmentWorker-monitor] Could not stop mirror environment. cause: %+v", err)
		}

		if err = w.storage.Destroy(); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_monitor.failure-storage_destroy-unknown", DestroyFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_monitor.failure-storage_destroy", "unknown", DeletedFail(err))
			logger.Errorf("[EnvironmentWorker-monitor] Could not destroy mirror environment. Cause: %+v", err)
			return err
		}

		stopCh <- true
	}

	return nil
}
