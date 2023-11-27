package handler

import (
	"context"
	"encoding/json"
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	clusterRelationship "github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster_relationship"
	instancetemplate "github.com/datacommand2/cdm-disaster-recovery/manager/internal/instance_template"
	protectionGroup "github.com/datacommand2/cdm-disaster-recovery/manager/internal/protection_group"
	recoveryJob "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_job"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	recoveryReport "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_report"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	microErrors "github.com/micro/go-micro/v2/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"sync"
	"time"
)

const defaultRecoveryJobMonitorInterval = 5

// DisasterRecoveryManagerHandler CDM Disaster Recovery Manager RPC 핸들러
type DisasterRecoveryManagerHandler struct {
	subs []broker.Subscriber

	Operation map[string]*migrator.RecoveryJobOperation
	Status    map[string]*migrator.RecoveryJobStatus
	Result    map[string]*migrator.RecoveryJobResult
	Detail    map[string]*drms.RecoveryJob

	Task           map[string]*migrator.RecoveryJobTask
	ClearTask      map[string]*migrator.RecoveryJobTask
	TaskStatus     map[string]*migrator.RecoveryJobTaskStatus
	TaskResult     map[string]*migrator.RecoveryJobTaskResult
	VolumeStatus   map[string]*migrator.RecoveryJobVolumeStatus
	InstanceStatus map[string]*migrator.RecoveryJobInstanceStatus
}

// CheckDeletableCluster 클러스터 삭제 가능여부 확인
func (h *DisasterRecoveryManagerHandler) CheckDeletableCluster(ctx context.Context, req *drms.CheckDeletableClusterRequest, rsp *drms.CheckDeletableClusterResponse) error {
	logger.Debug("Received DisasterRecoveryManager.CheckDeletableCluster request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-CheckDeletableCluster] Could not check deletable cluster. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.check_deletable_cluster.failure-validate_request", err)
	}

	if err = cluster.CheckDeletableCluster(req); err != nil {
		return createError(ctx, "cdm-dr.manager.check_deletable_cluster.failure-check", err)
	}

	rsp.Deletable = true
	rsp.Message = &drms.Message{Code: "cdm-dr.manager.check_deletable_cluster.success"}
	logger.Debug("Respond DisasterRecoveryManager.CheckDeletableCluster request")
	return errors.StatusOK(ctx, "cdm-dr.manager.check_deletable_cluster.success", nil)
}

// GetUnprotectedInstanceList 클러스터 비보호 인스턴스 목록 조회
func (h *DisasterRecoveryManagerHandler) GetUnprotectedInstanceList(ctx context.Context, req *drms.UnprotectedInstanceListRequest, rsp *drms.UnprotectedInstanceListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetUnprotectedInstanceList request")

	var err error
	defer func() {
		if err != nil && !errors.Equal(err, internal.ErrNotFoundClusterInstance) {
			logger.Errorf("[Handler-GetUnprotectedInstanceList] Could not get unprotected instance list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_unprotected_instance_list.failure-validate_request", err)
	}

	rsp.Instances, rsp.Pagination, err = protectionGroup.GetUnprotectedInstanceList(ctx, req)
	if errors.Equal(err, internal.ErrNotFoundClusterInstance) {
		return createError(ctx, "cdm-dr.manager.get_unprotected_instance_list.success-get", err)
	} else if err != nil {
		return createError(ctx, "cdm-dr.manager.get_unprotected_instance_list.failure-get", err)
	}

	if len(rsp.Instances) == 0 {
		return createError(ctx, "cdm-dr.manager.get_unprotected_instance_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_unprotected_instance_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetUnprotectedInstanceList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_unprotected_instance_list.success", nil)
}

// GetProtectionGroupList 보호 그룹 목록 조회
func (h *DisasterRecoveryManagerHandler) GetProtectionGroupList(ctx context.Context, req *drms.ProtectionGroupListRequest, rsp *drms.ProtectionGroupListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetProtectionGroupList request")

	var err error
	defer func() {
		if err != nil && !errors.Equal(err, internal.ErrIPCNoContent) {
			logger.Errorf("[Handler-GetProtectionGroupList] Could not get protection group list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_list.failure-validate_request", err)
	}

	rsp.Groups, rsp.Pagination, err = protectionGroup.GetList(ctx, req)
	if errors.Equal(err, internal.ErrIPCNoContent) {
		return createError(ctx, "cdm-dr.manager.get_protection_group_list.success-get", err)
	} else if err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_list.failure-get", err)
	}

	if len(rsp.Groups) == 0 {
		return createError(ctx, "cdm-dr.manager.get_protection_group_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_protection_group_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetProtectionGroupList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_protection_group_list.success", nil)
}

// AddProtectionGroup 보호그룹 등록
func (h *DisasterRecoveryManagerHandler) AddProtectionGroup(ctx context.Context, req *drms.AddProtectionGroupRequest, rsp *drms.ProtectionGroupResponse) error {
	logger.Debug("Received DisasterRecoveryManager.AddProtectionGroup request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-AddProtectionGroup] Could not add protection group. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.add_protection_group.failure-validate_request", err)
	}

	if rsp.Group, err = protectionGroup.Add(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.add_protection_group.failure-add", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.add_protection_group.success"}
	logger.Debug("Respond DisasterRecoveryManager.AddProtectionGroup request")
	return errors.StatusOK(ctx, "cdm-dr.manager.add_protection_group.success", nil)
}

// GetProtectionGroup 보호그룹 조회
func (h *DisasterRecoveryManagerHandler) GetProtectionGroup(ctx context.Context, req *drms.ProtectionGroupRequest, rsp *drms.ProtectionGroupResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetProtectionGroup request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetProtectionGroup] Could not get protection group. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group.failure-validate_request", err)
	}

	if rsp.Group, err = protectionGroup.Get(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_protection_group.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetProtectionGroup request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_protection_group.success", nil)
}

// UpdateProtectionGroup 보호그룹 수정
func (h *DisasterRecoveryManagerHandler) UpdateProtectionGroup(ctx context.Context, req *drms.UpdateProtectionGroupRequest, rsp *drms.ProtectionGroupResponse) error {
	logger.Debug("Received DisasterRecoveryManager.UpdateProtectionGroup request")

	var err error
	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.update_protection_group.failure-validate_request", err)
	}

	unlock, err := internal.ProtectionGroupDBLock(req.GroupId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.update_protection_group.failure-protection_group_db_lock", err)
	}

	defer unlock()

	if rsp.Group, err = protectionGroup.Update(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.update_protection_group.failure-update", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.update_protection_group.success"}
	logger.Debug("Respond DisasterRecoveryManager.UpdateProtectionGroup request")
	return errors.StatusOK(ctx, "cdm-dr.manager.update_protection_group.success", nil)
}

