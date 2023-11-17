package internal

import (
	"encoding/json"
	"fmt"
	"github.com/datacommand2/cdm-cloud/common/errors"
	microErr "github.com/micro/go-micro/v2/errors"
	errors2 "github.com/pkg/errors"
)

var (
	// ErrNotFoundCluster 클러스터를 찾을 수 없음
	ErrNotFoundCluster = errors.New("not found cluster")

	// ErrNotFoundClusterInstance 클러스터 인스턴스를 찾을 수 없음
	ErrNotFoundClusterInstance = errors.New("not found cluster instance")

	// ErrNotFoundProtectionGroup 보호 그룹을 찾을 수 없음
	ErrNotFoundProtectionGroup = errors.New("not found protection group")

	// ErrProtectionGroupExisted 클러스터에 보호그룹이 존재함
	ErrProtectionGroupExisted = errors.New("protection group is existed")

	// ErrNotFoundRecoveryPlan 재해복구계획을 찾을 수 없음
	ErrNotFoundRecoveryPlan = errors.New("not found recovery plan")

	// ErrNotFoundRecoveryPlanSnapshot 재해복구계획 스냅샷을 찾을 수 없음
	ErrNotFoundRecoveryPlanSnapshot = errors.New("not found recovery plan snapshot")

	// ErrRecoveryPlanExisted 클러스터에 재해복구계획이 존재함
	ErrRecoveryPlanExisted = errors.New("recovery plan is existed")

	// ErrNotFoundRecoveryJob 재해복구작업을 찾을 수 없음
	ErrNotFoundRecoveryJob = errors.New("not found recovery job")

	// ErrAlreadyMigrationJobRegistered 재해복구가 진행중이거나 특정일시로 예약되어 있음
	ErrAlreadyMigrationJobRegistered = errors.New("already recovery job registered")

	// ErrUnavailableInstanceExisted 모의훈련이나 재해복구가 불가능한 인스턴스가 존재
	ErrUnavailableInstanceExisted = errors.New("unavailable instance is existed")

	// ErrUndeletableRecoveryJob 삭제할 수 없는 재해복구 작업
	ErrUndeletableRecoveryJob = errors.New("undeletable recovery job")

	// ErrUnchangeableRecoveryJob 수정할 수 없는 재해복구 작업
	ErrUnchangeableRecoveryJob = errors.New("unchangeable recovery job")

	// ErrNotRunnableRecoveryJob 시작할 수 없는 재해복구 작업
	ErrNotRunnableRecoveryJob = errors.New("not runnable recovery job")

	// ErrNotExtendablePausingTimeState 일시중지 시간 연장 불가 상태
	ErrNotExtendablePausingTimeState = errors.New("not extendable pause time state")

	// ErrNotPausableState 일시중지 불가 상태
	ErrNotPausableState = errors.New("not pausable state")

	// ErrNotCancelableState 취소 불가 상태
	ErrNotCancelableState = errors.New("not cancelable state")

	// ErrNotResumableState 재개 불가 상태
	ErrNotResumableState = errors.New("not resumable state")

	// ErrNotRetryableState 재시도 불가 상태
	ErrNotRetryableState = errors.New("not retryable state")

	// ErrNotRollbackableState 롤백 불가 상태
	ErrNotRollbackableState = errors.New("not rollbackable state")

	// ErrNotRollbackRetryableState 롤백 재시도 불가 상태
	ErrNotRollbackRetryableState = errors.New("not rollback retryable state")

	// ErrNotRollbackIgnorableState 롤백 무시 불가 상태
	ErrNotRollbackIgnorableState = errors.New("not rollback ignorable state")

	// ErrNotExtendableRollbackTimeState 롤백 시간 연장 불가 상태
	ErrNotExtendableRollbackTimeState = errors.New("not extendable simulation rollback time state")

	// ErrNotConfirmableState 확정 불가 상태
	ErrNotConfirmableState = errors.New("not confirmable state")

	// ErrNotConfirmRetryableState 확정 재시도 불가 상태
	ErrNotConfirmRetryableState = errors.New("not confirm retryable state")

	// ErrNotConfirmCancelableState 확정 취소 불가 상태
	ErrNotConfirmCancelableState = errors.New("not confirm cancelable state")

	// ErrFailbackRecoveryPlanExisted Failback 을 위한 재해복구 계획이 존재
	ErrFailbackRecoveryPlanExisted = errors.New("failback recovery plan existed")

	// ErrNotFoundTenantRecoveryPlan 테넌트 복구계획을 찾을 수 없음
	ErrNotFoundTenantRecoveryPlan = errors.New("not found tenant recovery plan")

	// ErrNotFoundAvailabilityZoneRecoveryPlan 가용구역 복구계획을 찾을 수 없음
	ErrNotFoundAvailabilityZoneRecoveryPlan = errors.New("not found availability zone recovery plan")

	// ErrNotFoundExternalNetworkRecoveryPlan 외부 네트워크 복구계획을 찾을 수 없음
	ErrNotFoundExternalNetworkRecoveryPlan = errors.New("not found external network recovery plan")

	// ErrNotFoundFloatingIPRecoveryPlan Floating IP 복구계획을 찾을 수 없음
	ErrNotFoundFloatingIPRecoveryPlan = errors.New("not found floating ip recovery plan")

	// ErrNotFoundStorageRecoveryPlan 스토리지 복구계획을 찾을 수 없음
	ErrNotFoundStorageRecoveryPlan = errors.New("not found storage recovery plan")

	// ErrNotFoundVolumeRecoveryPlan 볼륨 복구계획을 찾을 수 없음
	ErrNotFoundVolumeRecoveryPlan = errors.New("not found volume recovery plan")

	// ErrNotFoundInstanceRecoveryPlan 인스턴스 복구계획을 찾을 수 없음
	ErrNotFoundInstanceRecoveryPlan = errors.New("not found instance recovery plan")

	// ErrNotFoundVolumeRecoveryPlanSnapshot 볼륨 복구계획 스냅샷을 찾을 수 없음
	ErrNotFoundVolumeRecoveryPlanSnapshot = errors.New("not found volume recovery plan snapshot")

	// ErrNotFoundInstanceRecoveryPlanSnapshot 인스턴스 복구계획 스냅샷을 찾을 수 없음
	ErrNotFoundInstanceRecoveryPlanSnapshot = errors.New("not found instance recovery plan snapshot")

	// ErrNotFoundRecoverySecurityGroup 복구할 보안그룹을 찾을 수 없음
	ErrNotFoundRecoverySecurityGroup = errors.New("not found recovery security group")

	// ErrStoppingMirrorEnvironmentExisted 제거 중인 복제환경이 존재함
	ErrStoppingMirrorEnvironmentExisted = errors.New("stopping mirror environment is existed")

	// ErrStoppingMirrorVolumeExisted 중지 중인 볼륨이 존재함
	ErrStoppingMirrorVolumeExisted = errors.New("stopping mirror volume is existed")

	// ErrRecoveryJobExisted 재해복구작업이 존재함
	ErrRecoveryJobExisted = errors.New("recovery job is existed")

	// ErrRecoveryJobIsRunning 모의훈련이나 재해복구 작업이 진행 중
	ErrRecoveryJobIsRunning = errors.New("recovery job state code is running")

	// ErrSnapshotCreationTimeHasPassed 현재시간이 스냅샷 생성주기를 벗어남
	ErrSnapshotCreationTimeHasPassed = errors.New("snapshot creation time has passed")

	// ErrUnexpectedJobOperation 예상하지 못한 Job Operation
	ErrUnexpectedJobOperation = errors.New("unexpected job operation")

	// ErrNotFoundRecoveryResult 복구 결과를 찾을 수 없음
	ErrNotFoundRecoveryResult = errors.New("not found recovery result")

	// ErrUndeletableRecoveryResult 삭제할 수 없는 복구 결과
	ErrUndeletableRecoveryResult = errors.New("undeletable recovery result")

	// ErrIPCFailedBadRequest IPCFailed bad request
	ErrIPCFailedBadRequest = errors.New("ipc failed bad request")

	// ErrIPCNoContent IPC no content
	ErrIPCNoContent = errors.New("ipc no content")

	// ErrServerBusy server busy 에러
	ErrServerBusy = errors.New("server busy")

	// ErrMismatchJobTypeCode job type code 가 불 일치
	ErrMismatchJobTypeCode = errors.New("mismatch job type code")

	// ErrNotFoundExternalRoutingInterface 클러스터 라우터에 외부 네트워크 라우팅 인터페이스가 존재하지 않음
	ErrNotFoundExternalRoutingInterface = errors.New("not found external network routing interface")

	// ErrNotFoundRecoveryPlans 보호 그룹의 복구 계획 목록을 찾을 수 없음
	ErrNotFoundRecoveryPlans = errors.New("not found recovery plans")

	// ErrSnapshotCreationTimeout 보호 그룹 스냅샷 생성 타임 아웃 발생
	ErrSnapshotCreationTimeout = errors.New("snapshot creation timeout")

	// ErrInactiveCluster 클러스터 비 활성화 상태
	ErrInactiveCluster = errors.New("inactive cluster")

	// ErrDifferentInstanceList 보호 그룹의 인스턴스 목록과 재해 복구 계획의 인스턴스 목록이 다름
	ErrDifferentInstanceList = errors.New("different instances list of protection group and plan detail")

	// ErrInsufficientStorageSpace 스토리지 저장 공간 부족
	ErrInsufficientStorageSpace = errors.New("insufficient storage space")

	// ErrInstanceNumberExceeded 라이선스에 명시된 인스턴스 개수 초과
	ErrInstanceNumberExceeded = errors.New("maximum number of instances allowed exceeded")

	// ErrUnknownClusterNode 알 수 없는 노드 uuid
	ErrUnknownClusterNode = errors.New("unknown cluster node")

	// ErrFloatingIPAddressDuplicated floating ip 복구계획의 ip address 가 보호대상 인스턴스의 floating ip address 와 중복됨
	ErrFloatingIPAddressDuplicated = errors.New("protection instance floating IP address is duplicated of recovery cluster floating IP address")

	// ErrVolumeIsNotMirroring volume 이 mirroring 상태가 아님
	ErrVolumeIsNotMirroring = errors.New("volume is not mirroring")

	// ErrStorageKeyringRegistrationRequired storage metadata 에 keyring 이 없어서 등록이 필요함
	ErrStorageKeyringRegistrationRequired = errors.New("storage keyring registration required")

	// ErrVolumeIsAlreadyGrouped volume 이 이미 volume group 에 속해있음
	ErrVolumeIsAlreadyGrouped = errors.New("volume is already grouped")

	// ErrInsufficientRecoveryHypervisorResource 복구대상 하이퍼바이저 리소스 부족
	ErrInsufficientRecoveryHypervisorResource = errors.New("insufficient recovery hypervisor resource")

	// ErrNotFoundInstanceTemplate 인스턴스 템플릿을 찾을 수 없음
	ErrNotFoundInstanceTemplate = errors.New("not found instance template")

	// ErrSameNameIsAlreadyExisted 동일한 이름이 이미 존재함
	ErrSameNameIsAlreadyExisted = errors.New("same name is already existed")

	// ErrParentVolumeIsExisted 부모 볼륨이 존재함
	ErrParentVolumeIsExisted = errors.New("parent volume is existed")

	// ErrNotFoundOwnerGroup 오너 그룹을 찾을 수 없음.
	ErrNotFoundOwnerGroup = errors.New("not found owner group")

	// ErrMismatchParameter 파라미터가 일치하지 않음
	ErrMismatchParameter = errors.New("mismatch parameter")
)

