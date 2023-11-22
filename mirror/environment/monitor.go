package environment

import (
	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/internal"

	"time"
)

// Monitor 복제 환경 모니터링 구조체
type Monitor struct {
	stopCh chan interface{}
}

// Run 복제 환경 모니터링 시작 함수
func (m *Monitor) Run() {
	go func() {
		for {
			select {
			case <-m.stopCh:
				return

			case <-time.After(internal.DefaultMonitorInterval):
				envs, err := mirror.GetEnvironmentList()
				if err != nil {
					internal.ReportEvent("cdm-dr.mirror.env_monitor_run.failure-get_environment", "unknown", err)
					logger.Warnf("[EnvironmentMonitor-Run] Could not get mirror environment list, cause: %+v", err)
				}

				for _, env := range envs {
					if w := newWorker(env); w != nil {
						w.run()
					}
				}
			}
		}
	}()
}

// Stop 볼륨 복제 환경 모니터링 종료 함수
func (m *Monitor) Stop() {
	close(m.stopCh)
}

// NewMirrorEnvironmentMonitor 볼륨 복제 환경 모니터링 구조체 생성 함수
func NewMirrorEnvironmentMonitor() *Monitor {
	return &Monitor{
		stopCh: make(chan interface{}),
	}
}
