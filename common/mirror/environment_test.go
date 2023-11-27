package mirror

import (
	"encoding/json"
	"fmt"
	"github.com/datacommand2/cdm-center/cluster-manager/storage"
	"github.com/datacommand2/cdm-center/cluster-manager/volume"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func setup() {
	if err := helper.Init(); err != nil {
		panic(err)
	}
	_ = store.Delete(mirrorEnvironmentKeyBase, store.DeletePrefix())
	_ = store.Delete(volumeKeyBase, store.DeletePrefix())
	_ = store.Delete("clusters", store.DeletePrefix())
}

func teardown() {
	helper.Close()
}

func TestMain(m *testing.M) {
	setup()
	_ = m.Run()
	teardown()
}

func TestGetEnvironmentList(t *testing.T) {
	var envs = []*Environment{
		{
			SourceClusterStorage: &storage.ClusterStorage{
				StorageID: rand.Uint64(),
				ClusterID: rand.Uint64(),
			},
			TargetClusterStorage: &storage.ClusterStorage{
				StorageID: rand.Uint64(),
				ClusterID: rand.Uint64(),
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
		},
	}

	for _, env := range envs {
		assert.NoError(t, env.Put())
	}
	list, err := GetEnvironmentList()
	assert.NoError(t, err)
	assert.ElementsMatch(t, list, envs)
}

func TestEnvironment_GetStorages(t *testing.T) {
	centerStorages := []*storage.ClusterStorage{
		{
			ClusterID:   rand.Uint64(),
			StorageID:   rand.Uint64(),
			Type:        storage.ClusterStorageTypeCeph,
			BackendName: uuid.New().String()[:15],
		},
		{
			ClusterID:   rand.Uint64(),
			StorageID:   rand.Uint64(),
			Type:        storage.ClusterStorageTypeCeph,
			BackendName: uuid.New().String()[:15],
		},
	}
	for _, s := range centerStorages {
		b, _ := json.Marshal(s)
		assert.NoError(t, store.Put(fmt.Sprintf("clusters/%d/storages/%d", s.ClusterID, s.StorageID), string(b)))
	}
	e := &Environment{
		SourceClusterStorage: &storage.ClusterStorage{
			StorageID: centerStorages[0].StorageID,
			ClusterID: centerStorages[0].ClusterID,
		},
		TargetClusterStorage: &storage.ClusterStorage{
			StorageID: centerStorages[1].StorageID,
			ClusterID: centerStorages[1].ClusterID,
		},
	}
	b, _ := json.Marshal(e)
	assert.NoError(t, store.Put(fmt.Sprintf(mirrorEnvironmentKeyFormat, e.SourceClusterStorage.StorageID, e.TargetClusterStorage.StorageID), string(b)))

	source, err := e.GetSourceStorage()
	assert.NoError(t, err)
	assert.Equal(t, centerStorages[0], source)

	target, err := e.GetTargetStorage()
	assert.NoError(t, err)
	assert.Equal(t, centerStorages[1], target)
}

func TestEnvironment_SetStateAndGetState(t *testing.T) {
	e := &Environment{
		SourceClusterStorage: &storage.ClusterStorage{
			StorageID: rand.Uint64(),
			ClusterID: rand.Uint64(),
		},
		TargetClusterStorage: &storage.ClusterStorage{
			StorageID: rand.Uint64(),
			ClusterID: rand.Uint64(),
		},
	}

	b, _ := json.Marshal(e)
	k := fmt.Sprintf(mirrorEnvironmentKeyFormat, e.SourceClusterStorage.StorageID, e.TargetClusterStorage.StorageID)
	assert.NoError(t, store.Put(k, string(b)))

	for _, tc := range []struct {
		state    string
		code     string
		ok       bool
		expected error
	}{
		{
			state: constant.MirrorEnvironmentStateCodeError,
			code:  "code.not.defined",
			ok:    true,
		},
		{
			state: constant.MirrorEnvironmentStateCodeStopping,
			ok:    true,
		},
		{
			state: constant.MirrorEnvironmentStateCodeMirroring,
			ok:    true,
		},
		{
			state:    uuid.New().String()[:15],
			expected: ErrUnknownMirrorEnvironmentState,
		},
	} {
		err := e.SetStatus(tc.state, tc.code, nil)
		if tc.ok {
			assert.NoError(t, err)
			status, err := e.GetStatus()
			assert.NoError(t, err)
			assert.NotNil(t, status)
			assert.Equal(t, tc.state, status.StateCode)
			assert.Equal(t, tc.code, status.ErrorReason.Code)
		} else {
			assert.Equal(t, true, errors.Equal(tc.expected, err))
		}
	}
}

func TestEnvironment_SetOperationAndGetOperation(t *testing.T) {
	e := &Environment{
		SourceClusterStorage: &storage.ClusterStorage{
			StorageID: rand.Uint64(),
			ClusterID: rand.Uint64(),
		},
		TargetClusterStorage: &storage.ClusterStorage{
			StorageID: rand.Uint64(),
			ClusterID: rand.Uint64(),
		},
	}

	b, _ := json.Marshal(e)
	k := fmt.Sprintf(mirrorEnvironmentKeyFormat, e.SourceClusterStorage.StorageID, e.TargetClusterStorage.StorageID)
	assert.NoError(t, store.Put(k, string(b)))

	for _, tc := range []struct {
		operation string
		ok        bool
		expected  error
	}{
		{
			operation: constant.MirrorEnvironmentOperationStart,
			ok:        true,
		},
		{
			operation: constant.MirrorEnvironmentOperationStop,
			ok:        true,
		},
		{
			operation: uuid.New().String()[:15],
			expected:  ErrUnknownMirrorEnvironmentOperation,
		},
	} {
		err := e.SetOperation(tc.operation)
		if tc.ok {
			assert.NoError(t, err)
			op, err := e.GetOperation()
			assert.NoError(t, err)
			assert.NotNil(t, op)
			assert.Equal(t, op.Operation, tc.operation)
		} else {
			assert.Equal(t, true, errors.Equal(tc.expected, err))
		}
	}
}

func TestEnvironment_CheckMirrorVolumeExisting(t *testing.T) {
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

	e := &Environment{
		SourceClusterStorage: sourceStorage,
		TargetClusterStorage: targetStorage,
	}

	assert.NoError(t, e.Put())

	err := e.IsMirrorVolumeExisted()
	assert.NoError(t, err)

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

	err = e.IsMirrorVolumeExisted()
	assert.Error(t, err)
	assert.True(t, errors.Equal(err, ErrVolumeExisted))

	assert.NoError(t, e.Delete())
	assert.NoError(t, v.Delete())
}
