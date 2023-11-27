package recoveryreport

import (
	"context"
	"github.com/asaskevich/govalidator"
	cmsConstant "github.com/datacommand2/cdm-center/cluster-manager/constant"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
)

const (
	unknownString = "unknown"
	unknownUint64 = uint64(9223372036854775807)
	unknownInt64  = int64(-1)
)

func (r *recoveryResult) validate() {
	var err error

	// operator
	if r.JobDetail.Operator.GetId() == 0 {
		err = errors.RequiredParameter("operator.id")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Id = unknownUint64
	}

	if r.JobDetail.Operator.Account == "" {
		err = errors.RequiredParameter("operator.account")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Account = unknownString
	}
	if len(r.JobDetail.Operator.Account) > 30 {
		err = errors.LengthOverflowParameterValue("operator.account", r.JobDetail.Operator.Account, 30)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Account = r.JobDetail.Operator.Account[:30]
	}

	if r.JobDetail.Operator.Name == "" {
		err = errors.RequiredParameter("operator.name")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Name = unknownString
	}
	if len(r.JobDetail.Operator.Name) > 255 {
		err = errors.LengthOverflowParameterValue("operator.name", r.JobDetail.Operator.Name, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Name = r.JobDetail.Operator.Account[:255]
	}

	if len(r.JobDetail.Operator.Department) > 255 {
		err = errors.LengthOverflowParameterValue("operator.department", r.JobDetail.Operator.Department, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Department = r.JobDetail.Operator.Department[:255]
	}

	if len(r.JobDetail.Operator.Position) > 255 {
		err = errors.LengthOverflowParameterValue("operator.position", r.JobDetail.Operator.Position, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Operator.Position = r.JobDetail.Operator.Position[:255]
	}

	// approver
	if r.Job.Approver != nil {
		if len(r.Job.Approver.Account) > 30 {
			err = errors.LengthOverflowParameterValue("approver.account", r.Job.Approver.Account, 30)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.Job.Approver.Account = r.Job.Approver.Account[:30]
		}

		if len(r.Job.Approver.Name) > 255 {
			err = errors.LengthOverflowParameterValue("approver.name", r.Job.Approver.Name, 255)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.Job.Approver.Name = r.Job.Approver.Name[:255]
		}

		if len(r.Job.Approver.Department) > 255 {
			err = errors.LengthOverflowParameterValue("approver.department", r.Job.Approver.Department, 255)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.Job.Approver.Department = r.Job.Approver.Department[:255]
		}

		if len(r.Job.Approver.Position) > 255 {
			err = errors.LengthOverflowParameterValue("approver.position", r.Job.Approver.Position, 255)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.Job.Approver.Position = r.Job.Approver.Position[:255]
		}
	}

	// protection group
	if r.JobDetail.Group.GetId() == 0 {
		err = errors.RequiredParameter("group.id")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Group.Id = unknownUint64
	}

	if r.JobDetail.Group.Name == "" {
		err = errors.RequiredParameter("group.name")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Group.Name = unknownString
	}
	if len(r.JobDetail.Group.Name) > 255 {
		err = errors.LengthOverflowParameterValue("group.name", r.JobDetail.Group.Name, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Group.Name = r.JobDetail.Group.Name[:255]
	}

	if len(r.JobDetail.Group.Remarks) > 300 {
		err = errors.LengthOverflowParameterValue("group.remarks", r.JobDetail.Group.Remarks, 300)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Group.Remarks = r.JobDetail.Group.Remarks[:300]
	}

	// plan
	if r.JobDetail.Plan.GetId() == 0 {
		err = errors.RequiredParameter("plan.id")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.Id = unknownUint64
	}

	if r.JobDetail.Plan.Name == "" {
		err = errors.RequiredParameter("plan.name")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.Name = unknownString
	}
	if len(r.JobDetail.Plan.Name) > 255 {
		err = errors.LengthOverflowParameterValue("plan.name", r.JobDetail.Plan.Name, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.Name = r.JobDetail.Plan.Name[:255]
	}

	if len(r.JobDetail.Plan.Remarks) > 300 {
		err = errors.LengthOverflowParameterValue("plan.remarks", r.JobDetail.Plan.Remarks, 300)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.Remarks = r.JobDetail.Plan.Remarks[:300]
	}

	// protection cluster
	if r.JobDetail.Plan.ProtectionCluster.GetId() == 0 {
		err = errors.RequiredParameter("plan.protection_cluster.id")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.ProtectionCluster.Id = unknownUint64
	}

	if r.JobDetail.Plan.ProtectionCluster.TypeCode == "" {
		err = errors.RequiredParameter("plan.protection_cluster.type_code")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.ProtectionCluster.TypeCode = unknownString
	}
	if len(r.JobDetail.Plan.ProtectionCluster.TypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("plan.protection_cluster.type_code", r.JobDetail.Plan.ProtectionCluster.TypeCode, 100)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.ProtectionCluster.TypeCode = r.JobDetail.Plan.ProtectionCluster.TypeCode[:100]
	}

	if r.JobDetail.Plan.ProtectionCluster.Name == "" {
		err = errors.RequiredParameter("plan.protection_cluster.name")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.ProtectionCluster.Name = unknownString
	}
	if len(r.JobDetail.Plan.ProtectionCluster.Name) > 255 {
		err = errors.LengthOverflowParameterValue("plan.protection_cluster.name", r.JobDetail.Plan.ProtectionCluster.Name, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.ProtectionCluster.Name = r.JobDetail.Plan.ProtectionCluster.Name[:255]
	}

	if len(r.JobDetail.Plan.ProtectionCluster.Remarks) > 300 {
		err = errors.LengthOverflowParameterValue("plan.protection_cluster.remarks", r.JobDetail.Plan.ProtectionCluster.Remarks, 300)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.ProtectionCluster.Remarks = r.JobDetail.Plan.ProtectionCluster.Remarks[:300]
	}

	// recovery cluster
	if r.JobDetail.Plan.RecoveryCluster.GetId() == 0 {
		err = errors.RequiredParameter("plan.recovery_cluster.id")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.RecoveryCluster.Id = unknownUint64
	}

	if r.JobDetail.Plan.RecoveryCluster.TypeCode == "" {
		err = errors.RequiredParameter("plan.recovery_cluster.type_code")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.RecoveryCluster.TypeCode = unknownString
	}
	if len(r.JobDetail.Plan.RecoveryCluster.TypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("plan.recovery_cluster.type_code", r.JobDetail.Plan.RecoveryCluster.TypeCode, 100)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.RecoveryCluster.TypeCode = r.JobDetail.Plan.RecoveryCluster.TypeCode[:100]
	}

	if r.JobDetail.Plan.RecoveryCluster.Name == "" {
		err = errors.RequiredParameter("plan.recovery_cluster.name")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.RecoveryCluster.Name = unknownString
	}
	if len(r.JobDetail.Plan.RecoveryCluster.Name) > 255 {
		err = errors.LengthOverflowParameterValue("plan.recovery_cluster.name", r.JobDetail.Plan.RecoveryCluster.Name, 255)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.RecoveryCluster.Name = r.JobDetail.Plan.RecoveryCluster.Name[:255]
	}

	if len(r.JobDetail.Plan.RecoveryCluster.Remarks) > 300 {
		err = errors.LengthOverflowParameterValue("plan.recovery_cluster.remarks", r.JobDetail.Plan.RecoveryCluster.Remarks, 300)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.RecoveryCluster.Remarks = r.JobDetail.Plan.RecoveryCluster.Remarks[:300]
	}

	// recovery job
	if r.JobDetail.TypeCode == "" {
		err = errors.RequiredParameter("type_code")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.TypeCode = unknownString
	}
	if len(r.JobDetail.TypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("type_code", r.JobDetail.TypeCode, 100)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.TypeCode = r.JobDetail.TypeCode[:100]
	}

	if r.JobDetail.Plan.DirectionCode == "" {
		err = errors.RequiredParameter("plan.direction_code")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.DirectionCode = unknownString
	}
	if len(r.JobDetail.Plan.DirectionCode) > 100 {
		err = errors.LengthOverflowParameterValue("plan.direction_code", r.JobDetail.Plan.DirectionCode, 100)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Plan.DirectionCode = r.JobDetail.Plan.DirectionCode[:100]
	}

	if r.JobDetail.Group.RecoveryPointObjectiveType == "" {
		err = errors.RequiredParameter("group.recovery_point_objective_type")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Group.RecoveryPointObjectiveType = unknownString
	}

	switch r.JobDetail.Group.RecoveryPointObjectiveType {
	case constant.RecoveryPointObjectiveTypeMinute:
		if r.JobDetail.Group.RecoveryPointObjective < 10 || r.JobDetail.Group.RecoveryPointObjective > 59 {
			err = errors.OutOfRangeParameterValue("group.recovery_point_objective", r.JobDetail.Group.RecoveryPointObjective, 10, 59)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		}

	case constant.RecoveryPointObjectiveTypeHour:
		if r.JobDetail.Group.RecoveryPointObjective < 1 || r.JobDetail.Group.RecoveryPointObjective > 23 {
			err = errors.OutOfRangeParameterValue("group.recovery_point_objective", r.JobDetail.Group.RecoveryPointObjective, 1, 23)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		}

	case constant.RecoveryPointObjectiveTypeDay:
		if r.JobDetail.Group.RecoveryPointObjective < 1 || r.JobDetail.Group.RecoveryPointObjective > 30 {
			err = errors.OutOfRangeParameterValue("group.recovery_point_objective", r.JobDetail.Group.RecoveryPointObjective, 1, 30)
			logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		}

	default:
		err = errors.UnavailableParameterValue("group.recovery_point_objective_type", r.JobDetail.Group.RecoveryPointObjectiveType, []interface{}{
			constant.RecoveryPointObjectiveTypeMinute,
			constant.RecoveryPointObjectiveTypeHour,
			constant.RecoveryPointObjectiveTypeDay})
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
	}

	if r.JobDetail.Group.RecoveryTimeObjective == 0 {
		err = errors.RequiredParameter("group.recovery_time_objective")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.Group.RecoveryTimeObjective = 4294967295
	}

	// recovery point
	if r.JobDetail.RecoveryPointTypeCode == "" {
		err = errors.RequiredParameter("recovery_point_type_code")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.RecoveryPointTypeCode = unknownString
	}
	if len(r.JobDetail.RecoveryPointTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_point_type_code", r.JobDetail.RecoveryPointTypeCode, 100)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobDetail.RecoveryPointTypeCode = r.JobDetail.RecoveryPointTypeCode[:100]
	}

	if r.Job.RecoveryPoint == 0 {
		err = errors.RequiredParameter("recovery_point")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		r.Job.RecoveryPoint = unknownInt64
	}

	// status
	if r.JobStatus.StartedAt == 0 {
		err = errors.RequiredParameter("started_at")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		r.JobStatus.StartedAt = unknownInt64
	}

	if r.JobStatus.FinishedAt == 0 {
		err = errors.RequiredParameter("finished_at")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		r.JobStatus.FinishedAt = unknownInt64
	}

	if r.JobStatus.ElapsedTime == 0 {
		err = errors.RequiredParameter("elapsed_time")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job detail: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)
		r.JobStatus.ElapsedTime = unknownInt64
	}

	// result
	if r.JobResult.ResultCode == "" {
		err = errors.RequiredParameter("result_code")
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job result: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobResult.ResultCode = unknownString
	}

	if len(r.JobResult.ResultCode) > 100 {
		err = errors.LengthOverflowParameterValue("result_code", r.JobResult.ResultCode, 100)
		logger.Warnf("[RecoveryReport-validate] Error occurred during validating job result: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

		r.JobResult.ResultCode = r.JobResult.ResultCode[:100]
	}
}

func validateTaskLogRecoveryResult(logs []*migrator.RecoveryJobLog) {
	var err error

	for _, log := range logs {
		if log.Code == "" {
			err = errors.RequiredParameter("code")
			logger.Warnf("[validateTaskLogRecoveryResult] Error occurred during validating task log. Cause: %+v", err)

			log.Code = unknownString
		}
		if len(log.Code) > 100 {
			err = errors.LengthOverflowParameterValue("code", log.Code, 100)
			logger.Warnf("[validateTaskLogRecoveryResult] Error occurred during validating task log. Cause: %+v", err)

			log.Code = log.Code[:100]
		}

		if log.LogDt == 0 {
			err = errors.RequiredParameter("log_dt")
			logger.Warnf("[validateTaskLogRecoveryResult] Error occurred during validating task log. Cause: %+v", err)
		}
	}
}

func (r *recoveryResult) validateFailedReasonRecoveryResult() {
	var err error

	for idx, f := range r.JobResult.FailedReasons {
		if f == nil {
			err = errors.RequiredParameter("failed_reasons.code")
			logger.Warnf("[validateFailedReasonRecoveryResult] Error occurred during validating failed reason: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.JobResult.FailedReasons[idx] = &migrator.Message{Code: unknownString}

		} else if f.Code == "" {
			err = errors.RequiredParameter("failed_reasons.code")
			logger.Warnf("[validateFailedReasonRecoveryResult] Error occurred during validating failed reason: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.JobResult.FailedReasons[idx].Code = unknownString

		} else if len(f.Code) > 100 {
			err = errors.LengthOverflowParameterValue("failed_reasons.code", f.Code, 100)
			logger.Warnf("[validateFailedReasonRecoveryResult] Error occurred during validating failed reason: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.JobResult.FailedReasons[idx].Code = f.Code[:100]
		}
	}
}

func (r *recoveryResult) validateWarningReasonRecoveryResult() {
	var err error

	for idx, w := range r.JobResult.WarningReasons {
		if w == nil {
			err = errors.RequiredParameter("warning_reasons.code")
			logger.Warnf("[validateWarningReasonRecoveryResult] Error occurred during validating warning reason: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.JobResult.WarningReasons[idx] = &migrator.Message{Code: unknownString}

		} else if w.Code == "" {
			err = errors.RequiredParameter("warning_reasons.code")
			logger.Warnf("[validateWarningReasonRecoveryResult] Error occurred during validating warning reason: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.JobResult.WarningReasons[idx].Code = unknownString

		} else if len(w.Code) > 100 {
			err = errors.LengthOverflowParameterValue("warning_reasons.code", w.Code, 100)
			logger.Warnf("[validateWarningReasonRecoveryResult] Error occurred during validating warning reason: job(%d). Cause: %+v", r.Job.RecoveryJobID, err)

			r.JobResult.WarningReasons[idx].Code = w.Code[:100]
		}
	}
}

func validateTenantRecoveryResult(m *model.RecoveryResultTenant) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterTenantUUID == "" {
		err = errors.RequiredParameter("protection_cluster_tenant_uuid")
		logger.Warnf("[validateTenantRecoveryResult] Error occurred during validating protection tenant(%d) result. Cause: %+v", m.ProtectionClusterTenantID, err)

		m.ProtectionClusterTenantUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterTenantUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_tenant_uuid", m.ProtectionClusterTenantUUID, "UUID")
		logger.Warnf("[validateTenantRecoveryResult] Error occurred during validating protection tenant(%d) result. Cause: %+v",
			m.ProtectionClusterTenantID, err)
	}

	if m.ProtectionClusterTenantName == "" {
		err = errors.RequiredParameter("protection_cluster_tenant_name")
		logger.Warnf("[validateTenantRecoveryResult] Error occurred during validating protection tenant(%d) result. Cause: %+v",
			m.ProtectionClusterTenantID, err)

		m.ProtectionClusterTenantName = unknownString
	}
	if len(m.ProtectionClusterTenantName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_tenant_name", m.ProtectionClusterTenantName, 255)
		logger.Warnf("[validateTenantRecoveryResult] Error occurred during validating protection tenant(%d) result. Cause: %+v",
			m.ProtectionClusterTenantID, err)

		m.ProtectionClusterTenantName = m.ProtectionClusterTenantName[:255]
	}

	if m.RecoveryClusterTenantUUID != nil && *m.RecoveryClusterTenantUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterTenantUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_tenant_uuid", m.RecoveryClusterTenantUUID, "UUID")
			logger.Warnf("[validateTenantRecoveryResult] Error occurred during validating protection tenant(%d) result. Cause: %+v",
				m.ProtectionClusterTenantID, err)
		}
	}

	if m.RecoveryClusterTenantName != nil && len(*m.RecoveryClusterTenantName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_tenant_name", *m.RecoveryClusterTenantName, 255)
		logger.Warnf("[validateTenantRecoveryResult] Error occurred during validating protection tenant(%d) result. Cause: %+v",
			m.ProtectionClusterTenantID, err)

		name := (*m.RecoveryClusterTenantName)[:255]
		m.RecoveryClusterTenantName = &name
	}

	return nil
}

func validateNetworkRecoveryResult(m *model.RecoveryResultNetwork) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterNetworkID == 0 {
		return errors.RequiredParameter("protection_cluster_network_id")
	}

	if m.ProtectionClusterNetworkUUID == "" {
		err = errors.RequiredParameter("protection_cluster_network_uuid")
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterNetworkUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterNetworkUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_network_uuid", m.ProtectionClusterNetworkUUID, "UUID")
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)
	}

	if m.ProtectionClusterNetworkName == "" {
		err = errors.RequiredParameter("protection_cluster_network_name")
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterNetworkName = unknownString
	}
	if len(m.ProtectionClusterNetworkName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_network_name", m.ProtectionClusterNetworkName, 255)
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterNetworkName = m.ProtectionClusterNetworkName[:255]
	}

	if m.ProtectionClusterNetworkDescription != nil && len(*m.ProtectionClusterNetworkDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_network_description", *m.ProtectionClusterNetworkDescription, 255)
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		description := (*m.ProtectionClusterNetworkDescription)[:255]
		m.ProtectionClusterNetworkDescription = &description
	}

	if m.ProtectionClusterNetworkTypeCode == "" {
		err = errors.RequiredParameter("protection_cluster_network_type_code")
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterNetworkTypeCode = unknownString
	}
	if len(m.ProtectionClusterNetworkTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("protection_cluster_network_type_code", m.ProtectionClusterNetworkTypeCode, 100)
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterNetworkTypeCode = m.ProtectionClusterNetworkTypeCode[:100]
	}

	if m.ProtectionClusterNetworkState == "" {
		err = errors.RequiredParameter("protection_cluster_network_state")
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterNetworkState = unknownString
	}
	if m.ProtectionClusterNetworkState != "up" && m.ProtectionClusterNetworkState != "down" {
		err = errors.UnavailableParameterValue("protection_cluster_network_state", m.ProtectionClusterNetworkState, []interface{}{"up", "down"})
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)
	}

	if m.RecoveryClusterNetworkUUID != nil && *m.RecoveryClusterNetworkUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterNetworkUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_network_uuid", m.RecoveryClusterNetworkUUID, "UUID")
			logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)
		}
	}

	if m.RecoveryClusterNetworkName != nil && len(*m.RecoveryClusterNetworkName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_network_name", *m.RecoveryClusterNetworkName, 255)
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		name := (*m.RecoveryClusterNetworkName)[:255]
		m.RecoveryClusterNetworkName = &name
	}

	if m.RecoveryClusterNetworkDescription != nil && len(*m.RecoveryClusterNetworkDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_network_description", *m.RecoveryClusterNetworkDescription, 255)
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		description := (*m.RecoveryClusterNetworkDescription)[:255]
		m.RecoveryClusterNetworkDescription = &description
	}

	if m.RecoveryClusterNetworkTypeCode != nil && len(*m.RecoveryClusterNetworkTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_network_type_code", *m.RecoveryClusterNetworkTypeCode, 100)
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)

		typeCode := (*m.RecoveryClusterNetworkTypeCode)[:255]
		m.RecoveryClusterNetworkTypeCode = &typeCode
	}

	if m.RecoveryClusterNetworkState != nil && *m.RecoveryClusterNetworkState != "up" && *m.RecoveryClusterNetworkState != "down" {
		err = errors.UnavailableParameterValue("recovery_cluster_network_state", m.RecoveryClusterNetworkState, []interface{}{"up", "down"})
		logger.Warnf("[validateNetworkRecoveryResult] Error occurred during validating network result: tenant(%d) protection network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterNetworkID, err)
	}

	return nil
}

func validateSubnetRecoveryResult(m *model.RecoveryResultSubnet) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterNetworkID == 0 {
		return errors.RequiredParameter("protection_cluster_network_id")
	}

	if m.ProtectionClusterSubnetID == 0 {
		return errors.RequiredParameter("protection_cluster_subnet_id")
	}

	if m.ProtectionClusterSubnetUUID == "" {
		err = errors.RequiredParameter("protection_cluster_subnet_uuid")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterSubnetUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterSubnetUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_subnet_uuid", m.ProtectionClusterSubnetUUID, "UUID")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.ProtectionClusterSubnetName == "" {
		err = errors.RequiredParameter("protection_cluster_subnet_name")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterSubnetName = unknownString
	}
	if len(m.ProtectionClusterSubnetName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_subnet_name", m.ProtectionClusterSubnetName, 255)
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterSubnetName = m.ProtectionClusterSubnetName[:255]
	}

	if m.ProtectionClusterSubnetDescription != nil && len(*m.ProtectionClusterSubnetDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_subnet_description", *m.ProtectionClusterSubnetDescription, 255)
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		description := (*m.ProtectionClusterSubnetDescription)[:255]
		m.ProtectionClusterSubnetDescription = &description
	}

	if m.ProtectionClusterSubnetNetworkCidr == "" {
		err = errors.RequiredParameter("protection_cluster_subnet_network_cidr")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterSubnetNetworkCidr = unknownString
	}
	if !govalidator.IsCIDR(m.ProtectionClusterSubnetNetworkCidr) {
		err = errors.FormatMismatchParameterValue("protection_cluster_subnet_network_cidr", m.ProtectionClusterSubnetNetworkCidr, "CIDR")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.ProtectionClusterSubnetGatewayIPAddress != nil &&
		*m.ProtectionClusterSubnetGatewayIPAddress != "" &&
		!govalidator.IsIP(*m.ProtectionClusterSubnetGatewayIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_subnet_gateway_ip_address", *m.ProtectionClusterSubnetGatewayIPAddress, "IP")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.ProtectionClusterSubnetIpv6AddressModeCode != nil &&
		*m.ProtectionClusterSubnetIpv6AddressModeCode != "" &&
		*m.ProtectionClusterSubnetIpv6AddressModeCode != "slaac" &&
		*m.ProtectionClusterSubnetIpv6AddressModeCode != "dhcpv6-stateful" &&
		*m.ProtectionClusterSubnetIpv6AddressModeCode != "dhcpv6-stateless" {
		err = errors.UnavailableParameterValue("protection_cluster_subnet_ipv6_address_mode_code", *m.ProtectionClusterSubnetIpv6AddressModeCode, []interface{}{"slaac", "dhcpv6-stateful", "dhcpv6-stateless"})
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.ProtectionClusterSubnetIpv6RaModeCode != nil &&
		*m.ProtectionClusterSubnetIpv6RaModeCode != "" &&
		*m.ProtectionClusterSubnetIpv6RaModeCode != "slaac" &&
		*m.ProtectionClusterSubnetIpv6RaModeCode != "dhcpv6-stateful" &&
		*m.ProtectionClusterSubnetIpv6RaModeCode != "dhcpv6-stateless" {
		err = errors.UnavailableParameterValue("protection_cluster_subnet_ipv6_ra_mode_code", *m.ProtectionClusterSubnetIpv6RaModeCode, []interface{}{"slaac", "dhcpv6-stateful", "dhcpv6-stateless"})
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}
	if m.RecoveryClusterSubnetUUID != nil {
		if _, err = uuid.Parse(*m.RecoveryClusterSubnetUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_subnet_uuid", m.RecoveryClusterSubnetUUID, "UUID")
			logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
		}
	}

	if m.RecoveryClusterSubnetName != nil && len(*m.RecoveryClusterSubnetName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_subnet_name", *m.RecoveryClusterSubnetName, 255)
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		name := (*m.RecoveryClusterSubnetName)[:255]
		m.RecoveryClusterSubnetName = &name
	}

	if m.RecoveryClusterSubnetDescription != nil && len(*m.RecoveryClusterSubnetDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_subnet_description", *m.RecoveryClusterSubnetDescription, 255)
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		description := (*m.RecoveryClusterSubnetDescription)[:255]
		m.RecoveryClusterSubnetDescription = &description
	}

	if m.RecoveryClusterSubnetNetworkCidr != nil && *m.RecoveryClusterSubnetNetworkCidr != "" && !govalidator.IsCIDR(*m.RecoveryClusterSubnetNetworkCidr) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_subnet_network_cidr", m.RecoveryClusterSubnetNetworkCidr, "CIDR")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.RecoveryClusterSubnetGatewayIPAddress != nil &&
		*m.RecoveryClusterSubnetGatewayIPAddress != "" &&
		!govalidator.IsIP(*m.RecoveryClusterSubnetGatewayIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_subnet_gateway_ip_address", *m.RecoveryClusterSubnetGatewayIPAddress, "IP")
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.RecoveryClusterSubnetIpv6AddressModeCode != nil &&
		*m.RecoveryClusterSubnetIpv6AddressModeCode != "" &&
		*m.RecoveryClusterSubnetIpv6AddressModeCode != "slaac" &&
		*m.RecoveryClusterSubnetIpv6AddressModeCode != "dhcpv6-stateful" &&
		*m.RecoveryClusterSubnetIpv6AddressModeCode != "dhcpv6-stateless" {
		err = errors.UnavailableParameterValue("recovery_cluster_subnet_ipv6_address_mode_code", *m.RecoveryClusterSubnetIpv6AddressModeCode, []interface{}{"slaac", "dhcpv6-stateful", "dhcpv6-stateless"})
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.RecoveryClusterSubnetIpv6RaModeCode != nil &&
		*m.RecoveryClusterSubnetIpv6RaModeCode != "" &&
		*m.RecoveryClusterSubnetIpv6RaModeCode != "slaac" &&
		*m.RecoveryClusterSubnetIpv6RaModeCode != "dhcpv6-stateful" &&
		*m.RecoveryClusterSubnetIpv6RaModeCode != "dhcpv6-stateless" {
		err = errors.UnavailableParameterValue("recovery_cluster_subnet_ipv6_ra_mode_code", *m.RecoveryClusterSubnetIpv6RaModeCode, []interface{}{"slaac", "dhcpv6-stateful", "dhcpv6-stateless"})
		logger.Warnf("[validateSubnetRecoveryResult] Error occurred during validating subnet result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	return nil
}

func validateSubnetDHCPPoolRecoveryResult(m *model.RecoveryResultSubnetDHCPPool) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterNetworkID == 0 {
		return errors.RequiredParameter("protection_cluster_network_id")
	}

	if m.ProtectionClusterSubnetID == 0 {
		return errors.RequiredParameter("protection_cluster_subnet_id")
	}

	if m.ProtectionClusterStartIPAddress == "" {
		err = errors.RequiredParameter("protection_cluster_start_ip_address")
		logger.Warnf("[validateSubnetDHCPPoolRecoveryResult] Error occurred during validating subnet DHCP pool result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterStartIPAddress = unknownString
	}
	if !govalidator.IsIP(m.ProtectionClusterStartIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_start_ip_address", m.ProtectionClusterStartIPAddress, "IP")
		logger.Warnf("[validateSubnetDHCPPoolRecoveryResult] Error occurred during validating subnet DHCP pool result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.ProtectionClusterEndIPAddress == "" {
		err = errors.RequiredParameter("protection_cluster_end_ip_address")
		logger.Warnf("[validateSubnetDHCPPoolRecoveryResult] Error occurred during validating subnet DHCP pool result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterEndIPAddress = unknownString
	}
	if !govalidator.IsIP(m.ProtectionClusterEndIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_end_ip_address", m.ProtectionClusterEndIPAddress, "IP")
		logger.Warnf("[validateSubnetDHCPPoolRecoveryResult] Error occurred during validating subnet DHCP pool result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.RecoveryClusterStartIPAddress != nil && *m.RecoveryClusterStartIPAddress != "" && !govalidator.IsIP(*m.RecoveryClusterStartIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_start_ip_address", m.RecoveryClusterStartIPAddress, "IP")
		logger.Warnf("[validateSubnetDHCPPoolRecoveryResult] Error occurred during validating subnet DHCP pool result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	if m.RecoveryClusterEndIPAddress != nil && *m.RecoveryClusterEndIPAddress != "" && !govalidator.IsIP(*m.RecoveryClusterEndIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_end_ip_address", m.RecoveryClusterEndIPAddress, "IP")
		logger.Warnf("[validateSubnetDHCPPoolRecoveryResult] Error occurred during validating subnet DHCP pool result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)
	}

	return nil
}

func validateSubnetNameserverRecoveryResult(m *model.RecoveryResultSubnetNameserver) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterNetworkID == 0 {
		return errors.RequiredParameter("protection_cluster_network_id")
	}

	if m.ProtectionClusterSubnetID == 0 {
		return errors.RequiredParameter("protection_cluster_subnet_id")
	}

	if m.ProtectionClusterNameserver == "" {
		err = errors.RequiredParameter("protection_cluster_nameserver")
		logger.Warnf("[validateSubnetNameserverRecoveryResult] Error occurred during validating subnet name server result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterNameserver = unknownString
	}
	if len(m.ProtectionClusterNameserver) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_nameserver", m.ProtectionClusterNameserver, 255)
		logger.Warnf("[validateSubnetNameserverRecoveryResult] Error occurred during validating subnet name server result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		m.ProtectionClusterNameserver = m.ProtectionClusterNameserver[:255]
	}

	if m.RecoveryClusterNameserver != nil && len(*m.RecoveryClusterNameserver) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_nameserver", *m.RecoveryClusterNameserver, 255)
		logger.Warnf("[validateSubnetNameserverRecoveryResult] Error occurred during validating subnet name server result: tenant(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSubnetID, err)

		name := (*m.RecoveryClusterNameserver)[:255]
		m.RecoveryClusterNameserver = &name
	}

	return nil
}

func validateSecurityGroupRecoveryResult(m *model.RecoveryResultSecurityGroup) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterSecurityGroupID == 0 {
		return errors.RequiredParameter("protection_cluster_security_group_id")
	}

	if m.ProtectionClusterSecurityGroupUUID == "" {
		err = errors.RequiredParameter("protection_cluster_security_group_uuid")
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)

		m.ProtectionClusterSecurityGroupUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterSecurityGroupUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_security_group_uuid", m.ProtectionClusterSecurityGroupUUID, "UUID")
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)
	}

	if m.ProtectionClusterSecurityGroupName == "" {
		err = errors.RequiredParameter("protection_cluster_security_group_name")
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)

		m.ProtectionClusterSecurityGroupName = unknownString
	}
	if len(m.ProtectionClusterSecurityGroupName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_security_group_name", m.ProtectionClusterSecurityGroupName, 255)
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)

		m.ProtectionClusterSecurityGroupName = m.ProtectionClusterSecurityGroupName[:255]
	}

	if m.ProtectionClusterSecurityGroupDescription != nil && len(*m.ProtectionClusterSecurityGroupDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_security_group_description", *m.ProtectionClusterSecurityGroupDescription, 255)
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)

		description := (*m.ProtectionClusterSecurityGroupDescription)[:255]
		m.ProtectionClusterSecurityGroupDescription = &description
	}
	if m.RecoveryClusterSecurityGroupUUID != nil && *m.RecoveryClusterSecurityGroupUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterSecurityGroupUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_security_group_uuid", m.RecoveryClusterSecurityGroupUUID, "UUID")
			logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)
		}
	}

	if m.RecoveryClusterSecurityGroupName != nil && len(*m.RecoveryClusterSecurityGroupName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_security_group_name", *m.RecoveryClusterSecurityGroupName, 255)
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)

		name := (*m.RecoveryClusterSecurityGroupName)[:255]
		m.RecoveryClusterSecurityGroupName = &name
	}

	if m.RecoveryClusterSecurityGroupDescription != nil && len(*m.RecoveryClusterSecurityGroupDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_security_group_description", *m.RecoveryClusterSecurityGroupDescription, 255)
		logger.Warnf("[validateSecurityGroupRecoveryResult] Error occurred during validating security group result: tenant(%d) security group(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, err)

		description := (*m.RecoveryClusterSecurityGroupDescription)[:255]
		m.RecoveryClusterSecurityGroupDescription = &description
	}

	return nil
}

func validateSecurityGroupRuleRecoveryResult(m *model.RecoveryResultSecurityGroupRule) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterSecurityGroupID == 0 {
		return errors.RequiredParameter("protection_cluster_security_group_id")
	}

	if m.ProtectionClusterSecurityGroupRuleID == 0 {
		return errors.RequiredParameter("protection_cluster_security_group_rule_id")
	}

	if m.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID != nil && *m.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID == 0 {
		return errors.RequiredParameter("protection_cluster_security_group_rule_remote_security_group_id")
	}

	if m.ProtectionClusterSecurityGroupRuleUUID == "" {
		err = errors.RequiredParameter("protection_cluster_security_group_rule_uuid")
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		m.ProtectionClusterSecurityGroupRuleUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterSecurityGroupRuleUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_security_group_rule_uuid", m.ProtectionClusterSecurityGroupRuleUUID, "UUID")
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)
	}
	if m.ProtectionClusterSecurityGroupRuleDescription != nil && len(*m.ProtectionClusterSecurityGroupRuleDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_security_group_rule_description", *m.ProtectionClusterSecurityGroupRuleDescription, 255)
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		description := (*m.ProtectionClusterSecurityGroupRuleDescription)[:255]
		m.ProtectionClusterSecurityGroupRuleDescription = &description
	}

	if m.ProtectionClusterSecurityGroupRuleNetworkCidr != nil && *m.ProtectionClusterSecurityGroupRuleNetworkCidr != "" {
		if !govalidator.IsCIDR(*m.ProtectionClusterSecurityGroupRuleNetworkCidr) {
			err = errors.FormatMismatchParameterValue("protection_cluster_security_group_rule_network_cidr", *m.ProtectionClusterSecurityGroupRuleNetworkCidr, "CIDR")
			logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)
		}
	}

	if m.ProtectionClusterSecurityGroupRuleDirection == "" {
		err = errors.RequiredParameter("protection_cluster_security_group_rule_direction")
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		m.ProtectionClusterSecurityGroupRuleDirection = unknownString
	}
	if m.ProtectionClusterSecurityGroupRuleDirection != "ingress" && m.ProtectionClusterSecurityGroupRuleDirection != "egress" {
		err = errors.UnavailableParameterValue("protection_cluster_security_group_rule_direction", m.ProtectionClusterSecurityGroupRuleDirection, []interface{}{"ingress", "egress"})
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)
	}

	if m.ProtectionClusterSecurityGroupRuleProtocol != nil && len(*m.ProtectionClusterSecurityGroupRuleProtocol) > 20 {
		err = errors.LengthOverflowParameterValue("protection_cluster_security_group_rule_protocol", *m.ProtectionClusterSecurityGroupRuleProtocol, 20)
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		protocol := (*m.ProtectionClusterSecurityGroupRuleProtocol)[:20]
		m.ProtectionClusterSecurityGroupRuleProtocol = &protocol
	}

	if m.RecoveryClusterSecurityGroupRuleRemoteSecurityGroupID != nil && *m.RecoveryClusterSecurityGroupRuleRemoteSecurityGroupID == 0 {
		err = errors.RequiredParameter("recovery_cluster_security_group_rule_remote_security_group_id")
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		*m.RecoveryClusterSecurityGroupRuleRemoteSecurityGroupID = unknownUint64
	}
	if m.RecoveryClusterSecurityGroupRuleUUID != nil && *m.RecoveryClusterSecurityGroupRuleUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterSecurityGroupRuleUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_security_group_rule_uuid", m.RecoveryClusterSecurityGroupRuleUUID, "UUID")
			logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)
		}
	}

	if m.RecoveryClusterSecurityGroupRuleDescription != nil && len(*m.RecoveryClusterSecurityGroupRuleDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_security_group_rule_description", *m.RecoveryClusterSecurityGroupRuleDescription, 255)
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		description := (*m.RecoveryClusterSecurityGroupRuleDescription)[:255]
		m.RecoveryClusterSecurityGroupRuleDescription = &description
	}

	if m.RecoveryClusterSecurityGroupRuleNetworkCidr != nil && *m.RecoveryClusterSecurityGroupRuleNetworkCidr != "" && !govalidator.IsCIDR(*m.RecoveryClusterSecurityGroupRuleNetworkCidr) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_security_group_rule_network_cidr", m.RecoveryClusterSecurityGroupRuleNetworkCidr, "CIDR")
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)
	}

	if m.RecoveryClusterSecurityGroupRuleDirection != nil && *m.RecoveryClusterSecurityGroupRuleDirection != "ingress" && *m.RecoveryClusterSecurityGroupRuleDirection != "egress" {
		err = errors.UnavailableParameterValue("recovery_cluster_security_group_rule_direction", m.RecoveryClusterSecurityGroupRuleDirection, []interface{}{"ingress", "egress"})
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)
	}

	if m.RecoveryClusterSecurityGroupRuleProtocol != nil && len(*m.RecoveryClusterSecurityGroupRuleProtocol) > 20 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_security_group_rule_protocol", *m.RecoveryClusterSecurityGroupRuleProtocol, 20)
		logger.Warnf("[validateSecurityGroupRuleRecoveryResult] Error occurred during validating security group rule result: tenant(%d) protection security group(%d) rule(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterSecurityGroupID, m.ProtectionClusterSecurityGroupRuleID, err)

		protocol := (*m.RecoveryClusterSecurityGroupRuleProtocol)[:20]
		m.RecoveryClusterSecurityGroupRuleProtocol = &protocol
	}

	return nil
}