// DeleteProtectionGroup 보호그룹 삭제
func (h *DisasterRecoveryManagerHandler) DeleteProtectionGroup(ctx context.Context, req *drms.DeleteProtectionGroupRequest, rsp *drms.DeleteProtectionGroupResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DeleteProtectionGroup request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DeleteProtectionGroup] Could not delete protection group. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_protection_group.failure-validate_request", err)
	}

	unlock, err := internal.ProtectionGroupDBLock(req.GroupId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.delete_protection_group.failure-protection_group_db_lock", err)
	}

	defer unlock()

	if err = protectionGroup.Delete(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_protection_group.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.delete_protection_group.success"}
	logger.Debug("Respond DisasterRecoveryManager.DeleteProtectionGroup request")
	return errors.StatusOK(ctx, "cdm-dr.manager.delete_protection_group.success", nil)
}

// GetProtectionGroupSnapshotList 보호 그룹 스냅샷 목록 조회
func (h *DisasterRecoveryManagerHandler) GetProtectionGroupSnapshotList(ctx context.Context, req *drms.ProtectionGroupSnapshotListRequest, rsp *drms.ProtectionGroupSnapshotListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetProtectionGroupSnapshotList request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetProtectionGroupSnapshotList] Could not get protection group snapshot list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_snapshot_list.failure-validate_request", err)
	}

	if rsp.Snapshots, err = protectionGroup.GetSnapshotList(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_snapshot_list.failure-get", err)
	}

	if len(rsp.Snapshots) == 0 {
		return createError(ctx, "cdm-dr.manager.get_protection_group_snapshot_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_protection_group_snapshot_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetProtectionGroupSnapshotList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_protection_group_snapshot_list.success", nil)
}

// AddProtectionGroupSnapshotQueue 보호 그룹 스냅샷 추가를 대기열에 추가하는 함수
func (h *DisasterRecoveryManagerHandler) AddProtectionGroupSnapshotQueue(ctx context.Context, req *drms.ProtectionGroupSnapshotRequest, rsp *drms.ProtectionGroupSnapshotMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.AddProtectionGroupSnapshotQueue request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-AddProtectionGroupSnapshotQueue] Could not add protection group(%d) snapshot queue. Cause: %+v", req.GetGroupId(), err)
		} else {
			logger.Infof("[Handler-AddProtectionGroupSnapshotQueue] Success to add protection group(%d) snapshot queue.", req.GetGroupId())
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.add_protection_group_snapshot_queue.failure-validate_request", err)
	}

	if err = recoveryPlan.CreateProtectionGroupSnapshot(req.GetGroupId()); err != nil {
		return createError(ctx, "cdm-dr.manager.add_protection_group_snapshot_queue.failure-add", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.add_protection_group_snapshot_queue.success"}
	logger.Debug("Respond DisasterRecoveryManager.AddProtectionGroupSnapshotQueue request")
	return errors.StatusOK(ctx, "cdm-dr.manager.add_protection_group_snapshot_queue.success", nil)
}

// DeleteProtectionGroupSnapshot 보호 그룹 스냅샷 삭제
func (h *DisasterRecoveryManagerHandler) DeleteProtectionGroupSnapshot(ctx context.Context, req *drms.DeleteProtectionGroupSnapshotRequest, rsp *drms.DeleteProtectionGroupSnapshotResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DeleteProtectionGroupSnapshot request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DeleteProtectionGroupSnapshot] Could not delete protection group snapshot. Cause: %+v", err)
		}
	}()

	if err = protectionGroup.DeleteSnapshot(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_protection_group_snapshot.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.delete_protection_group_snapshot.success"}
	logger.Debug("Respond DisasterRecoveryManager.DeleteProtectionGroupSnapshot request")
	return nil
}

// GetRecoveryPlanList 재해복구계획 목록 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryPlanList(ctx context.Context, req *drms.RecoveryPlanListRequest, rsp *drms.RecoveryPlanListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryPlanList request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryPlanList] Could not get disaster recovery plan list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_plan_list.failure-validate_request", err)
	}

	if rsp.Plans, rsp.Pagination, err = recoveryPlan.GetList(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_plan_list.failure-get", err)
	}

	if len(rsp.Plans) == 0 {
		return createError(ctx, "cdm-dr.manager.get_recovery_plan_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_plan_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryPlanList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_plan_list.success", nil)
}

// AddRecoveryPlan 재해복구계획 등록
func (h *DisasterRecoveryManagerHandler) AddRecoveryPlan(ctx context.Context, req *drms.AddRecoveryPlanRequest, rsp *drms.RecoveryPlanResponse) error {
	logger.Debug("Received DisasterRecoveryManager.AddRecoveryPlan request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-AddRecoveryPlan] Could not add disaster recovery plan. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.add_recovery_plan.failure-validate_request", err)
	}

	if rsp.Plan, err = recoveryPlan.Add(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.add_recovery_plan.failure-add", err)
	}

	if (len(rsp.Plan.AbnormalStateReasons.Warning) > 0 ||
		len(rsp.Plan.AbnormalStateReasons.Critical) > 0 ||
		len(rsp.Plan.AbnormalStateReasons.Emergency) > 0) && !req.Force {
		abnormalRsp := struct {
			Reasons *drms.RecoveryPlanAbnormalStateReason `json:"reasons,omitempty"`
			Message *drms.Message                         `json:"message,omitempty"`
		}{}

		abnormalRsp.Reasons = rsp.Plan.AbnormalStateReasons
		abnormalRsp.Message = &drms.Message{Code: "cdm-dr.manager.add_recovery_plan.failure-add-abnormal_state"}

		bytes, _ := json.Marshal(&abnormalRsp)

		logger.Error("[Handler-AddRecoveryPlan] Could not add recovery plan because abnormal state")

		reportEvent(ctx, "cdm-dr.manager.add_recovery_plan.failure-add", "abnormal_state", &abnormalRsp)
		return microErrors.New("cdm", string(bytes), http.StatusPreconditionFailed)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.add_recovery_plan.success"}
	logger.Debug("Respond DisasterRecoveryManager.AddRecoveryPlan request")
	return errors.StatusOK(ctx, "cdm-dr.manager.add_recovery_plan.success", nil)
}

// GetRecoveryPlan 재해복구계획 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryPlan(ctx context.Context, req *drms.RecoveryPlanRequest, rsp *drms.RecoveryPlanResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryPlan request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryPlan] Could not get disaster recovery plan. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_plan.failure-validate_request", err)
	}

	if rsp.Plan, err = recoveryPlan.Get(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_plan.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_plan.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryPlan request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_plan.success", nil)
}

