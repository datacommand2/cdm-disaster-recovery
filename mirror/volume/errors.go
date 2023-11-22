package volume

import (
	"fmt"

	"10.1.1.220/cdm/cdm-cloud/common/errors"
)

var (
	// ErrCreateStorageFailed 볼륨 복제에 사용 하기 위한 storage 생성 실패
	ErrCreateStorageFailed = errors.New("mirror volume storage create failed")

	// ErrStartFailed 볼륨 복제 시작 에러
	ErrStartFailed = errors.New("mirror volume start failed")

	// ErrStopFailed 볼륨 복제 중지 에러
	ErrStopFailed = errors.New("mirror volume stop failed")

	// ErrPauseFailed 볼륨 복제 일시 중지 에러
	ErrPauseFailed = errors.New("mirror volume pause failed")

	// ErrResumeFailed 볼륨 복제 재개 에러
	ErrResumeFailed = errors.New("mirror volume resume failed")

	// ErrStopAndDeleteFailed 볼륨 복제 중지 및 삭제 에러
	ErrStopAndDeleteFailed = errors.New("mirror volume stop and delete failed")

	// ErrIsNotEnvironmentStateCodeMirroring 복제 환경의 상태 코드가 mirroring 이 아님
	ErrIsNotEnvironmentStateCodeMirroring = errors.New("mirror environment state code is not mirroring")

	// ErrUnusableMirrorEnvironment 복제 환경을 이용 할 수 없음
	ErrUnusableMirrorEnvironment = errors.New("unusable mirror environment")

	// ErrDestroyFailed 볼륨 복제 메타데이터 제거 에러
	ErrDestroyFailed = errors.New("mirror volume destroy failed")
)

// CreateStorageFailed 볼륨 복제에 사용 하기 위한 storage 생성 실패
func CreateStorageFailed(err error) error {
	return errors.Wrap(
		ErrCreateStorageFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// StartFailed 볼륨 복제 시작 에러
func StartFailed(err error) error {
	return errors.Wrap(
		ErrStartFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// DestroyFailed 볼륨 복제 메타데이터 삭제 에러
func DestroyFailed(err error) error {
	return errors.Wrap(
		ErrDestroyFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// StopFailed 볼륨 복제 중지 에러
func StopFailed(err error) error {
	return errors.Wrap(
		ErrStopFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// StopAndDeleteFailed 볼륨 복제 중지 및 삭제 에러
func StopAndDeleteFailed(err error) error {
	return errors.Wrap(
		ErrStopAndDeleteFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// PauseFailed 볼륨 복제 일시 중지 에러
func PauseFailed(err error) error {
	return errors.Wrap(
		ErrPauseFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// ResumeFailed 볼륨 복제 재개 에러
func ResumeFailed(err error) error {
	return errors.Wrap(
		ErrResumeFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// UnusableMirrorEnvironment 복제 환경을 이용 할 수 없음
func UnusableMirrorEnvironment(err error) error {
	return errors.Wrap(
		ErrUnusableMirrorEnvironment,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}
