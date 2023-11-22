package main

import (
	"10.1.1.220/cdm/cdm-cloud/common"
	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/constant"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/migrator/migrator"

	"github.com/micro/go-micro/v2"
	"github.com/micro/go-micro/v2/client"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/server"

	"os"
	"os/signal"
	"syscall"
)

// version 은 서비스 버전의 정보이다.
var version string

func main() {
	logger.Infof("Initializing daemon(%s:%s)", constant.DaemonMigratorName, version)
	//if err := cmd.Init(); err != nil {
	//	logger.Fatalf("Could not init daemon(%s). Cause: %+v", constant.DaemonMigratorName, err)
	//	return
	//}
	//
	//logger.Infof("Initializing daemon(%s:%s)", constant.DaemonMigratorName, version)

	logger.Infof("Creating service(%s)", constant.DaemonMigratorName)
	s := micro.NewService(
		micro.Name(constant.DaemonMigratorName),
		micro.Version(version),
		micro.Metadata(map[string]string{
			"CDM_SOLUTION_NAME":       constant.SolutionName,
			"CDM_SERVICE_DESCRIPTION": constant.DaemonMigratorDescription,
		}),
	)
	s.Init()

	// Registry 의 Kubernetes 기능을 go-micro server 옵션에 추가
	if err := s.Server().Init(server.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init server options by service(%s). cause: %v", constant.DaemonMigratorName, err)
	}

	if err := s.Client().Init(client.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init client options by service(%s). cause: %v", constant.DaemonMigratorName, err)
	}

	if err := selector.DefaultSelector.Init(selector.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init selector options by service(%s). cause: %v", constant.DaemonMigratorName, err)
	}

	defer common.Destroy()

	logger.Infof("Creating daemon(%s:%s)", constant.DaemonMigratorName, version)
	m, err := migrator.NewMigratorMonitor()
	if err != nil {
		logger.Fatalf("Cloud not init new migrator by service(%s). cause: %v", constant.DaemonMigratorName, err)
	}

	logger.Infof("Running daemon(%s)", constant.DaemonMigratorName)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, []os.Signal{
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL,
	}...)

	<-ch
	logger.Infof("Stopping daemon(%s)", constant.DaemonMigratorName)
	m.Stop()
}
