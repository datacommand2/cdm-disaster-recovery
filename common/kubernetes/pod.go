package kubernetes

import (
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"

	"encoding/json"
	"fmt"
	"regexp"
)

const (
	controllerPodKeyBase   = "dr.controller/pod/"
	controllerPodFormat    = "dr.controller/pod/%s"
	controllerPodKeyRegexp = "^dr.controller/pod/\\S+$"

	controllerPodRunningJobKeyBase   = "dr.controller/pod/running_job/"
	controllerPodRunningJobFormat    = "dr.controller/pod/running_job/%d"
	controllerPodRunningJobKeyRegexp = "dr.controller/pod/running_job/\\d+$"
)

// Pod 의 정보 및 작업중인 개수
type Pod struct {
	IP   string `json:"ip,omitempty"`
	Name string `json:"name,omitempty"`
	// Count 는 해당 Pod 에서 작동중인 Job 의 개수
	Count int `json:"count,omitempty"`
}

// RunningJob 의 정보 및 작업중인 개수
type RunningJob struct {
	RecoveryJobID uint64                `json:"recovery_job_id,omitempty"`
	RunningJob    *migrator.RecoveryJob `json:"running_job,omitempty"`
	PodIP         string                `json:"pod_ip,omitempty"`
}

// Put job 추가
func (p *Pod) Put(txn store.Txn) error {
	var err error

	b, err := json.Marshal(p)
	if err != nil {
		return errors.Unknown(err)
	}

	k := fmt.Sprintf(controllerPodFormat, p.IP)
	txn.Put(k, string(b))

	return nil
}

// GetPod pod 조회
func GetPod(ip string) (*Pod, error) {
	v, err := store.Get(fmt.Sprintf(controllerPodFormat, ip))
	switch {
	case err == store.ErrNotFoundKey:
		return nil, NotFoundPod(ip)

	case err != nil:
		return nil, errors.UnusableStore(err)
	}

	var j Pod
	if err = json.Unmarshal([]byte(v), &j); err != nil {
		return nil, errors.Unknown(err)
	}

	return &j, nil
}

// GetPodList pod list 조회
func GetPodList() ([]*Pod, error) {
	keys, err := store.List(controllerPodKeyBase)
	if err != nil && err != store.ErrNotFoundKey {
		return nil, errors.UnusableStore(err)
	}

	var jobs []*Pod
	for _, k := range keys {
		var matched bool
		if matched, err = regexp.Match(controllerPodKeyRegexp, []byte(k)); err != nil {
			return nil, errors.Unknown(err)
		}
		if !matched {
			continue
		}

		var ip string
		if _, err = fmt.Sscanf(k, controllerPodFormat, &ip); err != nil {
			return nil, errors.Unknown(err)
		}

		var j *Pod
		if j, err = GetPod(ip); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}

	return jobs, nil
}

// Delete pod 삭제
func (p *Pod) Delete(txn store.Txn) {
	// dr.controller/pod/%s 삭제
	txn.Delete(fmt.Sprintf(controllerPodFormat, p.IP))
}

// GetRunningJob 특정 pod 에 진행중인 job 을 조회
func GetRunningJob(jid uint64) (*RunningJob, error) {
	k := fmt.Sprintf(controllerPodRunningJobFormat, jid)
	v, err := store.Get(k)

	switch {
	case err == store.ErrNotFoundKey:
		return nil, NotFoundRunningJob(jid)

	case err != nil:
		return nil, errors.UnusableStore(err)
	}

	var runningJob RunningJob
	if err = json.Unmarshal([]byte(v), &runningJob); err != nil {
		return nil, errors.Unknown(err)
	}

	return &runningJob, nil
}

// GetRunningJobList 특정 pod 에 진행중인 job list 조회
func GetRunningJobList() ([]*RunningJob, error) {
	keys, err := store.List(fmt.Sprintf(controllerPodRunningJobKeyBase))
	if err != nil && err != store.ErrNotFoundKey {
		return nil, errors.UnusableStore(err)
	}

	var runningJob []*RunningJob
	for _, k := range keys {
		var matched bool
		if matched, err = regexp.Match(controllerPodRunningJobKeyRegexp, []byte(k)); err != nil {
			return nil, errors.Unknown(err)
		}
		if !matched {
			continue
		}

		var jid uint64
		if _, err = fmt.Sscanf(k, controllerPodRunningJobFormat, &jid); err != nil {
			return nil, errors.Unknown(err)
		}

		var rj *RunningJob
		if rj, err = GetRunningJob(jid); err != nil {
			return nil, err
		}
		runningJob = append(runningJob, rj)
	}

	return runningJob, nil
}

// SetRunningJob 특정 pod 에 진행중인 job 을 변경한다.
func (r *RunningJob) SetRunningJob(txn store.Txn) error {
	b, err := json.Marshal(r)
	if err != nil {
		return errors.Unknown(err)
	}

	k := fmt.Sprintf(controllerPodRunningJobFormat, r.RecoveryJobID)
	txn.Put(k, string(b))

	return nil
}

// Delete RunningJob 삭제
func (r *RunningJob) Delete(txn store.Txn) {
	// dr.controller/pod/running_job/%d 삭제
	txn.Delete(fmt.Sprintf(controllerPodRunningJobFormat, r.RecoveryJobID))
}