// UpdateRecoveryPlan 재해복구계획 수정
func (h *DisasterRecoveryManagerHandler) UpdateRecoveryPlan(ctx context.Context, req *drms.UpdateRecoveryPlanRequest, rsp *drms.RecoveryPlanResponse) error {
	logger.Debug("Received DisasterRecoveryManager.UpdateRecoveryPlan request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-UpdateRecoveryPlan] Could not update disaster recovery plan. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.update_recovery_plan.failure-validate_request", err)
	}

	unlock, err := internal.RecoveryPlanDBLock(req.PlanId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.update_recovery_plan.failure-recovery_plan_db_lock", err)
	}

	defer unlock()

	if rsp.Plan, err = recoveryPlan.Update(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.update_recovery_plan.failure-update", err)
	}

	if (len(rsp.Plan.AbnormalStateReasons.Warning) > 0 ||
		len(rsp.Plan.AbnormalStateReasons.Critical) > 0 ||
		len(rsp.Plan.AbnormalStateReasons.Emergency) > 0) && !req.Force {
		abnormalRsp := struct {
			Reasons *drms.RecoveryPlanAbnormalStateReason `json:"reasons,omitempty"`
			Message *drms.Message                         `json:"message,omitempty"`
		}{}

		abnormalRsp.Reasons = rsp.Plan.AbnormalStateReasons
		abnormalRsp.Message = &drms.Message{Code: "cdm-dr.manager.update_recovery_plan.success-update-abnormal_state"}

		logger.Warnf("[Handler-UpdateRecoveryPlan] Could not create job because recovery plan is abnormal state")

		reportEvent(ctx, "cdm-dr.manager.update_recovery_plan.success-update", "abnormal_state", &abnormalRsp)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.update_recovery_plan.success"}
	logger.Debug("Respond DisasterRecoveryManager.UpdateRecoveryPlan request")
	return errors.StatusOK(ctx, "cdm-dr.manager.update_recovery_plan.success", nil)
}

// DeleteRecoveryPlan 재해복구계획 삭제
func (h *DisasterRecoveryManagerHandler) DeleteRecoveryPlan(ctx context.Context, req *drms.RecoveryPlanRequest, rsp *drms.DeleteRecoveryPlanResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DeleteRecoveryPlan request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DeleteRecoveryPlan] Could not delete disaster recovery plan. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_plan.failure-validate_request", err)
	}

	unlock, err := internal.RecoveryPlanDBLock(req.PlanId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_plan.failure-recovery_plan_db_lock", err)
	}

	defer unlock()

	if err = recoveryPlan.Delete(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_plan.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.delete_recovery_plan.success"}
	logger.Debug("Respond DisasterRecoveryManager.DeleteRecoveryPlan request")
	return errors.StatusOK(ctx, "cdm-dr.manager.delete_recovery_plan.success", nil)
}

// GetRecoveryJobList 재해복구작업 목록 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryJobList(ctx context.Context, req *drms.RecoveryJobListRequest, rsp *drms.RecoveryJobListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryJobList request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryJobList] Could not get disaster recovery job list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_job_list.failure-validate_request", err)
	}

	if rsp.Jobs, rsp.Pagination, err = recoveryJob.GetList(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_job_list.failure-get", err)
	}

	if len(rsp.Jobs) == 0 {
		return createError(ctx, "cdm-dr.manager.get_recovery_job_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_job_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryJobList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_job_list.success", nil)
}

// AddRecoveryJob 재해복구작업 생성
func (h *DisasterRecoveryManagerHandler) AddRecoveryJob(ctx context.Context, req *drms.AddRecoveryJobRequest, rsp *drms.RecoveryJobResponse) error {
	logger.Debug("Received DisasterRecoveryManager.AddRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-AddRecoveryJob] Could not create disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.add_recovery_job.failure-validate_request", err)
	}

	rsp.Job, err = recoveryJob.Add(ctx, req)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.add_recovery_job.failure-add", err)
	}

	if req.Job.Schedule == nil {
		if err = internal.PublishMessage(constant.QueueTriggerRecoveryJob, &internal.ScheduleMessage{
			ProtectionGroupID: rsp.Job.Group.Id,
			RecoveryJobID:     rsp.Job.Id,
		}); err != nil {
			return createError(ctx, "cdm-dr.manager.add_recovery_job.failure-add", err)
		}
		rsp.Job.StateCode = constant.RecoveryJobStateCodePending
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.add_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.AddRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.add_recovery_job.success", nil)
}

// GetRecoveryJob 재해복구작업 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryJob] Could not get disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_job.failure-validate_request", err)
	}

	if rsp.Job, err = recoveryJob.Get(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_job.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_job.success", nil)
}

// UpdateRecoveryJob 재해복구작업 수정
func (h *DisasterRecoveryManagerHandler) UpdateRecoveryJob(ctx context.Context, req *drms.UpdateRecoveryJobRequest, rsp *drms.RecoveryJobResponse) error {
	logger.Debug("Received DisasterRecoveryManager.UpdateRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-UpdateRecoveryJob] Could not update disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.update_recovery_job.failure-validate_request", err)
	}

	unlock, err := internal.RecoveryJobDBLock(req.JobId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.update_recovery_job.failure-recovery_job_db_lock", err)
	}

	defer unlock()

	if rsp.Job, err = recoveryJob.Update(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.update_recovery_job.failure-update", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.update_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.UpdateRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.update_recovery_job.success", nil)
}

// DeleteRecoveryJob 재해복구작업 삭제
func (h *DisasterRecoveryManagerHandler) DeleteRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.DeleteRecoveryJobResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DeleteRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DeleteRecoveryJob] Could not delete disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_job.failure-validate_request", err)
	}

	unlock, err := internal.RecoveryJobDBLock(req.JobId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_job.failure-recovery_job_db_lock", err)
	}

	defer unlock()

	if err = recoveryJob.Delete(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_job.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.delete_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.DeleteRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.delete_recovery_job.success", nil)
}

// ForceDeleteRecoveryJob 재해복구작업 강제 삭제
func (h *DisasterRecoveryManagerHandler) ForceDeleteRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.DeleteRecoveryJobResponse) error {
	logger.Debug("Received DisasterRecoveryManager.ForceDeleteRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-ForceDeleteRecoveryJob] Could not delete disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_job.failure-validate_request", err)
	}

	unlock, err := internal.RecoveryJobDBLock(req.JobId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_job.failure-recovery_job_db_lock", err)
	}

	defer unlock()

	if err = recoveryJob.Delete(ctx, req, true); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_job.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.delete_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.ForceDeleteRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.delete_recovery_job.success", nil)
}

