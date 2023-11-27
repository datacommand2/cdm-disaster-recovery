package snapshot

import (
	"context"
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	sms "github.com/datacommand2/cdm-disaster-recovery/snapshot/proto"
)

// CreateSnapshot 는 보호 그룹 스냅샷, 복구 계획 스냅샷, 볼륨 스냅샷 생성 요청 함수
func CreateSnapshot(ctx context.Context, modelPG *model.ProtectionGroup, pgSnapshot *drms.ProtectionGroupSnapshot, plans []*drms.RecoveryPlan, cgSnapshotUUIDs []string) (*drms.ProtectionGroupSnapshot, error) {
	//TODO: Not implemented
	return nil, nil
}

// DeleteSnapshot 보호 그룹 스냅샷, 복구 계획 스냅샷, 볼륨 스냅샷 삭제 요청 함수
func DeleteSnapshot(ctx context.Context, pgid, pgsid uint64) error {
	//TODO: Not implemented
	return nil
}

// GetProtectionGroupSnapshotList 보호 그룹 스냅샷 목록 조회 요청 함수
func GetProtectionGroupSnapshotList(ctx context.Context, pgid uint64) ([]*drms.ProtectionGroupSnapshot, error) {
	//TODO: Not implemented
	return nil, nil
}

// GetProtectionGroupSnapshotListByPlan 복구 계획의 보호 그룹 스냅샷 목록 조회 요청 함수
func GetProtectionGroupSnapshotListByPlan(ctx context.Context, pgid, pid uint64) ([]*drms.ProtectionGroupSnapshot, error) {
	//TODO: Not implemented
	return nil, nil
}

// GetProtectionGroupPlans 보호 그룹 스냅샷으로 생성된 복구 계획 목록 조회 요청 함수
func GetProtectionGroupPlans(ctx context.Context, pgid uint64) ([]uint64, error) {
	//TODO: Not implemented
	return nil, nil

}

// GetPlanSnapshot 복구 계획 스냅샷 상세 조회 요청 함수
func GetPlanSnapshot(ctx context.Context, pgid, pgsid, pid uint64) (*drms.RecoveryPlan, error) {
	//TODO: Not implemented
	return nil, nil
}

// DeletePlanAllSnapshots 복구 계획의 전체 스냅샷 삭제 요청 함수
func DeletePlanAllSnapshots(ctx context.Context, pgid, pid uint64) error {
	//TODO: Not implemented
	return nil
}

// DeleteAllSnapshots 보호 그룹의 스냅샷 전체 삭제 요청 함수
func DeleteAllSnapshots(ctx context.Context, pgid uint64) error {
	//TODO: Not implemented
	return nil
}

func GetVolumeSnapshotMetadata(ctx context.Context, planID, snapshotID, volumeID uint64) (map[string]string, error) {
	//TODO: Not implemented
	return nil, nil
}

func CheckVolumeHasParent(ctx context.Context, vol *cms.ClusterVolume) (bool, error) {
	//TODO: Not implemented
	return false, nil
}

func FlattenVolumes(ctx context.Context, volList []*cms.ClusterVolume) (*sms.FlattenVolumeResponse, error) {
	//TODO: Not implemented
	return nil, nil
}
