package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/datacommand2/cdm-cloud/common"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/environment"
	"github.com/datacommand2/cdm-disaster-recovery/mirror/volume"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/config/cmd"
	"github.com/micro/go-micro/v2/registry"

	_ "github.com/datacommand2/cdm-disaster-recovery/mirror/environment/ceph" // register ceph environment
	_ "github.com/datacommand2/cdm-disaster-recovery/mirror/volume/ceph"      // register ceph volume

	_ "net/http/pprof"
)

// version 은 서비스 버전의 정보이다.
var version string

func main() {

	go func() {
		http.ListenAndServe("127.0.0.1:6060", nil)
	}()

	var err error
	if err = cmd.Init(); err != nil {
		logger.Fatalf("Could not init daemon(%s). cause: %v", constant.DaemonMirrorName, err)
		return
	}

	if err := selector.DefaultSelector.Init(selector.Registry(registry.DefaultRegistry)); err != nil {
		logger.Fatalf("Cloud not init selector options by service(%s). cause: %v", constant.DaemonMirrorName, err)
	}

	defer common.Destroy()

	envMon := environment.NewMirrorEnvironmentMonitor()
	volMon, err := volume.NewMirrorVolumeMonitor()
	if err != nil {
		logger.Errorf("Could not create daemon(%s), Cause :%v", constant.DaemonMirrorName, err)
		return
	}
	logger.Infof("Creating daemon(%s:%s)", constant.DaemonMirrorName, version)
	logger.Infof("Running daemon(%s)", constant.DaemonMirrorName)

	envMon.Run()
	volMon.Run()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, []os.Signal{
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL,
	}...)

	<-ch
	logger.Infof("Stopping daemon(%s)", constant.DaemonMirrorName)
	envMon.Stop()
	volMon.Stop()
}