// DifferentInstanceList 보호 그룹의 인스턴스 목록과 재해 복구 계획의 인스턴스 목록이 다름
func DifferentInstanceList(pgid, pid uint64) error {
	return errors.Wrap(
		ErrDifferentInstanceList,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgid,
			"plan_id":             pid,
		}),
	)
}

// InactiveCluster 클러스터 비 활성화 상태
func InactiveCluster(pgid, clusterID uint64) error {
	return errors.Wrap(
		ErrInactiveCluster,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgid,
			"cluster_id":          clusterID,
		}),
	)
}

// SnapshotCreationTimeout 보호 그룹 스냅샷 생성 타임 아웃 발생
func SnapshotCreationTimeout(pgid uint64) error {
	return errors.Wrap(
		ErrSnapshotCreationTimeout,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgid,
		}),
	)
}

// NotFoundRecoveryPlans 보호 그룹의 복구 계획 목록을 찾을 수 없음
func NotFoundRecoveryPlans(pgid uint64) error {
	return errors.Wrap(
		ErrNotFoundRecoveryPlans,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgid,
		}),
	)
}

// NotFoundCluster 클러스터를 찾을 수 없음
func NotFoundCluster(id uint64) error {
	return errors.Wrap(
		ErrNotFoundCluster,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id": id,
		}),
	)
}