// MonitorRecoveryJobStatus 재해복구작업 모니터링 재해복구작업을 모니터링하는데 기존 볼륨, 인스턴스, 작업내역, 상태를 따로 API 를 열었다면
// 이 함수는 모두 모아서 한번에 출력한다. 또한, etcd 에서 불러오는게 아니라 메모리에 저장된 데이터를 불러오는 방식으로 한다.
func (h *DisasterRecoveryManagerHandler) MonitorRecoveryJobStatus(ctx context.Context, req *drms.MonitorRecoveryJobRequest, stream drms.DisasterRecoveryManager_MonitorRecoveryJobStatusStream) error {
	logger.Debug("Received DisasterRecoveryManager.MonitorRecoveryJobStatus request")

	var err error
	var wg sync.WaitGroup
	var rsp = new(drms.MonitorRecoveryJobStatusResponse)

	defer func() {
		if err != nil {
			logger.Errorf("[Handler-MonitorRecoveryJobStatus] Could not monitor disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_status.failure-validate_request", err)
	}

	if err = recoveryJob.ValidateMonitorRecoveryJob(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_status.failure-validate_monitor_recovery_job", err)
	}

	interval := req.Interval
	if interval == 0 {
		interval = defaultRecoveryJobMonitorInterval
	}

	for {
		if rsp.Status = h.getMemoryRecoveryJobStatus(req.JobId); rsp.Status == nil {
			// 서비스가 재시작되면 메모리 데이터가 없기때문에 etcd 에서 불러오는걸로 대체한다.
			rsp.Status, _ = recoveryJob.Monitor(req)
		}

		if rsp.Status.StateCode == constant.RecoveryJobStateCodeClearing ||
			rsp.Status.StateCode == constant.RecoveryJobStateCodeClearFailed ||
			rsp.Status.StateCode == constant.RecoveryJobStateCodeReporting ||
			rsp.Status.StateCode == constant.RecoveryJobStateCodeFinished {

			// Rollback 이 시작되면 이 로직을 탄다.
			wg.Add(1)
			go func() {
				rsp.Tenants, rsp.SecurityGroups, rsp.Networks, rsp.Subnets, rsp.FloatingIp,
					rsp.Routers, rsp.Volumes, rsp.Keypair, rsp.InstanceSpecs, rsp.Instances = h.getMemoryClearTask(req.JobId)
				wg.Done()
			}()

		} else {
			// 복구 작업이 시작되면 이 로직을 탄다.
			wg.Add(3)
			go func() {
				rsp.Tenants, rsp.SecurityGroups, rsp.Networks, rsp.Subnets, rsp.FloatingIp,
					rsp.Routers, rsp.Keypair, rsp.InstanceSpecs = h.getMemoryTask(req.JobId)
				wg.Done()
			}()

			go func() {
				rsp.Instances = h.getMemoryInstancesStatus(req.JobId)
				wg.Done()
			}()

			go func() {
				rsp.Volumes = h.getMemoryVolumesStatus(req.JobId)
				wg.Done()
			}()
		}

		wg.Wait()

		if err = stream.Send(rsp); err != nil {
			if status.Code(err) == codes.Unavailable {
				logger.Infof("[MonitorRecoveryJobStatus] The streams were disconnected normally.")
				err = nil
				break
			}
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	if err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_status.failure-monitor", err)
	}

	logger.Debug("Respond DisasterRecoveryManager.MonitorRecoveryJobStatus request")
	return errors.StatusOK(ctx, "cdm-dr.manager.monitor_recovery_job_status.success", nil)
}

// MonitorRecoveryJob 재해복구작업 모니터링
func (h *DisasterRecoveryManagerHandler) MonitorRecoveryJob(ctx context.Context, req *drms.MonitorRecoveryJobRequest, stream drms.DisasterRecoveryManager_MonitorRecoveryJobStream) error {
	logger.Debug("Received DisasterRecoveryManager.MonitorRecoveryJob request")

	var err error
	var rsp = new(drms.MonitorRecoveryJobResponse)

	defer func() {
		if err != nil {
			logger.Errorf("[Handler-MonitorRecoveryJob] Could not monitor disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.ValidateMonitorRecoveryJob(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job.failure-validate_monitor_recovery_job", err)
	}

	interval := req.Interval
	if interval == 0 {
		interval = defaultRecoveryJobMonitorInterval
	}

	for {
		rsp.Status, err = recoveryJob.Monitor(req)
		if err != nil {
			break
		}

		if err = stream.Send(rsp); err != nil {
			if status.Code(err) == codes.Unavailable {
				logger.Infof("[MonitorRecoveryJob] Job monitor stream cancelled")
				err = nil
				break
			}
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	if err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job.failure-monitor", err)
	}

	logger.Debug("Respond DisasterRecoveryManager.MonitorRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.monitor_recovery_job.success", nil)
}

// MonitorRecoveryTaskLogs 재해복구작업 작업내역 모니터링
func (h *DisasterRecoveryManagerHandler) MonitorRecoveryTaskLogs(ctx context.Context, req *drms.MonitorRecoveryJobRequest, stream drms.DisasterRecoveryManager_MonitorRecoveryTaskLogsStream) error {
	logger.Debug("Received DisasterRecoveryManager.MonitorRecoveryTaskLogs request")

	var err error
	var rsp = new(drms.MonitorRecoveryTaskLogsResponse)
	var lastSeq uint64

	defer func() {
		if err != nil {
			logger.Errorf("[Handler-MonitorRecoveryTaskLogs] Could not monitor disaster recovery task logs. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_task_logs.failure-validate_request", err)
	}

	if err = recoveryJob.ValidateMonitorRecoveryJob(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_task_logs.failure-validate_monitor_recovery_job", err)
	}

	interval := req.Interval
	if interval == 0 {
		interval = defaultRecoveryJobMonitorInterval
	}

	for {
		rsp.TaskLogs, lastSeq, err = recoveryJob.MonitorTaskLogs(req, lastSeq)
		if err != nil {
			break
		}

		if err = stream.Send(rsp); err != nil {
			if status.Code(err) == codes.Unavailable {
				logger.Infof("[MonitorRecoveryTaskLogs] Task logs monitor stream cancelled")
				err = nil
				break
			}
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	if err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_task_logs.failure-monitor", err)
	}

	logger.Debug("Respond DisasterRecoveryManager.MonitorRecoveryTaskLogs request")
	return errors.StatusOK(ctx, "cdm-dr.manager.monitor_recovery_task_logs.success", nil)
}

// MonitorRecoveryJobInstanceList 재해복구작업 인스턴스 모니터링
func (h *DisasterRecoveryManagerHandler) MonitorRecoveryJobInstanceList(ctx context.Context, req *drms.MonitorRecoveryJobRequest, stream drms.DisasterRecoveryManager_MonitorRecoveryJobInstanceListStream) error {
	logger.Debug("Received DisasterRecoveryManager.MonitorRecoveryJobInstanceList request")

	var err error
	var rsp = new(drms.MonitorRecoveryJobInstanceListResponse)
	prevInstanceStatusList := make(map[uint64]string)

	defer func() {
		if err != nil {
			logger.Errorf("[Handler-MonitorRecoveryJobInstanceList] Could not monitor disaster recovery job instance list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_instance_list.failure-validate_request", err)
	}

	if err = recoveryJob.ValidateMonitorRecoveryJob(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_instance_list.failure-validate_monitor_recovery_job", err)
	}

	interval := req.Interval
	if interval == 0 {
		interval = defaultRecoveryJobMonitorInterval
	}

	for {
		rsp.Instances, err = recoveryJob.MonitorInstances(req, prevInstanceStatusList)
		if err != nil {
			break
		}

		if err = stream.Send(rsp); err != nil {
			if status.Code(err) == codes.Unavailable {
				logger.Infof("[MonitorRecoveryJobInstanceList] Instance monitor stream cancelled")
				err = nil
				break
			}
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	if err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_instance_list.failure-monitor", err)
	}

	logger.Debug("Respond DisasterRecoveryManager.MonitorRecoveryJobInstanceList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.monitor_recovery_job_instance_list.success", nil)
}

// MonitorRecoveryJobVolumeList 재해복구작업 볼륨 모니터링
func (h *DisasterRecoveryManagerHandler) MonitorRecoveryJobVolumeList(ctx context.Context, req *drms.MonitorRecoveryJobRequest, stream drms.DisasterRecoveryManager_MonitorRecoveryJobVolumeListStream) error {
	logger.Debug("Received DisasterRecoveryManager.MonitorRecoveryJobVolumeList request")

	var err error
	var rsp = new(drms.MonitorRecoveryJobVolumeListResponse)
	prevVolumeStatusList := make(map[uint64]string)

	defer func() {
		if err != nil {
			logger.Errorf("[Handler-MonitorRecoveryJobVolumeList] Could not monitor disaster recovery job volume list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_volume_list.failure-validate_request", err)
	}

	if err = recoveryJob.ValidateMonitorRecoveryJob(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_volume_list.failure-validate_monitor", err)
	}

	interval := req.Interval
	if interval == 0 {
		interval = defaultRecoveryJobMonitorInterval
	}

	for {
		rsp.Volumes, err = recoveryJob.MonitorVolumes(req, prevVolumeStatusList)
		if err != nil {
			break
		}

		if err = stream.Send(rsp); err != nil {
			if status.Code(err) == codes.Unavailable {
				logger.Infof("[MonitorRecoveryJobVolumeList] Volume monitor stream cancelled")
				err = nil
				break
			}
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	if err != nil {
		return createError(ctx, "cdm-dr.manager.monitor_recovery_job_volume_list.failure-monitor", err)
	}

	logger.Debug("Respond DisasterRecoveryManager.MonitorRecoveryJobVolumeList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.monitor_recovery_job_volume_list.success", nil)
}

// PauseRecoveryJob 재해복구작업 일시중지
func (h *DisasterRecoveryManagerHandler) PauseRecoveryJob(ctx context.Context, req *drms.PauseRecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.PauseRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-PauseRecoveryJob] Could not pause disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.pause_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.Pause(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.pause_recovery_job.failure-pause", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.pause_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.PauseRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.pause_recovery_job.success", nil)
}

// ExtendRecoveryJobPauseTime 재해복구작업 일시중지 시간 연장
func (h *DisasterRecoveryManagerHandler) ExtendRecoveryJobPauseTime(ctx context.Context, req *drms.ExtendRecoveryJobPausingTimeRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.ExtendRecoveryJobPauseTime request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-ExtendRecoveryJobPauseTime] Could not extend recovery job pause time. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.extend_recovery_job_pause_time.failure-validate_request", err)
	}

	if err = recoveryJob.ExtendPausingTime(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.extend_recovery_job_pause_time.failure-extend", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.extend_recovery_job_pause_time.success"}
	logger.Debug("Respond DisasterRecoveryManager.ExtendRecoveryJobPauseTime request")
	return errors.StatusOK(ctx, "cdm-dr.manager.extend_recovery_job_pause_time.success", nil)
}

// ResumeRecoveryJob 재해복구작업 재개
func (h *DisasterRecoveryManagerHandler) ResumeRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.ResumeRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-ResumeRecoveryJob] Could not resume recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.resume_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.Resume(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.resume_recovery_job.failure-resume", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.resume_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.ResumeRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.resume_recovery_job.success", nil)
}

// CancelRecoveryJob 재해복구작업 취소
func (h *DisasterRecoveryManagerHandler) CancelRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.CancelRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-CancelRecoveryJob] Could not cancel disaster recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.cancel_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.Cancel(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.cancel_recovery_job.failure-cancel", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.cancel_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.CancelRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.cancel_recovery_job.success", nil)
}

// DoSimulationJobRollback 재해복구작업 롤백: 모의훈련
func (h *DisasterRecoveryManagerHandler) DoSimulationJobRollback(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DoSimulationJobRollback request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DoSimulationJobRollback] Could not rollback recovery job(simulation). Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.do_simulation_job_rollback.failure-validate_request", err)
	}

	if err = recoveryJob.Rollback(ctx, req, constant.RecoveryTypeCodeSimulation); err != nil {
		return createError(ctx, "cdm-dr.manager.do_simulation_job_rollback.failure-rollback", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.do_simulation_job_rollback.success"}
	logger.Debug("Respond DisasterRecoveryManager.DoSimulationJobRollback request")
	return errors.StatusOK(ctx, "cdm-dr.manager.do_simulation_job_rollback.success", nil)
}

// DoMigrationJobRollback 재해복구작업 롤백: 재해복구
func (h *DisasterRecoveryManagerHandler) DoMigrationJobRollback(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DoMigrationJobRollback request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DoMigrationJobRollback] Could not rollback recovery job(migration). Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.do_migration_job_rollback.failure-validate_request", err)
	}

	if err = recoveryJob.Rollback(ctx, req, constant.RecoveryTypeCodeMigration); err != nil {
		return createError(ctx, "cdm-dr.manager.do_migration_job_rollback.failure-rollback", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.do_migration_job_rollback.success"}
	logger.Debug("Respond DisasterRecoveryManager.DoMigrationJobRollback request")
	return errors.StatusOK(ctx, "cdm-dr.manager.do_migration_job_rollback.success", nil)
}

// RetryRecoveryJobRollback 재해복구작업 롤백 재시도
func (h *DisasterRecoveryManagerHandler) RetryRecoveryJobRollback(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.RetryRecoveryJobRollback request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-RetryRecoveryJobRollback] Could not retry rollback recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.retry_recovery_job_rollback.failure-validate_request", err)
	}

	if err = recoveryJob.RetryRollback(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.retry_recovery_job_rollback.failure-retry", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.retry_recovery_job_rollback.success"}
	logger.Debug("Respond DisasterRecoveryManager.RetryRecoveryJobRollback request")
	return errors.StatusOK(ctx, "cdm-dr.manager.retry_recovery_job_rollback.success", nil)
}

// ExtendRecoveryJobRollbackTime 재해복구작업 롤백 대기시간 연장
func (h *DisasterRecoveryManagerHandler) ExtendRecoveryJobRollbackTime(ctx context.Context, req *drms.ExtendRecoveryJobRollbackTimeRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.ExtendRecoveryJobRollbackTime request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-ExtendRecoveryJobRollbackTime] Could not extend recovery job rollback time. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.extend_recovery_job_rollback_time.failure-validate_request", err)
	}

	if err = recoveryJob.ExtendRollbackTime(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.extend_recovery_job_rollback_time.failure-extend", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.extend_recovery_job_rollback_time.success"}
	logger.Debug("Respond DisasterRecoveryManager.ExtendRecoveryJobRollbackTime request")
	return errors.StatusOK(ctx, "cdm-dr.manager.extend_recovery_job_rollback_time.success", err)
}

