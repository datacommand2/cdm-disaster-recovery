package protectiongroup

import (
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"

	"context"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
)

const (
	// SnapshotNamePrefix 스냅샷 이름의 Prefix
	SnapshotNamePrefix = "CDM-DR"
)

// AddSnapshot 보호그룹 스냅샷 생성
func AddSnapshot(ctx context.Context, pgid uint64, runtime int64) error {
	return nil
}

// DeleteSnapshot 보호그룹 스냅샷 삭제
func DeleteSnapshot(ctx context.Context, req *drms.DeleteProtectionGroupSnapshotRequest) error {
	return nil
}

// 보호 그룹 스냅샷 삭제 이벤트를 publish 한다.
func deleteProtectionGroupSnapshot(pgID uint64) error {
	if err := internal.PublishMessage(constant.QueueDeleteProtectionGroupSnapshot, pgID); err != nil {
		return errors.UnusableBroker(err)
	}

	logger.Infof("[deleteProtectionGroupSnapshot] Publish Queue: delete protection group(%d) snapshot", pgID)

	return nil
}