// NotFoundClusterInstance 클러스터 인스턴스를 찾을 수 없음
func NotFoundClusterInstance(id uint64) error {
	return errors.Wrap(
		ErrNotFoundClusterInstance,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id": id,
		}),
	)
}

// NotFoundProtectionGroup 보호 그룹을 찾을 수 없음
func NotFoundProtectionGroup(id, tid uint64) error {
	return errors.Wrap(
		ErrNotFoundProtectionGroup,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": id,
			"tenant_id":           tid,
		}),
	)
}

// ProtectionGroupExisted 실행중인 재해복구작업이 존재함
func ProtectionGroupExisted(clusterID uint64) error {
	return errors.Wrap(
		ErrProtectionGroupExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id": clusterID,
		}),
	)
}

// NotFoundRecoveryPlan 재해복구계획을 찾을 수 없음
func NotFoundRecoveryPlan(gid, pid uint64) error {
	return errors.Wrap(
		ErrNotFoundRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": gid,
			"recovery_plan_id":    pid,
		}),
	)
}

// NotFoundRecoveryPlanSnapshot 재해복구계획 스냅샷을 찾을 수 없음
func NotFoundRecoveryPlanSnapshot(gid, pid, sid uint64) error {
	return errors.Wrap(
		ErrNotFoundRecoveryPlanSnapshot,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id":          gid,
			"recovery_plan_id":             pid,
			"protection_group_snapshot_id": sid,
		}),
	)
}