// IgnoreRecoveryJobRollback 재해복구작업 롤백 무시
func (h *DisasterRecoveryManagerHandler) IgnoreRecoveryJobRollback(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.IgnoreRecoveryJobRollback request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-IgnoreRecoveryJobRollback] Could not ignore recovery job rollback. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.ignore_recovery_job_rollback.failure-validate_request", err)
	}

	if err = recoveryJob.IgnoreRollback(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.ignore_recovery_job_rollback.failure-ignore", err)
	}

	logger.Debug("Respond DisasterRecoveryManager.IgnoreRecoveryJobRollback request")
	rsp.Message = &drms.Message{Code: "cdm-dr.manager.ignore_recovery_job_rollback.success"}
	return errors.StatusOK(ctx, "cdm-dr.manager.ignore_recovery_job_rollback.success", nil)
}

// ConfirmRecoveryJob 재해복구작업 확정
func (h *DisasterRecoveryManagerHandler) ConfirmRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.ConfirmRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-ConfirmRecoveryJob] Could not confirm recovery job(migration). Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.confirm_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.Confirm(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.confirm_recovery_job.failure-confirm", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.confirm_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.ConfirmRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.confirm_recovery_job.success", nil)
}

// RetryConfirmRecoveryJob 재해복구작업 확정 재시도
func (h *DisasterRecoveryManagerHandler) RetryConfirmRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.RetryConfirmRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-RetryConfirmRecoveryJob] Could not retry confirm recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.retry_confirm_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.RetryConfirm(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.retry_confirm_recovery_job.failure-retry", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.retry_confirm_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.RetryConfirmRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.retry_confirm_recovery_job.success", nil)
}

