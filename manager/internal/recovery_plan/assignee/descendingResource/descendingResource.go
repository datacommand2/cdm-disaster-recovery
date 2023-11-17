package descendingresource

import (
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal/recovery_plan/assignee"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"github.com/jinzhu/copier"
	"sort"
)

type hypervisorInfo struct {
	*cms.ClusterHypervisor
	leftMem uint64
	usedCPU uint32
}

type hypervisorInfoList []hypervisorInfo

// Len sort interface
func (h *hypervisorInfoList) Len() int {
	return len(*h)
}

// Swap sort interface
func (h *hypervisorInfoList) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

// Less sort interface
func (h *hypervisorInfoList) Less(i, j int) bool {
	// Hypervisor 의 mem 내림차순 정리. mem 값이 같은 경우는 cpu 사용률 오름차순
	return (*h)[i].leftMem > (*h)[j].leftMem || ((*h)[i].leftMem == (*h)[j].leftMem && ((*h)[i].usedCPU/(*h)[i].GetVcpuTotalCnt()) < ((*h)[j].usedCPU/(*h)[j].GetVcpuTotalCnt()))
}

func init() {
	assignee.RegisterHypervisorAssignAlgorithm(descendingResourceAlgorithm)
}

// descendingResourceAlgorithm resource 내림차순 우선순위 할당 알고리즘
func descendingResourceAlgorithm(insList []*drms.InstanceRecoveryPlan, hyperList []*cms.ClusterHypervisor) ([]*drms.InstanceRecoveryPlan, error) {
	var hypervisors hypervisorInfoList
	for _, h := range hyperList {
		hypervisors = append(hypervisors, hypervisorInfo{h, h.GetMemTotalBytes() - h.GetMemUsedBytes(), h.VcpuUsedCnt})
	}

	result := new([]*drms.InstanceRecoveryPlan)
	if err := copier.CopyWithOption(result, insList, copier.Option{DeepCopy: true}); err != nil {
		return nil, err
	}

	for _, i := range *result {
		sort.Sort(&hypervisors)
		i.RecoveryClusterHypervisor = nil
		for _, h := range hypervisors {
			if h.leftMem > i.GetProtectionClusterInstance().GetSpec().GetMemTotalBytes() &&
				h.VcpuTotalCnt > i.GetProtectionClusterInstance().GetSpec().GetVcpuTotalCnt() {
				i.RecoveryClusterHypervisor = h.ClusterHypervisor
				h.leftMem -= i.GetProtectionClusterInstance().GetSpec().GetMemTotalBytes()
				h.usedCPU += i.GetProtectionClusterInstance().GetSpec().GetVcpuTotalCnt()
				break
			}
		}
	}

	return *result, nil
}
