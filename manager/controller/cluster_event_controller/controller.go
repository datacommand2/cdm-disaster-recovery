package clustereventcontroller

import (
	"context"
	"encoding/json"
	"github.com/datacommand2/cdm-center/cluster-manager/constant"
	"github.com/datacommand2/cdm-center/cluster-manager/queue"
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	drConstant "github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/manager/controller"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	protectionGroup "github.com/datacommand2/cdm-disaster-recovery/manager/internal/protection_group"
	recoveryPlan "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan"
	"github.com/jinzhu/gorm"
	"reflect"
)

var (
	defaultController clusterEventController // singleton
	subscriberList    []broker.Subscriber
)

func init() {
	controller.RegisterController(&defaultController)
}

type clusterEventController struct {
}

func (c *clusterEventController) Run() {
	if len(subscriberList) != 0 {
		logger.Warn("[EventController-Run] Already started.")
		return
	}

	logger.Info("[EventController-Run] Starting cluster event controller.")

	for topic, handlerFunc := range map[string]broker.Handler{
		constant.QueueNoticeClusterStorageUpdated:          c.HandleStorageUpdatedEvent,
		constant.QueueNoticeClusterStorageDeleted:          c.HandleStorageDeletedEvent,
		constant.QueueNoticeClusterInstanceNetworkAttached: c.HandleInstanceNetworkAttachedEvent,
		constant.QueueNoticeClusterInstanceNetworkUpdated:  c.HandleInstanceNetworkUpdatedEvent,
		constant.QueueNoticeClusterInstanceNetworkDetached: c.HandleInstanceNetworkDetachedEvent,
		constant.QueueNoticeClusterInstanceVolumeAttached:  c.HandleInstanceVolumeAttachedEvent,
		constant.QueueNoticeClusterInstanceVolumeDetached:  c.HandleInstanceVolumeDetachedEvent,
		// TODO: hypervisor 자동할당 사용시 주석 제거(instance/hypervisor Created,Updated)
		//constant.QueueNoticeClusterInstanceCreated:         c.HandleInstanceCreatedEvent,
		//constant.QueueNoticeClusterInstanceUpdated:         c.HandleInstanceUpdatedEvent,
		constant.QueueNoticeClusterInstanceDeleted:         c.HandleInstanceDeletedEvent,
		constant.QueueNoticeClusterAvailabilityZoneDeleted: c.HandleAvailabilityZoneDeletedEvent,
		//constant.QueueNoticeClusterHypervisorCreated:       c.HandleHypervisorCreatedEvent,
		constant.QueueNoticeClusterHypervisorUpdated:       c.HandleHypervisorUpdatedEvent,
		constant.QueueNoticeClusterHypervisorDeleted:       c.HandleHypervisorDeletedEvent,
		constant.QueueNoticeClusterNetworkUpdated:          c.HandleNetworkUpdatedEvent,
		constant.QueueNoticeClusterNetworkDeleted:          c.HandleNetworkDeletedEvent,
		constant.QueueNoticeClusterSubnetDeleted:           c.HandleSubnetDeletedEvent,
		constant.QueueNoticeClusterFloatingIPCreated:       c.HandleFloatingIPCreatedEvent,
		constant.QueueNoticeClusterFloatingIPUpdated:       c.HandleFloatingIPUpdatedEvent,
		constant.QueueNoticeClusterFloatingIPDeleted:       c.HandleFloatingIPDeletedEvent,
		constant.QueueNoticeClusterRouterCreated:           c.HandleRouterCreatedEvent,
		constant.QueueNoticeClusterRouterDeleted:           c.HandleRouterDeletedEvent,
		constant.QueueNoticeClusterRoutingInterfaceCreated: c.HandleRoutingInterfaceCreatedEvent,
		constant.QueueNoticeClusterRoutingInterfaceDeleted: c.HandleRoutingInterfaceDeletedEvent,
		drConstant.QueueSyncPlanListByProtectionGroup:      c.HandleSyncPlanListByProtectionGroupID,
		drConstant.QueueSyncAllPlansByRecoveryPlan:         c.HandleSyncAllPlansByPlanID,
	} {
		logger.Debugf("Subscribe cluster event (%s).", topic)

		sub, err := broker.SubscribePersistentQueue(topic, handlerFunc, true)

		if err != nil {
			logger.Fatalf("Could not start cluster event controller. Cause: %+v", errors.UnusableBroker(err))
		}

		subscriberList = append(subscriberList, sub)
	}
}

