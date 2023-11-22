package environment

import (
	"10.1.1.220/cdm/cdm-center/services/cluster-manager/storage"
	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/internal"
)

// Storage storage type 에 대한 볼륨 복제 관련 인터 페이스
type Storage interface {
	// Prepare 복제 환경 구성 함수
	Prepare() error

	// Destroy 복제 환경 제거 함수
	Destroy() error

	// Start 복제 환경 시작 함수
	Start() error

	// Stop 복제 환경 중지 함수
	Stop() error

	// Monitor 복제 환경 모니터링 함수
	Monitor() error
}

// NewStorageFunc storage interface 생성 함수 타입 정의
type NewStorageFunc func(env *mirror.Environment) (Storage, error)

var newStorageFuncMap = make(map[string]NewStorageFunc)

// RegisterStorageFunc storage 타입별 생성 함수 등록 함수
func RegisterStorageFunc(storageType string, fc NewStorageFunc) {
	newStorageFuncMap[storageType] = fc
}

// NewStorage RegisterStorageFunc 로 등록 한 타입별 storage 생성 함수
func NewStorage(env *mirror.Environment) (Storage, error) {
	var err error

	var s *storage.ClusterStorage
	if s, err = env.GetSourceStorage(); err != nil {
		logger.Errorf("[Environment-NewStorage] Could not get source storage: clusters/%d/storages/%d. Cause: %+v",
			env.SourceClusterStorage.ClusterID, env.SourceClusterStorage.StorageID, err)
		return nil, err
	}

	newFunc, ok := newStorageFuncMap[s.Type]
	if !ok {
		return nil, internal.UnsupportedStorageType(s.Type)
	}

	return newFunc(env)
}
