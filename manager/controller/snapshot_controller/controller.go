package snapshotcontroller

import (
	"context"
	"encoding/json"
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/manager/controller"
	protectionGroup "github.com/datacommand2/cdm-disaster-recovery/manager/internal/protection_group"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal/snapshot"
	"github.com/jinzhu/gorm"
	"strconv"
)

var (
	defaultController snapshotController // singleton
	subscriberList    []broker.Subscriber
	closeCh           chan bool
)

func init() {
	controller.RegisterController(&defaultController)
}

type snapshotController struct {
}

func (c *snapshotController) Run() {
	if len(subscriberList) != 0 {
		logger.Warn("[SnapshotController-Run] Already protection group snapshot controller started")
		return
	}

	logger.Info("[SnapshotController-Run] Starting protection group snapshot controller.")

	for topic, handlerFunc := range map[string]broker.Handler{
		constant.QueueAddProtectionGroupSnapshot:    c.HandleAddProtectionGroupSnapshot,
		constant.QueueDeleteProtectionGroupSnapshot: c.HandleDeleteProtectionGroupSnapshot,
		constant.QueueDeletePlanSnapshot:            c.HandleDeletePlanSnapshot,
	} {
		logger.Debugf("Subscribe protection group snapshot event (%s)", topic)

		sub, err := broker.SubscribePersistentQueue(topic, handlerFunc, true)

		if err != nil {
			logger.Fatalf("Could not start protection group snapshot event controller. Cause: %+v", errors.UnusableBroker(err))
		}

		subscriberList = append(subscriberList, sub)
	}
}

func (c *snapshotController) Close() {
	if len(subscriberList) == 0 {
		logger.Warn("[SnapshotController-Close] Protection group snapshot controller is not started")
		return
	}

	logger.Info("[SnapshotController-Close] Stopping protection group snapshot controller")

	for _, sub := range subscriberList {
		logger.Debugf("Unsubscribe protection group snapshot event (%s)", sub.Topic())

		if err := sub.Unsubscribe(); err != nil {
			logger.Errorf("[SnapshotController-Close] Could not unsubscribe queue (%s). Cause: %+v", sub.Topic(), err)
		}
	}

	subscriberList = []broker.Subscriber{}
	closeCh <- true
}

func getRuntimeFromEvent(e broker.Event) (int64, error) {
	runtimeStr, ok := e.Message().Header["runtime"]
	if !ok {
		return 0, errors.New("not found runtime")
	}

	runtime, err := strconv.ParseInt(runtimeStr, 10, 64)
	if err != nil {
		logger.Warnf("[getRuntimeFromEvent] Could not parse schedule runtime. Cause: %v", err)
		return 0, err
	}

	return runtime, nil
}

// HandleAddProtectionGroupSnapshot 는 스케줄러에 의해 동작하는 스냅샷을 생성하는 함수이다.
// 브로커 이벤트를 매개변수로 받으며, 에러 시 큐에 작업이 다시 들어가는것을 막기위해 에러는 이벤트 알림으로 띄우고
// 리턴은 nil 로 한다.
func (c *snapshotController) HandleAddProtectionGroupSnapshot(e broker.Event) error {
	logger.Infof("[HandleAddProtectionGroupSnapshot] Start to add protection group snapshot")
	var pgID uint64
	var err error
	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		if ctx, err = helper.DefaultContext(db); err != nil {
			return err
		}
		return nil
	}); err != nil {
		logger.Warnf("[HandleAddProtectionGroupSnapshot] Could not add protection group(%d) snapshot. Cause: %v", pgID, err)
		return nil
	}

	if err = json.Unmarshal(e.Message().Body, &pgID); err != nil {
		logger.Warnf("[HandleAddProtectionGroupSnapshot] Could not parse broker message. Cause: %v", err)
		return nil
	}

	runtime, err := getRuntimeFromEvent(e)
	if err != nil {
		logger.Warnf("[HandleAddProtectionGroupSnapshot] Could not get runtime from event. Cause: %+v", err)
		return nil
	}

	logger.Infof("[HandleAddProtectionGroupSnapshot] Get runtime for try to add protection group(%d) snapshots. runtime : %v", pgID, runtime)
	go func() {
		if err = protectionGroup.AddSnapshot(ctx, pgID, runtime); err != nil {
			reportEvent(ctx, "cdm-dr.manager.add_protection_group_snapshot.failure-add", "add_snapshot_failed", err)
			logger.Warnf("[HandleAddProtectionGroupSnapshot] Could not add protection group(%d) snapshot. Cause: %+v", pgID, err)
			return
		}
		logger.Infof("[HandleAddProtectionGroupSnapshot] Success to add the snapshots of the protection group(%d).", pgID)
	}()

	return nil
}

func (c *snapshotController) HandleDeleteProtectionGroupSnapshot(e broker.Event) error {
	var (
		err  error
		pgID uint64
	)
	if err = json.Unmarshal(e.Message().Body, &pgID); err != nil {
		logger.Warnf("[HandleDeleteProtectionGroupSnapshot] Could not parse broker message. Cause: %v", err)
		return nil
	}

	var ctx context.Context
	if err = database.Execute(func(db *gorm.DB) error {
		if ctx, err = helper.DefaultContext(db); err != nil {
			return err
		}
		return nil
	}); err != nil {
		logger.Warnf("[HandleDeletePlanSnapshot] Could not delete protection group(%d) snapshot. Cause: %v", pgID, err)
		return nil
	}

	logger.Debugf("[HandleDeleteProtectionGroupSnapshot] Start to delete the protection group(%d) snapshots.", pgID)

	go func() {
		if err = snapshot.DeleteAllSnapshots(ctx, pgID); err != nil {
			logger.Warnf("[HandleDeleteProtectionGroupSnapshot] Could not delete protection group(%d) snapshot. Cause: %+v", pgID, err)
			return
		}

		logger.Infof("[HandleDeleteProtectionGroupSnapshot] Success to delete the snapshots of the protection group(%d).", pgID)
	}()

	logger.Debugf("[HandleDeleteProtectionGroupSnapshot] End to delete protection group(%d) snapshots.", pgID)

	return nil
}

func (c *snapshotController) HandleDeletePlanSnapshot(e broker.Event) error {
	return nil
}