func (c *clusterEventController) Close() {
	if len(subscriberList) == 0 {
		logger.Warn("[EventController-Close] Not started.")
		return
	}

	logger.Info("[EventController-Close] Stopping.")

	for _, sub := range subscriberList {
		logger.Debugf("Unsubscribe cluster event (%s).", sub.Topic())

		if err := sub.Unsubscribe(); err != nil {
			logger.Warnf("[EventController-Close] Could not unsubscribe queue (%s). Cause: %v", sub.Topic(), err)
		}
	}

	subscriberList = []broker.Subscriber{}
}

// HandleStorageUpdatedEvent 스토리지 수정 이벤트에 대한 재해 복구 스토리지 계획 동기화 함수
func (c *clusterEventController) HandleStorageUpdatedEvent(p broker.Event) error {
	var msg queue.UpdateClusterStorage
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleStorageUpdatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	// storage 의 backend name 이 변경된 경우에만 동기화한다.
	if msg.OrigVolumeBackendName == msg.VolumeBackendName {
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.SetStorageRecoveryPlanUnavailableFlag(ctx, msg.Cluster.ID, msg.OrigStorage.ID, drConstant.RecoveryPlanStorageTargetBackendNameChanged); err != nil {
		logger.Errorf("[HandleStorageUpdatedEvent] Could not update the cluster(%d) storage(%s) plan. Cause: %+v", msg.Cluster.ID, msg.OrigStorage.UUID, err)
		return err
	}

	if err = recoveryPlan.SetVolumeRecoveryPlanUnavailableFlag(msg.Cluster.ID, msg.OrigStorage.ID, drConstant.RecoveryPlanVolumeTargetBackendNameChanged); err != nil {
		logger.Errorf("[HandleStorageUpdatedEvent] Could not update the cluster(%d) volume plan(storage:%s). Cause: %+v", msg.Cluster.ID, msg.OrigStorage.UUID, err)
		return err
	}

	return nil
}

// HandleStorageDeletedEvent 스토리지 삭제 이벤트에 대한 재해 복구 스토리지 계획 동기화 함수
func (c *clusterEventController) HandleStorageDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterStorage
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleStorageDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.SetVolumeRecoveryPlanUnavailableFlag(msg.Cluster.ID, msg.OrigStorage.ID, drConstant.RecoveryPlanVolumeTargetDeleted); err != nil {
		logger.Errorf("[HandleStorageDeletedEvent] Could not update the cluster(%d) volume plan(storage:%s). Cause: %+v", msg.Cluster.ID, msg.OrigStorage.UUID, err)
		return err
	}

	if err = recoveryPlan.SetVolumeRecoveryPlanRecoveryClusterStorageNilAndUpdateFlagTrue(msg.Cluster.ID, msg.OrigStorage.ID, drConstant.RecoveryPlanVolumeTargetDeleted); err != nil {
		logger.Errorf("[HandleStorageDeletedEvent] Could not update the cluster(%d) volume plan(storage:%s). Cause: %+v", msg.Cluster.ID, msg.OrigStorage.UUID, err)
		return err
	}

	if err = recoveryPlan.SetStorageRecoveryPlanRecoveryClusterStorageUpdateFlag(ctx, msg.Cluster.ID, msg.OrigStorage.ID, drConstant.RecoveryPlanStorageTargetDeleted); err != nil {
		logger.Errorf("[HandleStorageDeletedEvent] Could not update the cluster(%d) storage(%s) plan. Cause: %+v", msg.Cluster.ID, msg.OrigStorage.UUID, err)
		return err
	}

	return nil
}

