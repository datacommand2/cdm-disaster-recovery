package migrator

import "time"

const (
	// defaultCreationRequestTimeout 리소스 생성 요청 timeout
	defaultCreationRequestTimeout = 1 * time.Hour

	// defaultDeletionRequestTimeout 리소스 삭제 요청 timeout
	defaultDeletionRequestTimeout = 1 * time.Hour

	// volumeCopyRequestTimeout 볼륨 복제 요청 timeout
	volumeCopyRequestTimeout = 24 * time.Hour

	// instanceCreationRequestTimeout 인스턴스 생성 요청 timeout
	instanceCreationRequestTimeout = 1 * time.Hour
)
