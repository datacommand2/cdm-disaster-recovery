package builder

import (
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
)

// TaskBuilderOption 는 clear task 생성 옵션 함수이다.
type TaskBuilderOption func(*taskBuilderOptions)

type taskBuilderOptions struct {
	PutTaskFunc func(store.Txn, *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error)

	SkipSuccessInstances bool
	skipTaskMap          map[string]bool

	RetryRecoveryPointTypeCode string
	RetryRecoveryPointSnapshot *drms.ProtectionGroupSnapshot
	RetryInstanceList          []*cms.ClusterInstance

	RetryRecoveryPlanSnapshot *drms.RecoveryPlan
}

// PutTaskFunc 은 job 에 task 를 put 하는 함수를 지정하는 옵션 함수이다.
func PutTaskFunc(fc func(store.Txn, *migrator.RecoveryJobTask) (*migrator.RecoveryJobTask, error)) TaskBuilderOption {
	return func(o *taskBuilderOptions) {
		o.PutTaskFunc = fc
	}
}

// SkipSuccessInstances 는 (확정시) 성공한 인스턴스에 대한 Clear Task 를 생성하지 않게 하는 옵션 함수이다.
func SkipSuccessInstances() TaskBuilderOption {
	return func(o *taskBuilderOptions) {
		o.SkipSuccessInstances = true
	}
}