// CancelConfirmRecoveryJob 재해복구작업 확정 취소
func (h *DisasterRecoveryManagerHandler) CancelConfirmRecoveryJob(ctx context.Context, req *drms.RecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.CancelConfirmRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-CancelConfirmRecoveryJob] Could not cancel confirm recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.cancel_confirm_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.CancelConfirm(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.cancel_confirm_recovery_job.failure-cancel", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.cancel_confirm_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.CancelConfirmRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.cancel_confirm_recovery_job.success", nil)
}

// RetryRecoveryJob 재해복구작업을 재시도하는 함수
func (h *DisasterRecoveryManagerHandler) RetryRecoveryJob(ctx context.Context, req *drms.RetryRecoveryJobRequest, rsp *drms.RecoveryJobMessageResponse) error {
	logger.Debug("Received DisasterRecoveryManager.RetryRecoveryJob request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-RetryRecoveryJob] Could not retry recovery job. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.retry_recovery_job.failure-validate_request", err)
	}

	if err = recoveryJob.Retry(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.retry_recovery_job.failure-retry", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.retry_recovery_job.success"}
	logger.Debug("Respond DisasterRecoveryManager.RetryRecoveryJob request")
	return errors.StatusOK(ctx, "cdm-dr.manager.retry_recovery_job.success", nil)
}

// GetProtectionGroupHistory 보호 그룹 history 조회
func (h *DisasterRecoveryManagerHandler) GetProtectionGroupHistory(ctx context.Context, _ *drms.Empty, rsp *drms.ProtectionGroupHistoryResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetProtectionGroupHistory request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetProtectionGroupHistory] Could not get protection group history. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_history.failure-validate_request", err)
	}

	if rsp.History, err = recoveryReport.GetProtectionGroupHistory(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_history.failure-get", err)
	}

	if len(rsp.History.Clusters) == 0 {
		return createError(ctx, "cdm-dr.manager.get_protection_group_history.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_protection_group_history.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetProtectionGroupHistory request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_protection_group_history.success", nil)
}

// GetRecoveryReportList 재해복구결과 보고서 목록 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryReportList(ctx context.Context, req *drms.RecoveryReportListRequest, rsp *drms.RecoveryReportListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryReportList request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryReportList] Could not get recovery report list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_report_list.failure-validate_request", err)
	}

	if rsp.Reports, rsp.Pagination, err = recoveryReport.GetList(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_report_list.failure-get", err)
	}

	if len(rsp.Reports) == 0 {
		return createError(ctx, "cdm-dr.manager.get_recovery_report_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_report_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryReportList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_report_list.success", nil)
}

// GetRecoveryReport 재해복구결과 보고서 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryReport(ctx context.Context, req *drms.RecoveryReportRequest, rsp *drms.RecoveryReportResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryReport request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryReport] Could not get recovery report. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_report.failure-validate_request", err)
	}

	if rsp.Report, err = recoveryReport.Get(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_report.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_report.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryReport request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_report.success", nil)
}

