package environment

import (
	"fmt"
	"github.com/datacommand2/cdm-cloud/common/errors"
)

var (
	// ErrCreateStorageFailed 복제 환경 구성, 삭제 에 사용 하기 위한 storage 생성 실패
	ErrCreateStorageFailed = errors.New("mirror environment storage create failed")

	// ErrInitializeFailed 복제 환경 구성 에러 발생
	ErrInitializeFailed = errors.New("mirror environment initialize failed")

	// ErrStartFailed 복제 환경 시작 에러 발생
	ErrStartFailed = errors.New("mirror environment start failed")

	// ErrMonitorFailed 복제 환경 구성/삭제 모니터 에러 발생
	ErrMonitorFailed = errors.New("mirror environment monitor failed")

	// ErrDestroyFailed 복제 환경 제거 에러 발생
	ErrDestroyFailed = errors.New("mirror environment destroy failed")

	// ErrStopFailed 복제 환경 중지 에러 발생
	ErrStopFailed = errors.New("mirror environment stop failed")

	// ErrDeletedFail kv store 에서 복제 환경 삭제 에러 발생
	ErrDeletedFail = errors.New("mirror environment delete failed")
)

// CreateStorageFailed 복제 환경 구성, 삭제 에 사용 하기 위한 storage 생성 실패
func CreateStorageFailed(err error) error {
	return errors.Wrap(
		ErrCreateStorageFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// InitializeFailed 복제 환경 구성 에러
func InitializeFailed(err error) error {
	return errors.Wrap(
		ErrInitializeFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// StartFailed 복제 환경 시작 에러
func StartFailed(err error) error {
	return errors.Wrap(
		ErrStartFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// MonitorFailed 복제 환경 모니터링 에러
func MonitorFailed(err error) error {
	return errors.Wrap(
		ErrMonitorFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// DestroyFailed 복제 환경 제거 에러
func DestroyFailed(err error) error {
	return errors.Wrap(
		ErrDestroyFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// StopFailed 복제 환경 중지 에러
func StopFailed(err error) error {
	return errors.Wrap(
		ErrStopFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}

// DeletedFail 복제 환경 중지 에러
func DeletedFail(err error) error {
	return errors.Wrap(
		ErrDeletedFail,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": fmt.Sprintf("%+v", err),
		}),
	)
}