// HandleInstanceNetworkAttachedEvent 인스턴스 네트워크 attached 에 대한 재해 복구 계획 동기화 함수
func (c *clusterEventController) HandleInstanceNetworkAttachedEvent(p broker.Event) error {
	var msg queue.AttachClusterInstanceNetwork
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceNetworkAttachedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByInstanceID(ctx, msg.Cluster.ID, msg.InstanceNetwork.ClusterInstanceID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleInstanceNetworkAttachedEvent] Could not update the plans related to cluster(%d) instance(%d) network. Cause: %+v",
			msg.Cluster.ID, msg.InstanceNetwork.ClusterInstanceID, err)
		return err
	}

	return nil
}

// HandleInstanceNetworkUpdatedEvent 인스턴스 네트워크 수정에 대한 재해 복구 계획 동기화 함수
func (c *clusterEventController) HandleInstanceNetworkUpdatedEvent(p broker.Event) error {
	var msg queue.UpdateClusterInstanceNetwork
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceNetworkUpdatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	if reflect.DeepEqual(msg.OrigInstanceNetwork.ClusterFloatingIPID, msg.InstanceNetwork.ClusterFloatingIPID) {
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByInstanceID(ctx, msg.Cluster.ID, msg.InstanceNetwork.ClusterInstanceID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleInstanceNetworkUpdatedEvent] Could not update the plans related to cluster(%d) instance(%d) network. Cause: %+v",
			msg.Cluster.ID, msg.InstanceNetwork.ClusterInstanceID, err)
		return err
	}

	return nil
}

// HandleInstanceNetworkDetachedEvent 인스턴스 네트워크 detached 에 대한 재해 복구 계획 동기화 함수
func (c *clusterEventController) HandleInstanceNetworkDetachedEvent(p broker.Event) error {
	var msg queue.DetachClusterInstanceNetwork
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceNetworkDetachedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByInstanceID(ctx, msg.Cluster.ID, msg.OrigInstanceNetwork.ClusterInstanceID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleInstanceNetworkDetachedEvent] Could not update the plans related to cluster(%d) instance(%d) network. Cause: %+v",
			msg.Cluster.ID, msg.OrigInstanceNetwork.ClusterInstanceID, err)
		return err
	}

	return nil
}

// HandleAvailabilityZoneDeletedEvent 가용구역 삭제 이벤트에 대한 재해 복구 가용구역 계획 동기화 함수
func (c *clusterEventController) HandleAvailabilityZoneDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterAvailabilityZone
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleAvailabilityZoneDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.SetAvailabilityZoneRecoveryPlanUpdateFlag(ctx, msg.Cluster.ID, msg.OrigAvailabilityZone.ID); err != nil {
		logger.Errorf("[HandleAvailabilityZoneDeletedEvent] Could not update the cluster(%d) availability zone(%s) plan. Cause: %+v", msg.Cluster.ID, msg.OrigAvailabilityZone.Name, err)
		return err
	}

	return nil
}

// HandleHypervisorCreatedEvent 하이퍼바이저 생성 이벤트에 대한 재해 복구 하이퍼바이저 계획 동기화 함수
func (c *clusterEventController) HandleHypervisorCreatedEvent(p broker.Event) error {
	var msg queue.CreateClusterHypervisor
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleHypervisorCreatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.ReassignHypervisor(ctx, msg.Cluster.ID, msg.Hypervisor.ClusterAvailabilityZoneID); err != nil {
		logger.Errorf("[HandleHypervisorCreatedEvent] Could not update the cluster(%d) hypervisor(%s) plan. Cause: %+v", msg.Cluster.ID, msg.Hypervisor.UUID, err)
		return err
	}

	return nil
}