func validateRouterRecoveryResult(m *model.RecoveryResultRouter) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterRouterID == 0 {
		return errors.RequiredParameter("protection_cluster_router_id")
	}

	if m.ProtectionClusterRouterUUID == "" {
		err = errors.RequiredParameter("protection_cluster_router_uuid")
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ProtectionClusterRouterUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterRouterUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_router_uuid", m.ProtectionClusterRouterUUID, "UUID")
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ProtectionClusterRouterName == "" {
		err = errors.RequiredParameter("protection_cluster_router_name")
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ProtectionClusterRouterName = unknownString
	}
	if len(m.ProtectionClusterRouterName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_router_name", m.ProtectionClusterRouterName, 255)
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ProtectionClusterRouterName = m.ProtectionClusterRouterName[:255]
	}

	if m.ProtectionClusterRouterState == "" {
		err = errors.RequiredParameter("protection_cluster_router_state")
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ProtectionClusterRouterState = unknownString
	}
	if m.ProtectionClusterRouterState != "up" && m.ProtectionClusterRouterState != "down" {
		err = errors.UnavailableParameterValue("protection_cluster_router_state", m.ProtectionClusterRouterState, []interface{}{"up", "down"})
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.RecoveryClusterRouterUUID != nil && *m.RecoveryClusterRouterUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterRouterUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_router_uuid", m.RecoveryClusterRouterUUID, "UUID")
			logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
		}
	}

	if m.RecoveryClusterRouterName != nil && len(*m.RecoveryClusterRouterName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_router_name", *m.RecoveryClusterRouterName, 255)
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		name := (*m.RecoveryClusterRouterName)[:255]
		m.RecoveryClusterRouterName = &name
	}

	if m.RecoveryClusterRouterState != nil && *m.RecoveryClusterRouterState != "up" && *m.RecoveryClusterRouterState != "down" {
		err = errors.UnavailableParameterValue("recovery_cluster_router_state", m.RecoveryClusterRouterState, []interface{}{"up", "down"})
		logger.Warnf("[validateRouterRecoveryResult] Error occurred during validating router result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	return nil
}

func validateExtraRoutesRecoveryResult(m *model.RecoveryResultExtraRoute) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterRouterID == 0 {
		return errors.RequiredParameter("protection_cluster_router_id")
	}

	if m.ProtectionClusterDestination == "" {
		err = errors.RequiredParameter("protection_cluster_destination")
		logger.Warnf("[validateExtraRoutesRecoveryResult] Error occurred during validating extra routes result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ProtectionClusterDestination = unknownString
	}
	if !govalidator.IsCIDR(m.ProtectionClusterDestination) {
		err = errors.FormatMismatchParameterValue("protection_cluster_destination", m.ProtectionClusterDestination, "CIDR")
		logger.Warnf("[validateExtraRoutesRecoveryResult] Error occurred during validating extra routes result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ProtectionClusterNexthop == "" {
		err = errors.RequiredParameter("protection_cluster_nexthop")
		logger.Warnf("[validateExtraRoutesRecoveryResult] Error occurred during validating extra routes result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ProtectionClusterDestination = unknownString
	}
	if !govalidator.IsIP(m.ProtectionClusterNexthop) {
		err = errors.FormatMismatchParameterValue("protection_cluster_nexthop", m.ProtectionClusterNexthop, "IP")
		logger.Warnf("[validateExtraRoutesRecoveryResult] Error occurred during validating extra routes result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.RecoveryClusterDestination != nil && *m.RecoveryClusterDestination != "" && !govalidator.IsCIDR(*m.RecoveryClusterDestination) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_destination", m.RecoveryClusterDestination, "CIDR")
		logger.Warnf("[validateExtraRoutesRecoveryResult] Error occurred during validating extra routes result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.RecoveryClusterNexthop != nil && *m.RecoveryClusterNexthop != "" && !govalidator.IsIP(*m.RecoveryClusterNexthop) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_nexthop", m.RecoveryClusterNexthop, "IP")
		logger.Warnf("[validateExtraRoutesRecoveryResult] Error occurred during validating extra routes result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	return nil
}

func validateInternalRoutingInterfaceRecoveryResult(m *model.RecoveryResultInternalRoutingInterface) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterRouterID == 0 {
		return errors.RequiredParameter("protection_cluster_router_id")
	}

	if m.ProtectionClusterNetworkID == 0 {
		return errors.RequiredParameter("protection_cluster_network_id")
	}

	if m.ProtectionClusterSubnetID == 0 {
		return errors.RequiredParameter("protection_cluster_subnet_id")
	}

	if m.ProtectionClusterIPAddress == "" {
		err = errors.RequiredParameter("protection_cluster_ip_address")
		logger.Warnf("[validateInternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, m.ProtectionClusterSubnetID, err)
	}
	if !govalidator.IsIP(m.ProtectionClusterIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_ip_address", m.ProtectionClusterIPAddress, "IP")
		logger.Warnf("[validateInternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d) protection subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, m.ProtectionClusterSubnetID, err)
	}

	if m.RecoveryClusterIPAddress != nil && !govalidator.IsIP(*m.RecoveryClusterIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_ip_address", m.RecoveryClusterIPAddress, "IP")
		logger.Warnf("[validateInternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d) recovery subnet(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, m.RecoveryClusterSubnetID, err)
	}

	return nil
}

func validateExternalRoutingInterfaceRecoveryResult(m *model.RecoveryResultExternalRoutingInterface) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterRouterID == 0 {
		return errors.RequiredParameter("protection_cluster_router_id")
	}

	if m.ClusterNetworkID == 0 {
		err = errors.RequiredParameter("cluster_network_id")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkID = unknownUint64
	}

	if m.ClusterNetworkUUID == "" {
		err = errors.RequiredParameter("cluster_network_uuid")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkUUID = unknownString
	}
	if _, err = uuid.Parse(m.ClusterNetworkUUID); err != nil {
		err = errors.FormatMismatchParameterValue("cluster_network_uuid", m.ClusterNetworkUUID, "UUID")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterNetworkName == "" {
		err = errors.RequiredParameter("cluster_network_name")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkName = unknownString
	}
	if len(m.ClusterNetworkName) > 255 {
		err = errors.LengthOverflowParameterValue("cluster_network_name", m.ClusterNetworkName, 255)
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkName = m.ClusterNetworkName[:255]
	}
	if m.ClusterNetworkDescription != nil && len(*m.ClusterNetworkDescription) > 255 {
		err = errors.LengthOverflowParameterValue("cluster_network_description", *m.ClusterNetworkDescription, 255)
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		description := (*m.ClusterNetworkDescription)[:255]
		m.ClusterNetworkDescription = &description
	}

	if m.ClusterNetworkTypeCode == "" {
		err = errors.RequiredParameter("cluster_network_type_code")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkTypeCode = unknownString
	}
	if len(m.ClusterNetworkTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("cluster_network_type_code", m.ClusterNetworkTypeCode, 100)
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkTypeCode = m.ClusterNetworkTypeCode[:100]
	}

	if m.ClusterNetworkState == "" {
		err = errors.RequiredParameter("cluster_network_state")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterNetworkState = unknownString
	}
	if m.ClusterNetworkState != "up" && m.ClusterNetworkState != "down" {
		err = errors.UnavailableParameterValue("cluster_network_state", m.ClusterNetworkState, []interface{}{"up", "down"})
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterSubnetID == 0 {
		err = errors.RequiredParameter("cluster_subnet_id")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterSubnetID = unknownUint64
	}

	if m.ClusterSubnetUUID == "" {
		err = errors.RequiredParameter("cluster_subnet_uuid")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterSubnetUUID = unknownString
	}
	if _, err = uuid.Parse(m.ClusterSubnetUUID); err != nil {
		err = errors.FormatMismatchParameterValue("cluster_subnet_uuid", m.ClusterSubnetUUID, "UUID")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterSubnetName == "" {
		err = errors.RequiredParameter("cluster_subnet_name")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterSubnetName = unknownString
	}
	if len(m.ClusterSubnetName) > 255 {
		err = errors.LengthOverflowParameterValue("cluster_subnet_name", m.ClusterSubnetName, 255)
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterSubnetName = m.ClusterSubnetName[:255]
	}

	if m.ClusterSubnetDescription != nil && len(*m.ClusterSubnetDescription) > 255 {
		err = errors.LengthOverflowParameterValue("cluster_subnet_description", *m.ClusterSubnetDescription, 255)
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		description := (*m.ClusterSubnetDescription)[:255]
		m.ClusterSubnetDescription = &description
	}

	if m.ClusterSubnetNetworkCidr == "" {
		err = errors.RequiredParameter("cluster_subnet_network_cidr")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterSubnetNetworkCidr = unknownString
	}
	if !govalidator.IsCIDR(m.ClusterSubnetNetworkCidr) {
		err = errors.FormatMismatchParameterValue("cluster_subnet_network_cidr", m.ClusterSubnetNetworkCidr, "CIDR")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterSubnetGatewayIPAddress != nil &&
		*m.ClusterSubnetGatewayIPAddress != "" &&
		!govalidator.IsIP(*m.ClusterSubnetGatewayIPAddress) {
		err = errors.FormatMismatchParameterValue("cluster_subnet_gateway_ip_address", *m.ClusterSubnetGatewayIPAddress, "IP")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterSubnetIpv6AddressModeCode != nil &&
		*m.ClusterSubnetIpv6AddressModeCode != "" &&
		*m.ClusterSubnetIpv6AddressModeCode != "slaac" &&
		*m.ClusterSubnetIpv6AddressModeCode != "dhcpv6-stateful" &&
		*m.ClusterSubnetIpv6AddressModeCode != "dhcpv6-stateless" {
		err = errors.UnavailableParameterValue("cluster_subnet_ipv6_address_mode_code", *m.ClusterSubnetIpv6AddressModeCode, []interface{}{"slaac", "dhcpv6-stateful", "dhcpv6-stateless"})
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterSubnetIpv6RaModeCode != nil &&
		*m.ClusterSubnetIpv6RaModeCode != "" &&
		*m.ClusterSubnetIpv6RaModeCode != "slaac" &&
		*m.ClusterSubnetIpv6RaModeCode != "dhcpv6-stateful" &&
		*m.ClusterSubnetIpv6RaModeCode != "dhcpv6-stateless" {
		err = errors.UnavailableParameterValue("cluster_subnet_ipv6_ra_mode_code", *m.ClusterSubnetIpv6RaModeCode, []interface{}{"slaac", "dhcpv6-stateful", "dhcpv6-stateless"})
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	if m.ClusterIPAddress == "" {
		err = errors.RequiredParameter("cluster_ip_address")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)

		m.ClusterIPAddress = unknownString
	}
	if !govalidator.IsIP(m.ClusterIPAddress) {
		err = errors.FormatMismatchParameterValue("cluster_ip_address", m.ClusterIPAddress, "IP")
		logger.Warnf("[validateExternalRoutingInterfaceRecoveryResult] Error occurred during validating routing interface result: tenant(%d) protection router(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterRouterID, err)
	}

	return nil
}

func validateFloatingIPRecoveryResult(m *model.RecoveryResultFloatingIP) error {
	var err error

	if m.ProtectionClusterFloatingIPID == 0 {
		return errors.RequiredParameter("protection_cluster_floating_ip_id")
	}

	if m.ProtectionClusterFloatingIPUUID == "" {
		err = errors.RequiredParameter("protection_cluster_floating_ip_uuid")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterFloatingIPUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterFloatingIPUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_floating_ip_uuid", m.ProtectionClusterFloatingIPUUID, "UUID")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)
	}

	if m.ProtectionClusterFloatingIPDescription != nil && len(*m.ProtectionClusterFloatingIPDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_floating_ip_description", *m.ProtectionClusterFloatingIPDescription, 255)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		description := (*m.ProtectionClusterFloatingIPDescription)[:255]
		m.ProtectionClusterFloatingIPDescription = &description
	}

	if m.ProtectionClusterFloatingIPIPAddress == "" {
		err = errors.RequiredParameter("protection_cluster_floating_ip_ip_address")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterFloatingIPIPAddress = unknownString
	}
	if !govalidator.IsIP(m.ProtectionClusterFloatingIPIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_floating_ip_ip_address", m.ProtectionClusterFloatingIPIPAddress, "IP")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)
	}

	if m.ProtectionClusterNetworkID == 0 {
		err = errors.RequiredParameter("protection_cluster_network_id")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterNetworkID = unknownUint64
	}

	if m.ProtectionClusterNetworkUUID == "" {
		err = errors.RequiredParameter("protection_cluster_network_uuid")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterFloatingIPUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterNetworkUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_network_uuid", m.ProtectionClusterNetworkUUID, "UUID")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)
	}

	if m.ProtectionClusterNetworkName == "" {
		err = errors.RequiredParameter("protection_cluster_network_name")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterNetworkName = unknownString
	}
	if len(m.ProtectionClusterNetworkName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_network_name", m.ProtectionClusterNetworkName, 255)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterNetworkName = m.ProtectionClusterNetworkName[:255]
	}

	if m.ProtectionClusterNetworkDescription != nil && len(*m.ProtectionClusterNetworkDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_network_description", *m.ProtectionClusterNetworkDescription, 255)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		description := (*m.ProtectionClusterFloatingIPDescription)[:255]
		m.ProtectionClusterFloatingIPDescription = &description
	}

	if m.ProtectionClusterNetworkTypeCode == "" {
		err = errors.RequiredParameter("protection_cluster_network_type_code")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterNetworkTypeCode = unknownString
	}
	if len(m.ProtectionClusterNetworkTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("protection_cluster_network_type_code", m.ProtectionClusterNetworkTypeCode, 100)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterNetworkTypeCode = m.ProtectionClusterNetworkTypeCode[:100]
	}

	if m.ProtectionClusterNetworkState == "" {
		err = errors.RequiredParameter("protection_cluster_network_state")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		m.ProtectionClusterNetworkState = unknownString
	}
	if m.ProtectionClusterNetworkState != "up" && m.ProtectionClusterNetworkState != "down" {
		err = errors.UnavailableParameterValue("protection_cluster_network_state", m.ProtectionClusterNetworkState, []interface{}{"up", "down"})
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)
	}
	if m.RecoveryClusterFloatingIPUUID != nil && *m.RecoveryClusterFloatingIPUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterFloatingIPUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_floating_ip_uuid", m.RecoveryClusterFloatingIPUUID, "UUID")
			logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
				m.ProtectionClusterFloatingIPID, err)
		}
	}

	if m.RecoveryClusterFloatingIPDescription != nil && len(*m.RecoveryClusterFloatingIPDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_floating_ip_description", *m.RecoveryClusterFloatingIPDescription, 255)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		description := (*m.RecoveryClusterFloatingIPDescription)[:255]
		m.RecoveryClusterFloatingIPDescription = &description
	}

	if m.RecoveryClusterFloatingIPIPAddress != nil && *m.RecoveryClusterFloatingIPIPAddress != "" && !govalidator.IsIP(*m.RecoveryClusterFloatingIPIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_floating_ip_ip_address", m.RecoveryClusterFloatingIPIPAddress, "IP")
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)
	}

	if m.RecoveryClusterNetworkUUID != nil && *m.RecoveryClusterNetworkUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterNetworkUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_network_uuid", m.RecoveryClusterNetworkUUID, "UUID")
			logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
				m.ProtectionClusterFloatingIPID, err)
		}
	}

	if m.RecoveryClusterNetworkName != nil && len(*m.RecoveryClusterNetworkName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_network_name", *m.RecoveryClusterNetworkName, 255)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		name := (*m.RecoveryClusterNetworkName)[:255]
		m.RecoveryClusterNetworkName = &name
	}

	if m.RecoveryClusterNetworkDescription != nil && len(*m.RecoveryClusterNetworkDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_network_description", *m.RecoveryClusterNetworkDescription, 255)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		description := (*m.RecoveryClusterNetworkDescription)[:255]
		m.RecoveryClusterNetworkDescription = &description
	}

	if m.RecoveryClusterNetworkTypeCode != nil && len(*m.RecoveryClusterNetworkTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_network_type_code", *m.RecoveryClusterNetworkTypeCode, 100)
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)

		typeCode := (*m.RecoveryClusterNetworkTypeCode)[:100]
		m.RecoveryClusterNetworkTypeCode = &typeCode
	}

	if m.RecoveryClusterNetworkState != nil && *m.RecoveryClusterNetworkState != "up" && *m.RecoveryClusterNetworkState != "down" {
		err = errors.UnavailableParameterValue("recovery_cluster_network_state", m.RecoveryClusterNetworkState, []interface{}{"up", "down"})
		logger.Warnf("[validateFloatingIPRecoveryResult] Error occurred during validating floating IP result: protection floating IP(%d). Cause: %+v",
			m.ProtectionClusterFloatingIPID, err)
	}

	return nil
}

func validateInstanceSpecRecoveryResult(m *model.RecoveryResultInstanceSpec) error {
	var err error

	if m.ProtectionClusterInstanceSpecID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_spec_id")
	}

	if m.ProtectionClusterInstanceSpecUUID == "" {
		err = errors.RequiredParameter("protection_cluster_instance_spec_uuid")
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		m.ProtectionClusterInstanceSpecUUID = unknownString
	}
	if len(m.ProtectionClusterInstanceSpecUUID) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_spec_uuid", m.ProtectionClusterInstanceSpecUUID, 255)
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		m.ProtectionClusterInstanceSpecUUID = m.ProtectionClusterInstanceSpecUUID[:255]
	}

	if m.ProtectionClusterInstanceSpecName == "" {
		err = errors.RequiredParameter("protection_cluster_instance_spec_name")
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		m.ProtectionClusterInstanceSpecName = unknownString
	}
	if len(m.ProtectionClusterInstanceSpecName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_spec_name", m.ProtectionClusterInstanceSpecName, 255)
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		m.ProtectionClusterInstanceSpecName = m.ProtectionClusterInstanceSpecName[:255]
	}

	if m.ProtectionClusterInstanceSpecDescription != nil && len(*m.ProtectionClusterInstanceSpecDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_spec_description", *m.ProtectionClusterInstanceSpecDescription, 255)
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		description := (*m.ProtectionClusterInstanceSpecDescription)[:255]
		m.ProtectionClusterInstanceSpecDescription = &description
	}

	if m.RecoveryClusterInstanceSpecUUID != nil && len(*m.RecoveryClusterInstanceSpecUUID) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_spec_uuid", *m.RecoveryClusterInstanceSpecUUID, 255)
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		u := (*m.RecoveryClusterInstanceSpecUUID)[:255]
		m.RecoveryClusterInstanceSpecUUID = &u
	}

	if m.RecoveryClusterInstanceSpecName != nil && len(*m.RecoveryClusterInstanceSpecName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_spec_name", *m.RecoveryClusterInstanceSpecName, 255)
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		name := (*m.RecoveryClusterInstanceSpecName)[:255]
		m.RecoveryClusterInstanceSpecName = &name
	}

	if m.RecoveryClusterInstanceSpecDescription != nil && len(*m.RecoveryClusterInstanceSpecDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_spec_description", *m.RecoveryClusterInstanceSpecDescription, 255)
		logger.Warnf("[validateInstanceSpecRecoveryResult] Error occurred during validating instance spec result: protection instance spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, err)

		description := (*m.RecoveryClusterInstanceSpecDescription)[:255]
		m.RecoveryClusterInstanceSpecDescription = &description
	}

	return nil
}

func validateExtraSpecRecoveryResult(m *model.RecoveryResultInstanceExtraSpec) error {
	var err error

	if m.ProtectionClusterInstanceSpecID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_spec_id")
	}

	if m.ProtectionClusterInstanceExtraSpecID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_extra_spec_id")
	}

	if m.ProtectionClusterInstanceExtraSpecKey == "" {
		err = errors.RequiredParameter("protection_cluster_instance_extra_spec_key")
		logger.Warnf("[validateExtraSpecRecoveryResult] Error occurred during validating extra spec result: protection instance spec(%d) extra spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, m.ProtectionClusterInstanceExtraSpecID, err)

		m.ProtectionClusterInstanceExtraSpecKey = unknownString
	}
	if len(m.ProtectionClusterInstanceExtraSpecKey) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_extra_spec_key", m.ProtectionClusterInstanceExtraSpecKey, 255)
		logger.Warnf("[validateExtraSpecRecoveryResult] Error occurred during validating extra spec result: protection instance spec(%d) extra spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, m.ProtectionClusterInstanceExtraSpecID, err)

		m.ProtectionClusterInstanceExtraSpecKey = m.ProtectionClusterInstanceExtraSpecKey[:255]
	}

	if m.ProtectionClusterInstanceExtraSpecValue == "" {
		err = errors.RequiredParameter("protection_cluster_instance_extra_spec_value")
		logger.Warnf("[validateExtraSpecRecoveryResult] Error occurred during validating extra spec result: protection instance spec(%d) extra spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, m.ProtectionClusterInstanceExtraSpecID, err)

		m.ProtectionClusterInstanceExtraSpecValue = unknownString
	}
	if len(m.ProtectionClusterInstanceExtraSpecValue) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_extra_spec_value", m.ProtectionClusterInstanceExtraSpecValue, 255)
		logger.Warnf("[validateExtraSpecRecoveryResult] Error occurred during validating extra spec result: protection instance spec(%d) extra spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, m.ProtectionClusterInstanceExtraSpecID, err)

		m.ProtectionClusterInstanceExtraSpecValue = m.ProtectionClusterInstanceExtraSpecValue[:255]
	}

	if m.RecoveryClusterInstanceExtraSpecKey != nil && len(*m.RecoveryClusterInstanceExtraSpecKey) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_extra_spec_key", *m.RecoveryClusterInstanceExtraSpecKey, 255)
		logger.Warnf("[validateExtraSpecRecoveryResult] Error occurred during validating extra spec result: protection instance spec(%d) extra spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, m.ProtectionClusterInstanceExtraSpecID, err)

		key := (*m.RecoveryClusterInstanceExtraSpecKey)[:255]
		m.RecoveryClusterInstanceExtraSpecKey = &key
	}

	if m.RecoveryClusterInstanceExtraSpecValue != nil && len(*m.RecoveryClusterInstanceExtraSpecValue) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_extra_spec_value", *m.RecoveryClusterInstanceExtraSpecValue, 255)
		logger.Warnf("[validateExtraSpecRecoveryResult] Error occurred during validating extra spec result: protection instance spec(%d) extra spec(%d). Cause: %+v",
			m.ProtectionClusterInstanceSpecID, m.ProtectionClusterInstanceExtraSpecID, err)

		value := (*m.RecoveryClusterInstanceExtraSpecValue)[:255]
		m.RecoveryClusterInstanceExtraSpecValue = &value
	}

	return nil
}

func validateKeypairRecoveryResult(m *model.RecoveryResultKeypair) error {
	var err error

	if m.ProtectionClusterKeypairID == 0 {
		return errors.RequiredParameter("protection_cluster_keypair_id")
	}

	if m.ProtectionClusterKeypairName == "" {
		err = errors.RequiredParameter("protection_cluster_keypair_name")
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairName = unknownString
	}
	if len(m.ProtectionClusterKeypairName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_keypair_name", m.ProtectionClusterKeypairName, 255)
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairName = m.ProtectionClusterKeypairName[:255]
	}

	if m.ProtectionClusterKeypairFingerprint == "" {
		err = errors.RequiredParameter("protection_cluster_keypair_fingerprint")
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairFingerprint = unknownString
	}
	if len(m.ProtectionClusterKeypairFingerprint) > 100 {
		err = errors.LengthOverflowParameterValue("protection_cluster_keypair_fingerprint", m.ProtectionClusterKeypairFingerprint, 100)
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairFingerprint = m.ProtectionClusterKeypairFingerprint[:100]
	}

	if m.ProtectionClusterKeypairPublicKey == "" {
		err = errors.RequiredParameter("protection_cluster_keypair_public_key")
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairPublicKey = unknownString
	}
	if len(m.ProtectionClusterKeypairPublicKey) > 2048 {
		err = errors.LengthOverflowParameterValue("protection_cluster_keypair_public_key", m.ProtectionClusterKeypairPublicKey, 2048)
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairPublicKey = m.ProtectionClusterKeypairPublicKey[:2048]
	}

	if m.ProtectionClusterKeypairTypeCode == "" {
		err = errors.RequiredParameter("protection_cluster_keypair_type_code")
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		m.ProtectionClusterKeypairTypeCode = unknownString
	}
	if m.ProtectionClusterKeypairTypeCode != cmsConstant.OpenstackKeypairTypeSSH && m.ProtectionClusterKeypairTypeCode != cmsConstant.OpenstackKeypairTypeX509 {
		err = errors.UnavailableParameterValue("protection_cluster_keypair_type_code", m.ProtectionClusterKeypairTypeCode, []interface{}{cmsConstant.OpenstackKeypairTypeSSH, cmsConstant.OpenstackKeypairTypeX509})
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)
	}

	if m.RecoveryClusterKeypairName != nil && len(*m.RecoveryClusterKeypairName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_keypair_name", *m.RecoveryClusterKeypairName, 255)
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		name := (*m.RecoveryClusterKeypairName)[:255]
		m.RecoveryClusterKeypairName = &name
	}

	if m.RecoveryClusterKeypairFingerprint != nil && len(*m.RecoveryClusterKeypairFingerprint) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_keypair_fingerprint", *m.RecoveryClusterKeypairFingerprint, 100)
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		fingerprint := (*m.RecoveryClusterKeypairFingerprint)[:100]
		m.RecoveryClusterKeypairFingerprint = &fingerprint
	}

	if m.RecoveryClusterKeypairPublicKey != nil && len(*m.RecoveryClusterKeypairPublicKey) > 2048 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_keypair_public_key", *m.RecoveryClusterKeypairPublicKey, 2048)
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)

		key := (*m.RecoveryClusterKeypairPublicKey)[:2048]
		m.RecoveryClusterKeypairPublicKey = &key
	}

	if m.RecoveryClusterKeypairTypeCode != nil && *m.RecoveryClusterKeypairTypeCode != cmsConstant.OpenstackKeypairTypeSSH && *m.RecoveryClusterKeypairTypeCode != cmsConstant.OpenstackKeypairTypeX509 {
		err = errors.UnavailableParameterValue("recovery_cluster_keypair_type_code", m.RecoveryClusterKeypairTypeCode, []interface{}{cmsConstant.OpenstackKeypairTypeSSH, cmsConstant.OpenstackKeypairTypeX509})
		logger.Warnf("[validateKeypairRecoveryResult] Error occurred during validating keypair result: protection keypair(%d). Cause: %+v",
			m.ProtectionClusterKeypairID, err)
	}

	return nil
}

