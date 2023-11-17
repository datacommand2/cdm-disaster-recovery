package assignee

import (
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"math"
)

// HypervisorAssignAlgorithm hypervisor assign algorithm interface
type HypervisorAssignAlgorithm func([]*drms.InstanceRecoveryPlan, []*cms.ClusterHypervisor) ([]*drms.InstanceRecoveryPlan, error)

var hypervisorAssignAlgorithmList []HypervisorAssignAlgorithm

// RegisterHypervisorAssignAlgorithm register HypervisorAssignAlgorithm
func RegisterHypervisorAssignAlgorithm(alg HypervisorAssignAlgorithm) {
	hypervisorAssignAlgorithmList = append(hypervisorAssignAlgorithmList, alg)
}

// AssignHypervisor HypervisorAssignAlgorithm 중 최적의 결과 도출
func AssignHypervisor(insList []*drms.InstanceRecoveryPlan, hyperList []*cms.ClusterHypervisor) ([]*drms.InstanceRecoveryPlan, error) {
	var result []*drms.InstanceRecoveryPlan
	for _, alg := range hypervisorAssignAlgorithmList {
		output, err := alg(insList, hyperList)
		if err != nil {
			return nil, err
		}
		result = getBetterResult(hyperList, result, output)
	}

	return result, nil
}

// getUnassignedInstance 배치안된 instance 수
func getUnassignedInstance(instList []*drms.InstanceRecoveryPlan) int {
	result := 0
	for _, i := range instList {
		if i.GetRecoveryClusterHypervisor().GetId() == 0 {
			result++
		}
	}
	return result
}

// getCPUDispersion cpu 사용 분산값
func getCPUDispersion(hyperList []*cms.ClusterHypervisor, instList []*drms.InstanceRecoveryPlan) float64 {
	var (
		result      float64   // 분산값
		avg         float64   // 평균
		utilization []float64 // hypervisor 별 사용률
	)
	cnt := float64(len(hyperList)) // hypervisor 개수

	usedCPUMap := make(map[uint64]uint32) // hypervisor 별 cpu 사용량
	for _, h := range hyperList {
		usedCPUMap[h.Id] = h.VcpuUsedCnt
	}
	for _, i := range instList {
		usedCPUMap[i.GetRecoveryClusterHypervisor().GetId()] += i.GetProtectionClusterInstance().GetSpec().GetVcpuTotalCnt()
	}

	// hypervisor 의 각 사용률
	for _, h := range hyperList {
		use := float64(usedCPUMap[h.GetId()]) / float64(h.GetVcpuTotalCnt())
		utilization = append(utilization, use)
		avg += use // 사용률 평균을 구하기 위해 사용률 합함
	}

	// hypervisorList 의 cpu 사용률 평균
	avg = math.Mod(avg, cnt)

	// 편차제곱의 합
	for _, u := range utilization {
		result += math.Pow(u-avg, 2)
	}

	// 분산 = (편차제곱의 합)의 평균
	result = math.Mod(result, cnt)

	return result
}

// getMemDispersion mem 사용 분산값
func getMemDispersion(hyperList []*cms.ClusterHypervisor, instList []*drms.InstanceRecoveryPlan) float64 {
	var (
		result      float64   // 분산값
		avg         float64   // 평균
		utilization []float64 // hypervisor 별 사용률
	)
	cnt := float64(len(hyperList)) // hypervisor 개수

	usedMemMap := make(map[uint64]uint64) // hypervisor 별 mem 사용량
	for _, h := range hyperList {
		usedMemMap[h.Id] = h.MemUsedBytes
	}
	for _, i := range instList {
		usedMemMap[i.GetRecoveryClusterHypervisor().GetId()] += i.GetProtectionClusterInstance().GetSpec().GetMemTotalBytes()
	}

	// hypervisor 의 각 사용률
	for _, h := range hyperList {
		use := float64(usedMemMap[h.GetId()]) / float64(h.GetMemTotalBytes())
		utilization = append(utilization, use)
		avg += use // 사용률 평균을 구하기 위해 사용률 합함
	}

	// hypervisorList 의 mem 사용률 평균
	avg = math.Mod(avg, cnt)

	// 편차제곱의 합
	for _, u := range utilization {
		result += math.Pow(u-avg, 2)
	}

	// 분산 = (편차제곱의 합)의 평균
	result = math.Mod(result, cnt)

	return result
}

// getLeftMem mem 잔량 총합
func getLeftMem(hyperList []*cms.ClusterHypervisor, instList []*drms.InstanceRecoveryPlan) uint64 {
	var result uint64
	for _, h := range hyperList {
		result += h.GetMemTotalBytes() - h.GetMemUsedBytes()
	}
	for _, i := range instList {
		if i.GetRecoveryClusterHypervisor() != nil {
			result -= i.GetProtectionClusterInstance().GetSpec().GetMemTotalBytes()
		}
	}
	return result
}

// getBetterResult AssignHypervisor 한 결과들 중 Best 얻기 위한 함수
func getBetterResult(hyperList []*cms.ClusterHypervisor, resultA []*drms.InstanceRecoveryPlan, resultB []*drms.InstanceRecoveryPlan) []*drms.InstanceRecoveryPlan {
	if resultA == nil {
		return resultB
	}
	if resultB == nil {
		return resultA
	}

	// 배치된 instance 의 수가 더 많은 경우
	insCntA := getUnassignedInstance(resultA)
	insCntB := getUnassignedInstance(resultB)
	if insCntA < insCntB {
		return resultA
	} else if insCntA > insCntB {
		return resultB
	}

	// CPU 사용 분산값이 더 작은 경우
	cpuA := getCPUDispersion(hyperList, resultA)
	cpuB := getCPUDispersion(hyperList, resultB)
	if cpuA < cpuB {
		return resultA
	} else if cpuA > cpuB {
		return resultB
	}

	// mem 사용 분산값이 더 작은 경우
	memA := getMemDispersion(hyperList, resultA)
	memB := getMemDispersion(hyperList, resultB)
	if memA < memB {
		return resultA
	} else if memA > memB {
		return resultB
	}

	// mem 잔여값이 더 작은 경우
	leftMemA := getLeftMem(hyperList, resultA)
	leftMemB := getLeftMem(hyperList, resultB)
	if leftMemA < leftMemB {
		return resultA
	} else if leftMemA > leftMemB {
		return resultB
	}

	return resultA
}