// HandleHypervisorUpdatedEvent 하이퍼바이저 수정 이벤트에 대한 재해 복구 하이퍼바이저 계획 동기화 함수
func (c *clusterEventController) HandleHypervisorUpdatedEvent(p broker.Event) error {
	var msg queue.UpdateClusterHypervisor
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleHypervisorUpdatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	// 복구대상 hypervisor 가 사용불가능 상태
	if msg.Hypervisor.State != "up" || msg.Hypervisor.Status != "enabled" {
		if err = recoveryPlan.SetInstanceRecoveryPlanUpdateFlag(msg.Cluster.ID, msg.Hypervisor.ID); err != nil {
			logger.Errorf("[HandleHypervisorUpdatedEvent] Could not update the cluster(%d) instance plan(hypervisor:%s). Cause: %+v", msg.Cluster.ID, msg.Hypervisor.UUID, err)
			return err
		}
	} else {
		// msg.Hypervisor.State == "up" && msg.Hypervisor.Status == "enabled"
		if err = recoveryPlan.UnsetInstanceRecoveryPlanUpdateFlag(msg.Cluster.ID, msg.Hypervisor.ID); err != nil {
			logger.Errorf("[HandleHypervisorUpdatedEvent] Could not update the cluster(%d) instance plan(hypervisor:%s). Cause: %+v", msg.Cluster.ID, msg.Hypervisor.UUID, err)
			return err
		}
	}

	// TODO: hypervisor 자동할당 사용시 주석 제거
	// 하이퍼바이저의 리소스가 변경된 경우에만 reassign 한다.
	//if msg.OrigHypervisor.MemTotalBytes != msg.Hypervisor.MemTotalBytes ||
	//	msg.OrigHypervisor.MemUsedBytes != msg.Hypervisor.MemUsedBytes ||
	//	msg.OrigHypervisor.VcpuTotalCnt != msg.Hypervisor.VcpuTotalCnt ||
	//	msg.OrigHypervisor.VcpuUsedCnt != msg.Hypervisor.VcpuUsedCnt {
	//	ctx, err := helper.DefaultContext(db)
	//	if err != nil {
	//		return err
	//	}
	//
	//	if err := recoveryPlan.ReassignHypervisor(ctx, msg.Cluster.ID, msg.Hypervisor.ClusterAvailabilityZoneID); err != nil {
	//		return err
	//	}
	//}

	return nil
}

// HandleHypervisorDeletedEvent 하이퍼바이저 삭제 이벤트에 대한 재해 복구 하이퍼바이저 계획 동기화 함수
func (c *clusterEventController) HandleHypervisorDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterHypervisor
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleHypervisorDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	// TODO: hypervisor 자동할당 사용시 주석 제거 및 SetInstanceRecoveryPlanUpdateFlag 삭제
	//if err = recoveryPlan.ReassignHypervisor(msg.Cluster.ID, msg.OrigHypervisor.ClusterAvailabilityZoneID); err != nil {
	// return err
	//}

	if err = recoveryPlan.SetInstanceRecoveryPlanUpdateFlagAndNil(msg.Cluster.ID, msg.OrigHypervisor.ID); err != nil {
		logger.Errorf("[HandleHypervisorDeletedEvent] Could not update the cluster(%d) instance plan(hypervisor:%s). Cause: %+v", msg.Cluster.ID, msg.OrigHypervisor.UUID, err)
		return err
	}

	return nil
}

