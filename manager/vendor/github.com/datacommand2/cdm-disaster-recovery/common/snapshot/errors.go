package snapshot

import "github.com/datacommand2/cdm-cloud/common/errors"

var (
	// ErrNotFoundMirrorVolume 보호 그룹 스냅샷 시점의 mirror volume 정보를 찾을 수 없음
	ErrNotFoundMirrorVolume = errors.New("not found mirror volume in protection group snapshot")

	// ErrNotFoundMirrorVolumeTargetMetadata 보호 그룹 스냅샷 시점의 mirror volume 의 target metadata 정보가 없음
	ErrNotFoundMirrorVolumeTargetMetadata = errors.New("not found mirror volume target metadata in protection group snapshot")
)

// NotFoundMirrorVolume 보호 그룹 스냅샷 시점의 mirror volume 정보를 찾을 수 없음
func NotFoundMirrorVolume(pid, pgsid, vid uint64) error {
	return errors.Wrap(
		ErrNotFoundMirrorVolume,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"plan_id":                      pid,
			"protection_group_snapshot_id": pgsid,
			"protection_cluster_volume_id": vid,
		}),
	)
}

// NotFoundMirrorVolumeTargetMetadata 보호 그룹 스냅샷 시점의 mirror volume 의 target metadata 정보가 없음
func NotFoundMirrorVolumeTargetMetadata(pid, pgsid, vid uint64) error {
	return errors.Wrap(
		ErrNotFoundMirrorVolumeTargetMetadata,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"plan_id":                      pid,
			"protection_group_snapshot_id": pgsid,
			"protection_cluster_volume_id": vid,
		}),
	)
}
