package ceph

import (
	"github.com/datacommand2/cdm-cloud/common/errors"
)

var (
	// ErrUnsupportedMultiplePeer 다중 peer 등록
	ErrUnsupportedMultiplePeer = errors.New("unsupported ceph multiple peer")

	// ErrDifferentPools 다른 풀 peer 등록
	ErrDifferentPools = errors.New("different ceph pools")

	// ErrTimeoutExecCommand exec command 실행 타임 아웃
	ErrTimeoutExecCommand = errors.New("timeout execution command")

	// ErrUnexpectedExecResult exec command 실행
	ErrUnexpectedExecResult = errors.New("unexpected run execution command")

	// ErrNotFoundRBDMirrorProcess rbd-mirror process 를 찾을 수 없음
	ErrNotFoundRBDMirrorProcess = errors.New("not found rbd-mirror process")

	// ErrNotMatchingPeerInfo peer 정보가 일치 하지 않음
	ErrNotMatchingPeerInfo = errors.New("not matching peer info")

	// ErrNotSetPromote rbd image 가 promote 로 설정 되어 있지 않음
	ErrNotSetPromote = errors.New("image not set promote")

	// ErrAlreadySetPromote 볼륨 복제 할 이미지가 target pool 에 존재 하며, promote 로 설정 되어 있음
	ErrAlreadySetPromote = errors.New("image already set promote")

	// ErrNotFoundMirrorImage 볼륨 복제 image 를 찾을 수 없음
	ErrNotFoundMirrorImage = errors.New("not found mirror image")

	// ErrNotSetMirrorEnable rbd image 가 enable 로 설정 되어 있지 않음
	ErrNotSetMirrorEnable = errors.New("image not set mirror enable")
)

// UnsupportedMultiplePeer 다중 rbd peer 등록을 지원 하지 않음
func UnsupportedMultiplePeer(target string) error {
	return errors.Wrap(
		ErrUnsupportedMultiplePeer,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"target_storage_type_key": target,
		}),
	)
}

// DifferentPools 다른 풀에 대한 mirroring 을 지원 하지 않음
func DifferentPools(source, target string) error {
	return errors.Wrap(
		ErrDifferentPools,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"source_ceph_rbd_pool": source,
			"target_ceph_rbd_pool": target,
		}),
	)
}

// TimeoutExecCommand exec 실행에 timeout 발생
func TimeoutExecCommand(c string) error {
	return errors.Wrap(
		ErrTimeoutExecCommand,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"command": c,
		}),
	)
}

// UnexpectedExecResult exec 실행에 에러 발생
func UnexpectedExecResult(r string, e error) error {
	return errors.Wrap(
		ErrUnexpectedExecResult,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"result":       r,
			"result_error": e.Error(),
		}),
	)
}

// NotFoundRBDMirrorProcess rbd-mirror process 를 찾을 수 없음
func NotFoundRBDMirrorProcess(sourceClusterID, sourceStorageID, targetClusterID, targetStorageID uint64) error {
	return errors.Wrap(
		ErrNotFoundRBDMirrorProcess,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"source_cluster_id": sourceClusterID,
			"source_storage_id": sourceStorageID,
			"target_cluster_id": targetClusterID,
			"target_storage_id": targetStorageID,
		}),
	)
}

// NotMatchingPeerInfo peer 정보가 일치 하지 않음
func NotMatchingPeerInfo(client, cluster string) error {
	return errors.Wrap(
		ErrNotMatchingPeerInfo,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"source_client":  client,
			"source_cluster": cluster,
		}),
	)
}

// NotSetMirrorEnable rbd image 가 enable 로 설정 되어 있지 않음
func NotSetMirrorEnable(pool, volume string) error {
	return errors.Wrap(
		ErrNotSetMirrorEnable,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"pool":   pool,
			"volume": volume,
		}),
	)
}

// NotSetPromote rbd image 가 promote 로 설정 되어 있지 않음
func NotSetPromote(pool, volume string) error {
	return errors.Wrap(
		ErrNotSetPromote,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"pool":   pool,
			"volume": volume,
		}),
	)
}

// AlreadySetPromote 볼륨 복제 할 이미지가 target pool 에 존재 하여 볼륨 복제를 진행 할수 없음
func AlreadySetPromote(pool, volume string) error {
	return errors.Wrap(
		ErrAlreadySetPromote,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"pool":   pool,
			"volume": volume,
		}),
	)
}

// NotFoundMirrorImage 볼륨 복제 image 를 찾을 수 없음
func NotFoundMirrorImage(pool, volume string) error {
	return errors.Wrap(
		ErrNotFoundMirrorImage,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"pool":   pool,
			"volume": volume,
		}),
	)
}
