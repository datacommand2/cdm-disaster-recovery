# mirror
## 정의

- 볼륨 복제 환경 구성, 볼륨 복제 및 스냅샷 복제 등을 처리 하는 daemon
    - daemon 형태이므로 open api 가 없음
    - kv etcd에 데이터를 polling 하여 복제 환경 및 볼륨 복제를 처리함
    - 볼륨 스냅샷 동기화(추가/삭제) 시 cluster-manager 에서 publish 하는 messege를 subsribe를 하여 복제 중인 볼륨의 스냅샷 복제 시작/중지를 처리

## storage 별 볼륨 관리 및 복제

### Ceph(RBD)

#### 특징

- rbd peer 등록은 pool 에 권한이 있는 사용자면 등록이 가능
    - **peer 해제는 admin 권한이 있는 사용자만 가능**
    - **1:N 에 대한 peer 등록은 가능 하나 N:1 에 대한 peer 등록은 불가능함**
        - **peer 등록이 pool 단위 이고, 같은 target 에 대해 여러 개의 peer 등록이 안되기 때문에**
    - **1:N 으로 구성된 상황에서 mirror image enable 설정 시, peer가 등록된 모든 pool에 대해서 복제하는 문제가 있음**
      
- image 복제 시, source image는 promote target image는 demote로 자동 설정됨
- 복제 중인 image의 스냅샷 생성 시 생성 된 스냅샷도 자동 복제됨
- demote 이미지(target 으로 복제되는)는 read only
    - 복제 중인 demote image의 스냅샷은 read only 일 지라도 rbd clone 명령어가 수행이 가능
        - clone을 수행 하기 위해서는 해당 스냅샷이 protect 설정이 되어 있어야 하며, protect 설정은 promote image 스냅샷에서 설정이 되어야함
        - 스냅샷을 삭제 하기 위해서는 protect로 설정 된 스냅샷을 unprotect로 설정 해야 함
- demote image를 write operation을 위해서 강제로 promote로 변경이 가능
    - demote image를 promote로 변경 후 다시 demote로 변경 하면 split brain 상황이 발생
        - 다시 sync를 맞추기 위해서는 resync 명령이 수행 되어야 하며, 이때 demote 이미지가 지워진 후 처음 부터 다시 복제됨
    - 특정 rbd, ceph에 요청 및 rbd-mirror 프로세스는 remote server에서 요청 가능함(client-server 구조)
        - client 서버에는 server 쪽 conf 파일과 client keyring 파일이 존재 해야함
- mirror disable 설정 시,
    - target rbd에 복제되는 image가 demote로 설정이 되어 있을 시 파일이 삭제가 됨
    - target rbd에 image 를 강제로 promote로 변경 후 source image 를 disable로 설정 시 파일이 삭제가 안됨

#### 복제 단위

- ceph 의 rbd 복제는 pool(pool 내 전체 이미지) 혹은 pool 특정 이미지 단위로 복제가 가능

#### image 복제 절차

1. source rbd image(복제 대상 이미지)의 journaling feature 설정
2. target rbd pool에 source rbd pool peer 등록 설정
3. source rbd image mirror image enable 설정
4. rbd-mirror 프로세스가 실행