// RecoveryPlanExisted 재해복구계획이 존재함.
func RecoveryPlanExisted(cid, gid uint64) error {
	return errors.Wrap(
		ErrRecoveryPlanExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id":          cid,
			"protection_group_id": gid,
		}),
	)
}

// NotFoundRecoveryJob 재해복구작업을 찾을 수 없음.
func NotFoundRecoveryJob(gid, jid uint64) error {
	return errors.Wrap(
		ErrNotFoundRecoveryJob,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": gid,
			"recovery_job_id":     jid,
		}),
	)
}

// AlreadyMigrationJobRegistered 재해복구가 진행중이거나 특정일시로 예약되어 있음
func AlreadyMigrationJobRegistered(pgID uint64) error {
	return errors.Wrap(
		ErrAlreadyMigrationJobRegistered,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
		}),
	)
}

// UnavailableInstanceExisted 사용할 수 없는 인스턴스가 존재
func UnavailableInstanceExisted(m map[uint64]*UnavailableInstance) error {
	var l []*UnavailableInstance
	for _, v := range m {
		l = append(l, v)
	}

	return errors.Wrap(
		ErrUnavailableInstanceExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"instances": l,
		}),
	)
}

// UnchangeableRecoveryJob 수정할 수 없는 재해복구작업
func UnchangeableRecoveryJob(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrUnchangeableRecoveryJob,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// UndeletableRecoveryJob 삭제할 수 없는 재해복구 작업
func UndeletableRecoveryJob(pgID, jobID uint64, stateCode string) error {
	return errors.Wrap(
		ErrUndeletableRecoveryJob,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id":     pgID,
			"recovery_job_id":         jobID,
			"recovery_job_state_code": stateCode,
		}),
	)
}

// NotPausableState 일시중지 불가 상태
func NotPausableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotPausableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotRunnableRecoveryJob 시작할 수 없는 재해복구 작업
func NotRunnableRecoveryJob(pgID, jobID uint64, cause error) error {
	return errors.Wrap(
		ErrNotRunnableRecoveryJob,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
			"cause":               fmt.Sprintf("%+v", errors2.WithStack(cause)),
		}),
	)
}

