package kubernetes

import "github.com/datacommand2/cdm-cloud/common/errors"

var (
	// ErrNotFoundPod pod 를 찾을 수 없습니다.
	ErrNotFoundPod = errors.New("not found pod")
	// ErrNotFoundRunningJob running job 을 찾을 수 없습니다.
	ErrNotFoundRunningJob = errors.New("not found running_job")
)

// NotFoundPod pod 를 찾을 수 없습니다.
func NotFoundPod(ip string) error {
	return errors.Wrap(
		ErrNotFoundPod,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"pod_ip": ip,
		}),
	)
}

// NotFoundRunningJob running job 을 찾을 수 없습니다.
func NotFoundRunningJob(jid uint64) error {
	return errors.Wrap(
		ErrNotFoundRunningJob,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"job_id": jid,
		}),
	)
}
