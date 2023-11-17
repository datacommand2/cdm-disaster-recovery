package mirror

import (
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-center/cluster-manager/volume"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"math/rand"
	"testing"
)

func panicIfError(err error) {
	if err != nil {
		panic(err)
	}
}

func TestVolumePutAndGet(t *testing.T) {
	volumes := []*Volume{
		{
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
			SourceAgent: &Agent{
				IP:   uuid.New().String()[:15],
				Port: uint(rand.Uint32()),
			},
		},
		{
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
			SourceAgent: &Agent{
				IP:   uuid.New().String()[:15],
				Port: uint(rand.Uint32()),
			},
		},
		{
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
			SourceAgent: &Agent{
				IP:   uuid.New().String()[:15],
				Port: uint(rand.Uint32()),
			},
		},
	}

	for _, v := range volumes {
		assert.NoError(t, v.Put())
	}
	list, err := GetVolumeList()
	assert.NoError(t, err)
	assert.ElementsMatch(t, list, volumes)
}

func TestVolume_GetMirrorEnvironment(t *testing.T) {
	sourceStorage := &storage.ClusterStorage{
		StorageID: rand.Uint64(),
		ClusterID: rand.Uint64(),
	}
	targetStorage := &storage.ClusterStorage{
		StorageID: rand.Uint64(),
		ClusterID: rand.Uint64(),
	}
	e := &Environment{
		SourceClusterStorage: sourceStorage,
		TargetClusterStorage: targetStorage,
	}
	assert.NoError(t, e.Put())

	v := &Volume{
		SourceClusterStorage: sourceStorage,
		TargetClusterStorage: targetStorage,
		SourceVolume: &volume.ClusterVolume{
			ClusterID:  rand.Uint64(),
			VolumeID:   rand.Uint64(),
			VolumeUUID: uuid.New().String()[:15],
		},
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}
	assert.NoError(t, v.Put())

	env, err := v.GetMirrorEnvironment()
	assert.NoError(t, err)
	assert.Equal(t, *env, *e)
}

func TestVolume_GetStorage(t *testing.T) {
	sourceStorage := &storage.ClusterStorage{
		StorageID: rand.Uint64(),
		ClusterID: rand.Uint64(),
		Type:      storage.ClusterStorageTypeCeph,
	}
	targetStorage := &storage.ClusterStorage{
		StorageID: rand.Uint64(),
		ClusterID: rand.Uint64(),
		Type:      storage.ClusterStorageTypeCeph,
	}
	assert.NoError(t, sourceStorage.Put())
	assert.NoError(t, targetStorage.Put())

	v := &Volume{
		SourceClusterStorage: sourceStorage,
		TargetClusterStorage: targetStorage,
		SourceVolume: &volume.ClusterVolume{
			ClusterID:  rand.Uint64(),
			VolumeID:   rand.Uint64(),
			VolumeUUID: uuid.New().String()[:15],
		},
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}
	assert.NoError(t, v.Put())

	s, err := v.GetSourceStorage()
	assert.NoError(t, err)
	assert.Equal(t, *sourceStorage, *s)

	s, err = v.GetTargetStorage()
	assert.NoError(t, err)
	assert.Equal(t, *targetStorage, *s)
}

func TestGetAndSetOperation(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}

	assert.NoError(t, v.Put())

	err := v.SetOperation("invalid")
	assert.Error(t, err)
	assert.True(t, errors.Equal(err, ErrUnknownMirrorVolumeOperation))

	assert.NoError(t, v.SetOperation(constant.MirrorVolumeOperationStart))

	op, err := v.GetOperation()
	assert.NoError(t, err)
	assert.Equal(t, constant.MirrorVolumeOperationStart, op.Operation)
}

func TestGetAndSetStatus(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}

	assert.NoError(t, v.Put())

	err := v.SetStatus("invalid", "code.not.defined", nil)
	assert.Error(t, err)
	assert.True(t, errors.Equal(err, ErrUnknownMirrorVolumeState))

	assert.NoError(t, v.SetStatus(constant.MirrorVolumeStateCodeMirroring, "", nil))

	status, err := v.GetStatus()
	assert.NoError(t, err)
	assert.Equal(t, constant.MirrorVolumeStateCodeMirroring, status.StateCode)
}

func TestGetAndSetTargetMetadataAgent(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}
	assert.NoError(t, v.Put())

	md := map[string]interface{}{
		"export": uuid.New().String()[:15],
	}
	assert.NoError(t, v.SetTargetMetadata(md))

	meta, err := v.GetTargetMetadata()
	assert.NoError(t, err)
	assert.Equal(t, meta, md)

	agent := &Agent{IP: uuid.New().String(), Port: uint(rand.Int())}
	assert.NoError(t, v.SetTargetAgent(agent))

	r, err := v.GetTargetAgent()
	assert.NoError(t, err)
	assert.Equal(t, *r, *agent)

	assert.NoError(t, v.Delete())

	_, err = store.List(v.GetKey())
	assert.Error(t, err)
	assert.True(t, errors.Equal(err, store.ErrNotFoundKey))

}

func TestVolume_SetAndGetProtectionVolumeSnapshotList(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}
	assert.NoError(t, v.Put())
	snapshots, err := v.GetSourceVolumeFileList()
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{}, snapshots)

	snapshots = []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}

	assert.NoError(t, v.SetSourceVolumeFileList(snapshots))
	expectSnapshotList, err := v.GetSourceVolumeFileList()
	assert.NoError(t, err)
	assert.ElementsMatch(t, expectSnapshotList, snapshots)
}

func TestVolume_SetAndGetRecoveryVolumeSnapshotList(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}

	assert.NoError(t, v.Put())
	snapshots, err := v.GetTargetVolumeFileList()
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{}, snapshots)

	snapshots = []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}

	assert.NoError(t, v.SetTargetVolumeFileList(snapshots))
	expectSnapshotList, err := v.GetTargetVolumeFileList()
	assert.NoError(t, err)
	assert.ElementsMatch(t, expectSnapshotList, snapshots)
}

func TestVolume_VolumeReferenceCount(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}

	err := store.Transaction(func(txn store.Txn) error {
		// reference 1 증가
		return v.IncreaseRefCount(txn)
	})
	panicIfError(err)

	id, err := v.GetRefCount()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), id)

	err = store.Transaction(func(txn store.Txn) error {
		// reference 1 증가
		return v.IncreaseRefCount(txn)
	})
	panicIfError(err)

	id, err = v.GetRefCount()
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), id)

	err = store.Transaction(func(txn store.Txn) error {
		// reference 1 감소
		return v.DecreaseRefCount(txn)
	})
	panicIfError(err)

	id, err = v.GetRefCount()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), id)

	err = store.Transaction(func(txn store.Txn) error {
		// reference 1 감소
		return v.DecreaseRefCount(txn)
	})
	panicIfError(err)

	id, err = v.GetRefCount()
	assert.EqualError(t, err, "not found source volume reference")
}

func TestVolume_SetAndGetPlanOfMirrorVolume(t *testing.T) {
	v := &Volume{
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
		SourceAgent: &Agent{
			IP:   uuid.New().String()[:15],
			Port: uint(rand.Uint32()),
		},
	}

	pid := rand.Uint64()

	assert.NoError(t, v.SetPlanOfMirrorVolume(pid))
	id, err := v.GetPlanOfMirrorVolume()
	assert.NoError(t, err)
	assert.Equal(t, pid, id)
}