// DeleteRecoveryReport 재해복구결과 보고서 삭제 : 모의훈련
func (h *DisasterRecoveryManagerHandler) DeleteRecoveryReport(ctx context.Context, req *drms.DeleteRecoveryReportRequest, rsp *drms.DeleteRecoveryReportResponse) error {
	logger.Debug("Received DisasterRecoveryManager.DeleteRecoveryReport request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-DeleteRecoveryReport] Could not delete recovery report. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_report.failure-validate_request", err)
	}

	unlock, err := internal.RecoveryReportDBLock(req.ResultId)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_report.failure-recovery_report_db_lock", err)
	}

	defer unlock()

	if err = recoveryReport.Delete(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_recovery_report.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_report.success"}
	logger.Debug("Respond DisasterRecoveryManager.DeleteRecoveryReport request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_report.success", nil)
}

// GetInstanceTemplateList 인스턴스 템플릿 목록을 조회한다.
func (h *DisasterRecoveryManagerHandler) GetInstanceTemplateList(ctx context.Context, req *drms.InstanceTemplateListRequest, rsp *drms.InstanceTemplateListResponse) error {
	logger.Infof("Received DisasterRecoveryManager.GetInstanceTemplateList request")
	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetInstanceTemplateList] Could not get instance template list. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_instance_template.failure-validate-request", err)
	}

	if rsp.Templates, rsp.Pagination, err = instancetemplate.GetList(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_instance_template_list.failure-list", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_instance_template_list.success"}
	return errors.StatusOK(ctx, "cdm-dr.manager.get_instance_template_list.success", nil)
}

// GetInstanceTemplate 인스턴스 템플릿을 조회한다.
func (h *DisasterRecoveryManagerHandler) GetInstanceTemplate(ctx context.Context, req *drms.InstanceTemplateRequest, rsp *drms.InstanceTemplateResponse) error {
	logger.Infof("Received DisasterRecoveryManager.GetInstanceTemplate request")
	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetInstanceTemplate] Could not get instance template. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_instance_template.failure-validate-request", err)
	}

	if rsp.Template, err = instancetemplate.Get(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_instance_template.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_instance_template.success"}
	logger.Debug("Response DisasterRecovery.GetInstanceTemplate request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_instance_template.success", nil)
}

// AddInstanceTemplate 인스턴스 템플릿을 추가한다.
func (h *DisasterRecoveryManagerHandler) AddInstanceTemplate(ctx context.Context, req *drms.AddInstanceTemplateRequest, rsp *drms.InstanceTemplateResponse) error {
	logger.Infof("Received DisasterRecoveryManager.AddInstanceTemplate request")
	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-AddInstanceTemplate] Could not add instance template. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.add_instance_template.failure-add", err)
	}

	if rsp.Template, err = instancetemplate.Add(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.add_instance_template.failure-add", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.add_instance_template.success"}
	return errors.StatusOK(ctx, "cdm-dr.manager.add_instance_template.success", nil)
}

// DeleteInstanceTemplate 인스턴스 템플릿을 삭제한다.
func (h *DisasterRecoveryManagerHandler) DeleteInstanceTemplate(ctx context.Context, req *drms.DeleteInstanceTemplateRequest, rsp *drms.DeleteInstanceTemplateResponse) error {
	logger.Infof("Received DisasterRecoveryManager.DeleteInstanceTemplate request")
	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetInstanceTemplate] Could not delete instance template. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_instance_template.failure-delete", err)
	}

	if err = instancetemplate.Delete(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.delete_instance_template.failure-delete", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.delete_instance_template.success"}
	return errors.StatusOK(ctx, "cdm-dr.manager.delete_instance_template.success", nil)
}

// Close handler
func (h *DisasterRecoveryManagerHandler) Close() {
	// broker unsubscribe
	for _, sub := range h.subs {
		if err := sub.Unsubscribe(); err != nil {
			logger.Warnf("[Handler-Close] Could not unsubscribe queue (%s). Cause: %+v", sub.Topic(), err)
		}
	}
}

// NewDisasterRecoveryManagerHandler RecoveryJob Monitor 생성 및 Disaster Recovery Manager 서비스 RPC 핸들러 생성
func NewDisasterRecoveryManagerHandler() (*DisasterRecoveryManagerHandler, error) {
	h := DisasterRecoveryManagerHandler{
		Operation: make(map[string]*migrator.RecoveryJobOperation),
		Status:    make(map[string]*migrator.RecoveryJobStatus),
		Result:    make(map[string]*migrator.RecoveryJobResult),
		Detail:    make(map[string]*drms.RecoveryJob),

		Task:           make(map[string]*migrator.RecoveryJobTask),
		ClearTask:      make(map[string]*migrator.RecoveryJobTask),
		TaskStatus:     make(map[string]*migrator.RecoveryJobTaskStatus),
		TaskResult:     make(map[string]*migrator.RecoveryJobTaskResult),
		VolumeStatus:   make(map[string]*migrator.RecoveryJobVolumeStatus),
		InstanceStatus: make(map[string]*migrator.RecoveryJobInstanceStatus),
	}

	// Temp Queue
	for topic, handlerFunc := range map[string]broker.Handler{
		constant.QueueTriggerRecoveryJobDeleted:    h.DeleteRecoveryJobQueue,
		constant.QueueRecoveryJobMonitor:           h.RecoveryJobStatusMonitor,
		constant.QueueRecoveryJobTaskMonitor:       h.RecoveryJobTaskMonitor,
		constant.QueueRecoveryJobClearTaskMonitor:  h.RecoveryJobClearTaskMonitor,
		constant.QueueRecoveryJobTaskStatusMonitor: h.RecoveryJobTaskStatusMonitor,
		constant.QueueRecoveryJobTaskResultMonitor: h.RecoveryJobTaskResultMonitor,
		constant.QueueRecoveryJobVolumeMonitor:     h.RecoveryJobVolumeMonitor,
		constant.QueueRecoveryJobInstanceMonitor:   h.RecoveryJobInstanceMonitor,
	} {
		logger.Debugf("Subscribe cluster event (%s).", topic)

		sub, err := broker.SubscribeTempQueue(topic, handlerFunc)
		if err != nil {
			logger.Errorf("[NewDisasterRecoveryManagerHandler] Could not start broker. Cause: %+v", errors.UnusableBroker(err))
			return nil, errors.UnusableBroker(err)
		}

		h.subs = append(h.subs, sub)
	}

	// Persistent Queue
	for topic, handlerFunc := range map[string]broker.Handler{
		constant.QueueTriggerRecoveryJob:            h.AddRecoveryJobQueue,
		constant.QueueTriggerWaitingRecoveryJob:     h.WaitRecoveryJobQueue,
		constant.QueueTriggerDoneRecoveryJob:        h.DoneRecoveryJobQueue,
		constant.QueueTriggerCancelRecoveryJob:      h.CancelRecoveryJobQueue,
		constant.QueueTriggerClearFailedRecoveryJob: h.ClearFailedRecoveryJobQueue,
		constant.QueueTriggerReportingRecoveryJob:   h.ReportRecoveryJobQueue,
	} {
		logger.Debugf("Subscribe cluster event (%s).", topic)

		sub, err := broker.SubscribePersistentQueue(topic, handlerFunc, true)
		if err != nil {
			logger.Errorf("[NewDisasterRecoveryManagerHandler] Could not start broker. Cause: %+v", errors.UnusableBroker(err))
			return nil, errors.UnusableBroker(err)
		}

		h.subs = append(h.subs, sub)
	}

	return &h, nil
}

