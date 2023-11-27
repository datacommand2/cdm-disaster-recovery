package snapshot

import (
	"10.1.1.220/cdm/cdm-center/services/cluster-manager/storage"
	"10.1.1.220/cdm/cdm-center/services/cluster-manager/volume"
	"10.1.1.220/cdm/cdm-cloud/common/store"
	"10.1.1.220/cdm/cdm-cloud/common/test/helper"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

var (
	pool               = "test"
	client             = "mirror.test"
	adminClient        = "admin_test"
	targetKeyring      = "AQA3xLVgRIA+AhAAm+OpQjJWqOAVyld/7BgxGg=="
	targetAdminKeyring = "AQBiIl1hznV7BBAAKDb/4+/78kb23dha8AgBbQ=="
	targetCephConf     = `
[global]
cluster network = 192.168.64.0/24
fsid = d1a4e34e-cc52-48a4-aa8a-45863a905d65
mon host = [v2:192.168.64.174:3300,v1:192.168.64.174:6789]
mon initial members = targetceph1
osd pool default crush rule = -1
public network = 192.168.64.0/24

[osd]

`

	targetCephMetadata = map[string]interface{}{
		"pool":          pool,
		"conf":          targetCephConf,
		"client":        client,
		"keyring":       targetKeyring,
		"admin_client":  adminClient,
		"admin_keyring": targetAdminKeyring,
	}
)

var (
	targetExports     = []string{uuid.New().String()[:12], uuid.New().String()[:12], uuid.New().String()[:12]}
	targetNFSMetadata = map[string]interface{}{
		"exports": targetExports,
	}
)

func setup() {
	if err := helper.Init(); err != nil {
		panic(err)
	}
	_ = store.Delete(planSnapshotBase, store.DeletePrefix())
}

func teardown() {
	helper.Close()
}

func TestMain(m *testing.M) {
	setup()
	_ = m.Run()
	teardown()
}

func TestMirrorVolumeGetAndSet(t *testing.T) {
	v := &mirror.Volume{
		SourceClusterStorage: &storage.ClusterStorage{
			StorageID: rand.Uint64(),
			ClusterID: rand.Uint64(),
		},
		TargetClusterStorage: &storage.ClusterStorage{
			StorageID: rand.Uint64(),
			ClusterID: rand.Uint64(),
		},
		SourceVolume: &volume.ClusterVolume{
			ClusterID:  rand.Uint64(),
			VolumeID:   rand.Uint64(),
			VolumeUUID: uuid.New().String()[:15],
		},
		SourceAgent: &mirror.Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}

	p := &PlanSnapshotInfo{
		PlanID:                    rand.Uint64(),
		ProtectionGroupSnapshotID: rand.Uint64(),
		VolumeID:                  rand.Uint64(),
	}

	assert.NoError(t, p.SetMirrorVolume(v))
	mv, err := p.GetMirrorVolume()
	assert.NoError(t, err)
	assert.Equal(t, v, mv)
}

func TestTargetMetadataGetAndSet(t *testing.T) {
	p := &PlanSnapshotInfo{
		PlanID:                    rand.Uint64(),
		ProtectionGroupSnapshotID: rand.Uint64(),
		VolumeID:                  rand.Uint64(),
	}

	// ceph
	assert.NoError(t, p.SetTargetMetadata(targetCephMetadata))
	md, err := p.GetTargetMetadata()
	assert.NoError(t, err)
	assert.Equal(t, targetCephMetadata, md)

	// NFS
	assert.NoError(t, p.SetTargetMetadata(targetNFSMetadata))
	md, err = p.GetTargetMetadata()
	assert.NoError(t, err)
	assert.Equal(t, targetNFSMetadata, md)
}

func TestDelete(t *testing.T) {
	p := &PlanSnapshotInfo{
		PlanID:                    rand.Uint64(),
		ProtectionGroupSnapshotID: rand.Uint64(),
		VolumeID:                  rand.Uint64(),
	}

	// ceph
	assert.NoError(t, p.SetTargetMetadata(targetCephMetadata))
	md, err := p.GetTargetMetadata()
	assert.NoError(t, err)
	assert.Equal(t, targetCephMetadata, md)

	// NFS
	assert.NoError(t, p.SetTargetMetadata(targetNFSMetadata))
	md, err = p.GetTargetMetadata()
	assert.NoError(t, err)
	assert.Equal(t, targetNFSMetadata, md)

	// delete
	assert.NoError(t, p.Delete())
}
