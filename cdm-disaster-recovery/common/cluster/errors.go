package cluster

import "github.com/datacommand2/cdm-cloud/common/errors"

var (
	// ErrUnavailableStorageExisted 사용할 수 없는 복구 스토리지가 존재함
	ErrUnavailableStorageExisted = errors.New("unavailable storage is existed")
)

// UnavailableStorageExisted 사용할 수 없는 복구 스토리지가 존재함
func UnavailableStorageExisted(storages []uint64) error {
	return errors.Wrap(
		ErrUnavailableStorageExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"storages": storages,
		}),
	)
}
