package migrator

import (
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"

	"encoding/json"
	"sync"
)

type jobInfo struct {
	runCount       int
	taskFailedFlag bool
	taskResultMap  map[string]bool
}

var (
	jobInfoMap = make(map[uint64]*jobInfo)
	// lock
	jobInfoMapLock = sync.Mutex{}
)

func getRunCount(jobID uint64) int {
	jobInfoMapLock.Lock()
	result := jobInfoMap[jobID].runCount
	jobInfoMapLock.Unlock()
	return result
}

func getTaskResultMap(jobID uint64, taskID string) (bool, bool) {
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	result, ok := jobInfoMap[jobID].taskResultMap[taskID]
	return result, ok
}

// updateJobInfo jobID에 대한 초기화, runCount increase
func updateJobInfo(jobID uint64) {
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	if jobInfoMap[jobID] == nil {
		jobInfoMap[jobID] = &jobInfo{}
	}
	jobInfoMap[jobID].runCount++
}

// newTaskResultMap taskResultMap 을 초기화
func newTaskResultMap(jobID uint64) {
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	if jobInfoMap[jobID].taskResultMap == nil {
		jobInfoMap[jobID].taskResultMap = make(map[string]bool)
	}
}

// makeTaskResultMap taskResultMap 을 초기화하고
// task, clearTask list 의 result 들을 taskResultMap 에 추가
func makeTaskResultMap(job *migrator.RecoveryJob, clearTaskList []*migrator.RecoveryJobTask) {
	jobInfoMapLock.Lock()
	jobInfoMap[job.RecoveryJobID].taskResultMap = make(map[string]bool)
	jobInfoMapLock.Unlock()

	var list []*migrator.RecoveryJobTask
	list = append(list, clearTaskList...)

	if taskList, err := job.GetTaskList(); err == nil {
		list = append(list, taskList...)
	}

	for _, task := range list {
		setTaskResultMap(job.RecoveryJobID, task)
	}
}

// setTaskResultMap task 의 result 값을 taskResultMap 에 추가
func setTaskResultMap(jobID uint64, task *migrator.RecoveryJobTask) {
	taskResult, err := task.GetResult()
	if err != nil {
		return
	}

	if taskResult.ResultCode == constant.MigrationTaskResultCodeSuccess {
		setSuccessTaskResultMap(jobID, task.RecoveryJobTaskID)
	} else {
		setFailedTaskResultMap(jobID, task)
	}
}

func setSuccessTaskResultMap(jobID uint64, taskID string) {
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	jobInfoMap[jobID].taskResultMap[taskID] = true
}

func setFailedTaskResultMap(jobID uint64, task *migrator.RecoveryJobTask) {
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	jobInfoMap[jobID].taskResultMap[task.RecoveryJobTaskID] = false

	// copy volume 이전의 task 에서 실패가 났다면 failed flag true 설정
	if taskSeqMap[task.TypeCode] < taskSeqMap[constant.MigrationTaskTypeCopyVolume] {
		jobInfoMap[jobID].taskFailedFlag = true
	}
}

func isTaskFailed(jobID uint64, task *migrator.RecoveryJobTask) bool {
	if taskSeqMap[task.TypeCode] >= taskSeqMap[constant.MigrationTaskTypeCopyVolume] {
		return false
	}

	return jobInfoMap[jobID].taskFailedFlag
}

// updateJobLogSeq logSeq 수정하고, log 의 seq 와 last_log_seq 가 맞지 않는 경우 수정
func getLogSeq(job *migrator.RecoveryJob) uint64 {
	lastSeq, err := job.GetLastLogSeq()
	if err != nil {
		return 0
	}

	for {
		lastSeq++
		l, _ := job.GetLog(lastSeq)
		if l == nil {
			break
		}
	}
	return lastSeq
}

// deleteTaskResultMap taskResultMap 초기화
func deleteTaskResultMap(jobID uint64) {
	jobInfoMapLock.Lock()
	jobInfoMap[jobID].taskResultMap = make(map[string]bool)
	jobInfoMapLock.Unlock()
}

// resetJobInfo runCount, logSeq 초기화
func resetJobInfo(jobID uint64) {
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	if _, ok := jobInfoMap[jobID]; ok {
		jobInfoMap[jobID].runCount = 0
	}
}

// clearMap 불필요한 데이터 정리
func clearMap(job *migrator.RecoveryJob) {
	existed := false
	jobInfoMapLock.Lock()
	defer jobInfoMapLock.Unlock()
	for key := range jobInfoMap {
		if key == job.RecoveryJobID {
			existed = true
			break
		}
		if !existed {
			delete(jobInfoMap, key)
		}
	}
}

// sort.Sort()
// jobTask 를 지정된 seq 순서에 맞게 정렬하기 위한 interface
type jobTaskList []*migrator.RecoveryJobTask

// Len sort interface
func (t *jobTaskList) Len() int {
	return len(*t)
}

// Swap sort interface
func (t *jobTaskList) Swap(i, j int) {
	(*t)[i], (*t)[j] = (*t)[j], (*t)[i]
}

// Less sort interface
func (t *jobTaskList) Less(i, j int) bool {
	// > : 내림차순 정리, < : 오름차순 정리
	return taskSeqMap[(*t)[i].TypeCode] < taskSeqMap[(*t)[j].TypeCode]
}

var taskSeqMap = map[string]int{
	constant.MigrationTaskTypeCreateTenant:               1,
	constant.MigrationTaskTypeCreateSecurityGroup:        2,
	constant.MigrationTaskTypeCreateSecurityGroupRule:    3,
	constant.MigrationTaskTypeCreateNetwork:              4,
	constant.MigrationTaskTypeCreateSubnet:               5,
	constant.MigrationTaskTypeCreateFloatingIP:           6,
	constant.MigrationTaskTypeCreateRouter:               7,
	constant.MigrationTaskTypeCreateKeypair:              8,
	constant.MigrationTaskTypeCreateSpec:                 9,
	constant.MigrationTaskTypeCopyVolume:                 10,
	constant.MigrationTaskTypeImportVolume:               11,
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: 12,
	constant.MigrationTaskTypeStopInstance:               13,
	// create 와 delete 를 구분하기 위해 사용하는 숫자대를 다르게 설정
	constant.MigrationTaskTypeDeleteInstance:      21,
	constant.MigrationTaskTypeUnmanageVolume:      22,
	constant.MigrationTaskTypeDeleteVolumeCopy:    23,
	constant.MigrationTaskTypeDeleteSpec:          24,
	constant.MigrationTaskTypeDeleteKeypair:       25,
	constant.MigrationTaskTypeDeleteRouter:        26,
	constant.MigrationTaskTypeDeleteFloatingIP:    27,
	constant.MigrationTaskTypeDeleteNetwork:       28,
	constant.MigrationTaskTypeDeleteSecurityGroup: 29,
	constant.MigrationTaskTypeDeleteTenant:        30,
}

// publishMessage message marshal 하여 publish
func publishMessage(topic string, obj interface{}) error {
	if topic == "" {
		return nil
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	return broker.Publish(topic, &broker.Message{Body: b})
}
