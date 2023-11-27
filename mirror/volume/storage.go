package volume

import (
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/internal"
)

// Storage 볼륨 복제 storage 인터 페이스
type Storage interface {
	// Start 볼륨 복제 시작 함수
	Start() error

	// Stop 볼륨 복제 중지 함수
	Stop() error

	// StopAndDelete 볼륨 복제 중지 및 삭제 함수
	StopAndDelete() error

	// Pause 볼륨 복제 일시 중지 함수
	Pause() error

	// Resume 볼륨 복제 재개 함수
	Resume() error

	// MirrorEnable 볼륨 복제 시작을 지속 적으로 활성화 시키는 함수
	MirrorEnable(string) error

	// PauseEnable 볼륨 복제를 일시 중지를 지속 적으로 활성화 시키는 함수
	PauseEnable(string) error
}

// NewStorageFunc Storage interface 생성 함수 타입 정의
type NewStorageFunc func(env *mirror.Volume) (Storage, error)

var newStorageFuncMap = make(map[string]NewStorageFunc)

// RegisterStorageFunc storage 타입별 생성 함수 등록 함수
func RegisterStorageFunc(storageType string, fc NewStorageFunc) {
	newStorageFuncMap[storageType] = fc
}

// NewStorage RegisterStorageFunc 로 등록 한 타입별 Storage 생성 함수
func NewStorage(vol *mirror.Volume) (Storage, error) {
	var err error

	var s *storage.ClusterStorage
	if s, err = vol.GetSourceStorage(); err != nil {
		logger.Errorf("[Volume-NewStorage] Could not get source storage: clusters/%d/storages/%d. Cause: %+v",
			vol.SourceClusterStorage.ClusterID, vol.SourceClusterStorage.StorageID, err)
		return nil, err
	}

	newFunc, ok := newStorageFuncMap[s.Type]
	if !ok {
		return nil, internal.UnsupportedStorageType(s.Type)
	}

	return newFunc(vol)
}