func validateVolumeRecoveryResult(m *model.RecoveryResultVolume) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterVolumeID == 0 {
		return errors.RequiredParameter("protection_cluster_volume_id")
	}

	if m.ProtectionClusterVolumeUUID == "" {
		err = errors.RequiredParameter("protection_cluster_volume_uuid")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterVolumeUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterVolumeUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_volume_uuid", m.ProtectionClusterVolumeUUID, "UUID")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)
	}

	if m.ProtectionClusterVolumeName != nil && len(*m.ProtectionClusterVolumeName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_volume_name", *m.ProtectionClusterVolumeName, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		name := (*m.ProtectionClusterVolumeName)[:255]
		m.ProtectionClusterVolumeName = &name
	}

	if m.ProtectionClusterVolumeDescription != nil && len(*m.ProtectionClusterVolumeDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_volume_description", *m.ProtectionClusterVolumeDescription, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		description := (*m.ProtectionClusterVolumeDescription)[:255]
		m.ProtectionClusterVolumeDescription = &description
	}

	if m.ProtectionClusterVolumeSizeBytes == 0 {
		err = errors.RequiredParameter("protection_cluster_volume_size_bytes")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterVolumeSizeBytes = unknownUint64
	}

	if m.ProtectionClusterStorageID == 0 {
		err = errors.RequiredParameter("protection_cluster_storage_id")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterStorageID = unknownUint64
	}

	if m.ProtectionClusterStorageUUID == "" {
		err = errors.RequiredParameter("protection_cluster_storage_uuid")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterStorageUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterStorageUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_storage_uuid", m.ProtectionClusterStorageUUID, "UUID")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)
	}

	if m.ProtectionClusterStorageName == "" {
		err = errors.RequiredParameter("protection_cluster_storage_name")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterStorageName = unknownString
	}
	if len(m.ProtectionClusterStorageName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_storage_name", m.ProtectionClusterStorageName, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterStorageName = m.ProtectionClusterStorageName[:255]
	}

	if m.ProtectionClusterStorageDescription != nil && len(*m.ProtectionClusterStorageDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_storage_description", *m.ProtectionClusterStorageDescription, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		description := (*m.ProtectionClusterStorageDescription)[:255]
		m.ProtectionClusterStorageDescription = &description
	}

	if m.ProtectionClusterStorageTypeCode == "" {
		err = errors.RequiredParameter("protection_cluster_storage_type_code")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterStorageTypeCode = unknownString
	}
	if len(m.ProtectionClusterStorageTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("protection_cluster_storage_type_code", m.ProtectionClusterStorageTypeCode, 100)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterStorageTypeCode = m.ProtectionClusterStorageTypeCode[:100]
	}

	if m.RecoveryClusterVolumeUUID != nil && *m.RecoveryClusterVolumeUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterVolumeUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_volume_uuid", m.RecoveryClusterVolumeUUID, "UUID")
			logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)
		}
	}

	if m.RecoveryClusterVolumeName != nil && len(*m.RecoveryClusterVolumeName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_volume_name", *m.RecoveryClusterVolumeName, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		name := (*m.RecoveryClusterVolumeName)[:255]
		m.RecoveryClusterVolumeName = &name
	}

	if m.RecoveryClusterVolumeDescription != nil && len(*m.RecoveryClusterVolumeDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_volume_description", *m.RecoveryClusterVolumeDescription, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		description := (*m.RecoveryClusterVolumeDescription)[:255]
		m.RecoveryClusterVolumeDescription = &description
	}

	if m.RecoveryClusterStorageUUID != nil && *m.RecoveryClusterStorageUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterStorageUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_storage_uuid", m.RecoveryClusterStorageUUID, "UUID")
			logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)
		}
	}

	if m.RecoveryClusterStorageName != nil && len(*m.RecoveryClusterStorageName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_storage_name", *m.RecoveryClusterStorageName, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		name := (*m.RecoveryClusterStorageName)[:255]
		m.RecoveryClusterStorageName = &name
	}

	if m.RecoveryClusterStorageDescription != nil && len(*m.RecoveryClusterStorageDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_storage_description", *m.RecoveryClusterStorageDescription, 255)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		description := (*m.RecoveryClusterStorageDescription)[:255]
		m.RecoveryClusterStorageDescription = &description
	}

	if m.RecoveryClusterStorageTypeCode != nil && len(*m.RecoveryClusterStorageTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_storage_type_code", *m.RecoveryClusterStorageTypeCode, 100)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		typeCode := (*m.RecoveryClusterStorageTypeCode)[:100]
		m.RecoveryClusterStorageTypeCode = &typeCode
	}

	if m.RecoveryPointTypeCode == "" {
		err = errors.RequiredParameter("recovery_point_type_code")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.RecoveryPointTypeCode = unknownString
	}
	if len(m.RecoveryPointTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_point_type_code", m.RecoveryPointTypeCode, 100)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.RecoveryPointTypeCode = m.RecoveryPointTypeCode[:100]
	}

	if m.RecoveryPoint == 0 {
		err = errors.RequiredParameter("recovery_point")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.RecoveryPoint = unknownInt64
	}

	if m.ResultCode == "" {
		err = errors.RequiredParameter("result_code")
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ResultCode = unknownString
	}
	if len(m.ResultCode) > 100 {
		err = errors.LengthOverflowParameterValue("result_code", m.ResultCode, 100)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		m.ResultCode = m.ResultCode[:100]
	}

	if m.FailedReasonCode != nil && len(*m.FailedReasonCode) > 100 {
		err = errors.LengthOverflowParameterValue("failed_reason_code", *m.FailedReasonCode, 100)
		logger.Warnf("[validateVolumeRecoveryResult] Error occurred during validating volume result: tenant(%d) protection volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterVolumeID, err)

		code := (*m.FailedReasonCode)[:100]
		m.FailedReasonCode = &code
	}

	return nil
}