// NotExtendablePausingTimeState 일시중지 시간 연장 불가 상태
func NotExtendablePausingTimeState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotExtendablePausingTimeState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotCancelableState 취소 불가 상태
func NotCancelableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotCancelableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotResumableState 재개 불가 상태
func NotResumableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotResumableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotRetryableState 재시도 불가 상태
func NotRetryableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotRetryableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotRollbackableState 롤백 불가 상태
func NotRollbackableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotRollbackableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotRollbackRetryableState 롤백 재시도 불가 상태
func NotRollbackRetryableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotRollbackRetryableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotRollbackIgnorableState 롤백 무시 불가 상태
func NotRollbackIgnorableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotRollbackIgnorableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotExtendableRollbackTimeState 롤백 시간 연장 불가 상태
func NotExtendableRollbackTimeState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotExtendableRollbackTimeState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotConfirmableState 확정 불가 상태
func NotConfirmableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotConfirmableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotConfirmRetryableState 확정 재시도 불가 상태
func NotConfirmRetryableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotConfirmRetryableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// NotConfirmCancelableState 확정 취소 불가 상태
func NotConfirmCancelableState(pgID, jobID uint64) error {
	return errors.Wrap(
		ErrNotConfirmCancelableState,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_job_id":     jobID,
		}),
	)
}

// FailbackRecoveryPlanExisted 삭제할 수 없는 재해복구 작업
func FailbackRecoveryPlanExisted(pgID, planID uint64) error {
	return errors.Wrap(
		ErrFailbackRecoveryPlanExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_plan_id":    planID,
		}),
	)
}

// NotFoundTenantRecoveryPlan 테넌트 복구계획을 찾을 수 없음
func NotFoundTenantRecoveryPlan(planID, tenantID uint64) error {
	return errors.Wrap(
		ErrNotFoundTenantRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": planID,
			"tenant_id":        tenantID,
		}),
	)
}

// NotFoundAvailabilityZoneRecoveryPlan 가용구역 복구계획을 찾을 수 없음
func NotFoundAvailabilityZoneRecoveryPlan(planID, zoneID uint64) error {
	return errors.Wrap(
		ErrNotFoundAvailabilityZoneRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id":     planID,
			"availability_zone_id": zoneID,
		}),
	)
}

// NotFoundExternalNetworkRecoveryPlan 외부 네트워크 복구계획을 찾을 수 없음
func NotFoundExternalNetworkRecoveryPlan(planID, netID uint64) error {
	return errors.Wrap(
		ErrNotFoundExternalNetworkRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id":    planID,
			"external_network_id": netID,
		}),
	)
}

// NotFoundFloatingIPRecoveryPlan Floating IP 복구계획을 찾을 수 없음
func NotFoundFloatingIPRecoveryPlan(planID, ipID uint64) error {
	return errors.Wrap(
		ErrNotFoundFloatingIPRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": planID,
			"floating_ip_id":   ipID,
		}),
	)
}

// NotFoundStorageRecoveryPlan 스토리지 복구계획을 찾을 수 없음
func NotFoundStorageRecoveryPlan(planID, storageID uint64) error {
	return errors.Wrap(
		ErrNotFoundStorageRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": planID,
			"storage_id":       storageID,
		}),
	)
}

// NotFoundVolumeRecoveryPlan 볼륨 복구계획을 찾을 수 없음
func NotFoundVolumeRecoveryPlan(planID, volumeID uint64) error {
	return errors.Wrap(
		ErrNotFoundVolumeRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": planID,
			"volume_id":        volumeID,
		}),
	)
}

// NotFoundInstanceRecoveryPlan 인스턴스 복구계획을 찾을 수 없음
func NotFoundInstanceRecoveryPlan(planID, instanceID uint64) error {
	return errors.Wrap(
		ErrNotFoundInstanceRecoveryPlan,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": planID,
			"instance_id":      instanceID,
		}),
	)
}

// NotFoundVolumeRecoveryPlanSnapshot 볼륨 복구계획 스냅샷을 찾을 수 없음
func NotFoundVolumeRecoveryPlanSnapshot(pid, sid, vid uint64) error {
	return errors.Wrap(
		ErrNotFoundVolumeRecoveryPlanSnapshot,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": pid,
			"snapshot_id":      sid,
			"volume_id":        vid,
		}),
	)
}

