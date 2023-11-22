package migrator

import (
	"10.1.1.220/cdm/cdm-cloud/common/errors"
)

var (
	// ErrDependencyTaskFailed 종속성 작업 실패
	ErrDependencyTaskFailed = errors.New("dependency task is failed")
)

// DependencyTaskFailed 종속성 작업 실패
func DependencyTaskFailed(jid uint64, tid string, failedTasks []string) error {
	return errors.Wrap(
		ErrDependencyTaskFailed,
		errors.CallerSkipCount(1),
		errors.WithValue(map[string]interface{}{
			"job_id":       jid,
			"task_id":      tid,
			"failed_tasks": failedTasks,
		}),
	)
}
