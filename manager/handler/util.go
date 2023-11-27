package handler

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/event"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/util"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
)

func validateRequest(ctx context.Context) error {
	if _, err := metadata.GetTenantID(ctx); err != nil {
		return errors.InvalidRequest(ctx)
	}

	if _, err := metadata.GetAuthenticatedUser(ctx); err != nil {
		return errors.InvalidRequest(ctx)
	}

	return nil
}

func reportEvent(ctx context.Context, eventCode, errorCode string, eventContents interface{}) {
	id, err := metadata.GetTenantID(ctx)
	if err != nil {
		logger.Warnf("[handler-reportEvent] Could not get the tenant ID. Cause: %v", err)
		return
	}

	err = event.ReportEvent(id, eventCode, errorCode, event.WithContents(eventContents))
	if err != nil {
		logger.Warnf("[handler-reportEvent] Could not report the event(%s:%s). Cause: %v", eventCode, errorCode, err)
	}
}

// createError 각 서비스의 internal error 를 처리
func createError(ctx context.Context, eventCode string, err error) error {
	if err == nil {
		return nil
	}

	var errorCode string
	switch {
	// service
	case errors.Equal(err, internal.ErrServerBusy):
		errorCode = "server_busy"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotPausableState):
		errorCode = "not_pausable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotExtendablePausingTimeState):
		errorCode = "not_extendable_pausing_time_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotResumableState):
		errorCode = "not_resumable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotCancelableState):
		errorCode = "not_cancelable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotRollbackRetryableState):
		errorCode = "not_rollback_retryable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotExtendableRollbackTimeState):
		errorCode = "not_extendable_rollback_time_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotRollbackIgnorableState):
		errorCode = "not_rollback_ignorable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotRollbackableState):
		errorCode = "not_rollbackable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotConfirmableState):
		errorCode = "not_confirmable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotConfirmRetryableState):
		errorCode = "not_confirm_retryable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotConfirmCancelableState):
		errorCode = "not_confirm_cancelable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotRetryableState):
		errorCode = "not_retryable_state"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundOwnerGroup):
		errorCode = "not_found_owner_group"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrMismatchParameter):
		errorCode = "mismatch_parameter"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	// ipc error
	case errors.Equal(err, internal.ErrIPCFailedBadRequest):
		errorCode = "ipc_bad_request"
		return errors.StatusBadRequest(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrIPCNoContent):
		errorCode = "ipc_no_content"
		return errors.StatusNoContent(ctx, eventCode, errorCode, err)

	// cluster
	case errors.Equal(err, internal.ErrInactiveCluster):
		errorCode = "inactive_cluster"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundCluster):
		errorCode = "not_found_cluster"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundClusterInstance):
		errorCode = "not_found_instance"
		return errors.StatusNoContent(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		errorCode = "not_found_external_routing_interface"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrDifferentInstanceList):
		errorCode = "different_instances_list"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// 재해복구/모의훈련이 불가능한 인스턴스 존재시
	case errors.Equal(err, internal.ErrUnavailableInstanceExisted):
		errorCode = "unavailable_instance_existed"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	// storage
	case errors.Equal(err, internal.ErrInsufficientStorageSpace):
		errorCode = "insufficient_storage_space"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// protection group
	case errors.Equal(err, internal.ErrNotFoundProtectionGroup):
		errorCode = "not_found_protection_group"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrProtectionGroupExisted):
		errorCode = "protection_group_existed"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	// recovery plan
	case errors.Equal(err, internal.ErrNotFoundRecoveryPlan):
		errorCode = "not_found_recovery_plan"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundRecoveryPlans):
		errorCode = "not_found_recovery_plans"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrRecoveryPlanExisted):
		errorCode = "recovery_plan_existed"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrParentVolumeIsExisted):
		errorCode = "parent_volume_is_existed"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrFailbackRecoveryPlanExisted):
		errorCode = "failback_recovery_plan_existed"
		return errors.StatusBadRequest(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundTenantRecoveryPlan):
		errorCode = "not_found_tenant_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundAvailabilityZoneRecoveryPlan):
		errorCode = "not_found_availability_zone_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundExternalNetworkRecoveryPlan):
		errorCode = "not_found_external_network_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundFloatingIPRecoveryPlan):
		errorCode = "not_found_floating_ip_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundStorageRecoveryPlan):
		errorCode = "not_found_storage_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundVolumeRecoveryPlan):
		errorCode = "not_found_volume_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundInstanceRecoveryPlan):
		errorCode = "not_found_instance_recovery_plan"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrFloatingIPAddressDuplicated):
		errorCode = "floating_ip_address_duplicated"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrStorageKeyringRegistrationRequired):
		errorCode = "storage_keyring_registration_required"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	// task
	case errors.Equal(err, internal.ErrNotFoundRecoverySecurityGroup):
		errorCode = "not_found_recovery_security_group"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrSameNameIsAlreadyExisted):
		errorCode = "same_name_is_already_existed"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// job
	case errors.Equal(err, internal.ErrNotFoundRecoveryJob):
		errorCode = "not_found_recovery_job"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrRecoveryJobExisted):
		errorCode = "recovery_job_existed"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrRecoveryJobIsRunning):
		errorCode = "recovery_job_running"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrUnchangeableRecoveryJob):
		errorCode = "unchangeable_recovery_job"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotRunnableRecoveryJob):
		errorCode = "not_runnable_recovery_job"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrUndeletableRecoveryJob):
		errorCode = "undeletable_recovery_job"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrAlreadyMigrationJobRegistered):
		errorCode = "already_migration_job_registered"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrMismatchJobTypeCode):
		errorCode = "not_simulation_recovery_job"
		return errors.StatusBadRequest(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrUnexpectedJobOperation):
		errorCode = "unexpected_job_operation"
		return errors.StatusBadRequest(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrInsufficientRecoveryHypervisorResource):
		errorCode = "insufficient_recovery_hypervisor_resource"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	// result
	case errors.Equal(err, internal.ErrNotFoundRecoveryResult):
		errorCode = "not_found_recovery_result"
		return errors.StatusNotFound(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrUndeletableRecoveryResult):
		errorCode = "undeletable_recovery_result"
		return errors.StatusPreconditionFailed(ctx, eventCode, errorCode, err)

	// snapshot
	case errors.Equal(err, internal.ErrSnapshotCreationTimeout):
		errorCode = "snapshot_creation_timeout"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrSnapshotCreationTimeHasPassed):
		errorCode = "snapshot_creation_time_has_passed"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrNotFoundVolumeRecoveryPlanSnapshot):
		errorCode = "not_found_volume_recovery_plan_snapshot"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// mirror
	case errors.Equal(err, internal.ErrStoppingMirrorEnvironmentExisted):
		errorCode = "stopping_mirror_environment_existed"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrStoppingMirrorVolumeExisted):
		errorCode = "stopping_mirror_volume_existed"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, internal.ErrVolumeIsNotMirroring):
		errorCode = "volume_is_not_mirroring"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	default:
		if err := util.CreateError(ctx, eventCode, err); err != nil {
			return err
		}
	}

	return nil
}
