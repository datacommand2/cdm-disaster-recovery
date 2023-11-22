package internal

import "time"

const (
	// DefaultMonitorInterval 모니터 인터벌 타임
	DefaultMonitorInterval = 5 * time.Second

	// DefaultVolumeSnapshotSyncMonitorInterval 볼륨 스냅샷 sync 모니터 인터벌 타임 볼륨 그룹 스냅샷 interval 의 최소 시간
	DefaultVolumeSnapshotSyncMonitorInterval = 10 * time.Minute
)
