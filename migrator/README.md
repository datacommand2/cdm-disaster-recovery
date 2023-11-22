# migrator

- 재해 복구 작업(job) 의 생성을 모니터링 하고, 생성된 job 의 task 들을 수행하는 역할

## monitor
- dr-manager 에서 생성된 job 이 있는지 모니터링 

### workflow
1. 일정 주기(5초) 로 etcd 에서 job list 를 가져와 job 을 실행(run)

```go
// Run migrator 모니터링 시작 함수
func (m *Monitor) Run() {
	go func() {
		for {
			select {
			case <-m.stopCh:
				return

			case <-time.After(defaultMonitorInterval):
				jobList, err := migrator.GetJobList()
				if err != nil {
					logger.Warnf("Could not get job list. cause: %+v", err)
					continue
				}

				for _, j := range jobList {
					runJob(j)
				}
			}
		}
	}()
}
```

2. job 의 state 에 따라 task list 를 가져옴

```go
    case constant.RecoveryJobStateCodeClearing:
		taskList, err = job.GetClearTaskList()

	case constant.RecoveryJobStateCodeRunning, constant.RecoveryJobStateCodeCanceling:
		taskList, err = job.GetTaskList()
```

3. task 를 go routine 으로 실행(run)

```go
    go w.run(ctx)
```


## worker
- task type 에 따라 task 를 수행하는 역할

### Workflow
1. task 를 수행할 leader 를 선출

```go
// task 를 수행할 leader 를 선출한다
	l, err := cloudSync.CampaignLeader(ctx, path.Join(defaultLeaderElectionPath, w.task.GetKey()))
	if err != nil {
		logger.Errorf("Could not get campaign leader(key: %s). cause: %+v", w.task.GetKey(), err)
		return
	}
```

2. 공유 task 처리
  2-1.공유 task reference count 관리

```go
	switch jobStatus.StateCode {
	// 이미 생성된 공유 자원을 생성하는 task 인 경우 reference count 를 증가시킨다.
	case constant.RecoveryJobStateCodeRunning:
		if err := increaseSharedTaskRefCount(w.task); err != nil {
			logger.Errorf("Could not increase job(%d)'s shared task reference count. cause:%+v", w.job.RecoveryJobID, err)
			return
		}

	// 아직 공유 중인 자원을 삭제하는 task 인 경우 reference count 를 감소시킨다.
	case constant.RecoveryJobStateCodeClearing:
		if err := decreaseSharedTaskRefCount(w.task); err != nil {
			logger.Errorf("Could not decrease job(%d)'s shared task reference count. cause:%+v", w.job.RecoveryJobID, err)
			return
		}
	}
```

  2-2. 공유 task 가 done 상태 이면 수행하지 않음

```go
if taskStatus.StateCode == constant.MigrationTaskStateCodeDone {
		// 이미 done 상태이고, 공유 task 가 아니면 이미 수행이 완료된 task 이므로 return 한다.
		if w.task.SharedTaskKey == "" {
			return
		}
```

3. job 의 state 에 따라 go routine task 수행

```go
		w.runTask(ctx)
```

4. task type 에 따른 task function 수행

```go
    output, taskErr := taskFuncMap[w.task.TypeCode](ctx, w)
```

- task type 종류

```go
var taskFuncMap = map[string]taskFunc{
	constant.MigrationTaskTypeCreateTenant:               createTenantTaskFunc,
	constant.MigrationTaskTypeCreateSecurityGroup:        createSecurityGroupTaskFunc,
	constant.MigrationTaskTypeCreateSecurityGroupRule:    createSecurityGroupRuleTaskFunc,
	constant.MigrationTaskTypeCopyVolume:                 copyVolumeTaskFunc,
	constant.MigrationTaskTypeImportVolume:               importVolumeTaskFunc,
	constant.MigrationTaskTypeCreateSpec:                 createSpecTaskFunc,
	constant.MigrationTaskTypeCreateKeypair:              createKeypairTaskFunc,
	constant.MigrationTaskTypeCreateFloatingIP:           createFloatingIPTaskFunc,
	constant.MigrationTaskTypeCreateNetwork:              createNetworkTaskFunc,
	constant.MigrationTaskTypeCreateSubnet:               createSubnetTaskFunc,
	constant.MigrationTaskTypeCreateRouter:               createRouterTaskFunc,
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: createAndDiagnosisInstanceTaskFunc,
	constant.MigrationTaskTypeStopInstance:               stopInstanceTaskFunc,
	constant.MigrationTaskTypeDeleteInstance:             deleteInstanceTaskFunc,
	constant.MigrationTaskTypeDeleteSpec:                 deleteSpecTaskFunc,
	constant.MigrationTaskTypeDeleteKeypair:              deleteKeypairTaskFunc,
	constant.MigrationTaskTypeUnmanageVolume:             unmanageVolumeTaskFunc,
	constant.MigrationTaskTypeDeleteVolumeCopy:           deleteVolumeCopyTaskFunc,
	constant.MigrationTaskTypeDeleteSecurityGroup:        deleteSecurityGroupTaskFunc,
	constant.MigrationTaskTypeDeleteFloatingIP:           deleteFloatingIPTaskFunc,
	constant.MigrationTaskTypeDeleteRouter:               deleteRouterTaskFunc,
	constant.MigrationTaskTypeDeleteNetwork:              deleteNetworkTaskFunc,
	constant.MigrationTaskTypeDeleteTenant:               deleteTenantTaskFunc,
}
```