// NotFoundInstanceRecoveryPlanSnapshot 인스턴스 복구계획 스냅샷을 찾을 수 없음
func NotFoundInstanceRecoveryPlanSnapshot(pid, sid, iid uint64) error {
	return errors.Wrap(
		ErrNotFoundInstanceRecoveryPlanSnapshot,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_plan_id": pid,
			"snapshot_id":      sid,
			"instance_id":      iid,
		}),
	)
}

// NotFoundRecoverySecurityGroup 복구할 보안그룹을 찾을 수 없음
func NotFoundRecoverySecurityGroup(pgID, sgID uint64) error {
	return errors.Wrap(
		ErrNotFoundRecoverySecurityGroup,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id":       pgID,
			"cluster_security_group_id": sgID,
		}),
	)
}

// StoppingMirrorEnvironmentExisted 제거 중인 복제환경이 존재함
func StoppingMirrorEnvironmentExisted(sid, tid uint64) error {
	return errors.Wrap(
		ErrStoppingMirrorEnvironmentExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"source_cluster_storage_id": sid,
			"target_cluster_storage_id": tid,
		}),
	)
}

// StoppingMirrorVolumeExisted 중지 중인 볼륨이 존재함
func StoppingMirrorVolumeExisted(sid, tid, vid uint64) error {
	return errors.Wrap(
		ErrStoppingMirrorVolumeExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"source_cluster_storage_id": sid,
			"target_cluster_storage_id": tid,
			"cluster_volume_id":         vid,
		}),
	)
}

// RecoveryJobExisted 재해복구작업이 존재함
func RecoveryJobExisted(pgID, planID uint64) error {
	return errors.Wrap(
		ErrRecoveryJobExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_plan_id":    planID,
		}),
	)
}

// RecoveryJobScheduleConflicted 동일한 시간의 스케줄이 존재함
func RecoveryJobScheduleConflicted(new string, id uint64, old string) error {
	return errors.Wrap(
		ErrRecoveryJobExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"input_schedule":          new,
			"conflicted_job_id":       id,
			"conflicted_job_schedule": old,
		}),
	)
}

// RecoveryJobIsRunning 모의훈련이나 재해복구가 진행 중
func RecoveryJobIsRunning(pgID, planID, jobID uint64) error {
	return errors.Wrap(
		ErrRecoveryJobIsRunning,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
			"recovery_plan_id":    planID,
			"recovery_job_id":     jobID,
		}),
	)
}

// SnapshotCreationTimeHasPassed 현재시간이 스냅샷 생성주기를 벗어남
func SnapshotCreationTimeHasPassed(pgID uint64) error {
	return errors.Wrap(
		ErrSnapshotCreationTimeHasPassed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgID,
		}),
	)
}

// UnexpectedJobOperation 예상하지 못한 Job Operation
func UnexpectedJobOperation(id uint64, state, op string) error {
	return errors.Wrap(
		ErrUnexpectedJobOperation,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_job_id":        id,
			"recovery_job_state":     state,
			"recovery_job_operation": op,
		}),
	)
}

// NotFoundRecoveryResult 복구 결과를 찾을 수 없음
func NotFoundRecoveryResult(resultID uint64) error {
	return errors.Wrap(
		ErrNotFoundRecoveryResult,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_result_id": resultID,
		}),
	)
}

// UndeletableRecoveryResult 삭제할 수 없는 복구 결과
func UndeletableRecoveryResult(resultID uint64, typeCode string) error {
	return errors.Wrap(
		ErrUndeletableRecoveryResult,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_result_id": resultID,
			"recovery_type_code": typeCode,
		}),
	)
}

// IPCFailedBadRequest IPCFailed bad request
func IPCFailedBadRequest(cause error) error {
	err := microErr.Parse(cause.Error())

	var msg errors.Message
	if json.Unmarshal([]byte(err.Detail), &msg) != nil || msg.Code == "" {
		msg.Code = err.Detail
	}

	return errors.Wrap(
		ErrIPCFailedBadRequest,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"code":    err.Code,
			"message": &msg,
		}),
	)
}

// IPCNoContent IPC no content
func IPCNoContent() error {
	return errors.Wrap(
		ErrIPCNoContent,
		errors.CallerSkipCount(1),
	)
}

