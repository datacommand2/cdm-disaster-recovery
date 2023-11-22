package internal

import (
	"10.1.1.220/cdm/cdm-cloud/common/errors"
)

var (
	// ErrUnsupportedStorageType mirror daemon 에서 지원 하지 않는 스토리지 타입
	ErrUnsupportedStorageType = errors.New("unsupported storage type")

	// ErrNotSetMirrorVolumeSnapshot 복제 중인 볼륨의 해당 스냅샷이 복제 설정이 안되어 있음
	ErrNotSetMirrorVolumeSnapshot = errors.New("not set mirror volume snapshot")

	// ErrUnableGetVolumeImage 볼륨 이미지를 가져올 수 없음
	ErrUnableGetVolumeImage = errors.New("unable to get volume image")
)

// UnsupportedStorageType mirror daemon 에서 지원 하지 않는 스토리지 타입
func UnsupportedStorageType(t string) error {
	return errors.Wrap(
		ErrUnsupportedStorageType,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"storage_type": t,
		}),
	)
}

// NotSetMirrorVolumeSnapshot 복제 중인 볼륨의 해당 스냅샷이 복제 설정이 안되어 있음
func NotSetMirrorVolumeSnapshot(volumeUUID, snapshotUUID string) error {
	return errors.Wrap(
		ErrNotSetMirrorVolumeSnapshot,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"volume_uuid":   volumeUUID,
			"snapshot_uuid": snapshotUUID,
		}),
	)
}

// UnableGetVolumeImage 볼륨 이미지를 가져올 수 없음
func UnableGetVolumeImage(volume string) error {
	return errors.Wrap(
		ErrUnableGetVolumeImage,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"volume": volume,
		}),
	)
}
