package recoveryreport

import (
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"strings"
)

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func getInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func getUint64(i *uint64) uint64 {
	if i == nil {
		return 0
	}
	return *i
}

func getUint32(i *uint32) uint32 {
	if i == nil {
		return 0
	}
	return *i
}

func getBool(i *bool) bool {
	if i == nil {
		return false
	}
	return *i
}

func getCreateTenantInputOutput(task *migrator.RecoveryJobTask) (*migrator.TenantCreateTaskInput, *migrator.TenantCreateTaskOutput, error) {
	var in migrator.TenantCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.TenantCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateNetworkInputOutput(task *migrator.RecoveryJobTask) (*migrator.NetworkCreateTaskInput, *migrator.NetworkCreateTaskOutput, error) {
	var in migrator.NetworkCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.NetworkCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateSubnetInputOutput(task *migrator.RecoveryJobTask) (*migrator.SubnetCreateTaskInput, *migrator.SubnetCreateTaskOutput, error) {
	var in migrator.SubnetCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.SubnetCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateSecurityGroupInputOutput(task *migrator.RecoveryJobTask) (*migrator.SecurityGroupCreateTaskInput, *migrator.SecurityGroupCreateTaskOutput, error) {
	var in migrator.SecurityGroupCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.SecurityGroupCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateSecurityGroupRuleInputOutput(task *migrator.RecoveryJobTask) (*migrator.SecurityGroupRuleCreateTaskInput, *migrator.SecurityGroupRuleCreateTaskOutput, error) {
	var in migrator.SecurityGroupRuleCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.SecurityGroupRuleCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateRouterInputOutput(task *migrator.RecoveryJobTask) (*migrator.RouterCreateTaskInput, *migrator.RouterCreateTaskOutput, error) {
	var in migrator.RouterCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.RouterCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateFloatingIPInputOutput(task *migrator.RecoveryJobTask) (*migrator.FloatingIPCreateTaskInput, *migrator.FloatingIPCreateTaskOutput, error) {
	var in migrator.FloatingIPCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.FloatingIPCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateVolumeInputOutput(task *migrator.RecoveryJobTask) (*migrator.VolumeImportTaskInput, *migrator.VolumeImportTaskOutput, error) {
	var in migrator.VolumeImportTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.VolumeImportTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateInstanceInputOutput(task *migrator.RecoveryJobTask) (*migrator.InstanceCreateTaskInput, *migrator.InstanceCreateOutput, error) {
	var in migrator.InstanceCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.InstanceCreateOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateInstanceSpecInputOutput(task *migrator.RecoveryJobTask) (*migrator.InstanceSpecCreateTaskInput, *migrator.InstanceSpecCreateTaskOutput, error) {
	var in migrator.InstanceSpecCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.InstanceSpecCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getCreateKeypairInputOutput(task *migrator.RecoveryJobTask) (*migrator.KeypairCreateTaskInput, *migrator.KeypairCreateTaskOutput, error) {
	var in migrator.KeypairCreateTaskInput
	if err := task.ResolveInput(&in); err != nil {
		return nil, nil, err
	}
	var out migrator.KeypairCreateTaskOutput
	if err := task.GetOutput(&out); err != nil {
		return nil, nil, err
	}
	return &in, &out, nil
}

func getJobData(jobID uint64) (*string, error) {
	rsp, err := migrator.GetAllJob(jobID)
	if err != nil {
		logger.Errorf("[RecoveryReport-getJobData] Could not get all job(%d) data. Cause: %+v", jobID, err)
	}

	if rsp == "" {
		logger.Infof("[RecoveryReport-getJobData] Not found job(%d) data.", jobID)
		return nil, nil
	}

	// rsp 에 있는 모든 \,"{,}" 문자는 삭제
	rsp = strings.ReplaceAll(rsp, "\\", "")
	rsp = strings.ReplaceAll(rsp, "\"{", "{")
	rsp = strings.ReplaceAll(rsp, "}\"", "}")
	rsp = strings.ReplaceAll(rsp, "\"\"\"\"", "\"\"")

	return &rsp, nil
}

func getSharedTaskData() (*string, error) {
	rsp, err := migrator.GetAllSharedTaskMapToString()
	if err != nil {
		logger.Warnf("[RecoveryReport-getSharedTaskData] Could not get all shared task data. Cause: %+v", err)
	}

	if rsp == "" {
		logger.Infof("[RecoveryReport-getSharedTaskData] Not found shared task data.")
		return nil, nil
	}

	// rsp 에 있는 모든 \,"{,}" 문자는 삭제
	rsp = strings.ReplaceAll(rsp, "\\", "")
	rsp = strings.ReplaceAll(rsp, "\"{", "{")
	rsp = strings.ReplaceAll(rsp, "}\"", "}")
	rsp = strings.ReplaceAll(rsp, "\"\"\"\"", "\"\"")

	return &rsp, nil
}

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
// return 이 true 이면 swap
func (t *jobTaskList) Less(i, j int) bool {
	// > : 내림차순 정리, < : 오름차순 정리
	return taskSeqMap[(*t)[i].TypeCode] < taskSeqMap[(*t)[j].TypeCode]
}

var taskSeqMap = map[string]int{
	constant.MigrationTaskTypeCreateTenant:               1,
	constant.MigrationTaskTypeCreateFloatingIP:           2,
	constant.MigrationTaskTypeCreateKeypair:              3,
	constant.MigrationTaskTypeCreateSpec:                 4,
	constant.MigrationTaskTypeCreateSecurityGroup:        5,
	constant.MigrationTaskTypeCreateSecurityGroupRule:    6,
	constant.MigrationTaskTypeCreateNetwork:              7,
	constant.MigrationTaskTypeCreateSubnet:               8,
	constant.MigrationTaskTypeCreateRouter:               9,
	constant.MigrationTaskTypeCopyVolume:                 10,
	constant.MigrationTaskTypeImportVolume:               11,
	constant.MigrationTaskTypeCreateAndDiagnosisInstance: 12,
	constant.MigrationTaskTypeStopInstance:               13,
}
