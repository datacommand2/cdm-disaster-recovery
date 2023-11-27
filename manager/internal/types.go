package internal

import (
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
)

// UnavailableInstance 재해 복구가 불가능한 인스턴스
type UnavailableInstance struct {
	Instance           *cms.ClusterInstance `json:"instance,omitempty"`
	UnavailableReasons []*drms.Message      `json:"unavailable_reasons,omitempty"`
}

// ScheduleMessage 스케줄 메시지
type ScheduleMessage struct {
	ProtectionGroupID uint64 `json:"protection_group_id,omitempty"`
	RecoveryJobID     uint64 `json:"recovery_job_id,omitempty"`
}

// SyncPlanMessage plan 동기화 메시지
type SyncPlanMessage struct {
	ProtectionGroupID uint64 `json:"protection_group_id,omitempty"`
	RecoveryPlanID    uint64 `json:"recovery_plan_id,omitempty"`
}
