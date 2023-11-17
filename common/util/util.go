package util

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/util"
	"github.com/datacommand2/cdm-disaster-recovery/common/cluster"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"github.com/datacommand2/cdm-disaster-recovery/common/snapshot"
)

// CreateError 각 서비스의 internal error 를 처리
func CreateError(ctx context.Context, eventCode string, err error) error {
	if err == nil {
		return nil
	}

	var errorCode string
	switch {
	// mirror
	case errors.Equal(err, mirror.ErrNotFoundMirrorEnvironment):
		errorCode = "not_found_mirror_environment"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorEnvironmentStatus):
		errorCode = "not_found_mirror_environment_status"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorEnvironmentOperation):
		errorCode = "not_found_mirror_environment_operation"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrUnknownMirrorEnvironmentOperation):
		errorCode = "unknown_mirror_environment_operation"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrUnknownMirrorEnvironmentState):
		errorCode = "unknown_mirror_environment_state"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorVolume):
		errorCode = "not_found_mirror_volume"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeOperation):
		errorCode = "not_found_mirror_volume_operation"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeStatus):
		errorCode = "not_found_mirror_volume_status"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrUnknownMirrorVolumeOperation):
		errorCode = "unknown_mirror_volume_operation"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrUnknownMirrorVolumeState):
		errorCode = "unknown_mirror_volume_state"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeTargetMetadata):
		errorCode = "not_found_mirror_volume_target_metadata"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundMirrorVolumeTargetAgent):
		errorCode = "not_found_mirror_volume_target_agent"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrVolumeExisted):
		errorCode = "volume_existed"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, mirror.ErrNotFoundSourceVolumeReference):
		errorCode = "not_found_source_volume_reference"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// migrator
	case errors.Equal(err, migrator.ErrUnknownJobStateCode):
		errorCode = "unknown_job_state_code"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrUnknownResultCode):
		errorCode = "unknown_job_result_code"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrUnknownJobOperation):
		errorCode = "unknown_job_operation"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundJob):
		errorCode = "not_found_job"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundJobResult):
		errorCode = "not_found_job_result"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundJobStatus):
		errorCode = "not_found_job_status"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundJobLog):
		errorCode = "not_found_job_log"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundTask):
		errorCode = "not_found_task"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundClearTask):
		errorCode = "not_found_clear_task"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundTaskResult):
		errorCode = "not_found_task_result"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotSharedTaskType):
		errorCode = "not_shared_task_type"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundSharedTask):
		errorCode = "not_found_shared_task"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundJobDetail):
		errorCode = "not_found_job_detail"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, migrator.ErrNotFoundKey):
		errorCode = "not_found_key"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// snapshot
	case errors.Equal(err, snapshot.ErrNotFoundMirrorVolume):
		errorCode = "not_found_mirror_volume"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	case errors.Equal(err, snapshot.ErrNotFoundMirrorVolumeTargetMetadata):
		errorCode = "not_found_mirror_volume_target_metadata"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	// cluster
	case errors.Equal(err, cluster.ErrUnavailableStorageExisted):
		errorCode = "unavailable_storage_existed"
		return errors.StatusInternalServerError(ctx, eventCode, errorCode, err)

	default:
		if err := util.CreateError(ctx, eventCode, err); err != nil {
			return err
		}
	}

	return nil
}
