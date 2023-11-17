package internal

import "github.com/datacommand2/cdm-disaster-recovery/common/constant"

// TenantRecoveryPlanTypeCodes 테넌트 복구유형 코드
var TenantRecoveryPlanTypeCodes = []interface{}{
	constant.TenantRecoveryPlanTypeMirroring,
}

// IsTenantRecoveryPlanTypeCode 테넌트 복구유형 코드 여부 확인
func IsTenantRecoveryPlanTypeCode(code string) bool {
	for _, c := range TenantRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// AvailabilityZoneRecoveryPlanTypeCodes 가용구역 복구유형 코드
var AvailabilityZoneRecoveryPlanTypeCodes = []interface{}{
	constant.AvailabilityZoneRecoveryPlanTypeMapping,
}

// IsAvailabilityZoneRecoveryPlanTypeCode 가용구역 복구유형 코드 여부 확인
func IsAvailabilityZoneRecoveryPlanTypeCode(code string) bool {
	for _, c := range AvailabilityZoneRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// ExternalNetworkRecoveryPlanTypeCodes 외부 네트워크 복구유형 코드
var ExternalNetworkRecoveryPlanTypeCodes = []interface{}{
	constant.ExternalNetworkRecoveryPlanTypeMapping,
}

// IsExternalNetworkRecoveryPlanTypeCode 외부 네트워크 복구유형 코드 여부 확인
func IsExternalNetworkRecoveryPlanTypeCode(code string) bool {
	for _, c := range ExternalNetworkRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// RouterRecoveryPlanTypeCodes 라우터 복구유형 코드
var RouterRecoveryPlanTypeCodes = []interface{}{
	constant.RouterRecoveryPlanTypeMirroring,
}

// IsRouterRecoveryPlanTypeCode 라우터 복구유형 코드 여부 확인
func IsRouterRecoveryPlanTypeCode(code string) bool {
	for _, c := range RouterRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// StorageRecoveryPlanTypeCodes 스토리지 복구유형 코드
var StorageRecoveryPlanTypeCodes = []interface{}{
	constant.StorageRecoveryPlanTypeMapping,
}

// IsStorageRecoveryPlanTypeCode 스토리지 복구유형 코드 여부 확인
func IsStorageRecoveryPlanTypeCode(code string) bool {
	for _, c := range StorageRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// InstanceRecoveryPlanTypeCodes 인스턴스 복구유형 코드
var InstanceRecoveryPlanTypeCodes = []interface{}{
	constant.InstanceRecoveryPlanTypeMirroring,
}

// IsInstanceRecoveryPlanTypeCode 인스턴스 복구유형 코드 여부 확인
func IsInstanceRecoveryPlanTypeCode(code string) bool {
	for _, c := range InstanceRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// VolumeRecoveryPlanTypeCodes 볼륨 복구유형 코드
var VolumeRecoveryPlanTypeCodes = []interface{}{
	constant.VolumeRecoveryPlanTypeMirroring,
}

// IsVolumeRecoveryPlanTypeCode 볼륨 복구유형 코드 여부 확인
func IsVolumeRecoveryPlanTypeCode(code string) bool {
	for _, c := range VolumeRecoveryPlanTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// InstanceDiagnosisMethodCodes 인스턴스 진단방식 코드
var InstanceDiagnosisMethodCodes = []interface{}{
	constant.InstanceDiagnosisMethodShellScript,
	constant.InstanceDiagnosisMethodPortScan,
	constant.InstanceDiagnosisMethodHTTPGet,
}

// IsInstanceDiagnosisMethodCode 인스턴스 진단방식 코드 여부 확인
func IsInstanceDiagnosisMethodCode(code string) bool {
	for _, c := range InstanceDiagnosisMethodCodes {
		if c == code {
			return true
		}
	}
	return false
}

// RecoveryJobTypeCodes 재해 복구 작업 종류
var RecoveryJobTypeCodes = []interface{}{
	constant.RecoveryTypeCodeSimulation,
	constant.RecoveryTypeCodeMigration,
}

// IsRecoveryJobTypeCode 재해 복구 작업 종류 확인
func IsRecoveryJobTypeCode(code string) bool {
	for _, c := range RecoveryJobTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}

// RecoveryJobResults 재해 복구 결과
var RecoveryJobResults = []interface{}{
	constant.RecoveryResultCodeSuccess,
	constant.RecoveryResultCodePartialSuccess,
	constant.RecoveryResultCodeFailed,
	constant.RecoveryResultCodeCanceled,
}

// IsRecoveryJobResult 재해 복구 결과 확인
func IsRecoveryJobResult(result string) bool {
	for _, r := range RecoveryJobResults {
		if r == result {
			return true
		}
	}
	return false
}

// RecoveryPointTypeCodes 재해 복구 시점
var RecoveryPointTypeCodes = []interface{}{
	// 최신 데이터
	constant.RecoveryPointTypeCodeLatest,
	// 최신 스냅샷
	constant.RecoveryPointTypeCodeLatestSnapshot,
	// 특정 스냅샷(즉시 실행)
	constant.RecoveryPointTypeCodeSnapshot,
}

// IsRecoveryPointTypeCode 재해 복구 시점 종류 확인
func IsRecoveryPointTypeCode(code string) bool {
	for _, c := range RecoveryPointTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}