상세 내용은 [sys 문서](http://10.1.1.220/workms/sys/-/blob/master/ceph/installation/setup_mirroring.md) 참조


#### image 복제 중지 절차

1. source rbd image 에 mirror disable 설정
2. pool 내에 복제 중인 이미지가 없을 경우, rbd-mirror 프로세스 종료 및 pool peer 해제


## data schema
### cdm-center(cluster-manager)

- cluster manager 에서 storage, volume 동기화 시 storage, volume metadata 를 다음과 같은 형태로 데이터를 저장

#### storage etcd kv data

```json
clusters/{cluster_id}/storages/{storage_id}
{
    "cluster_id": uint64,
    "storage_id": uint64,
    "type": "openstack.storage.type.ceph",
    "backend_name": ""
}
```

#### storage metadata 

```json
clusters/{cluster_id}/storages/{storage_id}/metadata
// Ceph Metadata
{
    "pool": "",
    "conf": "",
    "client": "",
    "keyring": "",
    "admin_client": "",
    "admin_keyring": ""
}
```

#### volume

```json
clusters/{cluster_id}/volume/{volume_id}
{
    "cluster_id": uint64,
    "volume_id": uint64,
    "volume_uuid": "bae4c30d-3119-4870-807e-0e1a9fcbf185"
}
```

- **rbd peer 해제를 위해서는 ceph rbd를 사용하는 cluster_storage 의 메타데이터를 admin, admin_keyring 을 업데이트 해주는 작업이 필요**

### cdm-DR

- dr-manager 와 dr-mirror-daemon 에서는 볼륨 복제 환경 및 볼륨 복제를 처리하기 위해 다음과 같이 데이터를 저장

#### 복제 환경

```json
mirror_environment/storage.{source_storage_id}.{target_storage_id} 
{
    "source_cluster_storage": {
        "cluster_id": uint64,
        "storage_id": uint64
    },
    "target_cluster_storage": {
        "cluster_id": uint64,
        "storage_id": uint64
    }
}

// operation
mirror_environment/storage.{source_storage_id}.{target_storage_id}/operation
{
    "operation": "start" or "stop"
}

// status
mirror_environment/storage.{source_storage_id}.{target_storage_id}/status
{
    "state_code": "",
    "error_reason": {
        "code": "",
        "contents": ""
    }
}

```

#### 볼륨 복제
```json
mirror_volume/storage.{source_storage_id}.{target_storage_id}/volume.{volume_id} 
{
    "source_cluster_storage": {
        "cluster_id": uint64,
        "storage_id": uint64
    },
    "target_cluster_storage": {
        "cluster_id": uint64,
        "storage_id": uint64
    },
    "source_volume": {
        "cluster_id": uint64,
        "volume_id": uint64,
        "volume_uuid": "bae4c30d-3119-4870-807e-0e1a9fcbf185"
    },
    "source_agent": {
        "ip": "192.168.137.101",
        "port": uint
    }
}

// operation
mirror_volume/storage.{source_storage_id}.{target_storage_id}/volume.{volume_id}/operation
{
  "operation": "start" or "resume" or "stop" or "stop_and_delete" or "destroy" or "pause"
}

// status
mirror_volume/storage.{source_storage_id}.{target_storage_id}/status
{
  "state_code": "",
  "error_reason": {
    "code": "",
    "contents": ""
  }
}
```
## 상태도

### 볼륨 복제 시작

------

#### 볼륨 복제 시작 state 상태도

------

```plantuml
state start_operation {
[*] --> waiting
[*] --> initializing
[*] --> mirroring
[*] --> error
waiting --> initializing
waiting --> error
initializing --> mirroring
initializing --> error
mirroring --> error
error --> waiting
error --> initializing
error --> mirroring
}
```

#### 볼륨 복제 시작 state 값에 process 상태도

------

```plantuml
[*] --> start_process : waiting
[*] --> start_process : error
[*] --> start_process : not found mirror volume status
start_process --> mirror_enable : not error
[*] --> mirror_enable : mirroring
[*] --> mirror_enable : initializing
start_process --> [*]: error
```

### 볼륨 복제 재개

- 볼륨 복제 재개는 현재 mirrord 에서 구현되어 있으나, dr-manager 에서 재해복구 상황이 발생하면 protection cluster 를 사용할 수 없다고 판단하여 따로 resume 을 수행 하지 않음
  - 추후 부분 migration(혹은 DRaas) 기능 추가 시 resume 을 수행해야 할수도 있음

----

#### 볼륨 복제 재개 state 상태도

----

```plantuml
state resume_operation {
[*] --> paused
[*] --> initializing
[*] --> mirroring
[*] --> error
paused --> error
paused --> initializing
initializing --> mirroring
initializing --> error
mirroring --> error
error --> initializing
}
```

#### 볼륨 복제 재개 state 값에 process 상태도

----

```plantuml
[*] --> resume_process : paused
[*] --> resume_process : error
resume_process --> mirror_enable : not error
[*] --> mirror_enable : mirroring
[*] --> mirror_enable : initializing
resume_process --> [*]: error
```

### 볼륨 복제 일시 중지

----

#### 볼륨 복제 일시 중지 state 상태도

----

```plantuml
state pause_operation {
[*] --> paused
[*] --> initializing
[*] --> mirroring
[*] --> error

mirroring --> paused
initializing --> paused
paused --> error
error --> paused
initializing --> error
mirroring --> error
}
```

#### 볼륨 복제 일시 중지 state 값에 process 상태도

----

```plantuml
[*] --> pause_process : mirroring
[*] --> pause_process : initializing
[*] --> pause_process : error
pause_process --> pause_enable : not error
[*] --> pause_enable : paused
pause_process --> [*]: error
```

### 볼륨 복제 중지

----

#### 볼륨 복제 중지 state 상태도

----

```plantuml
state stop_operation {
[*] --> initializing
[*] --> mirroring
[*] --> paused
[*] -> stopping
[*] -> error
paused --> stopping
mirroring --> stopping
initializing --> stopping
stopping -right-> error
error -left-> stopping
stopping --> [*]
}
```

#### 볼륨 복제 중지 state 값에 process 상태도

----

```plantuml
[*] --> stop_process : mirroring
[*] --> stop_process : initializing
[*] --> stop_process : paused
[*] --> stop_process : stopping
[*] --> stop_process : error
stop_process --> delete_data_in_kv : not error
delete_data_in_kv --> [*]
stop_process --> [*]: error
```

### 볼륨 복제 중지 및 삭제

----

#### 볼륨 복제 중지 및 삭제 state 상태도

----

```plantuml
state stop_and_del_operation {
[*] --> initializing
[*] --> mirroring
[*] --> stopping
[*] --> deleting
[*] --> paused
[*] --> error
paused --> stopping
mirroring --> stopping
initializing --> stopping
mirroring --> error
initializing --> error
stopping --> deleting
stopping --> error
error --> stopping
deleting --> [*]
deleting --> error
```

#### 볼륨 복제 중지 및 삭제 state 값에 process 상태도

----

```plantuml
[*] --> stop_and_del_process : mirroring
[*] --> stop_and_del_process : initializing
[*] --> stop_and_del_process : paused
[*] --> stop_and_del_process : stopping
[*] --> stop_and_del_process : deleting
[*] --> stop_and_del_process : error
stop_and_del_process --> delete_data_in_kv : not error
stop_and_del_process --> [*]: error
delete_data_in_kv --> [*]
```

## 문제점

### Ceph
- 중간 장애 이후 다시 연결되었을 때, mirroring 이 다시 시작되면 resync(initsync 상태) 로, 플랜이 prepare 상태가 되어 initsync 진행되는 동안 기존 스냅샷 시점의 복구도 안되게 막아놓은 문제 발생
- 같은 protection group 에 대해서 다른 plan 으로 구성 될 경우
    - 다른 recovery 클러스터로 protection 클러스터가 1:N 으로 구성 복제 환경이 구성 된 상황
    - 각각의 recovery 클러스터로 복제 되는 인스턴스 볼륨이 다른 상황
- 복제 대상이 아닌 instance 볼륨도 복제되는 문제가 발생함
    - rbd mirror 가 multiple peer 가 등록된 상황에서(1:n), source image를 enable로 바꾸면 peer 가 등록 된 모든 pool로 복제 하는 문제 발생

**기대 동작**

```plantuml
rectangle ProtectionGroup {
    rectangle ProtectionVolume1 [
        ProtectionInstanceVolume1
    ]

    rectangle ProtectionVolume2 [
            ProtectionInstanceVolume2
    ]
}

rectangle RecoveryPlan1 {
    rectangle RecoveryPlan1Volume1 [
        RecoveryInstanceRecoveryPlan1Volume1
    ]
}

rectangle RecoveryPlan2 {
    rectangle RecoveryPlan2Volume2 [
        RecoveryInstanceRecoveryPlan2
    ]
}

ProtectionVolume1 -do-> RecoveryPlan1Volume1 : mirroring
ProtectionVolume2 -do-> RecoveryPlan2Volume2 : mirroring
```


**실제 동작**

```plantuml
rectangle ProtectionGroup {
    rectangle ProtectionVolume1 [
        ProtectionInstanceVolume1
    ]

    rectangle ProtectionVolume2 [
            ProtectionInstanceVolume2
    ]
}

rectangle RecoveryPlan1 {
    rectangle RecoveryPlan1Volume1 [
        RecoveryInstanceRecoveryPlan1Volume1
    ]
    rectangle RecoveryPlan1Volume2 [
            RecoveryInstanceRecoveryPlan1Volume2
    ]
}

rectangle RecoveryPlan2 {
    rectangle RecoveryPlan2Volume1 [
        RecoveryInstanceRecoveryPlan2
    ]
    rectangle RecoveryPlan2Volume2 [
        RecoveryInstanceRecoveryPlan2
    ]
}

ProtectionVolume1 -do-> RecoveryPlan1Volume1 : mirroring(plan1)
ProtectionVolume2 -do-> RecoveryPlan1Volume2 : unexpected mirroring

ProtectionVolume1 -do-> RecoveryPlan2Volume1 : unexpected mirroring
ProtectionVolume2 -do-> RecoveryPlan2Volume2 : mirroring(plan2)
```

## Workflow

### environment

#### Init

1. source storage, target storage config 생성한다.
```
	var sourceKey = c.sourceStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(sourceKey, c.sourceCephMetadata); err != nil {
		return err
	}

	var targetKey = c.targetStorage.GetKey()
	if err = ceph.CreateRBDConfigFile(targetKey, c.targetCephMetadata); err != nil {
		return err
	}
```

#### Monitor
1. etcd 에서 volume mirror environment list 를 가져온 후 worker 를 생성하고 run 한다.(go routine)

```go
func (m *Monitor) Run() {
	go func() {
		for {
			select {
			case <-m.stopCh:
				return

			case <-time.After(internal.DefaultMonitorInterval):
				envs, err := mirror.GetEnvironmentList()
				if err != nil {
					internal.ReportEvent("cdm-dr.mirror.env_monitor_run.failure-get_environment", "unknown", err)
					logger.Warnf("Could not get mirror environment list, cause: %+v", err)
				}

				for _, env := range envs {
					if w := newWorker(env); w != nil {
						w.run()
					}
				}
			}
		}
	}()
}
```

- etcd data example

```
key : mirror_environment/storage.11.13
value : {"source_cluster_storage":{"cluster_id":5,"storage_id":11},"target_cluster_storage":{"cluster_id":6,"storage_id":13}} --> envs
```

#### Worker
1. storage type 별로 etcd 에서 source, target storage 정보 가져온다.

```go
        if w.storage, err = NewStorage(w.env); err != nil {
			_ = w.env.SetStatus(constant.MirrorEnvironmentStateCodeError, "cdm-dr.mirror.env_worker_run.failure-create_storage-unknown", CreateStorageFailed(err))
			internal.ReportEvent("cdm-dr.mirror.env_worker_run.failure-create_storage", "unknown", CreateStorageFailed(err))
			logger.Errorf("Could not create storage. Cause: %+v", err)
			return
		}
```

- etcd data example

```
key : clusters/6/storages/13
value : {"cluster_id":6,"storage_id":13,"type":"openstack.storage.type.ceph","backend_name":"ceph"}
```

2. 전처리(preprocess)
- 복제 환경 구성
  - config 생성, mirror pool image enable, peer 등록을 진행한다.

```go
func (c *cephStorage) Prepare() error {
	return ceph.RunRBDMirrorPeerAddProcess(c.sourceStorage.GetKey(), c.targetStorage.GetKey(), c.sourceCephMetadata, c.targetCephMetadata)
}
```

- parameter 별 etcd data example

-> **c.sourceStorage.GetKey()**

```
key :  clusters/5/storages/11
value : {"cluster_id":5,"storage_id":11,"type":"openstack.storage.type.ceph","backend_name":"ceph"}
```

-> **c.sourceStorage.GetKey()**

```
key : clusters/5/storages/11
value : {"cluster_id":5,"storage_id":11,"type":"openstack.storage.type.ceph","backend_name":"ceph"}
```

-> **c.targetStorage.GetKey()** 

```
key : clusters/6/storages/13
value : {"cluster_id":6,"storage_id":13,"type":"openstack.storage.type.ceph","backend_name":"ceph"}
```

->  **c.targetCephMetadata**

```
key : clusters/5/storages/11/metadata
value : {"admin_client":"admin","admin_keyring":"AQDYRuFhPS7hAhAAwWIqa5JedccgV+7iQ7Xj+g==","backend":"ceph","client":"cinder","conf":"# Please do not change this file directly since it is managed by Ansible and will be overwritten\n[global]\ncluster network = 192.168
.1.0/24\nfsid = c95ce9a0-97c5-45f8-ba97-eb9b11fbe664\nmon host = [v2:192.168.1.137:3300,v1:192.168.1.136:6789]\nmon initial members = localhost\nosd pool default crush rule = -1\npublic network = 192.168.1.0/24\n\n","driver":"cinder.volume.drivers.rbd.RBDDriv
er","keyring":"AQD9R+FhdwTFMRAADjntqI2NKiW93thZ523ClA==","pool":"volumes"}
```

-> **c.sourceCephMetadata**

```
key : clusters/6/storages/13/metadata
value : {"admin_client":"admin","admin_keyring":"AQDYRuFhPS7hAhAAwWIqa5JedccgV+7iQ7Xj+g==","backend":"ceph","client":"cinder","conf":"# Please do not change this file directly since it is managed by Ansible and will be overwritten\n[global]\ncluster network = 192.168
.1.0/24\nfsid = c95ce9a0-97c5-45f8-ba97-eb9b11fbe664\nmon host = [v2:192.168.1.137:3300,v1:192.168.1.137:6789]\nmon initial members = localhost\nosd pool default crush rule = -1\npublic network = 192.168.1.0/24\n\n","driver":"cinder.volume.drivers.rbd.RBDDriv
er","keyring":"AQD9R+FhdwTFMRAADjntqI2NKiW93thZ523ClA==","pool":"volumes"}
```

3. monitor
- 해당 복제 환경을 monitoring 하며 operation 에 따른 처리를 한다.

```go
        for {
			select {
			case <-stopCh:
				logger.Infof("Stop mirror environment(%s) monitoring.", w.env.GetKey())
				return

			case <-time.After(internal.DefaultMonitorInterval):
				if err = w.monitor(stopCh); err != nil {
					return
				}
			}
		}
```

### volume
#### Monitor
1. etcd 에서 mirroring 할 volume list 를 가져온 후 worker 를 생성하고 run 한다.(go routine)

```go
func (m *Monitor) Run() {
	go func() {
		for {
			select {
			case <-m.stopCh:
				return

			case <-time.After(internal.DefaultMonitorInterval):
				volumes, err := mirror.GetVolumeList()
				if err != nil {
					internal.ReportEvent("cdm-dr.mirror.volume_monitor_run.failure-get_volume_list", "unknown", err)
					logger.Warnf("Could not get mirror volume list, cause: %+v", err)
				}

				for _, v := range volumes {
					if w := newWorker(v); w != nil {
						w.run()
					}
				}
			}
		}
	}()
}
```


#### Worker
1. mirror daemon 이 여러 개일 때, 해당 volume mirroring 을 수행할 leader 를 선출한다.

```go
        l, err := cloudSync.CampaignLeader(context.Background(), path.Join(defaultLeaderElectionPath, w.vol.GetKey()))
		if err != nil {
			return
		}
```

2. storage type 별로 etcd 에서 source, target storage 정보 가져온다.

```go
        if w.storage, err = NewStorage(w.vol); err != nil {
			logger.Errorf("Could not create storage volume. Cause: %+v", err)
			_ = w.vol.SetStatus(constant.MirrorVolumeStateCodeError, "cdm-dr.mirror.volume_worker_run.failure-create_storage-unknown", CreateStorageFailed(err))
			internal.ReportEvent("cdm-dr.mirror.volume_worker_run.failure-create_storage", "unknown", CreateStorageFailed(err))
			return
		}
```

3. operate
- 해당 볼륨 mirroring 을 monitoring 하며 operation 에 따른 처리를 한다.
```go
        for {
			select {
			case <-l.Status():
				return

			case <-stopCh:
				logger.Infof("Stop mirror volume(%s) monitoring.", w.vol.GetKey())
				return

			case <-time.After(internal.DefaultMonitorInterval):
				if err = w.operate(stopCh); err != nil {
					return
				}
			}
		}
```

-----

## ETC

- 장애 발생 시 수동 처리 방법 정리 혹은 명령어 분석 내용에 대한 정리

##### 복구 작업, 복구 계획, 보호 그룹 강제 삭제 시 etcd 내용 초기화

- etcd 내용 초기화 후 mirror daemon 재시작 필요

```
// mirroring 내용 삭제
# etcdctl --user cdm:password del --prefix mirror

// 작업 내용 초기화
# etcdctl --user cdm:password del --prefix dr.recovery.job

// 공유 리소스 내용 초기화
# etcdctl --user cdm:password del --prefix cdm.dr
```

-----

##### unsupported ceph multiple peer 발생 시 수동 대처

- etcd 에서 기존 mirror 내용을 삭제하고 target ceph 쪽에서 수동으로 peer 삭제 필요
  - peer 삭제 명령어

```
# rbd mirror pool peer remove -p volumes --conf /etc/ceph/clusters.5.storages.11.conf --cluster clusters.5.storages.11 -n client.admin --format json
```

-----

#### mirror pool info 함수 분석

```go
    if p, err = getRBDMirrorPoolInfo(target["pool"].(string), targetKey, getCephClient(target)); err != nil {
		return err
    }
```

- parameter 별 data example
    - **target["pool"].(string)** : `volumes`
    - **targetKey** : `clusters/6/storages/13`
    - **getCephClient(target)** : `admin` (없으면 `cinder`)

- 호출 되는 명령 커맨드

```
# rbd mirror pool info -p volumes --conf /etc/ceph/clusters.6.storages.13.conf --cluster clusters.6.storages.13 -n client.admin --format json
```