// HandleNetworkUpdatedEvent 네트워크 수정 이벤트에 대한 재해 복구 네트워크 계획 동기화 함수
func (c *clusterEventController) HandleNetworkUpdatedEvent(p broker.Event) (err error) {
	var msg queue.UpdateClusterNetwork
	if err = json.Unmarshal(p.Message().Body, &msg); err != nil {
		logger.Errorf("[HandleNetworkUpdatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	// network 의 external flag 가 변경되었을 때만 동기화한다.
	if msg.OrigNetwork.ExternalFlag == msg.Network.ExternalFlag {
		return nil
	}

	if !msg.OrigNetwork.ExternalFlag && msg.Network.ExternalFlag { // external_flag=false -> true 변경: unsetNetwork, unsetRouter
		if err = recoveryPlan.UnsetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag(msg.Cluster.ID, msg.Network.ID); err != nil {
			logger.Errorf("[HandleNetworkUpdatedEvent] Could not update the cluster(%d) network(%s) plan. Cause: %+v", msg.Cluster.ID, msg.Network.UUID, err)
			return err
		}

		if err = recoveryPlan.UnsetRouterRecoveryPlanExternalNetworkUpdateFlag(msg.Cluster.ID, msg.Network.ID); err != nil {
			logger.Errorf("[HandleNetworkUpdatedEvent] Could not update the cluster(%d) router plan(network:%s). Cause: %+v", msg.Cluster.ID, msg.Network.UUID, err)
			return err
		}

	} else if msg.OrigNetwork.ExternalFlag && !msg.Network.ExternalFlag { // external_flag=true -> false 변경: setNetwork, setRouter
		if err = recoveryPlan.SetExternalNetworkRecoveryPlanExternalNetworkUpdateFlag(msg.Cluster.ID, msg.Network.ID); err != nil {
			logger.Errorf("[HandleNetworkUpdatedEvent] Could not update the cluster(%d) network(%s) plan. Cause: %+v", msg.Cluster.ID, msg.Network.UUID, err)
			return err
		}

		if err = recoveryPlan.SetRouterRecoveryPlanExternalNetworkUpdateFlag(msg.Cluster.ID, msg.Network.ID); err != nil {
			logger.Errorf("[HandleNetworkUpdatedEvent] Could not update the cluster(%d) router plan(network:%s). Cause: %+v", msg.Cluster.ID, msg.Network.UUID, err)
			return err
		}
	}

	return nil
}

// HandleNetworkDeletedEvent 네트워크 삭제 이벤트에 대한 재해 복구 네트워크 계획 동기화 함수
func (c *clusterEventController) HandleNetworkDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterNetwork
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleNetworkDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	if err = recoveryPlan.SetExternalNetworkRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue(msg.Cluster.ID, msg.OrigNetwork.ID); err != nil {
		logger.Errorf("[HandleNetworkDeletedEvent] Could not update the cluster(%d) network(%s) plan. Cause: %+v", msg.Cluster.ID, msg.OrigNetwork.UUID, err)
		return err
	}

	if err = recoveryPlan.SetRouterRecoveryPlanRecoveryExternalNetworkNilAndUpdateFlagTrue(msg.Cluster.ID, msg.OrigNetwork.ID); err != nil {
		logger.Errorf("[HandleNetworkDeletedEvent] Could not update the cluster(%d) router plan(network:%s). Cause: %+v", msg.Cluster.ID, msg.OrigNetwork.UUID, err)
		return err
	}

	return nil
}

// HandleSubnetDeletedEvent 서브넷 삭제 이벤트에 대한 재해 복구 서브넷 계획 동기화 함수
func (c *clusterEventController) HandleSubnetDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterSubnet
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleSubnetDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	// 보호대상 클러스터의 서브넷이 삭제됐을 때
	err = protectionGroup.UpdateNetworkRecoveryPlansByProtectionClusterID(ctx, msg.Cluster.ID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		break

	case err != nil:
		logger.Errorf("[HandleSubnetDeletedEvent] Could not update the cluster(%d) network plan(subnet:%s). Cause: %+v", msg.Cluster.ID, msg.OrigSubnet.UUID, err)
		return err
	}

	// 복구대상 클러스터의 서브넷이 삭제됐을 때
	err = recoveryPlan.DeleteExternalRoutingInterfacePlanRecord(msg.Cluster.ID, msg.OrigSubnet.ID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleSubnetDeletedEvent] Could not delete the cluster(%d) router plan(subnet:%s). Cause: %+v", msg.Cluster.ID, msg.OrigSubnet.UUID, err)
		return err
	}

	return nil
}

// HandleInstanceVolumeAttachedEvent 인스턴스 볼륨 attached 에 대한 재해 복구 계획 동기화 함수
func (c *clusterEventController) HandleInstanceVolumeAttachedEvent(p broker.Event) error {
	var msg queue.AttachClusterInstanceVolume
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceVolumeAttachedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = protectionGroup.CreateVolumeRecoveryPlans(ctx, msg.Cluster.ID, msg.InstanceVolume.ClusterInstanceID, msg.InstanceVolume.ClusterVolumeID); err != nil {
		logger.Errorf("[HandleInstanceVolumeAttachedEvent] Could not create the cluster(%d) instance(%d) volume(%s) plan. Cause: %+v",
			msg.Cluster.ID, msg.InstanceVolume.ClusterInstanceID, msg.Volume.UUID, err)
		return err
	}

	return nil
}