// GetClusterSummary 클러스터 요약 정보 조회
func (h *DisasterRecoveryManagerHandler) GetClusterSummary(ctx context.Context, _ *drms.Empty, rsp *drms.ClusterSummaryResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetClusterSummary request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetClusterSummary] Could not get cluster summary. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_cluster_summary.failure-validate_request", err)
	}

	rsp.Summary, err = cluster.GetClusterSummary(ctx)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.get_cluster_summary.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_cluster_summary.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetClusterSummary request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_cluster_summary.success", nil)
}

// GetProtectionGroupSummary 보호그룹 요약 정보 조회
func (h *DisasterRecoveryManagerHandler) GetProtectionGroupSummary(ctx context.Context, _ *drms.Empty, rsp *drms.ProtectionGroupSummaryResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetProtectionGroupSummary request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetProtectionGroupSummary] Could not get protection group summary. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_summary.failure-validate_request", err)
	}

	rsp.Summary, err = protectionGroup.GetProtectionGroupSummary(ctx)
	if errors.Equal(err, internal.ErrIPCNoContent) {
		return createError(ctx, "cdm-dr.manager.get_protection_group_summary.success-get", err)
	} else if err != nil {
		return createError(ctx, "cdm-dr.manager.get_protection_group_summary.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_protection_group_summary.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetProtectionGroupSummary request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_protection_group_summary.success", nil)
}

// GetInstanceSummary 인스턴스 요약 정보 조회
func (h *DisasterRecoveryManagerHandler) GetInstanceSummary(ctx context.Context, _ *drms.Empty, rsp *drms.InstanceSummaryResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetInstanceSummary request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetInstanceSummary] Could not get instance summary. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_instance_summary.failure-validate_request", err)
	}

	rsp.Summary, err = protectionGroup.GetInstanceSummary(ctx)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.get_instance_summary.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_instance_summary.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetInstanceSummary request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_instance_summary.success", nil)
}

// GetVolumeSummary 볼륨 요약 정보 조회
func (h *DisasterRecoveryManagerHandler) GetVolumeSummary(ctx context.Context, _ *drms.Empty, rsp *drms.VolumeSummaryResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetVolumeSummary request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetVolumeSummary] Could not get volume summary. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_volume_summary.failure-validate_request", err)
	}

	rsp.Summary, err = protectionGroup.GetVolumeSummary(ctx)
	if err != nil {
		return createError(ctx, "cdm-dr.manager.get_volume_summary.failure-get", err)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_volume_summary.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetVolumeSummary request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_volume_summary.success", nil)
}

// GetJobSummary 재해복구작업 요약 정보 조회
func (h *DisasterRecoveryManagerHandler) GetJobSummary(ctx context.Context, req *drms.JobSummaryRequest, rsp *drms.JobSummaryResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetJobSummary request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetJobSummary] Could not get job summary. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_job_summary.failure-validate_request", err)
	}

	if rsp.Summary, err = recoveryReport.GetJobSummary(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_job_summary.failure-get", err)
	}

	if len(rsp.Summary) == 0 {
		return createError(ctx, "cdm-dr.manager.get_job_summary.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_job_summary.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetJobSummary request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_job_summary.success", nil)
}

// GetClusterRelationshipList 클러스터 관계 목록 조회
func (h *DisasterRecoveryManagerHandler) GetClusterRelationshipList(ctx context.Context, req *drms.ClusterRelationshipListRequest, rsp *drms.ClusterRelationshipListResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetClusterRelationshipList request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetClusterRelationshipList] Could not get cluster relationships. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_cluster_relationship_list.failure-validate_request", err)
	}

	if rsp.ClusterRelationships, rsp.Pagination, err = clusterRelationship.GetClusterRelationList(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_cluster_relationship_list.failure-get", err)
	}

	if len(rsp.ClusterRelationships) == 0 {
		return createError(ctx, "cdm-dr.manager.get_cluster_relationship_list.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_cluster_relationship_list.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetClusterRelationshipList request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_cluster_relationship_list.success", nil)
}

// GetRecoveryClusterHypervisorResources 복구대상 클러스터 하이퍼바이저 리소스 조회
func (h *DisasterRecoveryManagerHandler) GetRecoveryClusterHypervisorResources(ctx context.Context, req *drms.RecoveryHypervisorResourceRequest, rsp *drms.RecoveryHypervisorResourceResponse) error {
	logger.Debug("Received DisasterRecoveryManager.GetRecoveryClusterHypervisorResources request")

	var err error
	defer func() {
		if err != nil {
			logger.Errorf("[Handler-GetRecoveryClusterHypervisorResources] Could not get cluster relationships. Cause: %+v", err)
		}
	}()

	if err = validateRequest(ctx); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_cluster_hypervisor_resources.failure-validate_request", err)
	}

	if rsp.HypervisorResources, rsp.Usable, err = recoveryJob.GetRecoveryClusterHypervisorResources(ctx, req); err != nil {
		return createError(ctx, "cdm-dr.manager.get_recovery_cluster_hypervisor_resources.failure-get", err)
	}

	if len(rsp.HypervisorResources) == 0 {
		return createError(ctx, "cdm-dr.manager.get_recovery_cluster_hypervisor_resources.success-get", errors.ErrNoContent)
	}

	rsp.Message = &drms.Message{Code: "cdm-dr.manager.get_recovery_cluster_hypervisor_resources.success"}
	logger.Debug("Respond DisasterRecoveryManager.GetRecoveryClusterHypervisorResources request")
	return errors.StatusOK(ctx, "cdm-dr.manager.get_recovery_cluster_hypervisor_resources.success", nil)
}
