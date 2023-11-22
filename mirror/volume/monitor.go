package volume

import (
	"time"

	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/internal"
)

// Monitor 볼륨 복제 모니터링 구조체
type Monitor struct {
	stopCh chan interface{}
}

// Run 복제 모니터링 시작 함수
func (m *Monitor) Run() {
	go func() {
		for {
			select {
			case <-m.stopCh:
				return

			case <-time.After(internal.DefaultMonitorInterval):
				volumes, err := mirror.GetVolumeList()
				if err != nil {
					internal.ReportEvent("cdm-dr.mirror.volume_monitor_run.failure-get_volume_list", "unknown", err)
					logger.Warnf("[Worker-Run] Could not get mirror volume list, cause: %+v", err)
				}

				for _, v := range volumes {
					if w := newWorker(v); w != nil {
						w.run()
					}
				}
			}
		}
	}()
}

// Stop 볼륨 복제 모니터링 종료 함수
func (m *Monitor) Stop() {
	close(m.stopCh)
}

// NewMirrorVolumeMonitor 볼륨 복제 모니터링 구조체 생성 함수
func NewMirrorVolumeMonitor() (*Monitor, error) {
	m := &Monitor{stopCh: make(chan interface{})}

	return m, nil
}