// HandleInstanceVolumeDetachedEvent 인스턴스 볼륨 detached 에 대한 재해 복구 계획 동기화 함수
func (c *clusterEventController) HandleInstanceVolumeDetachedEvent(p broker.Event) error {
	var msg queue.DetachClusterInstanceVolume
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceVolumeDetachedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = protectionGroup.DeleteVolumeRecoveryPlans(ctx, msg.Cluster.ID, msg.OrigVolume.ClusterStorageID, msg.OrigInstanceVolume); err != nil {
		logger.Errorf("[HandleInstanceVolumeDetachedEvent] Could not delete the cluster(%d) instance(%d) volume(%d) plan. Cause: %+v",
			msg.Cluster.ID, msg.OrigInstanceVolume.ClusterInstanceID, msg.OrigInstanceVolume.ClusterVolumeID, err)
		return err
	}

	return nil
}

// HandleInstanceCreatedEvent 인스턴스 생성 이벤트에 대한 재해 복구 인스턴스 계획 동기화 함수
func (c *clusterEventController) HandleInstanceCreatedEvent(p broker.Event) (err error) {
	var msg queue.CreateClusterInstance
	err = json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceCreatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.ReassignHypervisor(ctx, msg.Cluster.ID, msg.Instance.ClusterAvailabilityZoneID); err != nil {
		logger.Errorf("[HandleInstanceCreatedEvent] Could not update the cluster(%d) instance(%d) plan. Cause: %+v", msg.Cluster.ID, msg.Instance.UUID, err)
		return err
	}

	return nil
}

