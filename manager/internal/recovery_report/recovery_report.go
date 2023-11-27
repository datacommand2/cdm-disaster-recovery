package recoveryreport

import (
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	"github.com/datacommand2/cdm-cloud/common/config"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/common/migrator"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/cluster"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jinzhu/gorm"

	"context"
	"sort"
	"strings"
	"time"
)

type recoveryResult struct {
	*model.RecoveryResult

	Job          *migrator.RecoveryJob
	JobDetail    *drms.RecoveryJob
	JobStatus    *migrator.RecoveryJobStatus
	JobResult    *migrator.RecoveryJobResult
	JobOperation *migrator.RecoveryJobOperation

	TenantIDMap        map[string]uint64
	NetworkIDMap       map[string]uint64
	SubnetIDMap        map[string]uint64
	SecurityGroupIDMap map[string]uint64

	SecurityGroupMap map[uint64]*cms.ClusterSecurityGroup
	RouterMap        map[uint64]*cms.ClusterRouter
	NetworkMap       map[uint64]*cms.ClusterNetwork

	IsAlreadyAppendTask map[string]bool
}

func (r *recoveryResult) createRecoveryResult() error {

	r.RecoveryResult = &model.RecoveryResult{
		OwnerGroupID:               r.JobDetail.Group.OwnerGroup.Id,
		OperatorID:                 r.JobDetail.Operator.Id,
		OperatorAccount:            r.JobDetail.Operator.Account,
		OperatorName:               r.JobDetail.Operator.Name,
		OperatorDepartment:         &r.JobDetail.Operator.Department,
		OperatorPosition:           &r.JobDetail.Operator.Position,
		ProtectionGroupID:          r.JobDetail.Group.Id,
		ProtectionGroupName:        r.JobDetail.Group.Name,
		ProtectionGroupRemarks:     &r.JobDetail.Group.Remarks,
		RecoveryPlanID:             r.JobDetail.Plan.Id,
		RecoveryPlanName:           r.JobDetail.Plan.Name,
		RecoveryPlanRemarks:        &r.JobDetail.Plan.Remarks,
		ProtectionClusterID:        r.JobDetail.Plan.ProtectionCluster.Id,
		ProtectionClusterTypeCode:  r.JobDetail.Plan.ProtectionCluster.TypeCode,
		ProtectionClusterName:      r.JobDetail.Plan.ProtectionCluster.Name,
		ProtectionClusterRemarks:   &r.JobDetail.Plan.ProtectionCluster.Remarks,
		RecoveryClusterID:          r.JobDetail.Plan.RecoveryCluster.Id,
		RecoveryClusterTypeCode:    r.JobDetail.Plan.RecoveryCluster.TypeCode,
		RecoveryClusterName:        r.JobDetail.Plan.RecoveryCluster.Name,
		RecoveryClusterRemarks:     &r.JobDetail.Plan.RecoveryCluster.Remarks,
		RecoveryTypeCode:           r.JobDetail.TypeCode,
		RecoveryDirectionCode:      r.JobDetail.Plan.DirectionCode,
		RecoveryPointObjectiveType: r.JobDetail.Group.RecoveryPointObjectiveType,
		RecoveryPointObjective:     r.JobDetail.Group.RecoveryPointObjective,
		RecoveryTimeObjective:      r.JobDetail.Group.RecoveryTimeObjective,
		RecoveryPointTypeCode:      r.JobDetail.RecoveryPointTypeCode,
		RecoveryPoint:              r.Job.RecoveryPoint,
		StartedAt:                  r.JobStatus.StartedAt,
		FinishedAt:                 r.JobStatus.FinishedAt,
		ElapsedTime:                r.JobStatus.ElapsedTime,
		ResultCode:                 r.JobResult.ResultCode,
	}

	if r.Job.Approver != nil {
		r.RecoveryResult.ApproverID = &r.Job.Approver.Id
		r.RecoveryResult.ApproverAccount = &r.Job.Approver.Account
		r.RecoveryResult.ApproverName = &r.Job.Approver.Name
		r.RecoveryResult.ApproverDepartment = &r.Job.Approver.Department
		r.RecoveryResult.ApproverPosition = &r.Job.Approver.Position
	}

	if len(r.JobResult.WarningReasons) > 0 {
		r.RecoveryResult.WarningFlag = true
	}

	if r.JobDetail.Schedule != nil {
		r.RecoveryResult.ScheduleType = &r.JobDetail.Schedule.Type
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Create(r.RecoveryResult).Error
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	logger.Infof("[createRecoveryResult] Created recovery result in db: job(%d)", r.Job.RecoveryJobID)
	return nil
}

func (r *recoveryResult) appendTenantRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateTenantInputOutput(task)
	if err != nil {
		logger.Errorf("[appendTenantRecoveryResult] Could not get tenant input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultTenant{
		RecoveryResultID:                   r.ID,
		ProtectionClusterTenantID:          input.Tenant.Id,
		ProtectionClusterTenantUUID:        input.Tenant.Uuid,
		ProtectionClusterTenantName:        input.Tenant.Name,
		ProtectionClusterTenantDescription: &input.Tenant.Description,
		ProtectionClusterTenantEnabled:     input.Tenant.Enabled,
		RecoveryClusterTenantUUID:          &output.Tenant.Uuid,
		RecoveryClusterTenantName:          &output.Tenant.Name,
		RecoveryClusterTenantDescription:   &output.Tenant.Description,
		RecoveryClusterTenantEnabled:       &output.Tenant.Enabled,
	}

	if err = validateTenantRecoveryResult(&m); err != nil {
		logger.Errorf("[appendTenantRecoveryResult] Errors occurred during validating tenant(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendTenantRecoveryResult] Could not save tenant(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	r.TenantIDMap[output.Tenant.Uuid] = input.Tenant.Id

	return nil
}

func (r *recoveryResult) appendFailedTenantRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.TenantCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedTenantRecoveryResult] Could not get tenant input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultTenant{
		RecoveryResultID:                   r.ID,
		ProtectionClusterTenantID:          input.Tenant.Id,
		ProtectionClusterTenantUUID:        input.Tenant.Uuid,
		ProtectionClusterTenantName:        input.Tenant.Name,
		ProtectionClusterTenantDescription: &input.Tenant.Description,
		ProtectionClusterTenantEnabled:     input.Tenant.Enabled,
	}

	if err := validateTenantRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedTenantRecoveryResult] Errors occurred during validating tenant(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedTenantRecoveryResult] Could not save tenant(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendNetworkRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateNetworkInputOutput(task)
	if err != nil {
		logger.Errorf("[appendNetworkRecoveryResult] Could not get network input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	tid, ok := r.TenantIDMap[input.Tenant.Uuid]
	if !ok {
		logger.Errorf("[appendNetworkRecoveryResult] Not found tenant(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found tenant creation task result"))
	}

	m := model.RecoveryResultNetwork{
		RecoveryResultID:                    r.ID,
		ProtectionClusterTenantID:           tid,
		ProtectionClusterNetworkID:          input.Network.Id,
		ProtectionClusterNetworkUUID:        input.Network.Uuid,
		ProtectionClusterNetworkName:        input.Network.Name,
		ProtectionClusterNetworkDescription: &input.Network.Description,
		ProtectionClusterNetworkTypeCode:    input.Network.TypeCode,
		ProtectionClusterNetworkState:       input.Network.State,
		RecoveryClusterNetworkUUID:          &output.Network.Uuid,
		RecoveryClusterNetworkName:          &output.Network.Name,
		RecoveryClusterNetworkDescription:   &output.Network.Description,
		RecoveryClusterNetworkTypeCode:      &output.Network.TypeCode,
		RecoveryClusterNetworkState:         &output.Network.State,
	}

	if err = validateNetworkRecoveryResult(&m); err != nil {
		logger.Errorf("[appendNetworkRecoveryResult] Errors occurred during validating network(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendNetworkRecoveryResult] Could not save network(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	r.NetworkIDMap[output.Network.Uuid] = input.Network.Id

	return nil
}

func (r *recoveryResult) appendFailedNetworkRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.NetworkCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedNetworkRecoveryResult] Could not get network input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultNetwork{
		RecoveryResultID:                    r.ID,
		ProtectionClusterTenantID:           input.Network.Tenant.Id,
		ProtectionClusterNetworkID:          input.Network.Id,
		ProtectionClusterNetworkUUID:        input.Network.Uuid,
		ProtectionClusterNetworkName:        input.Network.Name,
		ProtectionClusterNetworkDescription: &input.Network.Description,
		ProtectionClusterNetworkTypeCode:    input.Network.TypeCode,
		ProtectionClusterNetworkState:       input.Network.State,
	}

	if err := validateNetworkRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedNetworkRecoveryResult] Errors occurred during validating network(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedNetworkRecoveryResult] Could not save network(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendSubnetRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateSubnetInputOutput(task)
	if err != nil {
		logger.Errorf("[appendSubnetRecoveryResult] Could not get subnet input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	tid, ok := r.TenantIDMap[input.Tenant.Uuid]
	if !ok {
		logger.Errorf("[appendSubnetRecoveryResult] Not found tenant(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found tenant creation task result"))
	}

	nid, ok := r.NetworkIDMap[input.Network.Uuid]
	if !ok {
		logger.Errorf("[appendSubnetRecoveryResult] Not found network(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found network creation task result"))
	}

	m := model.RecoveryResultSubnet{
		RecoveryResultID:                           r.ID,
		ProtectionClusterTenantID:                  tid,
		ProtectionClusterNetworkID:                 nid,
		ProtectionClusterSubnetID:                  input.Subnet.Id,
		ProtectionClusterSubnetUUID:                input.Subnet.Uuid,
		ProtectionClusterSubnetName:                input.Subnet.Name,
		ProtectionClusterSubnetDescription:         &input.Subnet.Description,
		ProtectionClusterSubnetNetworkCidr:         input.Subnet.NetworkCidr,
		ProtectionClusterSubnetDhcpEnabled:         input.Subnet.DhcpEnabled,
		ProtectionClusterSubnetGatewayEnabled:      input.Subnet.GatewayEnabled,
		ProtectionClusterSubnetGatewayIPAddress:    &input.Subnet.GatewayIpAddress,
		ProtectionClusterSubnetIpv6AddressModeCode: &input.Subnet.Ipv6AddressModeCode,
		ProtectionClusterSubnetIpv6RaModeCode:      &input.Subnet.Ipv6RaModeCode,
		RecoveryClusterSubnetUUID:                  &output.Subnet.Uuid,
		RecoveryClusterSubnetName:                  &output.Subnet.Name,
		RecoveryClusterSubnetDescription:           &output.Subnet.Description,
		RecoveryClusterSubnetNetworkCidr:           &output.Subnet.NetworkCidr,
		RecoveryClusterSubnetDhcpEnabled:           &output.Subnet.DhcpEnabled,
		RecoveryClusterSubnetGatewayEnabled:        &output.Subnet.GatewayEnabled,
		RecoveryClusterSubnetGatewayIPAddress:      &output.Subnet.GatewayIpAddress,
		RecoveryClusterSubnetIpv6AddressModeCode:   &output.Subnet.Ipv6AddressModeCode,
		RecoveryClusterSubnetIpv6RaModeCode:        &output.Subnet.Ipv6RaModeCode,
	}

	if err = validateSubnetRecoveryResult(&m); err != nil {
		logger.Errorf("[appendSubnetRecoveryResult] Errors occurred during validating subnet(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendSubnetRecoveryResult] Could not save subnet(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	r.SubnetIDMap[output.Subnet.Uuid] = input.Subnet.Id

	// subnet DHCPPool
	for _, p := range output.Subnet.DhcpPools {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultSubnetDHCPPool{
			RecoveryResultID:                r.ID,
			ProtectionClusterTenantID:       tid,
			ProtectionClusterNetworkID:      nid,
			ProtectionClusterSubnetID:       input.Subnet.Id,
			DhcpPoolSeq:                     seq,
			ProtectionClusterStartIPAddress: p.StartIpAddress, // mirroring
			ProtectionClusterEndIPAddress:   p.EndIpAddress,   // mirroring
			RecoveryClusterStartIPAddress:   &p.StartIpAddress,
			RecoveryClusterEndIPAddress:     &p.EndIpAddress,
		}

		if err = validateSubnetDHCPPoolRecoveryResult(&m); err != nil {
			logger.Errorf("[appendSubnetRecoveryResult] Errors occurred during validating subnet(%s) DHCP pool result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendSubnetRecoveryResult] Could not save subnet(%s) DHCP pool result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// subnet Namespace
	for _, ns := range output.Subnet.Nameservers {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultSubnetNameserver{
			RecoveryResultID:            r.ID,
			ProtectionClusterTenantID:   tid,
			ProtectionClusterNetworkID:  nid,
			ProtectionClusterSubnetID:   input.Subnet.Id,
			NameserverSeq:               seq,
			ProtectionClusterNameserver: ns.Nameserver, // mirroring
			RecoveryClusterNameserver:   &ns.Nameserver,
		}

		if err = validateSubnetNameserverRecoveryResult(&m); err != nil {
			logger.Errorf("[appendSubnetRecoveryResult] Errors occurred during validating subnet(%s) name server result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendSubnetRecoveryResult] Could not save subnet(%s) name server result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendFailedSubnetRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.SubnetCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedSubnetRecoveryResult] Could not get subnet input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	network, ok := r.NetworkMap[input.Subnet.Id]
	if !ok {
		logger.Errorf("[appendFailedSubnetRecoveryResult] Not found network task result(%d): subnet(%s) job(%d) typeCode(%s) task(%s).",
			r.ID, input.Subnet.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found network creation task result"))
	}

	m := model.RecoveryResultSubnet{
		RecoveryResultID:                           r.ID,
		ProtectionClusterTenantID:                  network.Tenant.Id,
		ProtectionClusterNetworkID:                 network.Id,
		ProtectionClusterSubnetID:                  input.Subnet.Id,
		ProtectionClusterSubnetUUID:                input.Subnet.Uuid,
		ProtectionClusterSubnetName:                input.Subnet.Name,
		ProtectionClusterSubnetDescription:         &input.Subnet.Description,
		ProtectionClusterSubnetNetworkCidr:         input.Subnet.NetworkCidr,
		ProtectionClusterSubnetDhcpEnabled:         input.Subnet.DhcpEnabled,
		ProtectionClusterSubnetGatewayEnabled:      input.Subnet.GatewayEnabled,
		ProtectionClusterSubnetGatewayIPAddress:    &input.Subnet.GatewayIpAddress,
		ProtectionClusterSubnetIpv6AddressModeCode: &input.Subnet.Ipv6AddressModeCode,
		ProtectionClusterSubnetIpv6RaModeCode:      &input.Subnet.Ipv6RaModeCode,
	}

	if err := validateSubnetRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedSubnetRecoveryResult] Errors occurred during validating subnet(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedSubnetRecoveryResult] Could not save subnet(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// subnet DHCPPool
	for _, p := range input.Subnet.DhcpPools {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultSubnetDHCPPool{
			RecoveryResultID:                r.ID,
			ProtectionClusterTenantID:       network.Tenant.Id,
			ProtectionClusterNetworkID:      network.Id,
			ProtectionClusterSubnetID:       input.Subnet.Id,
			DhcpPoolSeq:                     seq,
			ProtectionClusterStartIPAddress: p.StartIpAddress,
			ProtectionClusterEndIPAddress:   p.EndIpAddress,
		}

		if err := validateSubnetDHCPPoolRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedSubnetRecoveryResult] Errors occurred during validating subnet(%s) DHCP pool result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err := database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedSubnetRecoveryResult] Could not save subnet(%s) DHCP pool result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// subnet Namespace
	for _, ns := range input.Subnet.Nameservers {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultSubnetNameserver{
			RecoveryResultID:            r.ID,
			ProtectionClusterTenantID:   network.Tenant.Id,
			ProtectionClusterNetworkID:  network.Id,
			ProtectionClusterSubnetID:   input.Subnet.Id,
			NameserverSeq:               seq,
			ProtectionClusterNameserver: ns.Nameserver,
		}

		if err := validateSubnetNameserverRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedSubnetRecoveryResult] Errors occurred during validating subnet(%s) name server result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err := database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedSubnetRecoveryResult] Could not save subnet(%s) name server result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendRouterRecoveryResult(ctx context.Context, task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateRouterInputOutput(task)
	if err != nil {
		logger.Errorf("[appendRouterRecoveryResult] Could not get router input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	tid, ok := r.TenantIDMap[input.Tenant.Uuid]
	if !ok {
		logger.Errorf("[appendRouterRecoveryResult] Not found tenant(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found tenant creation task result"))
	}

	pr, ok := r.RouterMap[input.Router.Id]
	if !ok {
		logger.Errorf("[appendRouterRecoveryResult] Not found router(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Router.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found router creation task result"))
	}

	m := model.RecoveryResultRouter{
		RecoveryResultID:                   r.ID,
		ProtectionClusterTenantID:          tid,
		ProtectionClusterRouterID:          pr.Id,
		ProtectionClusterRouterUUID:        pr.Uuid,
		ProtectionClusterRouterName:        pr.Name,
		ProtectionClusterRouterDescription: &pr.Description,
		ProtectionClusterRouterState:       pr.State,
		RecoveryClusterRouterUUID:          &output.Router.Uuid,
		RecoveryClusterRouterName:          &output.Router.Name,
		RecoveryClusterRouterDescription:   &output.Router.Description,
		RecoveryClusterRouterState:         &output.Router.State,
	}

	if err = validateRouterRecoveryResult(&m); err != nil {
		logger.Errorf("[appendRouterRecoveryResult] Errors occurred during validating router(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			pr.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendRouterRecoveryResult] Could not save router(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			pr.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// extra route
	for _, er := range pr.ExtraRoutes {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultExtraRoute{
			RecoveryResultID:             r.ID,
			ProtectionClusterTenantID:    tid,
			ProtectionClusterRouterID:    pr.Id,
			ExtraRouteSeq:                seq,
			ProtectionClusterDestination: er.Destination,
			ProtectionClusterNexthop:     er.Nexthop,
			RecoveryClusterDestination:   &er.Destination, // mirroring
			RecoveryClusterNexthop:       &er.Nexthop,     // mirroring
		}

		if err = validateExtraRoutesRecoveryResult(&m); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Errors occurred during validating extra route result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Could not save extra route result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// internal routing interface
	for _, rri := range output.Router.InternalRoutingInterfaces {
		var seq uint64

		seq = seq + 1

		nid, ok := r.NetworkIDMap[rri.Network.Uuid]
		if !ok {
			logger.Errorf("[appendRouterRecoveryResult] Not found network(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
				rri.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
			return errors.Unknown(errors.New("not found network creation task result"))
		}

		sid, ok := r.SubnetIDMap[rri.Subnet.Uuid]
		if !ok {
			logger.Errorf("[appendRouterRecoveryResult] Not found subnet(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
				rri.Subnet.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
			return errors.Unknown(errors.New("not found subnet creation task result"))
		}

		m := model.RecoveryResultInternalRoutingInterface{
			RecoveryResultID:           r.ID,
			ProtectionClusterTenantID:  tid,
			ProtectionClusterRouterID:  pr.Id,
			RoutingInterfaceSeq:        seq,
			ProtectionClusterNetworkID: nid,
			ProtectionClusterSubnetID:  sid,
			ProtectionClusterIPAddress: rri.IpAddress, // mirroring
			RecoveryClusterIPAddress:   &rri.IpAddress,
		}

		if err = validateInternalRoutingInterfaceRecoveryResult(&m); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Errors occurred during validating internal routing interface result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Could not save internal routing interface result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// protection cluster external routing interface
	var seq uint64

	for _, pri := range pr.ExternalRoutingInterfaces {
		seq = seq + 1

		m := model.RecoveryResultExternalRoutingInterface{
			RecoveryResultID:                 r.ID,
			ProtectionClusterTenantID:        tid,
			ProtectionClusterRouterID:        pr.Id,
			RoutingInterfaceSeq:              seq,
			ClusterNetworkID:                 pri.Network.Id,
			ClusterNetworkUUID:               pri.Network.Uuid,
			ClusterNetworkName:               pri.Network.Name,
			ClusterNetworkDescription:        &pri.Network.Description,
			ClusterNetworkTypeCode:           pri.Network.TypeCode,
			ClusterNetworkState:              pri.Network.State,
			ClusterSubnetID:                  pri.Subnet.Id,
			ClusterSubnetUUID:                pri.Subnet.Uuid,
			ClusterSubnetName:                pri.Subnet.Name,
			ClusterSubnetDescription:         &pri.Subnet.Description,
			ClusterSubnetNetworkCidr:         pri.Subnet.NetworkCidr,
			ClusterSubnetDhcpEnabled:         pri.Subnet.DhcpEnabled,
			ClusterSubnetGatewayEnabled:      pri.Subnet.GatewayEnabled,
			ClusterSubnetGatewayIPAddress:    &pri.Subnet.GatewayIpAddress,
			ClusterSubnetIpv6AddressModeCode: &pri.Subnet.Ipv6AddressModeCode,
			ClusterSubnetIpv6RaModeCode:      &pri.Subnet.Ipv6RaModeCode,
			ClusterIPAddress:                 pri.IpAddress,
			ProtectionFlag:                   true,
		}

		if err = validateExternalRoutingInterfaceRecoveryResult(&m); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Errors occurred during validating protection external routing interface result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Could not save protection external routing interface result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// recovery cluster external routing interface
	for _, rri := range output.Router.ExternalRoutingInterfaces {
		var en *cms.ClusterNetwork
		var es *cms.ClusterSubnet

		seq = seq + 1

		en, err = cluster.GetClusterNetworkByUUID(ctx, r.Job.RecoveryCluster.Id, rri.Network.Uuid)
		if err != nil {
			logger.Warnf("[appendRouterRecoveryResult] Could not get network information: recoveryCluster(%d) network(%s). Cause: %+v",
				r.Job.RecoveryCluster.Id, rri.Network.Uuid, err)
			continue
		}

		for _, s := range en.Subnets {
			if s.Uuid == rri.Subnet.Uuid {
				es = s
				break
			}
		}

		if es == nil {
			logger.Warnf("[appendRouterRecoveryResult] Could not get subnet information: recoveryCluster(%d) network(%s) result(%d) job(%d) typeCode(%s) task(%s). Cause: subnet information is nil",
				r.Job.RecoveryCluster.Id, rri.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
			continue
		}

		m := model.RecoveryResultExternalRoutingInterface{
			RecoveryResultID:                 r.ID,
			ProtectionClusterTenantID:        tid,
			ProtectionClusterRouterID:        pr.Id,
			RoutingInterfaceSeq:              seq,
			ClusterNetworkID:                 en.Id,
			ClusterNetworkUUID:               en.Uuid,
			ClusterNetworkName:               en.Name,
			ClusterNetworkDescription:        &en.Description,
			ClusterNetworkTypeCode:           en.TypeCode,
			ClusterNetworkState:              en.State,
			ClusterSubnetID:                  es.Id,
			ClusterSubnetUUID:                es.Uuid,
			ClusterSubnetName:                es.Name,
			ClusterSubnetDescription:         &es.Description,
			ClusterSubnetNetworkCidr:         es.NetworkCidr,
			ClusterSubnetDhcpEnabled:         es.DhcpEnabled,
			ClusterSubnetGatewayEnabled:      es.GatewayEnabled,
			ClusterSubnetGatewayIPAddress:    &es.GatewayIpAddress,
			ClusterSubnetIpv6AddressModeCode: &es.Ipv6AddressModeCode,
			ClusterSubnetIpv6RaModeCode:      &es.Ipv6RaModeCode,
			ClusterIPAddress:                 rri.IpAddress,
			ProtectionFlag:                   false,
		}

		if err = validateExternalRoutingInterfaceRecoveryResult(&m); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Errors occurred during validating recovery external routing interface result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendRouterRecoveryResult] Could not save recovery external routing interface result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, pr.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendFailedRouterRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.RouterCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedRouterRecoveryResult] Could not get router input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	router, ok := r.RouterMap[input.Router.ID]
	if !ok {
		logger.Errorf("[appendFailedRouterRecoveryResult] Not found router(%d) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Router.ID, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found router creation task result"))
	}

	m := model.RecoveryResultRouter{
		RecoveryResultID:                   r.ID,
		ProtectionClusterTenantID:          router.Tenant.Id,
		ProtectionClusterRouterID:          router.Id,
		ProtectionClusterRouterUUID:        router.Uuid,
		ProtectionClusterRouterName:        router.Name,
		ProtectionClusterRouterDescription: &router.Description,
		ProtectionClusterRouterState:       router.State,
	}

	if err := validateRouterRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedRouterRecoveryResult] Errors occurred during validating router(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			router.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedRouterRecoveryResult] Could not save router(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			router.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// extra route
	for _, er := range router.ExtraRoutes {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultExtraRoute{
			RecoveryResultID:             r.ID,
			ProtectionClusterTenantID:    router.Tenant.Id,
			ProtectionClusterRouterID:    router.Id,
			ExtraRouteSeq:                seq,
			ProtectionClusterDestination: er.Destination,
			ProtectionClusterNexthop:     er.Nexthop,
		}

		if err := validateExtraRoutesRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedRouterRecoveryResult] Errors occurred during validating extra route result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, router.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err := database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedRouterRecoveryResult] Could not save extra route result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, router.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// internal routing interface
	for _, ri := range router.InternalRoutingInterfaces {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultInternalRoutingInterface{
			RecoveryResultID:           r.ID,
			ProtectionClusterTenantID:  router.Tenant.Id,
			ProtectionClusterRouterID:  router.Id,
			RoutingInterfaceSeq:        seq,
			ProtectionClusterNetworkID: ri.Network.Id,
			ProtectionClusterSubnetID:  ri.Subnet.Id,
			ProtectionClusterIPAddress: ri.IpAddress,
		}

		if err := validateInternalRoutingInterfaceRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedRouterRecoveryResult] Errors occurred during validating internal routing interface result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, router.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err := database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedRouterRecoveryResult] Could not save internal routing interface result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, router.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// protection cluster external routing interface
	var seq uint64

	for _, e := range router.ExternalRoutingInterfaces {
		seq = seq + 1

		m := model.RecoveryResultExternalRoutingInterface{
			RecoveryResultID:                 r.ID,
			ProtectionClusterTenantID:        router.Tenant.Id,
			ProtectionClusterRouterID:        router.Id,
			RoutingInterfaceSeq:              seq,
			ClusterNetworkID:                 e.Network.Id,
			ClusterNetworkUUID:               e.Network.Uuid,
			ClusterNetworkName:               e.Network.Name,
			ClusterNetworkDescription:        &e.Network.Description,
			ClusterNetworkTypeCode:           e.Network.TypeCode,
			ClusterNetworkState:              e.Network.State,
			ClusterSubnetID:                  e.Subnet.Id,
			ClusterSubnetUUID:                e.Subnet.Uuid,
			ClusterSubnetName:                e.Subnet.Name,
			ClusterSubnetDescription:         &e.Subnet.Description,
			ClusterSubnetNetworkCidr:         e.Subnet.NetworkCidr,
			ClusterSubnetDhcpEnabled:         e.Subnet.DhcpEnabled,
			ClusterSubnetGatewayEnabled:      e.Subnet.GatewayEnabled,
			ClusterSubnetGatewayIPAddress:    &e.Subnet.GatewayIpAddress,
			ClusterSubnetIpv6AddressModeCode: &e.Subnet.Ipv6AddressModeCode,
			ClusterSubnetIpv6RaModeCode:      &e.Subnet.Ipv6RaModeCode,
			ClusterIPAddress:                 e.IpAddress,
			ProtectionFlag:                   true,
		}

		if err := validateExternalRoutingInterfaceRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedRouterRecoveryResult] Errors occurred during validating external routing interface recovery result(%d): router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, router.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err := database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedRouterRecoveryResult] Could not save external routing interface result(%d) db: router(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, router.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendFloatingIPRecoveryResult(ctx context.Context, task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateFloatingIPInputOutput(task)
	if err != nil {
		logger.Errorf("[appendFloatingIPRecoveryResult] Could not get floating IP input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultFloatingIP{
		RecoveryResultID:                       r.ID,
		ProtectionClusterFloatingIPID:          input.FloatingIP.Id,
		ProtectionClusterFloatingIPUUID:        input.FloatingIP.Uuid,
		ProtectionClusterFloatingIPDescription: &input.FloatingIP.Description,
		ProtectionClusterFloatingIPIPAddress:   input.FloatingIP.IpAddress,
		ProtectionClusterNetworkID:             input.FloatingIP.Network.Id,
		ProtectionClusterNetworkUUID:           input.FloatingIP.Network.Uuid,
		ProtectionClusterNetworkName:           input.FloatingIP.Network.Name,
		ProtectionClusterNetworkDescription:    &input.FloatingIP.Network.Description,
		ProtectionClusterNetworkTypeCode:       input.FloatingIP.Network.TypeCode,
		ProtectionClusterNetworkState:          input.FloatingIP.Network.State,
		RecoveryClusterFloatingIPUUID:          &output.FloatingIP.Uuid,
		RecoveryClusterFloatingIPDescription:   &output.FloatingIP.Description,
		RecoveryClusterFloatingIPIPAddress:     &output.FloatingIP.IpAddress,
	}

	rn, err := cluster.GetClusterNetworkByUUID(ctx, r.Job.RecoveryCluster.Id, input.Network.Uuid)
	if err == nil {
		m.RecoveryClusterNetworkID = &rn.Id
		m.RecoveryClusterNetworkUUID = &rn.Uuid
		m.RecoveryClusterNetworkName = &rn.Name
		m.RecoveryClusterNetworkDescription = &rn.Description
		m.RecoveryClusterNetworkTypeCode = &rn.TypeCode
		m.RecoveryClusterNetworkState = &rn.State
	} else {
		logger.Warnf("[appendFloatingIPRecoveryResult] Could not get recovery(%d) network(%s) information: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.Job.RecoveryCluster.Id, input.Network.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
	}

	if err = validateFloatingIPRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFloatingIPRecoveryResult] Errors occurred during validating floating IP(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.FloatingIP.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFloatingIPRecoveryResult] Could not save floating IP(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.FloatingIP.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendFailedFloatingIPRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.FloatingIPCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedFloatingIPRecoveryResult] Could not get floating IP input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultFloatingIP{
		RecoveryResultID:                       r.ID,
		ProtectionClusterFloatingIPID:          input.FloatingIP.Id,
		ProtectionClusterFloatingIPUUID:        input.FloatingIP.Uuid,
		ProtectionClusterFloatingIPDescription: &input.FloatingIP.Description,
		ProtectionClusterFloatingIPIPAddress:   input.FloatingIP.IpAddress,
		ProtectionClusterNetworkID:             input.FloatingIP.Network.Id,
		ProtectionClusterNetworkUUID:           input.FloatingIP.Network.Uuid,
		ProtectionClusterNetworkName:           input.FloatingIP.Network.Name,
		ProtectionClusterNetworkDescription:    &input.FloatingIP.Network.Description,
		ProtectionClusterNetworkTypeCode:       input.FloatingIP.Network.TypeCode,
		ProtectionClusterNetworkState:          input.FloatingIP.Network.State,
	}

	if err := validateFloatingIPRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedFloatingIPRecoveryResult] Errors occurred during validating floating IP(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.FloatingIP.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedFloatingIPRecoveryResult] Could not save floating IP(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.FloatingIP.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendSecurityGroupRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateSecurityGroupInputOutput(task)
	if err != nil {
		logger.Errorf("[appendSecurityGroupRecoveryResult] Could not get security group input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	tid, ok := r.TenantIDMap[input.Tenant.Uuid]
	if !ok {
		logger.Errorf("[appendSecurityGroupRecoveryResult] Not found tenant(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found tenant creation task result"))
	}

	m := model.RecoveryResultSecurityGroup{
		RecoveryResultID:                          r.ID,
		ProtectionClusterTenantID:                 tid,
		ProtectionClusterSecurityGroupID:          input.SecurityGroup.Id,
		ProtectionClusterSecurityGroupUUID:        input.SecurityGroup.Uuid,
		ProtectionClusterSecurityGroupName:        input.SecurityGroup.Name,
		ProtectionClusterSecurityGroupDescription: &input.SecurityGroup.Description,
		RecoveryClusterSecurityGroupUUID:          &output.SecurityGroup.Uuid,
		RecoveryClusterSecurityGroupName:          &output.SecurityGroup.Name,
		RecoveryClusterSecurityGroupDescription:   &output.SecurityGroup.Description,
	}

	if err = validateSecurityGroupRecoveryResult(&m); err != nil {
		logger.Errorf("[appendSecurityGroupRecoveryResult] Errors occurred during validating security group(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.SecurityGroup.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendSecurityGroupRecoveryResult] Could not save security group(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.SecurityGroup.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	r.SecurityGroupIDMap[output.SecurityGroup.Uuid] = input.SecurityGroup.Id

	return nil
}

func (r *recoveryResult) appendFailedSecurityGroupRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.SecurityGroupCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedSecurityGroupRecoveryResult] Could not get security group input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	securityGroup, ok := r.SecurityGroupMap[input.SecurityGroup.Id]
	if !ok {
		logger.Errorf("[appendFailedSecurityGroupRecoveryResult] Not found security group(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.SecurityGroup.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found security group creation task result"))
	}

	m := model.RecoveryResultSecurityGroup{
		RecoveryResultID:                          r.ID,
		ProtectionClusterTenantID:                 securityGroup.Tenant.Id,
		ProtectionClusterSecurityGroupID:          securityGroup.Id,
		ProtectionClusterSecurityGroupUUID:        securityGroup.Uuid,
		ProtectionClusterSecurityGroupName:        securityGroup.Name,
		ProtectionClusterSecurityGroupDescription: &securityGroup.Description,
	}
	if err := validateSecurityGroupRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedSecurityGroupRecoveryResult] Errors occurred during validating security group(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.SecurityGroup.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedSecurityGroupRecoveryResult] Could not save security group(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.SecurityGroup.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendSecurityGroupRuleRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateSecurityGroupRuleInputOutput(task)
	if err != nil {
		logger.Errorf("[appendSecurityGroupRuleRecoveryResult] Could not get security group rule input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	tid, ok := r.TenantIDMap[input.Tenant.Uuid]
	if !ok {
		logger.Errorf("[appendSecurityGroupRuleRecoveryResult] Not found tenant(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.Tenant.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found tenant creation task result"))
	}

	sid, ok := r.SecurityGroupIDMap[input.SecurityGroup.Uuid]
	if !ok {
		logger.Errorf("[appendSecurityGroupRuleRecoveryResult] Not found security group(%s) task result(%d): job(%d) typeCode(%s) task(%s).",
			input.SecurityGroup.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found security group creation task result"))
	}

	min := uint64(output.SecurityGroupRule.PortRangeMin)
	max := uint64(output.SecurityGroupRule.PortRangeMax)

	m := model.RecoveryResultSecurityGroupRule{
		RecoveryResultID:                               r.ID,
		ProtectionClusterTenantID:                      tid,
		ProtectionClusterSecurityGroupID:               sid,
		ProtectionClusterSecurityGroupRuleID:           input.SecurityGroupRule.Id,
		ProtectionClusterSecurityGroupRuleUUID:         input.SecurityGroupRule.Uuid,
		ProtectionClusterSecurityGroupRuleDescription:  &input.SecurityGroupRule.Description,
		ProtectionClusterSecurityGroupRuleNetworkCidr:  &input.SecurityGroupRule.NetworkCidr,
		ProtectionClusterSecurityGroupRuleDirection:    input.SecurityGroupRule.Direction,
		ProtectionClusterSecurityGroupRulePortRangeMin: &min, // mirroring
		ProtectionClusterSecurityGroupRulePortRangeMax: &max, // mirroring
		ProtectionClusterSecurityGroupRuleProtocol:     &input.SecurityGroupRule.Protocol,
		RecoveryClusterSecurityGroupRuleUUID:           &output.SecurityGroupRule.Uuid,
		RecoveryClusterSecurityGroupRuleDescription:    &output.SecurityGroupRule.Description,
		RecoveryClusterSecurityGroupRuleNetworkCidr:    &output.SecurityGroupRule.NetworkCidr,
		RecoveryClusterSecurityGroupRuleDirection:      &output.SecurityGroupRule.Direction,
		RecoveryClusterSecurityGroupRulePortRangeMin:   &min,
		RecoveryClusterSecurityGroupRulePortRangeMax:   &max,
		RecoveryClusterSecurityGroupRuleProtocol:       &output.SecurityGroupRule.Protocol,
	}

	if input.SecurityGroupRule.GetRemoteSecurityGroup().GetId() != 0 {
		m.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID = &input.SecurityGroupRule.RemoteSecurityGroup.Id
	}

	if output.SecurityGroupRule.GetRemoteSecurityGroup().GetId() != 0 {
		m.RecoveryClusterSecurityGroupRuleRemoteSecurityGroupID = &output.SecurityGroupRule.RemoteSecurityGroup.Id
	}

	if err = validateSecurityGroupRuleRecoveryResult(&m); err != nil {
		logger.Errorf("[appendSecurityGroupRuleRecoveryResult] Errors occurred during validating security group rule(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.SecurityGroupRule.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendSecurityGroupRuleRecoveryResult] Could not save security group rule(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.SecurityGroupRule.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendFailedSecurityGroupRuleRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.SecurityGroupRuleCreateTaskInputData
	var securityGroup *cms.ClusterSecurityGroup
	var securityGroupRule *cms.ClusterSecurityGroupRule

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedSecurityGroupRuleRecoveryResult] Could not get security group rule input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	for _, sg := range r.SecurityGroupMap {
		for _, rule := range sg.Rules {
			if rule.Id == input.SecurityGroupRule.ID {
				securityGroup = sg
				securityGroupRule = rule
				break
			}
		}
	}

	min := uint64(securityGroupRule.PortRangeMin)
	max := uint64(securityGroupRule.PortRangeMax)

	m := model.RecoveryResultSecurityGroupRule{
		RecoveryResultID:                               r.ID,
		ProtectionClusterTenantID:                      securityGroup.Tenant.Id,
		ProtectionClusterSecurityGroupID:               securityGroup.Id,
		ProtectionClusterSecurityGroupRuleID:           securityGroupRule.Id,
		ProtectionClusterSecurityGroupRuleUUID:         securityGroupRule.Uuid,
		ProtectionClusterSecurityGroupRuleDescription:  &securityGroupRule.Description,
		ProtectionClusterSecurityGroupRuleNetworkCidr:  &securityGroupRule.NetworkCidr,
		ProtectionClusterSecurityGroupRuleDirection:    securityGroupRule.Direction,
		ProtectionClusterSecurityGroupRulePortRangeMin: &min,
		ProtectionClusterSecurityGroupRulePortRangeMax: &max,
		ProtectionClusterSecurityGroupRuleProtocol:     &securityGroupRule.Protocol,
	}

	if securityGroupRule.GetRemoteSecurityGroup().GetId() != 0 {
		m.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID = &securityGroupRule.RemoteSecurityGroup.Id
	}

	if err := validateSecurityGroupRuleRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedSecurityGroupRuleRecoveryResult] Errors occurred during validating security group rule(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			securityGroupRule.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedSecurityGroupRuleRecoveryResult] Could not save security group rule(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			securityGroupRule.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendVolumeRecoveryResult(task *migrator.RecoveryJobTask) error {
	var volume *cms.ClusterVolume

	input, output, err := getCreateVolumeInputOutput(task)
	if err != nil {
		logger.Errorf("[appendVolumeRecoveryResult] Could not get volume input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	for _, v := range r.JobDetail.Plan.Detail.Volumes {
		if v.ProtectionClusterVolume.Id == input.VolumeID {
			volume = v.ProtectionClusterVolume
		}
	}
	if volume == nil {
		logger.Errorf("[appendVolumeRecoveryResult] Not found volume(%d) information: result(%d) job(%d) typeCode(%s) task(%s).",
			input.VolumeID, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found volume creation task result"))
	}

	vStatus, err := r.Job.GetVolumeStatus(volume.Id)
	if err != nil {
		logger.Errorf("[appendVolumeRecoveryResult] Could not get job volume status: dr.recovery.job/%d/result/volume/%d/status. Cause: %+v",
			r.Job.RecoveryJobID, volume.Id, err)
		return err
	}

	m := model.RecoveryResultVolume{
		RecoveryResultID:                    r.ID,
		ProtectionClusterTenantID:           volume.Tenant.Id,
		ProtectionClusterVolumeID:           volume.Id,
		ProtectionClusterVolumeDescription:  &volume.Description,
		ProtectionClusterVolumeSizeBytes:    volume.SizeBytes,
		ProtectionClusterVolumeMultiattach:  volume.Multiattach,
		ProtectionClusterVolumeBootable:     volume.Bootable,
		ProtectionClusterVolumeReadonly:     volume.Readonly,
		ProtectionClusterStorageID:          input.SourceStorage.Id,
		ProtectionClusterStorageUUID:        input.SourceStorage.Uuid,
		ProtectionClusterStorageName:        input.SourceStorage.Name,
		ProtectionClusterStorageDescription: &input.SourceStorage.Description,
		ProtectionClusterStorageTypeCode:    input.SourceStorage.TypeCode,
		RecoveryPointTypeCode:               vStatus.RecoveryPointTypeCode,
		RecoveryPoint:                       vStatus.RecoveryPoint,
		StartedAt:                           &vStatus.StartedAt,
		FinishedAt:                          &vStatus.FinishedAt,
		ResultCode:                          vStatus.ResultCode,
		RollbackFlag:                        vStatus.RollbackFlag,
		RecoveryClusterVolumeUUID:           &output.VolumePair.Target.Uuid,
		RecoveryClusterVolumeName:           &output.VolumePair.Target.Name,
		RecoveryClusterVolumeDescription:    &output.VolumePair.Target.Description,
		RecoveryClusterVolumeSizeBytes:      &output.VolumePair.Target.SizeBytes,
		RecoveryClusterVolumeMultiattach:    &output.VolumePair.Target.Multiattach,
		RecoveryClusterVolumeBootable:       &output.VolumePair.Target.Bootable,
		RecoveryClusterVolumeReadonly:       &output.VolumePair.Target.Readonly,
		RecoveryClusterStorageID:            &input.TargetStorage.Id,
		RecoveryClusterStorageUUID:          &input.TargetStorage.Uuid,
		RecoveryClusterStorageName:          &input.TargetStorage.Name,
		RecoveryClusterStorageDescription:   &input.TargetStorage.Description,
		RecoveryClusterStorageTypeCode:      &input.TargetStorage.TypeCode,
	}

	// 원본 볼륨을 이용해 복구한 경우 볼륨의 uuid 와 이름은 그대로 사용하며,
	// 스냅샷 볼륨을 이용해 복구한 경우 볼륨의 uuid 는 그대로, 이름은 uuid@snap-날짜 형식으로 저장한다.
	slice := strings.Split(volume.Uuid, "@")
	if len(slice) == 1 {
		m.ProtectionClusterVolumeUUID = volume.Uuid
		m.ProtectionClusterVolumeName = &volume.Name
	} else if len(slice) == 2 {
		m.ProtectionClusterVolumeUUID = slice[0]
		m.ProtectionClusterVolumeName = &volume.Uuid
	}

	if err = validateVolumeRecoveryResult(&m); err != nil {
		logger.Errorf("[appendVolumeRecoveryResult] Errors occurred during validating volume(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			volume.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendVolumeRecoveryResult] Could not save volume(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			volume.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendFailedVolumeRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.VolumeImportTaskInputData
	var volume *cms.ClusterVolume

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedVolumeRecoveryResult] Could not get volume input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	for _, v := range r.JobDetail.Plan.Detail.Volumes {
		if v.ProtectionClusterVolume.Id == input.VolumeID {
			volume = v.ProtectionClusterVolume
		}
	}
	if volume == nil {
		logger.Errorf("[appendFailedVolumeRecoveryResult] Not found volume(%d) information: result(%d) job(%d) typeCode(%s) task(%s).",
			input.VolumeID, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return errors.Unknown(errors.New("not found volume creation task result"))
	}

	vStatus, err := r.Job.GetVolumeStatus(volume.Id)
	if err != nil {
		logger.Errorf("[appendFailedVolumeRecoveryResult] Could not get job volume status: dr.recovery.job/%d/result/volume/%d/status. Cause: %+v",
			r.Job.RecoveryJobID, volume.Id, err)
		return err
	}

	m := model.RecoveryResultVolume{
		RecoveryResultID:                    r.ID,
		ProtectionClusterTenantID:           volume.Tenant.Id,
		ProtectionClusterVolumeID:           volume.Id,
		ProtectionClusterVolumeDescription:  &volume.Description,
		ProtectionClusterVolumeSizeBytes:    volume.SizeBytes,
		ProtectionClusterVolumeMultiattach:  volume.Multiattach,
		ProtectionClusterVolumeBootable:     volume.Bootable,
		ProtectionClusterVolumeReadonly:     volume.Readonly,
		ProtectionClusterStorageID:          volume.Storage.Id,
		ProtectionClusterStorageUUID:        volume.Storage.Uuid,
		ProtectionClusterStorageName:        volume.Storage.Name,
		ProtectionClusterStorageDescription: &volume.Storage.Description,
		ProtectionClusterStorageTypeCode:    volume.Storage.TypeCode,
		RecoveryPointTypeCode:               vStatus.RecoveryPointTypeCode,
		RecoveryPoint:                       vStatus.RecoveryPoint,
		StartedAt:                           &vStatus.StartedAt,
		FinishedAt:                          &vStatus.FinishedAt,
		ResultCode:                          vStatus.ResultCode,
		RollbackFlag:                        vStatus.RollbackFlag,
	}

	// 원본 볼륨을 이용해 복구한 경우 볼륨의 uuid 와 이름은 그대로 사용하며,
	// 스냅샷 볼륨을 이용해 복구한 경우 볼륨의 uuid 는 그대로, 이름은 uuid@snap-날짜 형식으로 저장한다.
	slice := strings.Split(volume.Uuid, "@")
	if len(slice) == 1 {
		m.ProtectionClusterVolumeUUID = volume.Uuid
		m.ProtectionClusterVolumeName = &volume.Name
	} else if len(slice) == 2 {
		m.ProtectionClusterVolumeUUID = slice[0]
		m.ProtectionClusterVolumeName = &volume.Uuid
	}

	if vStatus.FailedReason != nil {
		m.FailedReasonCode = &vStatus.FailedReason.Code
		m.FailedReasonContents = &vStatus.FailedReason.Contents
	}

	if err = validateVolumeRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedVolumeRecoveryResult] Errors occurred during validating volume(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			volume.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedVolumeRecoveryResult] Could not save volume(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			volume.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendInstanceSpecRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateInstanceSpecInputOutput(task)
	if err != nil {
		logger.Errorf("[appendInstanceSpecRecoveryResult] Could not get instance spec input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	vCPUTotalCnt := uint64(output.Spec.VcpuTotalCnt)

	m := model.RecoveryResultInstanceSpec{
		RecoveryResultID:                                 r.ID,
		ProtectionClusterInstanceSpecID:                  input.Spec.Id,
		ProtectionClusterInstanceSpecUUID:                input.Spec.Uuid,
		ProtectionClusterInstanceSpecName:                input.Spec.Name,
		ProtectionClusterInstanceSpecDescription:         &input.Spec.Description,
		ProtectionClusterInstanceSpecVcpuTotalCnt:        vCPUTotalCnt,
		ProtectionClusterInstanceSpecMemTotalBytes:       input.Spec.MemTotalBytes,
		ProtectionClusterInstanceSpecDiskTotalBytes:      input.Spec.DiskTotalBytes,
		ProtectionClusterInstanceSpecSwapTotalBytes:      input.Spec.SwapTotalBytes,
		ProtectionClusterInstanceSpecEphemeralTotalBytes: input.Spec.EphemeralTotalBytes,
		RecoveryClusterInstanceSpecUUID:                  &output.Spec.Uuid,
		RecoveryClusterInstanceSpecName:                  &output.Spec.Name,
		RecoveryClusterInstanceSpecDescription:           &output.Spec.Description,
		RecoveryClusterInstanceSpecVcpuTotalCnt:          &vCPUTotalCnt,
		RecoveryClusterInstanceSpecMemTotalBytes:         &output.Spec.MemTotalBytes,
		RecoveryClusterInstanceSpecDiskTotalBytes:        &output.Spec.DiskTotalBytes,
		RecoveryClusterInstanceSpecSwapTotalBytes:        &output.Spec.SwapTotalBytes,
		RecoveryClusterInstanceSpecEphemeralTotalBytes:   &output.Spec.EphemeralTotalBytes,
	}

	if err = validateInstanceSpecRecoveryResult(&m); err != nil {
		logger.Errorf("[appendInstanceSpecRecoveryResult] Errors occurred during validating instance spec(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Spec.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendInstanceSpecRecoveryResult] Could not save instance spec(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Spec.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// instance extra spec
	for i, e := range output.ExtraSpecs {
		id := uint64(i)

		m := model.RecoveryResultInstanceExtraSpec{
			RecoveryResultID:                        r.ID,
			ProtectionClusterInstanceSpecID:         input.Spec.Id,
			ProtectionClusterInstanceExtraSpecID:    id,
			ProtectionClusterInstanceExtraSpecKey:   e.Key,   // mirroring
			ProtectionClusterInstanceExtraSpecValue: e.Value, // mirroring
			RecoveryClusterInstanceExtraSpecID:      &id,
			RecoveryClusterInstanceExtraSpecKey:     &e.Key,
			RecoveryClusterInstanceExtraSpecValue:   &e.Value,
		}

		if err = validateExtraSpecRecoveryResult(&m); err != nil {
			logger.Errorf("[appendInstanceSpecRecoveryResult] Errors occurred during validating extra spec result(%d): instanceSpec(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, input.Spec.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendInstanceSpecRecoveryResult] Could not save extra spec result(%d) db: instanceSpec(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, input.Spec.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendFailedInstanceSpecRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.InstanceSpecCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedInstanceSpecRecoveryResult] Could not get instance spec input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultInstanceSpec{
		RecoveryResultID:                                 r.ID,
		ProtectionClusterInstanceSpecID:                  input.Spec.Id,
		ProtectionClusterInstanceSpecUUID:                input.Spec.Uuid,
		ProtectionClusterInstanceSpecName:                input.Spec.Name,
		ProtectionClusterInstanceSpecDescription:         &input.Spec.Description,
		ProtectionClusterInstanceSpecVcpuTotalCnt:        uint64(input.Spec.VcpuTotalCnt),
		ProtectionClusterInstanceSpecMemTotalBytes:       input.Spec.MemTotalBytes,
		ProtectionClusterInstanceSpecDiskTotalBytes:      input.Spec.DiskTotalBytes,
		ProtectionClusterInstanceSpecSwapTotalBytes:      input.Spec.SwapTotalBytes,
		ProtectionClusterInstanceSpecEphemeralTotalBytes: input.Spec.EphemeralTotalBytes,
	}

	if err := validateInstanceSpecRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedInstanceSpecRecoveryResult] Errors occurred during validating instance spec(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Spec.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedInstanceSpecRecoveryResult] Could not save instance spec(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Spec.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// instance extra spec
	for i, e := range input.ExtraSpecs {
		m := model.RecoveryResultInstanceExtraSpec{
			RecoveryResultID:                        r.ID,
			ProtectionClusterInstanceSpecID:         input.Spec.Id,
			ProtectionClusterInstanceExtraSpecID:    uint64(i),
			ProtectionClusterInstanceExtraSpecKey:   e.Key,
			ProtectionClusterInstanceExtraSpecValue: e.Value,
		}

		if err := validateExtraSpecRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedInstanceSpecRecoveryResult] Errors occurred during validating extra spec result(%d): instanceSpec(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, input.Spec.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err := database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedInstanceSpecRecoveryResult] Could not save extra spec result(%d) db: instanceSpec(%s) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, input.Spec.Uuid, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendKeypairRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateKeypairInputOutput(task)
	if err != nil {
		logger.Errorf("[appendKeypairRecoveryResult] Could not get keypair input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultKeypair{
		RecoveryResultID:                    r.ID,
		ProtectionClusterKeypairID:          input.Keypair.Id,
		ProtectionClusterKeypairName:        input.Keypair.Name,
		ProtectionClusterKeypairFingerprint: input.Keypair.Fingerprint,
		ProtectionClusterKeypairPublicKey:   input.Keypair.PublicKey,
		ProtectionClusterKeypairTypeCode:    input.Keypair.TypeCode,
		RecoveryClusterKeypairID:            &output.Keypair.Id,
		RecoveryClusterKeypairName:          &output.Keypair.Name,
		RecoveryClusterKeypairFingerprint:   &output.Keypair.Fingerprint,
		RecoveryClusterKeypairPublicKey:     &output.Keypair.PublicKey,
		RecoveryClusterKeypairTypeCode:      &output.Keypair.TypeCode,
	}

	if err = validateKeypairRecoveryResult(&m); err != nil {
		logger.Errorf("[appendKeypairRecoveryResult] Errors occurred during validating keypair(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Keypair.Name, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendKeypairRecoveryResult] Could not save keypair(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Keypair.Name, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendFailedKeypairRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.KeypairCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedKeypairRecoveryResult] Could not get keypair input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	m := model.RecoveryResultKeypair{
		RecoveryResultID:                    r.ID,
		ProtectionClusterKeypairID:          input.Keypair.Id,
		ProtectionClusterKeypairName:        input.Keypair.Name,
		ProtectionClusterKeypairFingerprint: input.Keypair.Fingerprint,
		ProtectionClusterKeypairPublicKey:   input.Keypair.PublicKey,
		ProtectionClusterKeypairTypeCode:    input.Keypair.TypeCode,
	}

	if err := validateKeypairRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedKeypairRecoveryResult] Errors occurred during validating keypair(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Keypair.Name, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedKeypairRecoveryResult] Could not save keypair(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Keypair.Name, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func (r *recoveryResult) appendInstanceRecoveryResult(task *migrator.RecoveryJobTask) error {
	input, output, err := getCreateInstanceInputOutput(task)
	if err != nil {
		logger.Errorf("[appendInstanceRecoveryResult] Could not get instance input and output: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	iPlan := internal.GetInstanceRecoveryPlan(r.JobDetail.Plan, input.Instance.Id)
	if iPlan == nil {
		logger.Errorf("[appendInstanceRecoveryResult] Not found instance(%d) plan(%d): result(%d) job(%d) typeCode(%s) task(%s).",
			input.Instance.Id, r.JobDetail.Plan.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return internal.NotFoundInstanceRecoveryPlan(r.JobDetail.Plan.Id, input.Instance.Id)
	}

	iStatus, err := r.Job.GetInstanceStatus(input.Instance.Id)
	if err != nil {
		logger.Errorf("[appendInstanceRecoveryResult] Could not get job instance status: dr.recovery.job/%d/result/instance/%d/status. Cause: %+v",
			r.Job.RecoveryJobID, input.Instance.Id, err)
		return errors.Unknown(errors.New("not found instance status"))
	}

	m := model.RecoveryResultInstance{
		RecoveryResultID:                      r.ID,
		ProtectionClusterTenantID:             input.Instance.Tenant.Id,
		ProtectionClusterInstanceID:           input.Instance.Id,
		ProtectionClusterInstanceUUID:         input.Instance.Uuid,
		ProtectionClusterInstanceName:         input.Instance.Name,
		ProtectionClusterInstanceDescription:  &input.Instance.Description,
		ProtectionClusterInstanceSpecID:       input.Instance.Spec.Id,
		ProtectionClusterAvailabilityZoneID:   input.Instance.AvailabilityZone.Id,
		ProtectionClusterAvailabilityZoneName: input.Instance.AvailabilityZone.Name,
		ProtectionClusterHypervisorID:         input.Instance.Hypervisor.Id,
		ProtectionClusterHypervisorTypeCode:   input.Instance.Hypervisor.TypeCode,
		ProtectionClusterHypervisorHostname:   input.Instance.Hypervisor.Hostname,
		ProtectionClusterHypervisorIPAddress:  input.Instance.Hypervisor.IpAddress,
		RecoveryClusterInstanceUUID:           &output.Instance.Uuid,
		RecoveryClusterInstanceName:           &output.Instance.Name,
		RecoveryClusterInstanceDescription:    &output.Instance.Description,
		RecoveryClusterAvailabilityZoneID:     &input.AvailabilityZone.Id,
		RecoveryClusterAvailabilityZoneName:   &input.AvailabilityZone.Name,
		RecoveryClusterHypervisorID:           &input.Hypervisor.Id,
		RecoveryClusterHypervisorTypeCode:     &input.Hypervisor.TypeCode,
		RecoveryClusterHypervisorHostname:     &input.Hypervisor.Hostname,
		RecoveryClusterHypervisorIPAddress:    &input.Hypervisor.IpAddress,
		RecoveryPointTypeCode:                 iStatus.RecoveryPointTypeCode,
		RecoveryPoint:                         iStatus.RecoveryPoint,
		AutoStartFlag:                         iPlan.AutoStartFlag,
		DiagnosisFlag:                         iPlan.DiagnosisFlag,
		DiagnosisMethodCode:                   &iPlan.DiagnosisMethodCode,
		DiagnosisMethodData:                   &iPlan.DiagnosisMethodData,
		DiagnosisTimeout:                      &iPlan.DiagnosisTimeout,
		StartedAt:                             &iStatus.StartedAt,
		FinishedAt:                            &iStatus.FinishedAt,
		ResultCode:                            iStatus.ResultCode,
	}

	if input.Instance.GetKeypair().GetId() != 0 {
		m.ProtectionClusterKeypairID = &input.Instance.Keypair.Id
	}

	if err = validateInstanceRecoveryResult(&m); err != nil {
		logger.Errorf("[appendInstanceRecoveryResult] Errors occurred during validating instance(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendInstanceRecoveryResult] Could not save instance(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// instance volume
	for _, pv := range input.Instance.Volumes {
		m := model.RecoveryResultInstanceVolume{
			RecoveryResultID:            r.ID,
			ProtectionClusterTenantID:   input.Instance.Tenant.Id,
			ProtectionClusterInstanceID: input.Instance.Id,
			ProtectionClusterVolumeID:   pv.Volume.Id,
			ProtectionClusterDevicePath: pv.DevicePath,
			ProtectionClusterBootIndex:  pv.BootIndex,
			RecoveryClusterDevicePath:   &pv.DevicePath, // mirroring
			RecoveryClusterBootIndex:    &pv.BootIndex,  // mirroring
		}

		if err = validateInstanceVolumeRecoveryResult(&m); err != nil {
			logger.Errorf("[appendInstanceRecoveryResult] Errors occurred during validating instance(%s) volume(%d) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pv.Volume.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendInstanceRecoveryResult] Could not save instance(%s) volume(%d) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pv.Volume.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// 기존 복구 시점이 아닌 다른 시점의 데이터로 재시도한 작업의 경우,
	// 인스턴스의 네트워크, 보안그룹, 선행 인스턴스를 복구 하지 않기 때문에 복구 결과가 존재하지 않는다.
	if !task.ForceRetryFlag {
		// instance dependency
		for _, d := range iPlan.Dependencies {
			m := model.RecoveryResultInstanceDependency{
				RecoveryResultID:                      r.ID,
				ProtectionClusterTenantID:             input.Instance.Tenant.Id,
				ProtectionClusterInstanceID:           input.Instance.Id,
				DependencyProtectionClusterInstanceID: d.Id,
			}

			if err = validateInstanceDependencyRecoveryResult(&m); err != nil {
				logger.Errorf("[appendInstanceRecoveryResult] Errors occurred during validating instance(%s) dependency result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
					input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return err
			}

			if err = database.Execute(func(db *gorm.DB) error {
				return db.Save(&m).Error
			}); err != nil {
				logger.Errorf("[appendInstanceRecoveryResult] Could not save instance(%s) dependency result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
					input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return errors.UnusableDatabase(err)
			}
		}

		// instance network
		for _, pn := range input.Instance.Networks {
			var seq uint64

			seq = seq + 1

			m := model.RecoveryResultInstanceNetwork{
				RecoveryResultID:            r.ID,
				ProtectionClusterTenantID:   input.Instance.Tenant.Id,
				ProtectionClusterInstanceID: input.Instance.Id,
				InstanceNetworkSeq:          seq,
				ProtectionClusterNetworkID:  pn.Network.Id,
				ProtectionClusterSubnetID:   pn.Subnet.Id,
				ProtectionClusterDhcpFlag:   pn.DhcpFlag,
				ProtectionClusterIPAddress:  pn.IpAddress,
				RecoveryClusterDhcpFlag:     &pn.DhcpFlag,  // mirroring
				RecoveryClusterIPAddress:    &pn.IpAddress, // mirroring
			}

			if pn.GetFloatingIp().GetId() != 0 {
				m.ProtectionClusterFloatingIPID = &pn.FloatingIp.Id
			}

			if err = validateInstanceNetworkRecoveryResult(&m); err != nil {
				logger.Errorf("[appendInstanceRecoveryResult] Errors occurred during validating instance(%s) network(%d) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
					input.Instance.Uuid, pn.Network.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return err
			}

			if err = database.Execute(func(db *gorm.DB) error {
				return db.Save(&m).Error
			}); err != nil {
				logger.Errorf("[appendInstanceRecoveryResult] Could not save instance(%s) network(%d) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
					input.Instance.Uuid, pn.Network.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return errors.UnusableDatabase(err)
			}
		}

		// instance security group
		for _, pSg := range input.Instance.SecurityGroups {
			m := model.RecoveryResultInstanceSecurityGroup{
				RecoveryResultID:                 r.ID,
				ProtectionClusterTenantID:        input.Instance.Tenant.Id,
				ProtectionClusterInstanceID:      input.Instance.Id,
				ProtectionClusterSecurityGroupID: pSg.Id,
			}

			if err = validateInstanceSecurityGroupRecoveryResult(&m); err != nil {
				logger.Errorf("[appendInstanceRecoveryResult] Errors occurred during validating instance(%s) security group(%d) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
					input.Instance.Uuid, pSg.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return err
			}

			if err = database.Execute(func(db *gorm.DB) error {
				return db.Save(&m).Error
			}); err != nil {
				logger.Errorf("[appendInstanceRecoveryResult] Could not save instance(%s) security group(%d) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
					input.Instance.Uuid, pSg.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
				return errors.UnusableDatabase(err)
			}
		}
	}

	return nil
}

func (r *recoveryResult) appendFailedInstanceRecoveryResult(task *migrator.RecoveryJobTask) error {
	var input migrator.InstanceCreateTaskInputData

	if err := task.GetInputData(&input); err != nil {
		logger.Errorf("[appendFailedInstanceRecoveryResult] Could not get instance input: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	iPlan := internal.GetInstanceRecoveryPlan(r.JobDetail.Plan, input.Instance.Id)
	if iPlan == nil {
		return internal.NotFoundInstanceRecoveryPlan(r.JobDetail.Plan.Id, input.Instance.Id)
	}

	instance := iPlan.ProtectionClusterInstance

	iStatus, err := r.Job.GetInstanceStatus(instance.Id)
	if err != nil {
		logger.Errorf("[appendFailedInstanceRecoveryResult] Could not get job instance status: dr.recovery.job/%d/result/instance/%d. Cause: %+v",
			r.Job.RecoveryJobID, instance.Id, err)
		return errors.Unknown(errors.New("not found instance status"))
	}

	m := model.RecoveryResultInstance{
		RecoveryResultID:                      r.ID,
		ProtectionClusterTenantID:             instance.Tenant.Id,
		ProtectionClusterInstanceID:           instance.Id,
		ProtectionClusterInstanceUUID:         instance.Uuid,
		ProtectionClusterInstanceName:         instance.Name,
		ProtectionClusterInstanceDescription:  &instance.Description,
		ProtectionClusterInstanceSpecID:       instance.Spec.Id,
		ProtectionClusterAvailabilityZoneID:   instance.AvailabilityZone.Id,
		ProtectionClusterAvailabilityZoneName: instance.AvailabilityZone.Name,
		ProtectionClusterHypervisorID:         instance.Hypervisor.Id,
		ProtectionClusterHypervisorTypeCode:   instance.Hypervisor.TypeCode,
		ProtectionClusterHypervisorHostname:   instance.Hypervisor.Hostname,
		ProtectionClusterHypervisorIPAddress:  instance.Hypervisor.IpAddress,
		RecoveryPointTypeCode:                 iStatus.RecoveryPointTypeCode,
		RecoveryPoint:                         iStatus.RecoveryPoint,
		AutoStartFlag:                         iPlan.AutoStartFlag,
		DiagnosisFlag:                         iPlan.DiagnosisFlag,
		DiagnosisMethodCode:                   &iPlan.DiagnosisMethodCode,
		DiagnosisMethodData:                   &iPlan.DiagnosisMethodData,
		DiagnosisTimeout:                      &iPlan.DiagnosisTimeout,
		StartedAt:                             &iStatus.StartedAt,
		FinishedAt:                            &iStatus.FinishedAt,
		ResultCode:                            iStatus.ResultCode,
	}

	if instance.GetKeypair().GetId() != 0 {
		m.ProtectionClusterKeypairID = &instance.Keypair.Id
	}

	if iStatus.FailedReason != nil {
		m.FailedReasonCode = &iStatus.FailedReason.Code
		m.FailedReasonContents = &iStatus.FailedReason.Contents
	}

	if err = validateInstanceRecoveryResult(&m); err != nil {
		logger.Errorf("[appendFailedInstanceRecoveryResult] Errors occurred during validating instance(%s) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		return db.Save(&m).Error
	}); err != nil {
		logger.Errorf("[appendFailedInstanceRecoveryResult] Could not save instance(%s) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
			input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		return errors.UnusableDatabase(err)
	}

	// instance dependency
	for _, d := range iPlan.Dependencies {
		m := model.RecoveryResultInstanceDependency{
			RecoveryResultID:                      r.ID,
			ProtectionClusterTenantID:             instance.Tenant.Id,
			ProtectionClusterInstanceID:           instance.Id,
			DependencyProtectionClusterInstanceID: d.Id,
		}

		if err = validateInstanceDependencyRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Errors occurred during validating instance(%s) dependency result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Could not save instance(%s) dependency result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// instance volume
	for _, pv := range instance.Volumes {
		m := model.RecoveryResultInstanceVolume{
			RecoveryResultID:            r.ID,
			ProtectionClusterTenantID:   instance.Tenant.Id,
			ProtectionClusterInstanceID: instance.Id,
			ProtectionClusterVolumeID:   pv.Volume.Id,
			ProtectionClusterDevicePath: pv.DevicePath,
			ProtectionClusterBootIndex:  pv.BootIndex,
		}

		if err = validateInstanceVolumeRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Errors occurred during validating instance(%s) volume(%d) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pv.Volume.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Could not save instance(%s) volume(%d) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pv.Volume.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// instance network
	for _, pn := range instance.Networks {
		var seq uint64

		seq = seq + 1

		m := model.RecoveryResultInstanceNetwork{
			RecoveryResultID:            r.ID,
			ProtectionClusterTenantID:   instance.Tenant.Id,
			ProtectionClusterInstanceID: instance.Id,
			InstanceNetworkSeq:          seq,
			ProtectionClusterNetworkID:  pn.Network.Id,
			ProtectionClusterSubnetID:   pn.Subnet.Id,
			ProtectionClusterDhcpFlag:   pn.DhcpFlag,
			ProtectionClusterIPAddress:  pn.IpAddress,
		}

		if pn.GetFloatingIp().GetId() != 0 {
			m.ProtectionClusterFloatingIPID = &pn.FloatingIp.Id
		}

		if err = validateInstanceNetworkRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Errors occurred during validating instance(%s) network(%d) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pn.Network.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Could not save instance(%s) network(%d) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pn.Network.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	// instance security group
	for _, pSg := range instance.SecurityGroups {
		m := model.RecoveryResultInstanceSecurityGroup{
			RecoveryResultID:                 r.ID,
			ProtectionClusterTenantID:        instance.Tenant.Id,
			ProtectionClusterInstanceID:      instance.Id,
			ProtectionClusterSecurityGroupID: pSg.Id,
		}

		if err = validateInstanceSecurityGroupRecoveryResult(&m); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Errors occurred during validating instance(%s) security group(%d) result(%d): job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pSg.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return err
		}

		if err = database.Execute(func(db *gorm.DB) error {
			return db.Save(&m).Error
		}); err != nil {
			logger.Errorf("[appendFailedInstanceRecoveryResult] Could not save instance(%s) security group(%d) result(%d) db: job(%d) typeCode(%s) task(%s). Cause: %+v",
				input.Instance.Uuid, pSg.Id, r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
			return errors.UnusableDatabase(err)
		}
	}

	return nil
}

func (r *recoveryResult) appendTaskRecoveryResult(ctx context.Context, task *migrator.RecoveryJobTask) error {
	var err error

	if task.TypeCode == constant.MigrationTaskTypeStopInstance {
		return nil
	}

	if r.IsAlreadyAppendTask[task.RecoveryJobTaskID] {
		logger.Infof("[appendTaskRecoveryResult] Already appended task: result(%d) job(%d) typeCode(%s) task(%s)",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		return nil
	}

	defer func() {
		if err == nil {
			r.IsAlreadyAppendTask[task.RecoveryJobTaskID] = true
		} else {
			logger.Errorf("[appendTaskRecoveryResult] Could not complete: result(%d) job(%d) typeCode(%s) task(%s). Cause: %+v",
				r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID, err)
		}
	}()

	if task.TypeCode == constant.MigrationTaskTypeCreateAndDiagnosisInstance {
		for _, d := range task.Dependencies {
			if r.IsAlreadyAppendTask[d] {
				continue
			}

			var t *migrator.RecoveryJobTask
			t, err = r.Job.GetTask(d)
			if errors.Equal(err, migrator.ErrNotFoundTask) {
				logger.Warnf("[appendTaskRecoveryResult] ErrNotFoundTask: result(%d) job(%d) depTask(%s) typeCode(%s) task(%s)",
					r.ID, task.RecoveryJobID, d, task.TypeCode, task.RecoveryJobTaskID)
				continue
			} else if err != nil {
				logger.Errorf("[appendTaskRecoveryResult] Could not get job task: dr.recovery.job/%d/task/%s. Cause: %+v",
					task.RecoveryJobID, d, err)
				return err
			}

			logger.Infof("[appendTaskRecoveryResult] append dependency task recovery result: result(%d) job(%d) depTask(%s:%s) task(%s:%s).",
				r.ID, task.RecoveryJobID, t.TypeCode, t.RecoveryJobTaskID, task.TypeCode, task.RecoveryJobTaskID)
			if err = r.appendTaskRecoveryResult(ctx, t); err != nil {
				continue
			}
		}
	}

	result, err := task.GetResult()
	if errors.Equal(err, migrator.ErrNotFoundTaskResult) {
		logger.Warnf("[appendTaskRecoveryResult] ErrNotFoundTaskResult: result(%d) job(%d) typeCode(%s) task(%s)",
			r.ID, task.RecoveryJobID, task.TypeCode, task.RecoveryJobTaskID)
		result.ResultCode = constant.MigrationTaskResultCodeUnknown
	} else if err != nil {
		logger.Errorf("[appendTaskRecoveryResult] Could not get job task: dr.recovery.job/%d/task/%s/result. Cause: %+v",
			task.RecoveryJobID, task.RecoveryJobTaskID, err)
		return err
	}

	logger.Infof("[appendTaskRecoveryResult] Start appendRecoveryResult - %s: job(%d) result(%s) task(%s)",
		task.TypeCode, task.RecoveryJobID, result.ResultCode, task.RecoveryJobTaskID)

	if result.ResultCode == constant.MigrationTaskResultCodeSuccess {
		switch task.TypeCode {
		case constant.MigrationTaskTypeCreateTenant:
			err = r.appendTenantRecoveryResult(task)

		case constant.MigrationTaskTypeCreateFloatingIP:
			err = r.appendFloatingIPRecoveryResult(ctx, task)

		case constant.MigrationTaskTypeCreateKeypair:
			err = r.appendKeypairRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSpec:
			err = r.appendInstanceSpecRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSecurityGroup:
			err = r.appendSecurityGroupRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSecurityGroupRule:
			err = r.appendSecurityGroupRuleRecoveryResult(task)

		case constant.MigrationTaskTypeCreateNetwork:
			err = r.appendNetworkRecoveryResult(task)

		case constant.MigrationTaskTypeCreateRouter:
			err = r.appendRouterRecoveryResult(ctx, task)

		case constant.MigrationTaskTypeCreateSubnet:
			err = r.appendSubnetRecoveryResult(task)

		case constant.MigrationTaskTypeImportVolume:
			err = r.appendVolumeRecoveryResult(task)

		case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
			err = r.appendInstanceRecoveryResult(task)
		}

	} else {
		switch task.TypeCode {
		case constant.MigrationTaskTypeCreateTenant:
			err = r.appendFailedTenantRecoveryResult(task)

		case constant.MigrationTaskTypeCreateFloatingIP:
			err = r.appendFailedFloatingIPRecoveryResult(task)

		case constant.MigrationTaskTypeCreateKeypair:
			err = r.appendFailedKeypairRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSpec:
			err = r.appendFailedInstanceSpecRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSecurityGroup:
			err = r.appendFailedSecurityGroupRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSecurityGroupRule:
			err = r.appendFailedSecurityGroupRuleRecoveryResult(task)

		case constant.MigrationTaskTypeCreateNetwork:
			err = r.appendFailedNetworkRecoveryResult(task)

		case constant.MigrationTaskTypeCreateRouter:
			err = r.appendFailedRouterRecoveryResult(task)

		case constant.MigrationTaskTypeCreateSubnet:
			err = r.appendFailedSubnetRecoveryResult(task)

		case constant.MigrationTaskTypeImportVolume:
			err = r.appendFailedVolumeRecoveryResult(task)

		case constant.MigrationTaskTypeCreateAndDiagnosisInstance:
			err = r.appendFailedInstanceRecoveryResult(task)
		}
	}

	if err == nil {
		logger.Infof("[appendTaskRecoveryResult] Success appendRecoveryResult - %s: job(%d) result(%s) task(%s)",
			task.TypeCode, task.RecoveryJobID, result.ResultCode, task.RecoveryJobTaskID)
	}

	return err
}

func (r *recoveryResult) appendRecoveryResult(ctx context.Context) error {
	for _, i := range r.JobDetail.Plan.Detail.Instances {
		pi := i.ProtectionClusterInstance
		for _, rt := range pi.Routers {
			r.RouterMap[rt.Id] = rt
			r.RouterMap[rt.Id].Tenant = pi.Tenant
		}

		for _, sg := range pi.SecurityGroups {
			r.SecurityGroupMap[sg.Id] = sg
			r.SecurityGroupMap[sg.Id].Tenant = pi.Tenant
		}

		for _, n := range pi.Networks {
			for _, s := range n.Network.Subnets {
				r.NetworkMap[s.Id] = n.Network
			}
		}
	}

	var taskList jobTaskList
	taskList, err := r.Job.GetTaskList()
	if err != nil {
		logger.Errorf("[appendRecoveryResult] Could not get task list: dr.recovery.job/%d/task. Cause: %+v",
			r.Job.RecoveryJobID, err)
		return err
	}
	sort.Sort(&taskList)

	for _, task := range taskList {
		if err = r.appendTaskRecoveryResult(ctx, task); err != nil {
			continue
		}
	}

	return nil
}

func (r *recoveryResult) appendTaskLogRecoveryResult() error {
	logs, err := r.Job.GetLogs()
	if err != nil {
		logger.Errorf("[appendTaskLogRecoveryResult] Could not get the logs: dr.recovery.job/%d/log.(result:%d) Cause: %v", r.Job.RecoveryJobID, r.ID, err)
		return err
	}

	if len(logs) == 0 {
		return nil
	}

	logger.Infof("[appendTaskLogRecoveryResult] Start : job(%d) result(%d)", r.Job.RecoveryJobID, r.ID)

	validateTaskLogRecoveryResult(logs)

	if err = database.GormTransaction(func(db *gorm.DB) error {
		for _, l := range logs {
			if err = db.Save(&model.RecoveryResultTaskLog{
				RecoveryResultID: r.ID,
				LogSeq:           l.LogSeq,
				Code:             l.Code,
				Contents:         &l.Contents,
				LogDt:            l.LogDt,
			}).Error; err != nil {
				logger.Errorf("[appendTaskLogRecoveryResult] Could not save result(%d) log(%d:%s). Cause: %v", r.ID, l.LogSeq, l.Code, err)
				return errors.UnusableDatabase(err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[appendTaskLogRecoveryResult] Success : job(%d) result(%d)", r.Job.RecoveryJobID, r.ID)
	return nil
}

func (r *recoveryResult) appendFailedReasonRecoveryResult() error {
	if len(r.JobResult.FailedReasons) == 0 {
		return nil
	}

	logger.Infof("[appendFailedReasonRecoveryResult] Start : job(%d) result(%d)", r.Job.RecoveryJobID, r.ID)

	r.validateFailedReasonRecoveryResult()

	if err := database.GormTransaction(func(db *gorm.DB) error {
		for _, f := range r.JobResult.FailedReasons {
			var seq uint64

			seq = seq + 1

			if err := db.Save(&model.RecoveryResultFailedReason{
				RecoveryResultID: r.ID,
				ReasonSeq:        seq,
				Code:             f.Code,
				Contents:         &f.Contents,
			}).Error; err != nil {
				logger.Errorf("[appendFailedReasonRecoveryResult] Could not save result(%d) failed reason(%d:%s). Cause: %v", r.ID, seq, f.Code, err)
				return errors.UnusableDatabase(err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[appendFailedReasonRecoveryResult] Success : job(%d) result(%d)", r.Job.RecoveryJobID, r.ID)
	return nil
}

func (r *recoveryResult) appendWarningReasonRecoveryResult() error {
	if len(r.JobResult.WarningReasons) == 0 {
		return nil
	}

	logger.Infof("[appendWarningReasonRecoveryResult] Start : job(%d) result(%d)", r.Job.RecoveryJobID, r.ID)

	r.validateWarningReasonRecoveryResult()

	if err := database.Execute(func(db *gorm.DB) error {
		for _, w := range r.JobResult.WarningReasons {
			var seq uint64

			seq = seq + 1

			if err := db.Save(&model.RecoveryResultWarningReason{
				RecoveryResultID: r.ID,
				ReasonSeq:        seq,
				Code:             w.Code,
				Contents:         &w.Contents,
			}).Error; err != nil {
				logger.Errorf("[appendWarningReasonRecoveryResult] Could not save result(%d) warning reason(%d:%s). Cause: %v", r.ID, seq, w.Code, err)
				return errors.UnusableDatabase(err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	logger.Infof("[appendWarningReasonRecoveryResult] Success : job(%d) result(%d)", r.Job.RecoveryJobID, r.ID)
	return nil
}

// Create 재해복구결과 보고서 생성
func Create(ctx context.Context, job *migrator.RecoveryJob) (*drms.RecoveryResult, error) {
	logger.Infof("[RecoveryReport-Create] Start: job(%d)", job.RecoveryJobID)
	var err error
	var r = recoveryResult{
		Job:                 job,
		JobDetail:           new(drms.RecoveryJob),
		TenantIDMap:         make(map[string]uint64),
		NetworkIDMap:        make(map[string]uint64),
		SubnetIDMap:         make(map[string]uint64),
		SecurityGroupIDMap:  make(map[string]uint64),
		SecurityGroupMap:    make(map[uint64]*cms.ClusterSecurityGroup),
		RouterMap:           make(map[uint64]*cms.ClusterRouter),
		NetworkMap:          make(map[uint64]*cms.ClusterNetwork),
		IsAlreadyAppendTask: make(map[string]bool),
	}
	// bool 에 따라 함수 재시도를 위한 map
	retryMap := map[string]bool{
		"createRecoveryResult":              false,
		"appendRecoveryResult":              false,
		"appendTaskLogRecoveryResult":       false,
		"appendFailedReasonRecoveryResult":  false,
		"appendWarningReasonRecoveryResult": false,
	}

	if err = job.GetDetail(r.JobDetail); err != nil {
		logger.Errorf("[RecoveryReport-Create] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	if r.JobStatus, err = job.GetStatus(); err != nil {
		logger.Errorf("[RecoveryReport-Create] Could not get job status: dr.recovery.job/%d/status. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	if r.JobResult, err = job.GetResult(); err != nil {
		logger.Errorf("[RecoveryReport-Create] Could not get job result: dr.recovery.job/%d/result. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	if r.JobOperation, err = job.GetOperation(); err != nil {
		logger.Errorf("[RecoveryReport-Create] Could not get job operation: dr.recovery.job/%d/operation. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	r.validate()

	// true: 작업이 수행 됨.
	// false: 작업이 수행되지 않았으며, 작업 시도가 필요.
	for isRetryable := 0; isRetryable < 3; {
		if !retryMap["createRecoveryResult"] {
			if err = r.createRecoveryResult(); err != nil {
				logger.Warnf("[RecoveryReport-Create] Could not create recovery result: job(%d). Retry count(%d). Cause: %+v", job.RecoveryJobID, isRetryable, err)
				isRetryable++
				continue
			}

			retryMap["createRecoveryResult"] = true
		}

		if !retryMap["appendRecoveryResult"] {
			if err = r.appendRecoveryResult(ctx); err != nil {
				logger.Warnf("[RecoveryReport-Create] Could not append recovery result: job(%d). Retry count(%d). Cause: %+v", job.RecoveryJobID, isRetryable, err)
				isRetryable++
				continue
			}
			retryMap["appendRecoveryResult"] = true
		}

		if !retryMap["appendTaskLogRecoveryResult"] {
			if err = r.appendTaskLogRecoveryResult(); err != nil {
				logger.Warnf("[RecoveryReport-Create] Could not append task log recovery result: job(%d). Retry count(%d). Cause: %+v", job.RecoveryJobID, isRetryable, err)
				isRetryable++
				continue
			}
			retryMap["appendTaskLogRecoveryResult"] = true
		}

		if !retryMap["appendFailedReasonRecoveryResult"] {
			if err = r.appendFailedReasonRecoveryResult(); err != nil {
				logger.Warnf("[RecoveryReport-Create] Could not append failed reason recovery result: job(%d). Retry count(%d). Cause: %+v", job.RecoveryJobID, isRetryable, err)
				isRetryable++
				continue
			}
			retryMap["appendFailedReasonRecoveryResult"] = true
		}

		if !retryMap["appendWarningReasonRecoveryResult"] {
			if err = r.appendWarningReasonRecoveryResult(); err != nil {
				logger.Warnf("[RecoveryReport-Create] Could not append warning reason recovery result: job(%d). Retry count(%d). Cause: %+v", job.RecoveryJobID, isRetryable, err)
				isRetryable++
				continue
			}
			retryMap["appendWarningReasonRecoveryResult"] = true
		}

		// err 가 발생하지 않으면 retry 를 하지 않고 for 문 빠져나가기
		if err == nil {
			break
		}
	}

	var result *drms.RecoveryResult
	if result, err = getNoErrorRecoveryResult(ctx, &drms.RecoveryReportRequest{
		GroupId:  job.ProtectionGroupID,
		ResultId: r.RecoveryResult.ID,
	}); err != nil {
		logger.Errorf("[RecoveryReport-Create] Could not get recovery result: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	logger.Infof("[RecoveryReport-Create] Success: job(%d) result(%d)", job.RecoveryJobID, r.RecoveryResult.ID)
	return result, nil
}

// `Get`함수를 쓰면 `Create`에서 데이터 저장을 성공한 부분(1954줄)만 출력이 안됨.
func getNoErrorRecoveryResult(ctx context.Context, req *drms.RecoveryReportRequest) (*drms.RecoveryResult, error) {
	var result *model.RecoveryResult
	var recoveryResult drms.RecoveryResult
	var err error

	if req.GetGroupId() == 0 {
		return nil, errors.RequiredParameter("group_id")
	}

	if req.GetResultId() == 0 {
		return nil, errors.RequiredParameter("result_id")
	}

	if result, err = getRecoveryResult(req.GroupId, req.ResultId); err != nil {
		logger.Errorf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result: group(%d) result(%d). Cause: %+v", req.GroupId, req.ResultId, err)
		return nil, err
	}

	if err = internal.IsAccessibleRecoveryReport(ctx, result.OwnerGroupID); err != nil {
		return nil, err
	}

	if err = recoveryResult.SetFromModel(result); err != nil {
		return nil, err
	}

	if recoveryResult.WarningReasons, err = getWarningReasons(result.ID); err != nil {
		logger.Warnf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result(%d) warning reasons. Cause: %+v", result.ID, err)
	}

	if recoveryResult.FailedReasons, err = getFailedReasons(result.ID); err != nil {
		logger.Warnf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result(%d) failed reasons. Cause: %+v", result.ID, err)
	}

	if recoveryResult.TaskLogs, err = getTaskLogs(result.ID); err != nil {
		logger.Warnf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result(%d) task logs. Cause: %+v", result.ID, err)
	}

	if recoveryResult.Instances, err = getResultEssentialInstances(result); err != nil {
		logger.Warnf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result(%d) instances. Cause: %+v", result.ID, err)
	}

	if recoveryResult.Volumes, err = getResultEssentialVolumes(result); err != nil {
		logger.Warnf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result(%d) volumes. Cause: %+v", result.ID, err)
	}

	if recoveryResult.Routers, err = getResultEssentialRouters(result); err != nil {
		logger.Warnf("[RecoveryReport-getNoErrorRecoveryResult] Could not get recovery result(%d) routers. Cause: %+v", result.ID, err)
	}

	return &recoveryResult, nil
}

func getResultEssentialInstances(result *model.RecoveryResult) ([]*drms.RecoveryResultInstance, error) {
	var instanceResults []model.RecoveryResultInstance
	var instances []*drms.RecoveryResultInstance

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInstance{
			RecoveryResultID: result.ID,
		}).Find(&instanceResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, i := range instanceResults {
		var pKeypair, rKeypair *cms.ClusterKeypair

		pTenant, rTenant, err := getTenant(result.ID, i.ProtectionClusterTenantID)
		if err != nil {
			return instances, err
		}

		pSpec, rSpec, err := getInstanceSpec(result.ID, i.ProtectionClusterInstanceSpecID)
		if err != nil {
			return instances, err
		}

		pNetworks, rNetworks, err := getInstanceNetworks(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return instances, err
		}

		pRouters, rRouters, err := getInstanceRouters(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return instances, err
		}

		pSecurityGroups, rSecurityGroups, err := getInstanceSecurityGroups(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return instances, err
		}

		pVolumes, rVolumes, err := getInstanceVolumes(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return instances, err
		}

		// protection cluster instance
		var pi = &cms.ClusterInstance{
			Id:          i.ProtectionClusterInstanceID,
			Uuid:        i.ProtectionClusterInstanceUUID,
			Name:        i.ProtectionClusterInstanceName,
			Description: getString(i.ProtectionClusterInstanceDescription),
			Cluster: &cms.Cluster{
				Id:       result.ProtectionClusterID,
				Name:     result.ProtectionClusterName,
				TypeCode: result.ProtectionClusterTypeCode,
				Remarks:  getString(result.ProtectionClusterRemarks),
			},
			AvailabilityZone: &cms.ClusterAvailabilityZone{
				Id:   i.ProtectionClusterAvailabilityZoneID,
				Name: i.ProtectionClusterAvailabilityZoneName,
			},
			Hypervisor: &cms.ClusterHypervisor{
				Id:        i.ProtectionClusterHypervisorID,
				TypeCode:  i.ProtectionClusterHypervisorTypeCode,
				Hostname:  i.ProtectionClusterHypervisorHostname,
				IpAddress: i.ProtectionClusterHypervisorIPAddress,
			},
			Tenant:         pTenant,
			Spec:           pSpec,
			Networks:       pNetworks,
			Routers:        pRouters,
			SecurityGroups: pSecurityGroups,
			Volumes:        pVolumes,
		}

		// recovery cluster instance
		var ri = &cms.ClusterInstance{
			Id:          getUint64(i.RecoveryClusterInstanceID),
			Uuid:        getString(i.RecoveryClusterInstanceUUID),
			Name:        getString(i.RecoveryClusterInstanceName),
			Description: getString(i.RecoveryClusterInstanceDescription),
			Cluster: &cms.Cluster{
				Id:       result.RecoveryClusterID,
				Name:     result.RecoveryClusterName,
				TypeCode: result.RecoveryClusterTypeCode,
				Remarks:  getString(result.RecoveryClusterRemarks),
			},
			AvailabilityZone: &cms.ClusterAvailabilityZone{
				Id:   getUint64(i.RecoveryClusterAvailabilityZoneID),
				Name: getString(i.RecoveryClusterAvailabilityZoneName),
			},
			Hypervisor: &cms.ClusterHypervisor{
				Id:        getUint64(i.RecoveryClusterHypervisorID),
				TypeCode:  getString(i.RecoveryClusterHypervisorTypeCode),
				Hostname:  getString(i.RecoveryClusterHypervisorHostname),
				IpAddress: getString(i.RecoveryClusterHypervisorIPAddress),
			},
			Tenant:         rTenant,
			Spec:           rSpec,
			Networks:       rNetworks,
			Routers:        rRouters,
			SecurityGroups: rSecurityGroups,
			Volumes:        rVolumes,
		}

		// instance 에 keypair 가 할당된 경우에만 keypair 를 넣어준다.
		if i.ProtectionClusterKeypairID != nil {
			pKeypair, rKeypair, err = getKeypair(result.ID, *i.ProtectionClusterKeypairID)
			if err != nil {
				return instances, err
			}

			pi.Keypair = pKeypair
			ri.Keypair = rKeypair
		}

		deps, err := getDependencies(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return instances, err
		}

		instance := &drms.RecoveryResultInstance{
			ProtectionClusterInstance: pi,
			RecoveryClusterInstance:   ri,
			RecoveryPointTypeCode:     i.RecoveryPointTypeCode,
			RecoveryPoint:             i.RecoveryPoint,
			AutoStartFlag:             i.AutoStartFlag,
			DiagnosisFlag:             i.DiagnosisFlag,
			DiagnosisMethodCode:       getString(i.DiagnosisMethodCode),
			DiagnosisMethodData:       getString(i.DiagnosisMethodData),
			DiagnosisTimeout:          getUint32(i.DiagnosisTimeout),
			Dependencies:              deps,
			StartedAt:                 getInt64(i.StartedAt),
			FinishedAt:                getInt64(i.FinishedAt),
			ResultCode:                i.ResultCode,
		}

		if getString(i.FailedReasonCode) != "" {
			instance.FailedReason = &drms.Message{
				Code:     getString(i.FailedReasonCode),
				Contents: getString(i.FailedReasonContents),
			}
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

func getResultEssentialVolumes(result *model.RecoveryResult) ([]*drms.RecoveryResultVolume, error) {
	var volumeResults []model.RecoveryResultVolume
	var volumes []*drms.RecoveryResultVolume

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultVolume{
			RecoveryResultID: result.ID,
		}).Find(&volumeResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, v := range volumeResults {
		pTenant, rTenant, err := getTenant(result.ID, v.ProtectionClusterTenantID)
		if err != nil {
			return volumes, err
		}

		// protection cluster volume
		var pv = cms.ClusterVolume{
			Id: v.ProtectionClusterVolumeID,
			Cluster: &cms.Cluster{
				Id:       result.ProtectionClusterID,
				Name:     result.ProtectionClusterName,
				TypeCode: result.ProtectionClusterTypeCode,
				Remarks:  getString(result.ProtectionClusterRemarks),
			},
			Tenant: pTenant,
			Storage: &cms.ClusterStorage{
				Id:          v.ProtectionClusterStorageID,
				Uuid:        v.ProtectionClusterStorageUUID,
				Name:        v.ProtectionClusterStorageName,
				TypeCode:    v.ProtectionClusterStorageTypeCode,
				Description: getString(v.ProtectionClusterStorageDescription),
			},
			Uuid:        v.ProtectionClusterVolumeUUID,
			Name:        getString(v.ProtectionClusterVolumeName),
			Description: getString(v.ProtectionClusterVolumeDescription),
			SizeBytes:   v.ProtectionClusterVolumeSizeBytes,
			Multiattach: v.ProtectionClusterVolumeMultiattach,
			Bootable:    v.ProtectionClusterVolumeBootable,
			Readonly:    v.ProtectionClusterVolumeReadonly,
		}

		// recovery cluster volume
		var rv = cms.ClusterVolume{
			Id: getUint64(v.RecoveryClusterVolumeID),
			Cluster: &cms.Cluster{
				Id:       result.RecoveryClusterID,
				Name:     result.RecoveryClusterName,
				TypeCode: result.RecoveryClusterTypeCode,
				Remarks:  getString(result.RecoveryClusterRemarks),
			},
			Tenant: rTenant,
			Storage: &cms.ClusterStorage{
				Id:          getUint64(v.RecoveryClusterStorageID),
				Uuid:        getString(v.RecoveryClusterStorageUUID),
				Name:        getString(v.RecoveryClusterStorageName),
				TypeCode:    getString(v.RecoveryClusterStorageTypeCode),
				Description: getString(v.RecoveryClusterStorageDescription),
			},
			Uuid:        getString(v.RecoveryClusterVolumeUUID),
			Name:        getString(v.RecoveryClusterVolumeName),
			Description: getString(v.RecoveryClusterVolumeDescription),
			SizeBytes:   getUint64(v.RecoveryClusterVolumeSizeBytes),
			Multiattach: getBool(v.RecoveryClusterVolumeMultiattach),
			Bootable:    getBool(v.RecoveryClusterVolumeBootable),
			Readonly:    getBool(v.RecoveryClusterVolumeReadonly),
		}

		volume := drms.RecoveryResultVolume{
			ProtectionClusterVolume: &pv,
			RecoveryClusterVolume:   &rv,
			RecoveryPointTypeCode:   v.RecoveryPointTypeCode,
			RecoveryPoint:           v.RecoveryPoint,
			StartedAt:               getInt64(v.StartedAt),
			FinishedAt:              getInt64(v.FinishedAt),
			ResultCode:              v.ResultCode,
			RollbackFlag:            v.RollbackFlag,
		}

		if getString(v.FailedReasonCode) != "" {
			volume.FailedReason = &drms.Message{
				Code:     getString(v.FailedReasonCode),
				Contents: getString(v.FailedReasonContents),
			}
		}

		volumes = append(volumes, &volume)
	}
	return volumes, nil
}

func getResultEssentialRouters(result *model.RecoveryResult) ([]*drms.RecoveryResultRouter, error) {
	var routerResults []model.RecoveryResultRouter
	var routers []*drms.RecoveryResultRouter

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultRouter{
			RecoveryResultID: result.ID,
		}).Find(&routerResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, r := range routerResults {
		pTenant, rTenant, err := getTenant(result.ID, r.ProtectionClusterTenantID)
		if err != nil {
			return routers, err
		}

		pInternals, rInternals, err := getInternalRoutingInterface(result.ID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return routers, err
		}

		pExternals, rExternals, err := getExternalRoutingInterface(result.ID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return routers, err
		}

		pExtras, rExtras, err := getExtraRoute(result.ID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return routers, err
		}

		// protection cluster router
		var pr = &cms.ClusterRouter{
			Id:                        r.ProtectionClusterRouterID,
			Tenant:                    pTenant,
			Uuid:                      r.ProtectionClusterRouterUUID,
			Name:                      r.ProtectionClusterRouterName,
			Description:               getString(r.ProtectionClusterRouterDescription),
			InternalRoutingInterfaces: pInternals,
			ExternalRoutingInterfaces: pExternals,
			ExtraRoutes:               pExtras,
		}

		// recovery cluster router
		var rr = &cms.ClusterRouter{
			Id:                        getUint64(r.RecoveryClusterRouterID),
			Tenant:                    rTenant,
			Uuid:                      getString(r.RecoveryClusterRouterUUID),
			Name:                      getString(r.RecoveryClusterRouterName),
			Description:               getString(r.RecoveryClusterRouterDescription),
			InternalRoutingInterfaces: rInternals,
			ExternalRoutingInterfaces: rExternals,
			ExtraRoutes:               rExtras,
		}

		routers = append(routers, &drms.RecoveryResultRouter{
			ProtectionClusterRouter: pr,
			RecoveryClusterRouter:   rr,
		})
	}

	return routers, nil
}

// CreateCanceledJob 대기시간 초과 또는 시작시간 중복으로 취소된 재해복구 작업의 결과 보고서 생성
func CreateCanceledJob(ctx context.Context, job *migrator.RecoveryJob, resultCode string) (*drms.RecoveryResult, error) {
	logger.Infof("[RecoveryReport-CreateCanceledJob] Start: job(%d)", job.RecoveryJobID)
	var err error
	var r = recoveryResult{
		Job:       job,
		JobDetail: new(drms.RecoveryJob),
		JobStatus: new(migrator.RecoveryJobStatus),
		JobResult: new(migrator.RecoveryJobResult),
	}

	if err = job.GetDetail(r.JobDetail); err != nil {
		logger.Errorf("[RecoveryReport-CreateCanceledJob] Could not get job detail: dr.recovery.job/%d/detail. Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	finished := time.Now().Unix()
	if job.TriggeredAt > finished {
		finished = job.TriggeredAt
	}
	r.JobStatus.StartedAt = job.TriggeredAt
	r.JobStatus.FinishedAt = finished
	r.JobStatus.ElapsedTime = r.JobStatus.FinishedAt - r.JobStatus.StartedAt
	r.JobResult.ResultCode = resultCode

	r.validate()

	var result *drms.RecoveryResult
	if err = r.createRecoveryResult(); err != nil {
		logger.Errorf("[RecoveryReport-CreateCanceledJob] Could not create recovery result: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	if result, err = Get(ctx, &drms.RecoveryReportRequest{
		GroupId:  job.ProtectionGroupID,
		ResultId: r.RecoveryResult.ID,
	}); err != nil {
		logger.Errorf("[RecoveryReport-CreateCanceledJob] Could not get recovery result: job(%d). Cause: %+v", job.RecoveryJobID, err)
		return nil, err
	}

	logger.Infof("[RecoveryReport-CreateCanceledJob] Success: job(%d) result(%d)", job.RecoveryJobID, result.Id)
	return result, nil
}

func getRecoveryResult(groupID, resultID uint64) (*model.RecoveryResult, error) {
	var result model.RecoveryResult
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResult{
			ID:                resultID,
			ProtectionGroupID: groupID,
		}).First(&result).Error
	}); err != nil {
		switch {
		case err == gorm.ErrRecordNotFound:
			return nil, internal.NotFoundRecoveryResult(resultID)

		case err != nil:
			return nil, errors.UnusableDatabase(err)
		}
	}

	return &result, nil
}

func getWarningReasons(resultID uint64) ([]*drms.Message, error) {
	var warnings []model.RecoveryResultWarningReason
	var warningReasons []*drms.Message
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultWarningReason{
			RecoveryResultID: resultID,
		}).Order("reason_seq").Find(&warnings).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, w := range warnings {
		warningReasons = append(warningReasons, &drms.Message{
			Code:     w.Code,
			Contents: getString(w.Contents),
		})
	}

	return warningReasons, nil
}

func getFailedReasons(resultID uint64) ([]*drms.Message, error) {
	var failed []model.RecoveryResultFailedReason
	var failedReasons []*drms.Message

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultFailedReason{
			RecoveryResultID: resultID,
		}).Order("reason_seq").Find(&failed).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, f := range failed {
		failedReasons = append(failedReasons, &drms.Message{
			Code:     f.Code,
			Contents: getString(f.Contents),
		})
	}

	return failedReasons, nil
}

func getTaskLogs(resultID uint64) ([]*drms.RecoveryTaskLog, error) {
	var logs []model.RecoveryResultTaskLog
	var taskLogs []*drms.RecoveryTaskLog

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultTaskLog{
			RecoveryResultID: resultID,
		}).Order("log_seq").Find(&logs).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, t := range logs {
		taskLogs = append(taskLogs, &drms.RecoveryTaskLog{
			Code:     t.Code,
			Contents: getString(t.Contents),
			LogSeq:   t.LogSeq,
			LogDt:    t.LogDt,
		})
	}

	return taskLogs, nil
}

func getDependencies(resultID, tenantID, instanceID uint64) ([]*cms.ClusterInstance, error) {
	var depResults []model.RecoveryResultInstanceDependency
	var instances []*cms.ClusterInstance

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInstanceDependency{
			RecoveryResultID:            resultID,
			ProtectionClusterTenantID:   tenantID,
			ProtectionClusterInstanceID: instanceID,
		}).Find(&depResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, d := range depResults {
		instances = append(instances, &cms.ClusterInstance{
			Id: d.DependencyProtectionClusterInstanceID,
		})
	}

	return instances, nil
}

func getTenant(resultID, tenantID uint64) (*cms.ClusterTenant, *cms.ClusterTenant, error) {
	var tenantResult model.RecoveryResultTenant

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultTenant{
			RecoveryResultID:          resultID,
			ProtectionClusterTenantID: tenantID,
		}).First(&tenantResult).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	pTenant := cms.ClusterTenant{
		Id:          tenantResult.ProtectionClusterTenantID,
		Uuid:        tenantResult.ProtectionClusterTenantUUID,
		Name:        tenantResult.ProtectionClusterTenantName,
		Enabled:     tenantResult.ProtectionClusterTenantEnabled,
		Description: getString(tenantResult.ProtectionClusterTenantDescription),
	}

	rTenant := cms.ClusterTenant{
		Id:          getUint64(tenantResult.RecoveryClusterTenantID),
		Uuid:        getString(tenantResult.RecoveryClusterTenantUUID),
		Name:        getString(tenantResult.RecoveryClusterTenantName),
		Enabled:     getBool(tenantResult.RecoveryClusterTenantEnabled),
		Description: getString(tenantResult.RecoveryClusterTenantDescription),
	}

	return &pTenant, &rTenant, nil
}

func getInstanceSpec(resultID, instanceSpecID uint64) (*cms.ClusterInstanceSpec, *cms.ClusterInstanceSpec, error) {
	var specResult model.RecoveryResultInstanceSpec
	var extraSpecResults []model.RecoveryResultInstanceExtraSpec
	var protectionExtraSpecs []*cms.ClusterInstanceExtraSpec
	var recoveryExtraSpecs []*cms.ClusterInstanceExtraSpec

	if err := database.Execute(func(db *gorm.DB) error {
		if err := db.Where(&model.RecoveryResultInstanceSpec{
			RecoveryResultID:                resultID,
			ProtectionClusterInstanceSpecID: instanceSpecID,
		}).First(&specResult).Error; err != nil {
			return err
		}

		return db.Where(&model.RecoveryResultInstanceExtraSpec{
			RecoveryResultID:                resultID,
			ProtectionClusterInstanceSpecID: instanceSpecID,
		}).Find(&extraSpecResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, e := range extraSpecResults {
		protectionExtraSpecs = append(protectionExtraSpecs, &cms.ClusterInstanceExtraSpec{
			Key:   e.ProtectionClusterInstanceExtraSpecKey,
			Value: e.ProtectionClusterInstanceExtraSpecValue,
		})

		recoveryExtraSpecs = append(recoveryExtraSpecs, &cms.ClusterInstanceExtraSpec{
			Key:   getString(e.RecoveryClusterInstanceExtraSpecKey),
			Value: getString(e.RecoveryClusterInstanceExtraSpecValue),
		})
	}

	pSpec := cms.ClusterInstanceSpec{
		Id:                  specResult.ProtectionClusterInstanceSpecID,
		Uuid:                specResult.ProtectionClusterInstanceSpecUUID,
		Name:                specResult.ProtectionClusterInstanceSpecName,
		Description:         getString(specResult.ProtectionClusterInstanceSpecDescription),
		VcpuTotalCnt:        uint32(specResult.ProtectionClusterInstanceSpecVcpuTotalCnt),
		MemTotalBytes:       specResult.ProtectionClusterInstanceSpecMemTotalBytes,
		DiskTotalBytes:      specResult.ProtectionClusterInstanceSpecDiskTotalBytes,
		SwapTotalBytes:      specResult.ProtectionClusterInstanceSpecSwapTotalBytes,
		EphemeralTotalBytes: specResult.ProtectionClusterInstanceSpecEphemeralTotalBytes,
		ExtraSpecs:          protectionExtraSpecs,
	}

	vCPUTotalCnt := getUint64(specResult.RecoveryClusterInstanceSpecVcpuTotalCnt)

	rSpec := cms.ClusterInstanceSpec{
		Id:                  getUint64(specResult.RecoveryClusterInstanceSpecID),
		Uuid:                getString(specResult.RecoveryClusterInstanceSpecUUID),
		Name:                getString(specResult.RecoveryClusterInstanceSpecName),
		Description:         getString(specResult.RecoveryClusterInstanceSpecDescription),
		VcpuTotalCnt:        uint32(vCPUTotalCnt),
		MemTotalBytes:       getUint64(specResult.RecoveryClusterInstanceSpecMemTotalBytes),
		DiskTotalBytes:      getUint64(specResult.RecoveryClusterInstanceSpecDiskTotalBytes),
		SwapTotalBytes:      getUint64(specResult.RecoveryClusterInstanceSpecSwapTotalBytes),
		EphemeralTotalBytes: getUint64(specResult.RecoveryClusterInstanceSpecEphemeralTotalBytes),
		ExtraSpecs:          recoveryExtraSpecs,
	}

	return &pSpec, &rSpec, nil
}

func getKeypair(resultID, keypairID uint64) (*cms.ClusterKeypair, *cms.ClusterKeypair, error) {
	var keypairResult model.RecoveryResultKeypair

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultKeypair{
			RecoveryResultID:           resultID,
			ProtectionClusterKeypairID: keypairID,
		}).First(&keypairResult).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	pKeypair := cms.ClusterKeypair{
		Id:          keypairResult.ProtectionClusterKeypairID,
		Name:        keypairResult.ProtectionClusterKeypairName,
		Fingerprint: keypairResult.ProtectionClusterKeypairFingerprint,
		PublicKey:   keypairResult.ProtectionClusterKeypairPublicKey,
		TypeCode:    keypairResult.ProtectionClusterKeypairTypeCode,
	}

	rKeypair := cms.ClusterKeypair{
		Id:          getUint64(keypairResult.RecoveryClusterKeypairID),
		Name:        getString(keypairResult.RecoveryClusterKeypairName),
		Fingerprint: getString(keypairResult.RecoveryClusterKeypairFingerprint),
		PublicKey:   getString(keypairResult.RecoveryClusterKeypairPublicKey),
		TypeCode:    getString(keypairResult.RecoveryClusterKeypairTypeCode),
	}

	return &pKeypair, &rKeypair, nil
}

func getInstanceNetworks(resultID, tenantID, instanceID uint64) ([]*cms.ClusterInstanceNetwork, []*cms.ClusterInstanceNetwork, error) {
	var instanceNetworkResults []model.RecoveryResultInstanceNetwork
	var pDhcpPools []*cms.ClusterSubnetDHCPPool
	var pNameservers []*cms.ClusterSubnetNameserver
	var rDhcpPools []*cms.ClusterSubnetDHCPPool
	var rNameservers []*cms.ClusterSubnetNameserver
	var pNetworks []*cms.ClusterInstanceNetwork
	var rNetworks []*cms.ClusterInstanceNetwork

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInstanceNetwork{
			RecoveryResultID:            resultID,
			ProtectionClusterTenantID:   tenantID,
			ProtectionClusterInstanceID: instanceID,
		}).Order("instance_network_seq").Find(&instanceNetworkResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, n := range instanceNetworkResults {
		var networkResult model.RecoveryResultNetwork
		var subnetResult model.RecoveryResultSubnet
		var floatingIPResult model.RecoveryResultFloatingIP
		var subnetDhcpPoolResults []model.RecoveryResultSubnetDHCPPool
		var subnetNameserverResults []model.RecoveryResultSubnetNameserver

		if err := database.Execute(func(db *gorm.DB) error {
			if err := db.Where(&model.RecoveryResultNetwork{
				RecoveryResultID:           resultID,
				ProtectionClusterTenantID:  n.ProtectionClusterTenantID,
				ProtectionClusterNetworkID: n.ProtectionClusterNetworkID,
			}).First(&networkResult).Error; err != nil {
				return err
			}

			if err := db.Where(&model.RecoveryResultSubnet{
				RecoveryResultID:           resultID,
				ProtectionClusterTenantID:  n.ProtectionClusterTenantID,
				ProtectionClusterNetworkID: n.ProtectionClusterNetworkID,
				ProtectionClusterSubnetID:  n.ProtectionClusterSubnetID,
			}).First(&subnetResult).Error; err != nil {
				return err
			}

			if err := db.Where(&model.RecoveryResultSubnetDHCPPool{
				RecoveryResultID:           resultID,
				ProtectionClusterTenantID:  n.ProtectionClusterTenantID,
				ProtectionClusterNetworkID: n.ProtectionClusterNetworkID,
				ProtectionClusterSubnetID:  subnetResult.ProtectionClusterSubnetID,
			}).Order("dhcp_pool_seq").Find(&subnetDhcpPoolResults).Error; err != nil {
				return err
			}

			if err := db.Where(&model.RecoveryResultSubnetNameserver{
				RecoveryResultID:           resultID,
				ProtectionClusterTenantID:  n.ProtectionClusterTenantID,
				ProtectionClusterNetworkID: n.ProtectionClusterNetworkID,
				ProtectionClusterSubnetID:  subnetResult.ProtectionClusterSubnetID,
			}).Order("nameserver_seq").Find(&subnetNameserverResults).Error; err != nil {
				return err
			}

			if getUint64(n.ProtectionClusterFloatingIPID) != 0 {
				if err := db.Where(&model.RecoveryResultFloatingIP{
					RecoveryResultID:              resultID,
					ProtectionClusterFloatingIPID: getUint64(n.ProtectionClusterFloatingIPID),
				}).First(&floatingIPResult).Error; err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return nil, nil, errors.UnusableDatabase(err)
		}

		for _, p := range subnetDhcpPoolResults {
			pDhcpPools = append(pDhcpPools, &cms.ClusterSubnetDHCPPool{
				StartIpAddress: p.ProtectionClusterStartIPAddress,
				EndIpAddress:   p.ProtectionClusterEndIPAddress,
			})

			rDhcpPools = append(rDhcpPools, &cms.ClusterSubnetDHCPPool{
				StartIpAddress: getString(p.RecoveryClusterStartIPAddress),
				EndIpAddress:   getString(p.RecoveryClusterEndIPAddress),
			})
		}

		for _, ns := range subnetNameserverResults {
			pNameservers = append(pNameservers, &cms.ClusterSubnetNameserver{
				Nameserver: ns.ProtectionClusterNameserver,
			})

			rNameservers = append(rNameservers, &cms.ClusterSubnetNameserver{
				Nameserver: getString(ns.RecoveryClusterNameserver),
			})
		}

		var network = &cms.ClusterInstanceNetwork{
			Id: networkResult.ProtectionClusterNetworkID,
			Network: &cms.ClusterNetwork{
				Id:          networkResult.ProtectionClusterNetworkID,
				TypeCode:    networkResult.ProtectionClusterNetworkTypeCode,
				Uuid:        networkResult.ProtectionClusterNetworkUUID,
				Name:        networkResult.ProtectionClusterNetworkName,
				Description: getString(networkResult.ProtectionClusterNetworkDescription),
			},
			Subnet: &cms.ClusterSubnet{
				Id:                  subnetResult.ProtectionClusterSubnetID,
				Uuid:                subnetResult.ProtectionClusterSubnetUUID,
				Name:                subnetResult.ProtectionClusterSubnetName,
				Description:         getString(subnetResult.ProtectionClusterSubnetDescription),
				NetworkCidr:         subnetResult.ProtectionClusterSubnetNetworkCidr,
				DhcpEnabled:         subnetResult.ProtectionClusterSubnetDhcpEnabled,
				DhcpPools:           pDhcpPools,
				GatewayEnabled:      subnetResult.ProtectionClusterSubnetGatewayEnabled,
				GatewayIpAddress:    getString(subnetResult.ProtectionClusterSubnetGatewayIPAddress),
				Ipv6AddressModeCode: getString(subnetResult.ProtectionClusterSubnetIpv6AddressModeCode),
				Ipv6RaModeCode:      getString(subnetResult.ProtectionClusterSubnetIpv6RaModeCode),
				Nameservers:         pNameservers,
			},
			DhcpFlag:  n.ProtectionClusterDhcpFlag,
			IpAddress: n.ProtectionClusterIPAddress,
		}

		if getUint64(n.ProtectionClusterFloatingIPID) != 0 {
			network.FloatingIp = &cms.ClusterFloatingIP{
				Id:          floatingIPResult.ProtectionClusterFloatingIPID,
				Uuid:        floatingIPResult.ProtectionClusterFloatingIPUUID,
				Description: getString(floatingIPResult.ProtectionClusterFloatingIPDescription),
				IpAddress:   floatingIPResult.ProtectionClusterFloatingIPIPAddress,
			}
		}

		pNetworks = append(pNetworks, network)

		network = &cms.ClusterInstanceNetwork{
			Id: getUint64(networkResult.RecoveryClusterNetworkID),
			Network: &cms.ClusterNetwork{
				Id:          getUint64(networkResult.RecoveryClusterNetworkID),
				TypeCode:    getString(networkResult.RecoveryClusterNetworkTypeCode),
				Uuid:        getString(networkResult.RecoveryClusterNetworkUUID),
				Name:        getString(networkResult.RecoveryClusterNetworkName),
				Description: getString(networkResult.RecoveryClusterNetworkDescription),
			},
			Subnet: &cms.ClusterSubnet{
				Id:                  getUint64(subnetResult.RecoveryClusterSubnetID),
				Uuid:                getString(subnetResult.RecoveryClusterSubnetUUID),
				Name:                getString(subnetResult.RecoveryClusterSubnetName),
				Description:         getString(subnetResult.RecoveryClusterSubnetDescription),
				NetworkCidr:         getString(subnetResult.RecoveryClusterSubnetNetworkCidr),
				DhcpEnabled:         getBool(subnetResult.RecoveryClusterSubnetDhcpEnabled),
				DhcpPools:           rDhcpPools,
				GatewayEnabled:      getBool(subnetResult.RecoveryClusterSubnetGatewayEnabled),
				GatewayIpAddress:    getString(subnetResult.RecoveryClusterSubnetGatewayIPAddress),
				Ipv6AddressModeCode: getString(subnetResult.RecoveryClusterSubnetIpv6AddressModeCode),
				Ipv6RaModeCode:      getString(subnetResult.RecoveryClusterSubnetIpv6RaModeCode),
				Nameservers:         rNameservers,
			},
			DhcpFlag:  getBool(n.RecoveryClusterDhcpFlag),
			IpAddress: getString(n.RecoveryClusterIPAddress),
		}

		if getUint64(n.ProtectionClusterFloatingIPID) != 0 {
			network.FloatingIp = &cms.ClusterFloatingIP{
				Id:          getUint64(floatingIPResult.RecoveryClusterFloatingIPID),
				Uuid:        getString(floatingIPResult.RecoveryClusterFloatingIPUUID),
				Description: getString(floatingIPResult.RecoveryClusterFloatingIPDescription),
				IpAddress:   getString(floatingIPResult.RecoveryClusterFloatingIPIPAddress),
			}
		}

		rNetworks = append(rNetworks, network)
	}

	return pNetworks, rNetworks, nil
}

func getInternalRoutingInterface(resultID, tenantID, routerID uint64) ([]*cms.ClusterNetworkRoutingInterface, []*cms.ClusterNetworkRoutingInterface, error) {
	var internalResults []model.RecoveryResultInternalRoutingInterface
	var pInternals []*cms.ClusterNetworkRoutingInterface
	var rInternals []*cms.ClusterNetworkRoutingInterface

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInternalRoutingInterface{
			RecoveryResultID:          resultID,
			ProtectionClusterTenantID: tenantID,
			ProtectionClusterRouterID: routerID,
		}).Order("routing_interface_seq").Find(&internalResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, i := range internalResults {
		var n model.RecoveryResultNetwork
		var s model.RecoveryResultSubnet

		if err := database.Execute(func(db *gorm.DB) error {
			if err := db.Where(&model.RecoveryResultNetwork{
				RecoveryResultID:           resultID,
				ProtectionClusterTenantID:  i.ProtectionClusterTenantID,
				ProtectionClusterNetworkID: i.ProtectionClusterNetworkID,
			}).First(&n).Error; err != nil {
				return err
			}

			return db.Where(&model.RecoveryResultSubnet{
				RecoveryResultID:           resultID,
				ProtectionClusterTenantID:  i.ProtectionClusterTenantID,
				ProtectionClusterNetworkID: i.ProtectionClusterNetworkID,
				ProtectionClusterSubnetID:  i.ProtectionClusterSubnetID,
			}).First(&s).Error
		}); err != nil {
			return nil, nil, errors.UnusableDatabase(err)
		}

		pInternals = append(pInternals, &cms.ClusterNetworkRoutingInterface{
			Network: &cms.ClusterNetwork{
				Id:           n.ProtectionClusterNetworkID,
				TypeCode:     n.ProtectionClusterNetworkTypeCode,
				Uuid:         n.ProtectionClusterNetworkUUID,
				Name:         n.ProtectionClusterNetworkName,
				Description:  getString(n.ProtectionClusterNetworkDescription),
				ExternalFlag: false,
			},
			Subnet: &cms.ClusterSubnet{
				Id:                  s.ProtectionClusterSubnetID,
				Uuid:                s.ProtectionClusterSubnetUUID,
				Name:                s.ProtectionClusterSubnetName,
				Description:         getString(s.ProtectionClusterSubnetDescription),
				NetworkCidr:         s.ProtectionClusterSubnetNetworkCidr,
				DhcpEnabled:         s.ProtectionClusterSubnetDhcpEnabled,
				GatewayEnabled:      s.ProtectionClusterSubnetGatewayEnabled,
				GatewayIpAddress:    getString(s.ProtectionClusterSubnetGatewayIPAddress),
				Ipv6AddressModeCode: getString(s.ProtectionClusterSubnetIpv6AddressModeCode),
				Ipv6RaModeCode:      getString(s.ProtectionClusterSubnetIpv6RaModeCode),
			},
			IpAddress: i.ProtectionClusterIPAddress,
		})

		rInternals = append(rInternals, &cms.ClusterNetworkRoutingInterface{
			Network: &cms.ClusterNetwork{
				Id:           getUint64(n.RecoveryClusterNetworkID),
				TypeCode:     getString(n.RecoveryClusterNetworkTypeCode),
				Uuid:         getString(n.RecoveryClusterNetworkUUID),
				Name:         getString(n.RecoveryClusterNetworkName),
				Description:  getString(n.RecoveryClusterNetworkDescription),
				ExternalFlag: false,
			},
			Subnet: &cms.ClusterSubnet{
				Id:                  getUint64(s.RecoveryClusterSubnetID),
				Uuid:                getString(s.RecoveryClusterSubnetUUID),
				Name:                getString(s.RecoveryClusterSubnetName),
				Description:         getString(s.RecoveryClusterSubnetDescription),
				NetworkCidr:         getString(s.RecoveryClusterSubnetNetworkCidr),
				DhcpEnabled:         getBool(s.RecoveryClusterSubnetDhcpEnabled),
				GatewayEnabled:      getBool(s.RecoveryClusterSubnetGatewayEnabled),
				GatewayIpAddress:    getString(s.RecoveryClusterSubnetGatewayIPAddress),
				Ipv6AddressModeCode: getString(s.RecoveryClusterSubnetIpv6AddressModeCode),
				Ipv6RaModeCode:      getString(s.RecoveryClusterSubnetIpv6RaModeCode),
			},
			IpAddress: getString(i.RecoveryClusterIPAddress),
		})
	}

	return pInternals, rInternals, nil
}

func getExternalRoutingInterface(resultID, tenantID, routerID uint64) ([]*cms.ClusterNetworkRoutingInterface, []*cms.ClusterNetworkRoutingInterface, error) {
	var externalResults []model.RecoveryResultExternalRoutingInterface
	var pExternals []*cms.ClusterNetworkRoutingInterface
	var rExternals []*cms.ClusterNetworkRoutingInterface

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultExternalRoutingInterface{
			RecoveryResultID:          resultID,
			ProtectionClusterTenantID: tenantID,
			ProtectionClusterRouterID: routerID,
		}).Order("routing_interface_seq").Find(&externalResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, e := range externalResults {
		external := cms.ClusterNetworkRoutingInterface{
			Network: &cms.ClusterNetwork{
				Id:           e.ClusterNetworkID,
				TypeCode:     e.ClusterNetworkTypeCode,
				Uuid:         e.ClusterNetworkUUID,
				Name:         e.ClusterNetworkName,
				Description:  getString(e.ClusterNetworkDescription),
				ExternalFlag: true,
			},
			Subnet: &cms.ClusterSubnet{
				Id:                  e.ClusterSubnetID,
				Uuid:                e.ClusterSubnetUUID,
				Name:                e.ClusterSubnetName,
				Description:         getString(e.ClusterSubnetDescription),
				NetworkCidr:         e.ClusterSubnetNetworkCidr,
				DhcpEnabled:         e.ClusterSubnetDhcpEnabled,
				GatewayEnabled:      e.ClusterSubnetGatewayEnabled,
				GatewayIpAddress:    getString(e.ClusterSubnetGatewayIPAddress),
				Ipv6AddressModeCode: getString(e.ClusterSubnetIpv6AddressModeCode),
				Ipv6RaModeCode:      getString(e.ClusterSubnetIpv6RaModeCode),
			},
			IpAddress: e.ClusterIPAddress,
		}
		if e.ProtectionFlag == true {
			pExternals = append(pExternals, &external)
		} else {
			rExternals = append(rExternals, &external)
		}
	}

	return pExternals, rExternals, nil
}

func getExtraRoute(resultID, tenantID, routerID uint64) ([]*cms.ClusterRouterExtraRoute, []*cms.ClusterRouterExtraRoute, error) {
	var extraResults []model.RecoveryResultExtraRoute
	var pExtras []*cms.ClusterRouterExtraRoute
	var rExtras []*cms.ClusterRouterExtraRoute

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultExtraRoute{
			RecoveryResultID:          resultID,
			ProtectionClusterTenantID: tenantID,
			ProtectionClusterRouterID: routerID,
		}).Order("extra_route_seq").Find(&extraResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, e := range extraResults {
		pExtras = append(pExtras, &cms.ClusterRouterExtraRoute{
			Destination: e.ProtectionClusterDestination,
			Nexthop:     e.ProtectionClusterNexthop,
		})

		rExtras = append(rExtras, &cms.ClusterRouterExtraRoute{
			Destination: getString(e.RecoveryClusterDestination),
			Nexthop:     getString(e.RecoveryClusterNexthop),
		})
	}

	return pExtras, rExtras, nil
}

func getInstanceRouters(resultID, tenantID, instanceID uint64) ([]*cms.ClusterRouter, []*cms.ClusterRouter, error) {
	var routerResults []model.RecoveryResultRouter
	var networkResults []model.RecoveryResultInstanceNetwork
	var pRouters []*cms.ClusterRouter
	var rRouters []*cms.ClusterRouter
	var nidList []uint64

	if err := database.Execute(func(db *gorm.DB) error {
		if err := db.Where(&model.RecoveryResultRouter{
			RecoveryResultID:          resultID,
			ProtectionClusterTenantID: tenantID,
		}).Find(&routerResults).Error; err != nil {
			return err
		}

		return db.Where(&model.RecoveryResultInstanceNetwork{
			RecoveryResultID:            resultID,
			ProtectionClusterInstanceID: instanceID,
		}).Find(&networkResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, n := range networkResults {
		nidList = append(nidList, n.ProtectionClusterNetworkID)
	}

	for _, r := range routerResults {
		err := database.Execute(func(db *gorm.DB) error {
			return db.
				Where("protection_cluster_network_id IN (?)", nidList).
				Where(&model.RecoveryResultInternalRoutingInterface{
					RecoveryResultID:          r.RecoveryResultID,
					ProtectionClusterTenantID: r.ProtectionClusterTenantID,
					ProtectionClusterRouterID: r.ProtectionClusterRouterID,
				}).First(&model.RecoveryResultInternalRoutingInterface{}).Error
		})
		switch {
		case err == gorm.ErrRecordNotFound:
			// 인스턴스에서 사용하는 라우터가 아님
			continue

		case err != nil:
			return nil, nil, errors.UnusableDatabase(err)
		}

		pInternals, rInternals, err := getInternalRoutingInterface(resultID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return nil, nil, err
		}

		pExternals, rExternals, err := getExternalRoutingInterface(resultID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return nil, nil, err
		}

		pExtras, rExtras, err := getExtraRoute(resultID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return nil, nil, err
		}

		pRouters = append(pRouters, &cms.ClusterRouter{
			Id:                        r.ProtectionClusterRouterID,
			Uuid:                      r.ProtectionClusterRouterUUID,
			Name:                      r.ProtectionClusterRouterName,
			Description:               getString(r.ProtectionClusterRouterDescription),
			InternalRoutingInterfaces: pInternals,
			ExternalRoutingInterfaces: pExternals,
			ExtraRoutes:               pExtras,
		})

		rRouters = append(rRouters, &cms.ClusterRouter{
			Id:                        getUint64(r.RecoveryClusterRouterID),
			Uuid:                      getString(r.RecoveryClusterRouterUUID),
			Name:                      getString(r.RecoveryClusterRouterName),
			Description:               getString(r.RecoveryClusterRouterDescription),
			InternalRoutingInterfaces: rInternals,
			ExternalRoutingInterfaces: rExternals,
			ExtraRoutes:               rExtras,
		})
	}

	return pRouters, rRouters, nil
}

func getInstanceSecurityGroups(resultID, tenantID, instanceID uint64) ([]*cms.ClusterSecurityGroup, []*cms.ClusterSecurityGroup, error) {
	var pRules []*cms.ClusterSecurityGroupRule
	var rRules []*cms.ClusterSecurityGroupRule
	var pSecurityGroups []*cms.ClusterSecurityGroup
	var rSecurityGroups []*cms.ClusterSecurityGroup

	var instanceSecurityGroupResults []model.RecoveryResultInstanceSecurityGroup
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInstanceSecurityGroup{
			RecoveryResultID:            resultID,
			ProtectionClusterTenantID:   tenantID,
			ProtectionClusterInstanceID: instanceID,
		}).Find(&instanceSecurityGroupResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, isg := range instanceSecurityGroupResults {
		var ruleResults []model.RecoveryResultSecurityGroupRule
		if err := database.Execute(func(db *gorm.DB) error {
			return db.Where(&model.RecoveryResultSecurityGroupRule{
				RecoveryResultID:                 resultID,
				ProtectionClusterTenantID:        isg.ProtectionClusterTenantID,
				ProtectionClusterSecurityGroupID: isg.ProtectionClusterSecurityGroupID,
			}).Find(&ruleResults).Error
		}); err != nil {
			return nil, nil, errors.UnusableDatabase(err)
		}

		for _, r := range ruleResults {
			var remoteSgResult model.RecoveryResultSecurityGroup
			if getUint64(r.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID) != 0 {
				if err := database.Execute(func(db *gorm.DB) error {
					return db.Where(&model.RecoveryResultSecurityGroup{
						RecoveryResultID:                 resultID,
						ProtectionClusterTenantID:        r.ProtectionClusterTenantID,
						ProtectionClusterSecurityGroupID: getUint64(r.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID),
					}).First(&remoteSgResult).Error
				}); err != nil {
					return nil, nil, errors.UnusableDatabase(err)
				}
			}

			var rule = &cms.ClusterSecurityGroupRule{
				Id:           r.ProtectionClusterSecurityGroupRuleID,
				Uuid:         r.ProtectionClusterSecurityGroupRuleUUID,
				Direction:    r.ProtectionClusterSecurityGroupRuleDirection,
				Description:  getString(r.ProtectionClusterSecurityGroupRuleDescription),
				NetworkCidr:  getString(r.ProtectionClusterSecurityGroupRuleNetworkCidr),
				PortRangeMax: uint32(getUint64(r.ProtectionClusterSecurityGroupRulePortRangeMax)),
				PortRangeMin: uint32(getUint64(r.ProtectionClusterSecurityGroupRulePortRangeMin)),
				Protocol:     getString(r.ProtectionClusterSecurityGroupRuleProtocol),
			}
			if getUint64(r.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID) != 0 {
				rule.RemoteSecurityGroup = &cms.ClusterSecurityGroup{
					Id:          remoteSgResult.ProtectionClusterSecurityGroupID,
					Uuid:        remoteSgResult.ProtectionClusterSecurityGroupUUID,
					Name:        remoteSgResult.ProtectionClusterSecurityGroupName,
					Description: getString(remoteSgResult.ProtectionClusterSecurityGroupDescription),
				}
			}
			pRules = append(pRules, rule)

			rule = &cms.ClusterSecurityGroupRule{
				Id:           getUint64(r.RecoveryClusterSecurityGroupRuleID),
				Uuid:         getString(r.RecoveryClusterSecurityGroupRuleUUID),
				Direction:    getString(r.RecoveryClusterSecurityGroupRuleDirection),
				Description:  getString(r.RecoveryClusterSecurityGroupRuleDescription),
				NetworkCidr:  getString(r.RecoveryClusterSecurityGroupRuleNetworkCidr),
				PortRangeMax: uint32(getUint64(r.RecoveryClusterSecurityGroupRulePortRangeMax)),
				PortRangeMin: uint32(getUint64(r.RecoveryClusterSecurityGroupRulePortRangeMin)),
				Protocol:     getString(r.RecoveryClusterSecurityGroupRuleProtocol),
			}
			if getUint64(r.ProtectionClusterSecurityGroupRuleRemoteSecurityGroupID) != 0 {
				rule.RemoteSecurityGroup = &cms.ClusterSecurityGroup{
					Id:          getUint64(remoteSgResult.RecoveryClusterSecurityGroupID),
					Uuid:        getString(remoteSgResult.RecoveryClusterSecurityGroupUUID),
					Name:        getString(remoteSgResult.RecoveryClusterSecurityGroupName),
					Description: getString(remoteSgResult.RecoveryClusterSecurityGroupDescription),
				}
			}
			rRules = append(rRules, rule)
		}

		var securityGroupResult model.RecoveryResultSecurityGroup
		if err := database.Execute(func(db *gorm.DB) error {
			return db.Where(&model.RecoveryResultSecurityGroup{
				RecoveryResultID:                 resultID,
				ProtectionClusterTenantID:        isg.ProtectionClusterTenantID,
				ProtectionClusterSecurityGroupID: isg.ProtectionClusterSecurityGroupID,
			}).First(&securityGroupResult).Error
		}); err != nil {
			return nil, nil, errors.UnusableDatabase(err)
		}

		pSecurityGroups = append(pSecurityGroups, &cms.ClusterSecurityGroup{
			Id:          securityGroupResult.ProtectionClusterSecurityGroupID,
			Uuid:        securityGroupResult.ProtectionClusterSecurityGroupUUID,
			Name:        securityGroupResult.ProtectionClusterSecurityGroupName,
			Description: getString(securityGroupResult.ProtectionClusterSecurityGroupDescription),
			Rules:       pRules,
		})

		rSecurityGroups = append(rSecurityGroups, &cms.ClusterSecurityGroup{
			Id:          getUint64(securityGroupResult.RecoveryClusterSecurityGroupID),
			Uuid:        getString(securityGroupResult.RecoveryClusterSecurityGroupUUID),
			Name:        getString(securityGroupResult.RecoveryClusterSecurityGroupName),
			Description: getString(securityGroupResult.RecoveryClusterSecurityGroupDescription),
			Rules:       rRules,
		})
	}

	return pSecurityGroups, rSecurityGroups, nil
}

func getInstanceVolumes(resultID, tenantID, instanceID uint64) ([]*cms.ClusterInstanceVolume, []*cms.ClusterInstanceVolume, error) {
	var instanceVolumeResults []model.RecoveryResultInstanceVolume
	var pVolumes []*cms.ClusterInstanceVolume
	var rVolumes []*cms.ClusterInstanceVolume

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInstanceVolume{
			RecoveryResultID:            resultID,
			ProtectionClusterTenantID:   tenantID,
			ProtectionClusterInstanceID: instanceID,
		}).Find(&instanceVolumeResults).Error
	}); err != nil {
		return nil, nil, errors.UnusableDatabase(err)
	}

	for _, v := range instanceVolumeResults {
		var volumeResult model.RecoveryResultVolume
		if err := database.Execute(func(db *gorm.DB) error {
			return db.Where(&model.RecoveryResultVolume{
				RecoveryResultID:          resultID,
				ProtectionClusterTenantID: v.ProtectionClusterTenantID,
				ProtectionClusterVolumeID: v.ProtectionClusterVolumeID,
			}).First(&volumeResult).Error
		}); err != nil {
			return nil, nil, errors.UnusableDatabase(err)
		}

		pVolumes = append(pVolumes, &cms.ClusterInstanceVolume{
			Storage: &cms.ClusterStorage{
				Id:          volumeResult.ProtectionClusterStorageID,
				Uuid:        volumeResult.ProtectionClusterStorageUUID,
				Name:        volumeResult.ProtectionClusterStorageName,
				TypeCode:    volumeResult.ProtectionClusterStorageTypeCode,
				Description: getString(volumeResult.ProtectionClusterStorageDescription),
			},
			Volume: &cms.ClusterVolume{
				Id:          volumeResult.ProtectionClusterVolumeID,
				Uuid:        volumeResult.ProtectionClusterVolumeUUID,
				Name:        getString(volumeResult.ProtectionClusterVolumeName),
				Description: getString(volumeResult.ProtectionClusterVolumeDescription),
				SizeBytes:   volumeResult.ProtectionClusterVolumeSizeBytes,
				Multiattach: volumeResult.ProtectionClusterVolumeMultiattach,
				Bootable:    volumeResult.ProtectionClusterVolumeBootable,
				Readonly:    volumeResult.ProtectionClusterVolumeReadonly,
			},
			DevicePath: v.ProtectionClusterDevicePath,
			BootIndex:  v.ProtectionClusterBootIndex,
		})

		rVolumes = append(rVolumes, &cms.ClusterInstanceVolume{
			Storage: &cms.ClusterStorage{
				Id:          getUint64(volumeResult.RecoveryClusterStorageID),
				Uuid:        getString(volumeResult.RecoveryClusterStorageUUID),
				Name:        getString(volumeResult.RecoveryClusterStorageName),
				TypeCode:    getString(volumeResult.RecoveryClusterStorageTypeCode),
				Description: getString(volumeResult.RecoveryClusterStorageDescription),
			},
			Volume: &cms.ClusterVolume{
				Id:          getUint64(volumeResult.RecoveryClusterVolumeID),
				Uuid:        getString(volumeResult.RecoveryClusterVolumeUUID),
				Name:        getString(volumeResult.RecoveryClusterVolumeName),
				Description: getString(volumeResult.RecoveryClusterVolumeDescription),
				SizeBytes:   getUint64(volumeResult.RecoveryClusterVolumeSizeBytes),
				Multiattach: getBool(volumeResult.RecoveryClusterVolumeMultiattach),
				Bootable:    getBool(volumeResult.RecoveryClusterVolumeBootable),
				Readonly:    getBool(volumeResult.RecoveryClusterVolumeReadonly),
			},
			DevicePath: getString(v.RecoveryClusterDevicePath),
			BootIndex:  getInt64(v.RecoveryClusterBootIndex),
		})
	}

	return pVolumes, rVolumes, nil
}

func getRecoveryResultInstances(result *model.RecoveryResult) ([]*drms.RecoveryResultInstance, error) {
	var instanceResults []model.RecoveryResultInstance
	var instances []*drms.RecoveryResultInstance

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultInstance{
			RecoveryResultID: result.ID,
		}).Find(&instanceResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, i := range instanceResults {
		var pKeypair, rKeypair *cms.ClusterKeypair

		pTenant, rTenant, err := getTenant(result.ID, i.ProtectionClusterTenantID)
		if err != nil {
			return nil, err
		}

		pSpec, rSpec, err := getInstanceSpec(result.ID, i.ProtectionClusterInstanceSpecID)
		if err != nil {
			return nil, err
		}

		pNetworks, rNetworks, err := getInstanceNetworks(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return nil, err
		}

		pRouters, rRouters, err := getInstanceRouters(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return nil, err
		}

		pSecurityGroups, rSecurityGroups, err := getInstanceSecurityGroups(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return nil, err
		}

		pVolumes, rVolumes, err := getInstanceVolumes(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return nil, err
		}

		// protection cluster instance
		var pi = &cms.ClusterInstance{
			Id:          i.ProtectionClusterInstanceID,
			Uuid:        i.ProtectionClusterInstanceUUID,
			Name:        i.ProtectionClusterInstanceName,
			Description: getString(i.ProtectionClusterInstanceDescription),
			Cluster: &cms.Cluster{
				Id:       result.ProtectionClusterID,
				Name:     result.ProtectionClusterName,
				TypeCode: result.ProtectionClusterTypeCode,
				Remarks:  getString(result.ProtectionClusterRemarks),
			},
			AvailabilityZone: &cms.ClusterAvailabilityZone{
				Id:   i.ProtectionClusterAvailabilityZoneID,
				Name: i.ProtectionClusterAvailabilityZoneName,
			},
			Hypervisor: &cms.ClusterHypervisor{
				Id:        i.ProtectionClusterHypervisorID,
				TypeCode:  i.ProtectionClusterHypervisorTypeCode,
				Hostname:  i.ProtectionClusterHypervisorHostname,
				IpAddress: i.ProtectionClusterHypervisorIPAddress,
			},
			Tenant:         pTenant,
			Spec:           pSpec,
			Networks:       pNetworks,
			Routers:        pRouters,
			SecurityGroups: pSecurityGroups,
			Volumes:        pVolumes,
		}

		// recovery cluster instance
		var ri = &cms.ClusterInstance{
			Id:          getUint64(i.RecoveryClusterInstanceID),
			Uuid:        getString(i.RecoveryClusterInstanceUUID),
			Name:        getString(i.RecoveryClusterInstanceName),
			Description: getString(i.RecoveryClusterInstanceDescription),
			Cluster: &cms.Cluster{
				Id:       result.RecoveryClusterID,
				Name:     result.RecoveryClusterName,
				TypeCode: result.RecoveryClusterTypeCode,
				Remarks:  getString(result.RecoveryClusterRemarks),
			},
			AvailabilityZone: &cms.ClusterAvailabilityZone{
				Id:   getUint64(i.RecoveryClusterAvailabilityZoneID),
				Name: getString(i.RecoveryClusterAvailabilityZoneName),
			},
			Hypervisor: &cms.ClusterHypervisor{
				Id:        getUint64(i.RecoveryClusterHypervisorID),
				TypeCode:  getString(i.RecoveryClusterHypervisorTypeCode),
				Hostname:  getString(i.RecoveryClusterHypervisorHostname),
				IpAddress: getString(i.RecoveryClusterHypervisorIPAddress),
			},
			Tenant:         rTenant,
			Spec:           rSpec,
			Networks:       rNetworks,
			Routers:        rRouters,
			SecurityGroups: rSecurityGroups,
			Volumes:        rVolumes,
		}

		// instance 에 keypair 가 할당된 경우에만 keypair 를 넣어준다.
		if i.ProtectionClusterKeypairID != nil {
			pKeypair, rKeypair, err = getKeypair(result.ID, *i.ProtectionClusterKeypairID)
			if err != nil {
				return nil, err
			}

			pi.Keypair = pKeypair
			ri.Keypair = rKeypair
		}

		deps, err := getDependencies(result.ID, i.ProtectionClusterTenantID, i.ProtectionClusterInstanceID)
		if err != nil {
			return nil, err
		}

		instance := &drms.RecoveryResultInstance{
			ProtectionClusterInstance: pi,
			RecoveryClusterInstance:   ri,
			RecoveryPointTypeCode:     i.RecoveryPointTypeCode,
			RecoveryPoint:             i.RecoveryPoint,
			AutoStartFlag:             i.AutoStartFlag,
			DiagnosisFlag:             i.DiagnosisFlag,
			DiagnosisMethodCode:       getString(i.DiagnosisMethodCode),
			DiagnosisMethodData:       getString(i.DiagnosisMethodData),
			DiagnosisTimeout:          getUint32(i.DiagnosisTimeout),
			Dependencies:              deps,
			StartedAt:                 getInt64(i.StartedAt),
			FinishedAt:                getInt64(i.FinishedAt),
			ResultCode:                i.ResultCode,
		}

		if getString(i.FailedReasonCode) != "" {
			instance.FailedReason = &drms.Message{
				Code:     getString(i.FailedReasonCode),
				Contents: getString(i.FailedReasonContents),
			}
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

func getRecoveryResultVolumes(result *model.RecoveryResult) ([]*drms.RecoveryResultVolume, error) {
	var volumeResults []model.RecoveryResultVolume
	var volumes []*drms.RecoveryResultVolume

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultVolume{
			RecoveryResultID: result.ID,
		}).Find(&volumeResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, v := range volumeResults {
		pTenant, rTenant, err := getTenant(result.ID, v.ProtectionClusterTenantID)
		if err != nil {
			return nil, err
		}

		// protection cluster volume
		var pv = cms.ClusterVolume{
			Id: v.ProtectionClusterVolumeID,
			Cluster: &cms.Cluster{
				Id:       result.ProtectionClusterID,
				Name:     result.ProtectionClusterName,
				TypeCode: result.ProtectionClusterTypeCode,
				Remarks:  getString(result.ProtectionClusterRemarks),
			},
			Tenant: pTenant,
			Storage: &cms.ClusterStorage{
				Id:          v.ProtectionClusterStorageID,
				Uuid:        v.ProtectionClusterStorageUUID,
				Name:        v.ProtectionClusterStorageName,
				TypeCode:    v.ProtectionClusterStorageTypeCode,
				Description: getString(v.ProtectionClusterStorageDescription),
			},
			Uuid:        v.ProtectionClusterVolumeUUID,
			Name:        getString(v.ProtectionClusterVolumeName),
			Description: getString(v.ProtectionClusterVolumeDescription),
			SizeBytes:   v.ProtectionClusterVolumeSizeBytes,
			Multiattach: v.ProtectionClusterVolumeMultiattach,
			Bootable:    v.ProtectionClusterVolumeBootable,
			Readonly:    v.ProtectionClusterVolumeReadonly,
		}

		// recovery cluster volume
		var rv = cms.ClusterVolume{
			Id: getUint64(v.RecoveryClusterVolumeID),
			Cluster: &cms.Cluster{
				Id:       result.RecoveryClusterID,
				Name:     result.RecoveryClusterName,
				TypeCode: result.RecoveryClusterTypeCode,
				Remarks:  getString(result.RecoveryClusterRemarks),
			},
			Tenant: rTenant,
			Storage: &cms.ClusterStorage{
				Id:          getUint64(v.RecoveryClusterStorageID),
				Uuid:        getString(v.RecoveryClusterStorageUUID),
				Name:        getString(v.RecoveryClusterStorageName),
				TypeCode:    getString(v.RecoveryClusterStorageTypeCode),
				Description: getString(v.RecoveryClusterStorageDescription),
			},
			Uuid:        getString(v.RecoveryClusterVolumeUUID),
			Name:        getString(v.RecoveryClusterVolumeName),
			Description: getString(v.RecoveryClusterVolumeDescription),
			SizeBytes:   getUint64(v.RecoveryClusterVolumeSizeBytes),
			Multiattach: getBool(v.RecoveryClusterVolumeMultiattach),
			Bootable:    getBool(v.RecoveryClusterVolumeBootable),
			Readonly:    getBool(v.RecoveryClusterVolumeReadonly),
		}

		volume := drms.RecoveryResultVolume{
			ProtectionClusterVolume: &pv,
			RecoveryClusterVolume:   &rv,
			RecoveryPointTypeCode:   v.RecoveryPointTypeCode,
			RecoveryPoint:           v.RecoveryPoint,
			StartedAt:               getInt64(v.StartedAt),
			FinishedAt:              getInt64(v.FinishedAt),
			ResultCode:              v.ResultCode,
			RollbackFlag:            v.RollbackFlag,
		}

		if getString(v.FailedReasonCode) != "" {
			volume.FailedReason = &drms.Message{
				Code:     getString(v.FailedReasonCode),
				Contents: getString(v.FailedReasonContents),
			}
		}

		volumes = append(volumes, &volume)
	}
	return volumes, nil
}

func getRecoveryResultRouters(result *model.RecoveryResult) ([]*drms.RecoveryResultRouter, error) {
	var routerResults []model.RecoveryResultRouter
	var routers []*drms.RecoveryResultRouter

	if err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.RecoveryResultRouter{
			RecoveryResultID: result.ID,
		}).Find(&routerResults).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, r := range routerResults {
		pTenant, rTenant, err := getTenant(result.ID, r.ProtectionClusterTenantID)
		if err != nil {
			return nil, err
		}

		pInternals, rInternals, err := getInternalRoutingInterface(result.ID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return nil, err
		}

		pExternals, rExternals, err := getExternalRoutingInterface(result.ID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return nil, err
		}

		pExtras, rExtras, err := getExtraRoute(result.ID, r.ProtectionClusterTenantID, r.ProtectionClusterRouterID)
		if err != nil {
			return nil, err
		}

		// protection cluster router
		var pr = &cms.ClusterRouter{
			Id:                        r.ProtectionClusterRouterID,
			Tenant:                    pTenant,
			Uuid:                      r.ProtectionClusterRouterUUID,
			Name:                      r.ProtectionClusterRouterName,
			Description:               getString(r.ProtectionClusterRouterDescription),
			InternalRoutingInterfaces: pInternals,
			ExternalRoutingInterfaces: pExternals,
			ExtraRoutes:               pExtras,
		}

		// recovery cluster router
		var rr = &cms.ClusterRouter{
			Id:                        getUint64(r.RecoveryClusterRouterID),
			Tenant:                    rTenant,
			Uuid:                      getString(r.RecoveryClusterRouterUUID),
			Name:                      getString(r.RecoveryClusterRouterName),
			Description:               getString(r.RecoveryClusterRouterDescription),
			InternalRoutingInterfaces: rInternals,
			ExternalRoutingInterfaces: rExternals,
			ExtraRoutes:               rExtras,
		}

		routers = append(routers, &drms.RecoveryResultRouter{
			ProtectionClusterRouter: pr,
			RecoveryClusterRouter:   rr,
		})
	}

	return routers, nil
}

// Get 재해복구결과 보고서 조회
func Get(ctx context.Context, req *drms.RecoveryReportRequest) (*drms.RecoveryResult, error) {
	var result *model.RecoveryResult
	var recoveryResult drms.RecoveryResult
	var err error

	if req.GetGroupId() == 0 {
		return nil, errors.RequiredParameter("group_id")
	}

	if req.GetResultId() == 0 {
		return nil, errors.RequiredParameter("result_id")
	}

	if result, err = getRecoveryResult(req.GroupId, req.ResultId); err != nil {
		logger.Errorf("[RecoveryReport-Get] Could not get recovery result: group(%d) result(%d). Cause: %+v", req.GroupId, req.ResultId, err)
		return nil, err
	}

	if err = internal.IsAccessibleRecoveryReport(ctx, result.OwnerGroupID); err != nil {
		return nil, err
	}

	if err = recoveryResult.SetFromModel(result); err != nil {
		return nil, err
	}

	if err = database.Execute(func(db *gorm.DB) error {
		if recoveryResult.WarningReasons, err = getWarningReasons(result.ID); err != nil {
			logger.Errorf("[RecoveryReport-Get] Could not get recovery result(%d) warning reasons. Cause: %+v", result.ID, err)
			return err
		}

		if recoveryResult.FailedReasons, err = getFailedReasons(result.ID); err != nil {
			logger.Errorf("[RecoveryReport-Get] Could not get recovery result(%d) failed reasons. Cause: %+v", result.ID, err)
			return err
		}

		if recoveryResult.TaskLogs, err = getTaskLogs(result.ID); err != nil {
			logger.Errorf("[RecoveryReport-Get] Could not get recovery result(%d) task logs. Cause: %+v", result.ID, err)
			return err
		}

		if recoveryResult.Instances, err = getRecoveryResultInstances(result); err != nil {
			logger.Errorf("[RecoveryReport-Get] Could not get recovery result(%d) instances. Cause: %+v", result.ID, err)
			return err
		}

		if recoveryResult.Volumes, err = getRecoveryResultVolumes(result); err != nil {
			logger.Errorf("[RecoveryReport-Get] Could not get recovery result(%d) volumes. Cause: %+v", result.ID, err)
			return err
		}

		if recoveryResult.Routers, err = getRecoveryResultRouters(result); err != nil {
			logger.Errorf("[RecoveryReport-Get] Could not get recovery result(%d) routers. Cause: %+v", result.ID, err)
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &recoveryResult, nil
}

func getRecoveryReportList(filters ...recoveryReportFilter) ([]model.RecoveryResult, error) {
	var err error
	var results []model.RecoveryResult
	if err = database.Execute(func(db *gorm.DB) error {
		for _, f := range filters {
			if db, err = f.Apply(db); err != nil {
				return err
			}
		}

		if err = db.Order("started_at DESC").Find(&results).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return results, nil
}

func getRecoveryReportListPagination(filters ...recoveryReportFilter) (*drms.Pagination, error) {
	var err error
	var offset, limit, total uint64
	if err = database.Execute(func(db *gorm.DB) error {
		cond := db
		for _, f := range filters {
			if _, ok := f.(*paginationFilter); ok {
				offset = f.(*paginationFilter).Offset
				limit = f.(*paginationFilter).Limit
				continue
			}

			if cond, err = f.Apply(cond); err != nil {
				return err
			}
		}

		return cond.Model(&model.RecoveryResult{}).Count(&total).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	if limit == 0 {
		return &drms.Pagination{
			Page:       &wrappers.UInt64Value{Value: 1},
			TotalPage:  &wrappers.UInt64Value{Value: 1},
			TotalItems: &wrappers.UInt64Value{Value: total},
		}, nil
	}

	return &drms.Pagination{
		Page:       &wrappers.UInt64Value{Value: offset/limit + 1},
		TotalPage:  &wrappers.UInt64Value{Value: (total + limit - 1) / limit},
		TotalItems: &wrappers.UInt64Value{Value: total},
	}, nil
}

// GetList 재해복구결과 보고서 목록 조회
func GetList(ctx context.Context, req *drms.RecoveryReportListRequest) ([]*drms.RecoveryResult, *drms.Pagination, error) {
	var err error
	var results []model.RecoveryResult

	if err = validateRecoveryReportListRequest(req); err != nil {
		logger.Errorf("[RecoveryReport-GetList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	var filters = makeRecoveryReportFilters(req)

	if results, err = getRecoveryReportList(filters...); err != nil {
		logger.Errorf("[RecoveryReport-GetList] Could not get the recovery report list. Cause: %+v", err)
		return nil, nil, errors.UnusableDatabase(err)
	}

	var recoveryResults []*drms.RecoveryResult
	for _, r := range results {
		if err = internal.IsAccessibleRecoveryReport(ctx, r.OwnerGroupID); err != nil {
			logger.Errorf("[RecoveryReport-GetList] Errors occurred during checking accessible status of the recovery report(%d). Cause: %+v", r.ID, err)
			return nil, nil, err
		}

		var recoveryResult drms.RecoveryResult
		if err = recoveryResult.SetFromModel(&r); err != nil {
			return nil, nil, err
		}

		if recoveryResult.WarningReasons, err = getWarningReasons(r.ID); err != nil {
			return nil, nil, err
		}

		if recoveryResult.FailedReasons, err = getFailedReasons(r.ID); err != nil {
			return nil, nil, err
		}

		recoveryResults = append(recoveryResults, &recoveryResult)
	}

	var pagination *drms.Pagination
	if err = database.Execute(func(db *gorm.DB) error {
		pagination, err = getRecoveryReportListPagination(filters...)
		if err != nil {
			logger.Errorf("[RecoveryReport-GetList] Could not get the recovery report list pagination. Cause: %+v", err)
			return err
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	return recoveryResults, pagination, nil
}

func checkValidJobSummaryRequest(req *drms.JobSummaryRequest) error {
	recoveryType := req.GetRecoveryType()
	if recoveryType == "" {
		return errors.RequiredParameter("recovery_type")
	}

	if !internal.IsRecoveryJobTypeCode(recoveryType) {
		return errors.UnavailableParameterValue("recovery_type", recoveryType, internal.RecoveryJobTypeCodes)
	}

	startDate := req.GetStartDate()
	endDate := req.GetEndDate()
	if startDate != nil && endDate != nil {
		if startDate.GetValue() >= endDate.GetValue() {
			return errors.InvalidParameterValue("end_date", endDate.GetValue(), "end_date is before than start_date")
		}
	}

	return nil
}

// GetJobSummary job 결과 요약 정보 조회
func GetJobSummary(ctx context.Context, req *drms.JobSummaryRequest) ([]*drms.JobSummary, error) {
	if err := checkValidJobSummaryRequest(req); err != nil {
		return nil, err
	}

	// get location from global time zone
	tid, _ := metadata.GetTenantID(ctx)

	var name string
	if err := database.Execute(func(db *gorm.DB) error {
		if tz := config.TenantConfig(db, tid, config.GlobalTimeZone); tz != nil {
			name = tz.Value.String()
		} else {
			name = time.UTC.String()
		}

		return nil
	}); err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, errors.Unknown(err)
	}

	// get recovery reports
	var clusterMap map[uint64]*cms.Cluster
	if clusterMap, err = cluster.GetClusterMap(ctx); err != nil {
		return nil, err
	}

	var reports []*model.RecoveryResult
	filters := makeJobSummaryRequestFilters(req, clusterMap)
	if reports, err = getJobSummary(filters...); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	if len(reports) == 0 {
		return nil, nil
	}

	// group by date
	m := make(map[string]*drms.JobSummary)
	for _, item := range reports {
		dt := time.Unix(item.FinishedAt, 0).In(loc).Format("20060102")

		if m[dt] == nil {
			m[dt] = &drms.JobSummary{Date: dt}
		}

		switch item.ResultCode {
		case constant.RecoveryResultCodeSuccess, constant.RecoveryResultCodePartialSuccess: // 부분 성공도 성공
			m[dt].Success++

		case constant.RecoveryResultCodeFailed:
			m[dt].Fail++
			//case constant.RecoveryResultCodeCanceled: // 취소한 작업은 집계하지 않음
		}
	}

	var keys []string
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var ret []*drms.JobSummary
	for _, k := range keys {
		ret = append(ret, m[k])
	}

	return ret, nil
}

func getJobSummary(filters ...recoveryReportFilter) ([]*model.RecoveryResult, error) {
	var err error
	var results []*model.RecoveryResult
	if err = database.Execute(func(db *gorm.DB) error {
		for _, f := range filters {
			if db, err = f.Apply(db); err != nil {
				return err
			}
		}

		if err = db.Find(&results).Error; err != nil {
			return errors.UnusableDatabase(err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return results, nil
}

func deleteTaskLogRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultTaskLog{RecoveryResultID: resultID}).Delete(&model.RecoveryResultTaskLog{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteTaskLogRecoveryResult] Could not delete task log recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteFailedReasonRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultFailedReason{RecoveryResultID: resultID}).Delete(&model.RecoveryResultFailedReason{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteFailedReasonRecoveryResult] Could not delete failed reason recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteWarningReasonRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultWarningReason{RecoveryResultID: resultID}).Delete(&model.RecoveryResultWarningReason{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteWarningReasonRecoveryResult] Could not delete warning reason recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteInstanceRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultInstanceVolume{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstanceVolume{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceRecoveryResult] Could not delete instance volume recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultInstanceNetwork{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstanceNetwork{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceRecoveryResult] Could not delete instance network recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultInstanceSecurityGroup{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstanceSecurityGroup{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceRecoveryResult] Could not delete instance security group recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultInstanceDependency{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstanceDependency{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceRecoveryResult] Could not delete instance dependency recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultInstance{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstance{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceRecoveryResult] Could not delete instance recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteVolumeRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultVolume{RecoveryResultID: resultID}).Delete(&model.RecoveryResultVolume{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteVolumeRecoveryResult] Could not delete volume recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteInstanceSpecRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultInstanceExtraSpec{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstanceExtraSpec{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceSpecRecoveryResult] Could not delete extra spec recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultInstanceSpec{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInstanceSpec{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteInstanceSpecRecoveryResult] Could not delete spec recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteKeypairRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultKeypair{RecoveryResultID: resultID}).Delete(&model.RecoveryResultKeypair{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteKeypairRecoveryResult] Could not delete keypair recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteRouterRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultExtraRoute{RecoveryResultID: resultID}).Delete(&model.RecoveryResultExtraRoute{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteRouterRecoveryResult] Could not delete router's extraRoutes. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultInternalRoutingInterface{RecoveryResultID: resultID}).Delete(&model.RecoveryResultInternalRoutingInterface{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteRouterRecoveryResult] Could not delete internal routing interface recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultExternalRoutingInterface{RecoveryResultID: resultID}).Delete(&model.RecoveryResultExternalRoutingInterface{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteRouterRecoveryResult] Could not delete external routing interface recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultRouter{RecoveryResultID: resultID}).Delete(&model.RecoveryResultRouter{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteRouterRecoveryResult] Could not delete router recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteFloatingIPRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultFloatingIP{RecoveryResultID: resultID}).Delete(&model.RecoveryResultFloatingIP{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteFloatingIPRecoveryResult] Could not delete floating ip recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteSecurityGroupRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultSecurityGroupRule{RecoveryResultID: resultID}).Delete(&model.RecoveryResultSecurityGroupRule{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteSecurityGroupRecoveryResult] Could not delete security group rule recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultSecurityGroup{RecoveryResultID: resultID}).Delete(&model.RecoveryResultSecurityGroup{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteSecurityGroupRecoveryResult] Could not delete security group recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteSubnetRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultSubnetNameserver{RecoveryResultID: resultID}).Delete(&model.RecoveryResultSubnetNameserver{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteSubnetRecoveryResult] Could not delete subnet nameserver recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultSubnetDHCPPool{RecoveryResultID: resultID}).Delete(&model.RecoveryResultSubnetDHCPPool{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteSubnetRecoveryResult] Could not delete subnet DHCP Pool recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	if err := db.Where(&model.RecoveryResultSubnet{RecoveryResultID: resultID}).Delete(&model.RecoveryResultSubnet{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteSubnetRecoveryResult] Could not delete subnet recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteNetworkRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultNetwork{RecoveryResultID: resultID}).Delete(&model.RecoveryResultNetwork{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteNetworkRecoveryResult] Could not delete network recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteTenantRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultTenant{RecoveryResultID: resultID}).Delete(&model.RecoveryResultTenant{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteTenantRecoveryResult] Could not delete tenant recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteRecoveryResultRaw(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResultRaw{RecoveryResultID: resultID}).Delete(&model.RecoveryResultRaw{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteRecoveryResultRaw] Could not delete recovery result raw. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

func deleteRecoveryResult(db *gorm.DB, resultID uint64) error {
	if err := db.Where(&model.RecoveryResult{ID: resultID}).Delete(&model.RecoveryResult{}).Error; err != nil {
		logger.Errorf("[RecoveryReport-deleteRecoveryResult] Could not delete recovery result. Cause: %v", err)
		return errors.UnusableDatabase(err)
	}

	return nil
}

// Delete 재해복구결과 보고서 삭제: 모의훈련
func Delete(ctx context.Context, req *drms.DeleteRecoveryReportRequest) error {
	logger.Info("[RecoveryReport-Delete] Start")

	var err error

	if req.GetGroupId() == 0 {
		err = errors.RequiredParameter("group_id")
		logger.Errorf("[RecoveryReport-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if req.GetResultId() == 0 {
		err = errors.RequiredParameter("result_id")
		logger.Errorf("[RecoveryReport-Delete] Errors occurred during validating the request. Cause: %+v", err)
		return err
	}

	if err = checkDeletableRecoveryResult(ctx, req); err != nil {
		logger.Errorf("[RecoveryReport-Delete] Errors occurred during checking deletable status of the recovery result(%d). Cause: %+v", req.ResultId, err)
		return err
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = deleteTaskLogRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the recovery result(%d) task log. Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteFailedReasonRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the recovery result(%d) failed reason. Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteWarningReasonRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the recovery result(%d) warning reason. Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteInstanceRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the instance recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteVolumeRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the volume recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteInstanceSpecRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the instance spec recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteKeypairRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the keypair recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteFloatingIPRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the floating IP recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteSecurityGroupRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the security group recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteRouterRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the router recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteSubnetRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the subnet recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteNetworkRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the network recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteTenantRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the tenant recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteRecoveryResultRaw(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the recovery result(%d) raw. Cause: %+v", req.ResultId, err)
			return err
		}

		if err = deleteRecoveryResult(db, req.ResultId); err != nil {
			logger.Errorf("[RecoveryReport-Delete] Could not delete the recovery result(%d). Cause: %+v", req.ResultId, err)
			return err
		}

		return nil
	}); err != nil {
		logger.Infof("[RecoveryReport-Delete] Failed (resultID:%d)", req.ResultId)
		return err
	}

	logger.Infof("[RecoveryReport-Delete] Success (resultID:%d)", req.ResultId)
	return nil
}

func getProtectionGroupHistory() ([]*drms.ProtectionCluster, error) {
	var err error
	var results []model.RecoveryResult
	var clusters []*drms.ProtectionCluster
	var protectionGroupMap = make(map[uint64][]*drms.ProtectionGroup)
	var isDupClusterID = make(map[uint64]bool)
	var isDupClusterName = make(map[string]bool)
	var isDupProtectionGroupID = make(map[uint64]bool)
	var isDupProtectionGroupName = make(map[string]bool)

	// recovery report 의 전체 list 를 가져온다.
	if results, err = getRecoveryReportList(); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	for _, r := range results {
		pc := &drms.ProtectionCluster{
			Id:   r.ProtectionClusterID,
			Name: r.ProtectionClusterName,
		}

		pg := &drms.ProtectionGroup{
			Id:   r.ProtectionGroupID,
			Name: r.ProtectionGroupName,
		}

		// 중복된 protection group 은 처리하지 않는다.
		if !isDupProtectionGroupID[pg.Id] || !isDupProtectionGroupName[pg.Name] {
			protectionGroupMap[pc.Id] = append(protectionGroupMap[pc.Id], pg)
			isDupProtectionGroupID[pg.Id] = true
			isDupProtectionGroupName[pg.Name] = true
		}

		// 중복된 protection cluster 는 처리하지 않는다.
		if !isDupClusterID[pc.Id] || !isDupClusterName[pc.Name] {
			clusters = append(clusters, pc)
			isDupClusterID[pc.Id] = true
			isDupClusterName[pc.Name] = true
		}
	}

	for _, c := range clusters {
		c.Groups = protectionGroupMap[c.Id]
	}

	return clusters, nil
}

// GetProtectionGroupHistory 재해복구결과 보고서를 통해 protection group 의 history 조회
func GetProtectionGroupHistory(_ context.Context) (*drms.History, error) {
	var err error
	var clusters []*drms.ProtectionCluster

	if clusters, err = getProtectionGroupHistory(); err != nil {
		return nil, err
	}

	return &drms.History{Clusters: clusters}, nil
}

// CreateRecoveryResultRaw 재해복구결과 raw 생성
func CreateRecoveryResultRaw(jobID, resultID uint64) {
	logger.Infof("[CreateRecoveryResultRaw] Start: job(%d) result(%d)", jobID, resultID)
	raw := &model.RecoveryResultRaw{
		RecoveryResultID: resultID,
		RecoveryJobID:    jobID,
	}

	job, err := getJobData(jobID)
	if err != nil {
		logger.Infof("[CreateRecoveryResultRaw] Could not get job data: job(%d) result(%d). Cause: %+v", jobID, resultID, err)
	}
	raw.Contents = job

	task, err := getSharedTaskData()
	if err != nil {
		logger.Infof("[CreateRecoveryResultRaw] Could not get shared task data: job(%d) result(%d). Cause: %+v", jobID, resultID, err)
	}
	raw.SharedTask = task

	if err := database.GormTransaction(func(db *gorm.DB) error {
		return db.Save(&raw).Error
	}); err != nil {
		logger.Warnf("[CreateRecoveryResultRaw] Could not save result raw: job(%d) result(%d). Cause: %+v", jobID, resultID, err)
	}

	logger.Infof("[CreateRecoveryResultRaw] Success: job(%d) result(%d)", jobID, resultID)
}