func validateInstanceRecoveryResult(m *model.RecoveryResultInstance) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterInstanceID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_id")
	}

	if m.ProtectionClusterInstanceUUID == "" {
		err = errors.RequiredParameter("protection_cluster_instance_uuid")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterInstanceUUID = unknownString
	}
	if _, err = uuid.Parse(m.ProtectionClusterInstanceUUID); err != nil {
		err = errors.FormatMismatchParameterValue("protection_cluster_instance_uuid", m.ProtectionClusterInstanceUUID, "UUID")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)
	}

	if m.ProtectionClusterInstanceName == "" {
		err = errors.RequiredParameter("protection_cluster_instance_name")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterInstanceName = unknownString
	}
	if len(m.ProtectionClusterInstanceName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_name", m.ProtectionClusterInstanceName, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterInstanceName = m.ProtectionClusterInstanceName[:255]
	}

	if m.ProtectionClusterInstanceDescription != nil && len(*m.ProtectionClusterInstanceDescription) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_instance_description", *m.ProtectionClusterInstanceDescription, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		description := (*m.ProtectionClusterInstanceDescription)[:255]
		m.ProtectionClusterInstanceDescription = &description
	}

	if m.ProtectionClusterInstanceSpecID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_spec_id")
	}

	if m.ProtectionClusterAvailabilityZoneID == 0 {
		err = errors.RequiredParameter("protection_cluster_availability_zone_id")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterAvailabilityZoneID = unknownUint64
	}

	if m.ProtectionClusterAvailabilityZoneName == "" {
		err = errors.RequiredParameter("protection_cluster_availability_zone_name")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterAvailabilityZoneName = unknownString
	}
	if len(m.ProtectionClusterAvailabilityZoneName) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_availability_zone_name", m.ProtectionClusterAvailabilityZoneName, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterAvailabilityZoneName = m.ProtectionClusterAvailabilityZoneName[:255]
	}

	if m.ProtectionClusterHypervisorID == 0 {
		err = errors.RequiredParameter("protection_cluster_hypervisor_id")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterHypervisorID = unknownUint64
	}

	if m.ProtectionClusterHypervisorTypeCode == "" {
		err = errors.RequiredParameter("protection_cluster_hypervisor_type_code")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterHypervisorTypeCode = unknownString
	}
	if len(m.ProtectionClusterHypervisorTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("protection_cluster_hypervisor_type_code", m.ProtectionClusterHypervisorTypeCode, 100)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterHypervisorTypeCode = m.ProtectionClusterHypervisorTypeCode[:100]
	}

	if m.ProtectionClusterHypervisorHostname == "" {
		err = errors.RequiredParameter("protection_cluster_hypervisor_hostname")

		m.ProtectionClusterHypervisorHostname = unknownString
	}
	if len(m.ProtectionClusterHypervisorHostname) > 255 {
		err = errors.LengthOverflowParameterValue("protection_cluster_hypervisor_hostname", m.ProtectionClusterHypervisorHostname, 255)

		m.ProtectionClusterHypervisorHostname = m.ProtectionClusterHypervisorHostname[:255]
	}

	if m.ProtectionClusterHypervisorIPAddress == "" {
		err = errors.RequiredParameter("protection_cluster_hypervisor_ip_address")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ProtectionClusterHypervisorIPAddress = unknownString
	}
	if !govalidator.IsIP(m.ProtectionClusterHypervisorIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_hypervisor_ip_address", m.ProtectionClusterHypervisorIPAddress, "IP")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)
	}

	if m.RecoveryClusterInstanceUUID != nil && *m.RecoveryClusterInstanceUUID != "" {
		if _, err = uuid.Parse(*m.RecoveryClusterInstanceUUID); err != nil {
			err = errors.FormatMismatchParameterValue("recovery_cluster_instance_uuid", m.RecoveryClusterInstanceUUID, "UUID")
			logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
				m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)
		}
	}

	if m.RecoveryClusterInstanceName != nil && len(*m.RecoveryClusterInstanceName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_name", *m.RecoveryClusterInstanceName, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		name := (*m.RecoveryClusterInstanceName)[:255]
		m.RecoveryClusterInstanceName = &name
	}

	if m.RecoveryClusterInstanceDescription != nil && len(*m.RecoveryClusterInstanceDescription) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_instance_description", *m.RecoveryClusterInstanceDescription, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		description := (*m.RecoveryClusterInstanceDescription)[:255]
		m.RecoveryClusterInstanceDescription = &description
	}

	if m.RecoveryClusterAvailabilityZoneName != nil && len(*m.RecoveryClusterAvailabilityZoneName) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_availability_zone_name", *m.RecoveryClusterAvailabilityZoneName, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		name := (*m.RecoveryClusterAvailabilityZoneName)[:255]
		m.RecoveryClusterAvailabilityZoneName = &name
	}

	if m.RecoveryClusterHypervisorTypeCode != nil && len(*m.RecoveryClusterHypervisorTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_hypervisor_type_code", *m.RecoveryClusterHypervisorTypeCode, 100)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		typeCode := (*m.RecoveryClusterHypervisorTypeCode)[:100]
		m.RecoveryClusterHypervisorTypeCode = &typeCode
	}

	if m.RecoveryClusterHypervisorHostname != nil && len(*m.RecoveryClusterHypervisorHostname) > 255 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_hypervisor_hostname", *m.RecoveryClusterHypervisorHostname, 255)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		name := (*m.RecoveryClusterHypervisorHostname)[:255]
		m.RecoveryClusterHypervisorHostname = &name
	}

	if m.RecoveryClusterHypervisorIPAddress != nil &&
		*m.RecoveryClusterHypervisorIPAddress != "" &&
		!govalidator.IsIP(*m.RecoveryClusterHypervisorIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_hypervisor_ip_address", m.RecoveryClusterHypervisorIPAddress, "IP")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)
	}

	if m.RecoveryPointTypeCode == "" {
		err = errors.RequiredParameter("recovery_point_type_code")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.RecoveryPointTypeCode = unknownString
	}
	if len(m.RecoveryPointTypeCode) > 100 {
		err = errors.LengthOverflowParameterValue("recovery_point_type_code", m.RecoveryPointTypeCode, 100)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.RecoveryPointTypeCode = m.RecoveryPointTypeCode[:100]
	}

	if m.RecoveryPoint == 0 {
		err = errors.RequiredParameter("recovery_point")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.RecoveryPoint = unknownInt64
	}

	if m.DiagnosisMethodCode != nil && len(*m.DiagnosisMethodCode) > 100 {
		err = errors.LengthOverflowParameterValue("diagnosis_method_code", *m.DiagnosisMethodCode, 100)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		code := (*m.DiagnosisMethodCode)[:100]
		m.DiagnosisMethodCode = &code
	}

	if m.DiagnosisMethodData != nil && len(*m.DiagnosisMethodData) > 300 {
		err = errors.LengthOverflowParameterValue("diagnosis_method_data", *m.DiagnosisMethodData, 300)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		data := (*m.DiagnosisMethodData)[:300]
		m.DiagnosisMethodData = &data
	}

	if m.ResultCode == "" {
		err = errors.RequiredParameter("result_code")
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ResultCode = unknownString
	}
	if len(m.ResultCode) > 100 {
		err = errors.LengthOverflowParameterValue("result_code", m.ResultCode, 100)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		m.ResultCode = m.ResultCode[:100]
	}

	if m.FailedReasonCode != nil && len(*m.FailedReasonCode) > 100 {
		err = errors.LengthOverflowParameterValue("failed_reason_code", *m.FailedReasonCode, 100)
		logger.Warnf("[validateInstanceRecoveryResult] Error occurred during validating instance result: tenant(%d) protection instance(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, err)

		code := (*m.FailedReasonCode)[:100]
		m.FailedReasonCode = &code
	}

	return nil
}