// HandleInstanceUpdatedEvent 인스턴스 수정 이벤트에 대한 재해 복구 인스턴스 계획 동기화 함수
func (c *clusterEventController) HandleInstanceUpdatedEvent(p broker.Event) (err error) {
	var msg queue.UpdateClusterInstance
	err = json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceUpdatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	// instance spec 이 변경되었을 때만 동기화한다.
	if msg.OrigInstance.ClusterInstanceSpecID == msg.Instance.ClusterInstanceSpecID {
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	// 복구 대상 클러스터의 인스턴스가 수정 됐을 때
	if err = recoveryPlan.ReassignHypervisor(ctx, msg.Cluster.ID, msg.Instance.ClusterAvailabilityZoneID); err != nil {
		logger.Errorf("[HandleInstanceUpdatedEvent] Could not update the cluster(%d) hypervisor plan(instance:%s). Cause: %+v", msg.Cluster.ID, msg.Instance.UUID, err)
		return err
	}

	// 보호 대상 클러스터 인스턴스가 수정 됐을 때
	if err = protectionGroup.ReassignHypervisorByProtectionInstance(ctx, msg.Cluster.ID, msg.Instance.ClusterAvailabilityZoneID, msg.Instance.ID); err != nil {
		logger.Errorf("[HandleInstanceUpdatedEvent] Could not update the cluster(%d) hypervisor plan(instance:%s). Cause: %+v", msg.Cluster.ID, msg.Instance.UUID, err)
		return err
	}

	return nil
}

// HandleInstanceDeletedEvent 인스턴스 삭제 이벤트에 대한 재해 복구 인스턴스 계획 동기화 함수
func (c *clusterEventController) HandleInstanceDeletedEvent(p broker.Event) (err error) {
	var msg queue.DeleteClusterInstance
	err = json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleInstanceDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	// TODO: hypervisor 자동할당 사용시 주석 제거
	// 복구 대상 클러스터의 인스턴스가 삭제 됐을 때
	//if err = recoveryPlan.ReassignHypervisor(msg.Cluster.ID, msg.OrigInstance.ClusterAvailabilityZoneID); err != nil {
	//	return err
	//}

	// 보호 대상 클러스터의 인스턴스가 삭제됐을 때
	if err = protectionGroup.DeleteProtectionGroupInstance(ctx, msg.Cluster.ID, msg.OrigInstance.ID); err != nil {
		logger.Errorf("[HandleInstanceDeletedEvent] Could not delete the plans related to the cluster(%d) instance(%d). Cause: %+v", msg.Cluster.ID, msg.OrigInstance.ID, err)
		return err
	}

	return nil
}

// HandleFloatingIPCreatedEvent Floating IP 생성 이벤트에 대한 재해 복구 Floating IP 계획 동기화 함수
func (c *clusterEventController) HandleFloatingIPCreatedEvent(p broker.Event) error {
	var msg queue.CreateClusterFloatingIP
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleFloatingIPCreatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.SetFloatingIPRecoveryPlanUnavailableFlag(ctx, msg.Cluster.ID, msg.FloatingIP.IPAddress); err != nil {
		logger.Errorf("[HandleFloatingIPCreatedEvent] Could not update the cluster(%d) floating IP(%s) plan. Cause: %+v", msg.Cluster.ID, msg.FloatingIP.UUID, err)
		return err
	}

	return nil
}

// HandleFloatingIPUpdatedEvent Floating IP 수정 이벤트에 대한 재해 복구 Floating IP 계획 동기화 함수
func (c *clusterEventController) HandleFloatingIPUpdatedEvent(p broker.Event) error {
	var msg queue.UpdateClusterFloatingIP
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleFloatingIPUpdatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	// floating ip 의 ip address 가 수정되었을 때만 동기화한다.
	if msg.OrigFloatingIP.IPAddress == msg.FloatingIP.IPAddress {
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.UnsetFloatingIPRecoveryPlanUnavailableFlag(ctx, msg.Cluster.ID, msg.OrigFloatingIP.IPAddress); err != nil {
		logger.Errorf("[HandleFloatingIPUpdatedEvent] Could not update the cluster(%d) floating IP(%s) plan. Cause: %+v", msg.Cluster.ID, msg.FloatingIP.UUID, err)
		return err
	}

	if err = recoveryPlan.SetFloatingIPRecoveryPlanUnavailableFlag(ctx, msg.Cluster.ID, msg.FloatingIP.IPAddress); err != nil {
		logger.Errorf("[HandleFloatingIPUpdatedEvent] Could not update the cluster(%d) floating IP(%s) plan. Cause: %+v", msg.Cluster.ID, msg.FloatingIP.UUID, err)
		return err
	}

	return nil
}

// HandleFloatingIPDeletedEvent Floating IP 삭제 이벤트에 대한 재해 복구 Floating IP 계획 동기화 함수
func (c *clusterEventController) HandleFloatingIPDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterFloatingIP
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleFloatingIPDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = recoveryPlan.UnsetFloatingIPRecoveryPlanUnavailableFlag(ctx, msg.Cluster.ID, msg.OrigFloatingIP.IPAddress); err != nil {
		logger.Errorf("[HandleFloatingIPDeletedEvent] Could not update the cluster(%d) floating IP(%s) plan. Cause: %+v", msg.Cluster.ID, msg.OrigFloatingIP.UUID, err)
		return err
	}

	return nil
}

// HandleRouterCreatedEvent 라우터 생성 이벤트에 대한 재해 복구 외부네트워크 계획 동기화 함수
func (c *clusterEventController) HandleRouterCreatedEvent(p broker.Event) error {
	var msg queue.CreateClusterRouter
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleRouterCreatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByProtectionClusterID(ctx, msg.Cluster.ID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleRouterCreatedEvent] Could not update the cluster(%d) network plan(router:%s). Cause: %+v", msg.Cluster.ID, msg.Router.UUID, err)
		return err
	}

	return nil
}

// HandleRouterDeletedEvent 라우터 삭제 이벤트에 대한 재해 복구 라우터, 라우팅 인터페이스, 외부 네트워크 계획 동기화 함수
func (c *clusterEventController) HandleRouterDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterRouter
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleRouterDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByProtectionClusterID(ctx, msg.Cluster.ID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleRouterDeletedEvent] Could not update the cluster(%d) network plan(router:%s). Cause: %+v", msg.Cluster.ID, msg.OrigRouter.Name, err)
		return err
	}

	return nil
}