// ServerBusy server busy 에러
func ServerBusy(cause error) error {
	return errors.Wrap(
		ErrServerBusy,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cause": cause,
		}),
	)
}

// MismatchJobTypeCode job type code 가 불 일치
func MismatchJobTypeCode(pid, jid uint64, jobTypeCode string) error {
	return errors.Wrap(
		ErrMismatchJobTypeCode,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id":     pid,
			"recovery_job_id":         jid,
			"recovery_job_type_code:": jobTypeCode,
		}),
	)
}

// NotFoundExternalRoutingInterface 클러스터 라우터에 외부 네트워크 라우팅 인터페이스가 존재하지 않음
func NotFoundExternalRoutingInterface(rid uint64) error {
	return errors.Wrap(
		ErrNotFoundExternalRoutingInterface,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"router_id": rid,
		}),
	)
}

// InsufficientStorageSpace 스토리지 저장 공간 부족
func InsufficientStorageSpace(pgid, pid, rsize, fsize uint64) error {
	return errors.Wrap(
		ErrInsufficientStorageSpace,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_group_id": pgid,
			"plan_id":             pid,
			"required_size":       rsize,
			"free_size":           fsize,
		}),
	)
}

// FloatingIPAddressDuplicated floating ip address 중복
func FloatingIPAddressDuplicated(uuid, ipAddress string) error {
	return errors.Wrap(
		ErrFloatingIPAddressDuplicated,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_instance_uuid": uuid,
			"floating_ip_address":      ipAddress,
		}),
	)
}

// VolumeIsNotMirroring volume 이 mirroring 상태가 아님
func VolumeIsNotMirroring(vid uint64, code string) error {
	return errors.Wrap(
		ErrVolumeIsNotMirroring,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"protection_cluster_volume_id": vid,
			"state_code":                   code,
		}),
	)
}

// StorageKeyringRegistrationRequired storage keyring 등록 필요
func StorageKeyringRegistrationRequired(cid uint64, uuid string) error {
	return errors.Wrap(
		ErrStorageKeyringRegistrationRequired,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id":   cid,
			"storage_uuid": uuid,
		}),
	)
}

// VolumeGroupIsExisted volume 이 이미 volume group 에 속해있음
func VolumeGroupIsExisted(cid, instanceID uint64, uuid string) error {
	return errors.Wrap(
		ErrVolumeIsAlreadyGrouped,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id":  cid,
			"instance_id": instanceID,
			"volume_uuid": uuid,
		}),
	)
}

func InsufficientRecoveryHypervisorResource(clusterID uint64, msg []string) error {
	return errors.Wrap(
		ErrInsufficientRecoveryHypervisorResource,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"recovery_cluster_id":          clusterID,
			"insufficient_hypervisor_info": msg,
		}),
	)
}

// NotFoundInstanceTemplate 인스턴스 템플릿을 찾을 수 없음
func NotFoundInstanceTemplate(templateID uint64) error {
	return errors.Wrap(
		ErrNotFoundInstanceTemplate,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"instance_template_id": templateID,
		}),
	)
}

// SameNameIsAlreadyExisted 동일한 이름이 이미 존재함
func SameNameIsAlreadyExisted(cid uint64, name string) error {
	return errors.Wrap(
		ErrSameNameIsAlreadyExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id": cid,
			"name":       name,
		}),
	)
}

// ParentVolumeIsExisted 부모 볼륨이 존재함
func ParentVolumeIsExisted(cid uint64, uuid string) error {
	return errors.Wrap(
		ErrParentVolumeIsExisted,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"cluster_id":  cid,
			"volume_uuid": uuid,
		}),
	)
}

// NotFoundOwnerGroup 오너 그룹을 찾을 수 없음
func NotFoundOwnerGroup(ownerGroupID uint64) error {
	return errors.Wrap(
		ErrNotFoundOwnerGroup,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"owner_group_id": ownerGroupID,
		}),
	)
}

// MismatchParameter 파라미터와 벨류가 일치하지 않음
func MismatchParameter(param1 string, value1 uint64, param2 string, value2 uint64) error {
	return errors.Wrap(
		ErrMismatchParameter,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"param1": param1,
			"value1": value1,
			"param2": param2,
			"value2": value2,
		}),
	)
}
