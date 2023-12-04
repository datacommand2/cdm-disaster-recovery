package main

import (
	"github.com/datacommand2/cdm-cloud/common"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/database/model"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/event"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/manager/controller"
	_ "github.com/datacommand2/cdm-disaster-recovery/manager/controller/check_recovery_job_controller"
	_ "github.com/datacommand2/cdm-disaster-recovery/manager/controller/cluster_event_controller"
	_ "github.com/datacommand2/cdm-disaster-recovery/manager/controller/mirror_volume_controller"
	"github.com/datacommand2/cdm-disaster-recovery/manager/handler"
	_ "github.com/datacommand2/cdm-disaster-recovery/manager/internal/recovery_plan/assignee/descendingResource"
	"github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
	"github.com/micro/go-micro/v2"
	"github.com/micro/go-micro/v2/client"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/server"
)

// version 은 서비스 버전의 정보이다.
var version string

func getDefaultTenantID() (uint64, error) {
	var tenant model.Tenant
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Find(&tenant, model.Tenant{Name: "default"}).Error

	}); err == gorm.ErrRecordNotFound {
		return 0, errors.Unknown(err)

	} else if err != nil {
		return 0, errors.UnusableDatabase(err)
	}

	return tenant.ID, nil
}

func reportEvent(eventCode, errorCode string, eventContents interface{}) {
	tid, err := getDefaultTenantID()
	if err != nil {
		logger.Warnf("[main-reportEvent] Could not get the default tenant ID. Cause: %+v", err)
		return
	}

	err = event.ReportEvent(tid, eventCode, errorCode, event.WithContents(eventContents))
	if err != nil {
		logger.Warnf("[main-reportEvent] Could not report the event(%s:%s). Cause: %+v", eventCode, errorCode, errors.Unknown(err))
	}
}

func main() {
	metadata := map[string]string{
		"CDM_SOLUTION_NAME":       constant.SolutionName,
		"CDM_SERVICE_DESCRIPTION": constant.ServiceManagerDescription,
	}

	service := micro.NewService(
		micro.Name(constant.ServiceManagerName),
		micro.Version(version),
		micro.Metadata(metadata),
	)

	logger.Infof("Creating service(%s)", constant.ServiceManagerName)
	service.Init()

	// Registry 의 Kubernetes 기능을 go-micro server 옵션에 추가
	if err := service.Server().Init(server.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init server options by service(%s). cause: %v", constant.ServiceManagerName, err)
	}

	if err := service.Client().Init(client.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init client options by service(%s). cause: %v", constant.ServiceManagerName, err)
	}

	if err := selector.DefaultSelector.Init(selector.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init selector options by service(%s). cause: %v", constant.ServiceManagerName, err)
	}

	defer common.Destroy()

	h, err := handler.NewDisasterRecoveryManagerHandler()
	if err != nil {
		h.Close()
		reportEvent("cdm-dr.manager.main.failure-create_handler", "unusable_broker", err)
		logger.Fatalf("Could not create handler. Cause: %+v", err)
	}

	defer h.Close()

	err = drms.RegisterDisasterRecoveryManagerHandler(service.Server(), h)
	if err != nil {
		err = errors.Unknown(err)
		reportEvent("cdm-dr.manager.main.failure-register_handler", "unknown", err)
		logger.Fatalf("Could not register handler. Cause: %+v", err)
	}

	cancelFunc := controller.Init()
	defer cancelFunc()

	logger.Infof("Running service(%s)", constant.ServiceManagerName)
	if err := service.Run(); err != nil {
		err = errors.Unknown(err)
		reportEvent("cdm-dr.manager.main.failure-run_service", "unknown", err)
		logger.Fatalf("Could not run service(%s). Cause: %+v", constant.ServiceManagerName, err)
	}
}