// HandleRoutingInterfaceCreatedEvent 라우팅 인터페이스 생성 이벤트에 대한 재해 복구 외부 네트워크 계획 동기화 함수
func (c *clusterEventController) HandleRoutingInterfaceCreatedEvent(p broker.Event) error {
	var msg queue.CreateClusterRoutingInterface
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleRoutingInterfaceCreatedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByProtectionClusterID(ctx, msg.Cluster.ID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleRoutingInterfaceCreatedEvent] Could not update the cluster(%d) network plan. Cause: %+v", msg.Cluster.ID, err)
		return err
	}

	return nil
}

// HandleRoutingInterfaceDeletedEvent 라우팅 인터페이스 삭제 이벤트에 대한 재해 복구 외부 네트워크 계획 동기화 함수
func (c *clusterEventController) HandleRoutingInterfaceDeletedEvent(p broker.Event) error {
	var msg queue.DeleteClusterRoutingInterface
	err := json.Unmarshal(p.Message().Body, &msg)
	if err != nil {
		logger.Errorf("[HandleRoutingInterfaceDeletedEvent] Error occurred during unmarshal the message. Cause: %+v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	err = protectionGroup.UpdateNetworkRecoveryPlansByProtectionClusterID(ctx, msg.Cluster.ID)
	switch {
	case errors.Equal(err, internal.ErrNotFoundExternalRoutingInterface):
		return nil

	case err != nil:
		logger.Errorf("[HandleRoutingInterfaceDeletedEvent] Could not update the cluster(%d) network plan. Cause: %+v", msg.Cluster.ID, err)
		return err
	}

	return nil
}

// HandleSyncPlanListByProtectionGroupID protection group id 로 plan 전체 sync
func (c *clusterEventController) HandleSyncPlanListByProtectionGroupID(p broker.Event) error {
	logger.Infof("[HandleSyncPlanListByProtectionGroupID] Start")

	var err error
	var pgID uint64

	if err = json.Unmarshal(p.Message().Body, &pgID); err != nil {
		logger.Warnf("[HandleSyncPlanListByProtectionGroupID] Could not parse broker message. Cause: %v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = protectionGroup.SyncRecoveryPlanListByProtectionGroupID(ctx, pgID); errors.Equal(err, internal.ErrNotFoundProtectionGroup) {
		logger.Warnf("[HandleSyncPlanListByProtectionGroupID] Protection group(%d) is not existed.", pgID)
		return nil
	} else if err != nil {
		logger.Errorf("[HandleSyncPlanListByProtectionGroupID] Could not sync the plan list by protection group id(%d). Cause: %+v", pgID, err)
		return err
	}

	logger.Infof("[HandleSyncPlanListByProtectionGroupID] Success")
	return nil
}

// HandleSyncAllPlansByPlanID recovery plan id 로 plan 전체 sync
func (c *clusterEventController) HandleSyncAllPlansByPlanID(p broker.Event) error {
	logger.Infof("[HandleSyncAllPlansByPlanID] Start")

	var err error
	var msg internal.SyncPlanMessage
	if err = json.Unmarshal(p.Message().Body, &msg); err != nil {
		logger.Warnf("[HandleSyncAllPlansByPlanID] Could not parse broker message. Cause: %v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return err
	}); err != nil {
		return err
	}

	if err = protectionGroup.SyncRecoveryPlansByRecoveryPlanID(ctx, msg.ProtectionGroupID, msg.RecoveryPlanID); err != nil {
		//if errors.Equal(err, internal.ErrNotFoundProtectionGroup) || errors.Equal(err, internal.ErrNotFoundRecoveryPlan) {
		//	logger.Warnf("[HandleSyncAllPlansByPlanID] Not existed: pg(%d) plan(%d). Cause: %v", msg.ProtectionGroupID, msg.RecoveryPlanID, err)
		//	return nil
		//}

		// 우선 동기화 중 error 발생시 requeue 하지 않는다.

		logger.Errorf("[HandleSyncAllPlansByPlanID] Could not sync the plans by recovery plan id(%d). Cause: %+v", msg.RecoveryPlanID, err)
		return nil
		//return err
	}

	logger.Infof("[HandleSyncAllPlansByPlanID] Success")
	return nil
}