5. task 수행 결과에 따른 상태 값을 etcd 에 저장
- task 성공

```go
func (w *Worker) setTaskResultSuccess(txn store.Txn, output interface{}) error
```

  - 공유 task type 의 task 성공 시, 공유 task 의 reference count 를 가져오고, count 가 0 인 경우 현재 리소스를 사용하고 있지 않으므로 공유 task 정보를 삭제한다.

```go
//DeleteReverseSharedTask 공유 task 삭제(reverse) 함수
func DeleteReverseSharedTask(txn store.Txn, typeCode, taskID string, recoveryClusterID uint64) error {
    return DeleteSharedTask(txn, reversedTaskTypeMap[typeCode], taskID, recoveryClusterID)
}
```

- task 실패
```go
func (w *Worker) setTaskResultFailed(txn store.Txn, reason interface{}) error
```

- task 무시
```go
func (w *Worker) setTaskResultIgnored(txn store.Txn, reason interface{}) error
```

- task 취소
```go
func (w *Worker) setTaskResultCanceled() error
```




## json reference
- dr-manager 에서 task build 시, dependency task 의 정보가 필요하지만 build 시에는 아직 recovery cluster 에서 리소스가 생성되지 않아 해당 정보를 알 수 없으므로 json reference 오픈소스를 이용한다.
- dependency task 의 결과값(output 의 result) 를 현재 task 의 실제값(input) 으로 mapping 한다.

### Workflow
#### Prepare
- instance create task build 시, denpendency 인 tenant create task build 후, tenant create task id 를 key 값으로 instance create task 의 input 의 tenant 항목을 json reference 로 설정한다.
  - 코드 예시

```go
	func (b *taskBuilder) buildInstanceCreateTask(txn store.Txn, plan *drms.InstanceRecoveryPlan) (string, error) {
		...

        // tenant
        if taskID, err := b.buildTenantCreateTask(txn, tenantPlan); err == nil {
            input.Tenant = migrator.NewJSONRef(taskID, "/tenant")
            dependencies = append(dependencies, taskID)
        } else {
            return "", err
        }
		
		...
	}
```

- reference mapping 구조 
  - `$ref` -> `{task_id}#/tenant`

```go
// NewJSONRef 다른 JSON 스키마 참조를 위한 구조체를 생성한다.
func NewJSONRef(taskID, path string) *JSONRef {
	return &JSONRef{Ref: fmt.Sprintf("%s#%s", taskID, path)}
}
```

#### resolve
1. 새로운 resolver 를 생성한다.

```go
    res := jsref.New()
```

2. dependency 의 task id 를 key 값으로, task 의 결과값(task result 의 output) value 로 가지는 map 을 만든다.(external reference 이용)
- `{task_id}#/tenant`

```go
    mp := provider.NewMap()
	if err := mp.Set(d, result.Output); err != nil {
		return errors.Unknown(err)
	}
```

3. mapping 정보를 1 에서 생성한 resolver 의 provider 로 추가한다.

```go
    if err := res.AddProvider(mp); err != nil {
			return errors.Unknown(err)
	}
```

4. task 의 input 항목에서 **task id + 해당 리소스 이름(tenant, spec, keypair ...)** 조합으로 같은 이름으로 2 에서 set 한 output 의 mapping 되는 정보를 가져온다.
- `input 의 tenant` -> `$ref` -> `{task_id}#tenant` -> `output 의 tenant`

```go
    r, err := res.Resolve(tempTask, "#/input", jsref.WithRecursiveResolution(true))
	if err != nil {
		return errors.Unknown(err)
	}
```

### 전체 코드

```go
// ResolveInput task 의 input data 의 json ref 를 resolve 하여 input 을 가져온다.
func (t *RecoveryJobTask) ResolveInput(out interface{}) error {
	if v, ok := out.(taskInput); !ok {
		return errors.Unknown(errors.New("parameter type is must be implements an taskInput"))

	} else if !v.IsTaskType(t.TypeCode) {
		return errors.Unknown(errors.New("parameter type is mismatched on task type"))
	}

	getJobTask := func(job *RecoveryJob, taskID string) (*RecoveryJobTask, error) {
		if clearTask, err := job.GetClearTask(taskID); err == nil {
			return clearTask, nil

		} else if !errors.Equal(err, ErrNotFoundClearTask) {
			return nil, err
		}

		return job.GetTask(taskID)
	}

	job, err := GetJob(t.RecoveryJobID)
	if err != nil {
		return err
	}

	res := jsref.New()

	for _, d := range t.Dependencies {
		depTask, err := getJobTask(job, d)
		if err != nil {
			return err
		}

		result, err := depTask.GetResult()
		if err != nil {
			return err
		}

		mp := provider.NewMap()
		if err := mp.Set(d, result.Output); err != nil {
			return errors.Unknown(err)
		}

		if err := res.AddProvider(mp); err != nil {
			return errors.Unknown(err)
		}
	}
	tempTask, err := getJobTask(job, t.RecoveryJobTaskID)
	if err != nil {
		return err
	}

	r, err := res.Resolve(tempTask, "#/input", jsref.WithRecursiveResolution(true))
	if err != nil {
		return errors.Unknown(err)
	}

	b, err := json.Marshal(r)
	if err != nil {
		return errors.Unknown(err)
	}

	if err := json.Unmarshal(b, out); err != nil {
		return errors.Unknown(err)
	}

	return nil
}
```

### 참조
- https://github.com/lestrrat-go/jsref