func validateInstanceVolumeRecoveryResult(m *model.RecoveryResultInstanceVolume) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterInstanceID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_id")
	}

	if m.ProtectionClusterVolumeID == 0 {
		return errors.RequiredParameter("protection_cluster_volume_id")
	}

	if m.ProtectionClusterDevicePath == "" {
		err = errors.RequiredParameter("protection_cluster_device_path")
		logger.Warnf("[validateInstanceVolumeRecoveryResult] Error occurred during validating instance volume result: tenant(%d) protection instance(%d) volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterDevicePath = unknownString
	}
	if len(m.ProtectionClusterDevicePath) > 4096 {
		err = errors.LengthOverflowParameterValue("protection_cluster_device_path", m.ProtectionClusterDevicePath, 4096)
		logger.Warnf("[validateInstanceVolumeRecoveryResult] Error occurred during validating instance volume result: tenant(%d) protection instance(%d) volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, m.ProtectionClusterVolumeID, err)

		m.ProtectionClusterDevicePath = m.ProtectionClusterDevicePath[:4096]
	}

	if m.RecoveryClusterDevicePath != nil && len(*m.RecoveryClusterDevicePath) > 4096 {
		err = errors.LengthOverflowParameterValue("recovery_cluster_device_path", *m.RecoveryClusterDevicePath, 4096)
		logger.Warnf("[validateInstanceVolumeRecoveryResult] Error occurred during validating instance volume result: tenant(%d) protection instance(%d) volume(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, m.ProtectionClusterVolumeID, err)

		path := (*m.RecoveryClusterDevicePath)[:4096]
		m.RecoveryClusterDevicePath = &path
	}

	return nil
}

func validateInstanceNetworkRecoveryResult(m *model.RecoveryResultInstanceNetwork) error {
	var err error

	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterInstanceID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_id")
	}

	if m.ProtectionClusterNetworkID == 0 {
		return errors.RequiredParameter("protection_cluster_network_id")
	}

	if m.ProtectionClusterSubnetID == 0 {
		return errors.RequiredParameter("protection_cluster_subnet_id")
	}

	if m.ProtectionClusterFloatingIPID != nil && *m.ProtectionClusterFloatingIPID == 0 {
		return errors.RequiredParameter("protection_cluster_floating_ip_id")
	}

	if m.ProtectionClusterIPAddress == "" {
		err = errors.RequiredParameter("protection_cluster_ip_address")
		logger.Warnf("[validateInstanceNetworkRecoveryResult] Error occurred during validating instance network result: tenant(%d) protection instance(%d) network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, m.ProtectionClusterNetworkID, err)

		m.ProtectionClusterIPAddress = unknownString
	}
	if !govalidator.IsIP(m.ProtectionClusterIPAddress) {
		err = errors.FormatMismatchParameterValue("protection_cluster_ip_address", m.ProtectionClusterIPAddress, "IP")
		logger.Warnf("[validateInstanceNetworkRecoveryResult] Error occurred during validating instance network result: tenant(%d) protection instance(%d) network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, m.ProtectionClusterNetworkID, err)
	}

	if m.RecoveryClusterIPAddress != nil && *m.RecoveryClusterIPAddress != "" && !govalidator.IsIP(*m.RecoveryClusterIPAddress) {
		err = errors.FormatMismatchParameterValue("recovery_cluster_ip_address", m.RecoveryClusterIPAddress, "IP")
		logger.Warnf("[validateInstanceNetworkRecoveryResult] Error occurred during validating instance network result: tenant(%d) protection instance(%d) network(%d). Cause: %+v",
			m.ProtectionClusterTenantID, m.ProtectionClusterInstanceID, m.ProtectionClusterNetworkID, err)
	}

	return nil
}

func validateInstanceSecurityGroupRecoveryResult(m *model.RecoveryResultInstanceSecurityGroup) error {
	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterInstanceID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_id")
	}

	if m.ProtectionClusterSecurityGroupID == 0 {
		return errors.RequiredParameter("protection_cluster_security_group_id")
	}

	return nil
}

func validateInstanceDependencyRecoveryResult(m *model.RecoveryResultInstanceDependency) error {
	if m.ProtectionClusterTenantID == 0 {
		return errors.RequiredParameter("protection_cluster_tenant_id")
	}

	if m.ProtectionClusterInstanceID == 0 {
		return errors.RequiredParameter("protection_cluster_instance_id")
	}

	if m.DependencyProtectionClusterInstanceID == 0 {
		return errors.RequiredParameter("dependency_protection_cluster_instance_id")
	}

	return nil
}

func validateRecoveryReportListRequest(req *drms.RecoveryReportListRequest) error {
	if req.GetGroupId() == 0 {
		return errors.RequiredParameter("group_id")
	}

	if len(req.GetGroupName()) == 0 {
		return errors.RequiredParameter("group_name")
	}

	if len(req.GetType()) != 0 && !internal.IsRecoveryJobTypeCode(req.GetType()) {
		return errors.UnavailableParameterValue("type", req.GetType(), internal.RecoveryJobTypeCodes)
	}

	if len(req.GetResult()) != 0 && !internal.IsRecoveryJobResult(req.GetResult()) {
		return errors.UnavailableParameterValue("result", req.GetResult(), internal.RecoveryJobResults)
	}

	return nil
}

func checkDeletableRecoveryResult(ctx context.Context, req *drms.DeleteRecoveryReportRequest) error {
	var result model.RecoveryResult
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResult{ID: req.ResultId, ProtectionGroupID: req.GroupId}).First(&result, req.ResultId).Error
	}); err != nil {
		switch {
		case err == gorm.ErrRecordNotFound:
			return internal.NotFoundRecoveryResult(req.ResultId)

		case err != nil:
			return errors.UnusableDatabase(err)
		}
	}

	if err := internal.IsAccessibleRecoveryReport(ctx, result.OwnerGroupID); err != nil {
		return err
	}

	if result.RecoveryTypeCode != constant.RecoveryTypeCodeSimulation {
		return internal.UndeletableRecoveryResult(req.ResultId, result.RecoveryTypeCode)
	}

	return nil
}
