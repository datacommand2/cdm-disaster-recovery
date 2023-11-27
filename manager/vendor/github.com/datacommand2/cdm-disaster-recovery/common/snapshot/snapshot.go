package snapshot

import (
	"encoding/json"
	"fmt"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/mirror"
	"path"
)

const (
	planSnapshotBase      = "dr.plan/%d/snapshot/%d/volume/%d"
	planSnapshotKeyPrefix = "dr.plan/%d/snapshot/%d/volume/%d/"
)

// PlanSnapshotInfo 특정 플랜의 보호 그룹 스냅샷 시점의 mirror volume 정보
type PlanSnapshotInfo struct {
	PlanID                    uint64
	ProtectionGroupSnapshotID uint64
	VolumeID                  uint64
}

// SetMirrorVolume 특정 플랜의 보호 그룹 스냅샷 시점의 mirror volume 정보를 etcd 에 저장한다.
func (p *PlanSnapshotInfo) SetMirrorVolume(mv *mirror.Volume) error {
	b, err := json.Marshal(mv)
	if err != nil {
		return errors.Unknown(err)
	}

	if err = store.Put(path.Join(fmt.Sprintf(planSnapshotBase, p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID), "mirror", "volume"), string(b)); err != nil {
		return errors.UnusableStore(err)
	}

	return nil
}

// GetMirrorVolume 특정 플랜의 보호 그룹 스냅샷 시점의 mirror volume 정보를 가져온다.
func (p *PlanSnapshotInfo) GetMirrorVolume() (*mirror.Volume, error) {
	s, err := store.Get(path.Join(fmt.Sprintf(planSnapshotBase, p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID), "mirror", "volume"))
	switch {
	case err == store.ErrNotFoundKey:
		return nil, NotFoundMirrorVolume(p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID)
	case err != nil:
		return nil, errors.UnusableStore(err)
	}

	var v mirror.Volume
	if err = json.Unmarshal([]byte(s), &v); err != nil {
		return nil, errors.Unknown(err)
	}

	return &v, nil
}

// SetTargetMetadata 특정 플랜의 보호 그룹 스냅샷 시점의 target metadata 정보를 etcd 에 저장한다.
func (p *PlanSnapshotInfo) SetTargetMetadata(metadata map[string]interface{}) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return errors.Unknown(err)
	}

	if err = store.Put(path.Join(fmt.Sprintf(planSnapshotBase, p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID), "target", "metadata"), string(b)); err != nil {
		return errors.UnusableStore(err)
	}

	return nil
}

// GetTargetMetadata 특정 플랜의 보호 그룹 스냅샷 시점의 target metadata 정보를 가져온다.
func (p *PlanSnapshotInfo) GetTargetMetadata() (map[string]interface{}, error) {
	s, err := store.Get(path.Join(fmt.Sprintf(planSnapshotBase, p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID), "target", "metadata"))
	switch {
	case err == store.ErrNotFoundKey:
		return nil, NotFoundMirrorVolumeTargetMetadata(p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID)
	case err != nil:
		return nil, errors.Unknown(err)
	}

	var m = make(map[string]interface{})
	if err = json.Unmarshal([]byte(s), &m); err != nil {
		return nil, errors.Unknown(err)
	}

	return m, nil
}

// Delete 특정 플랜의 보호 그룹 스냅샷 시점의 볼륨 정보를 삭제한다.
func (p *PlanSnapshotInfo) Delete() error {
	// dr.plan/%d/snapshot/%d/volume/%d/ 형태의 key prefix 옵션으로 삭제
	if err := store.Delete(fmt.Sprintf(planSnapshotKeyPrefix, p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID), store.DeletePrefix()); err != nil {
		return errors.Unknown(err)
	}
	// dr.plan/%d/snapshot/%d/volume/%d 해당 key 삭제
	return store.Delete(fmt.Sprintf(planSnapshotBase, p.PlanID, p.ProtectionGroupSnapshotID, p.VolumeID))
}
