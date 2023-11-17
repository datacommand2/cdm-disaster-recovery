package controller

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/sync"
	"github.com/datacommand2/cdm-disaster-recovery/common/constant"
)

// Controller 컨트롤러 인터페이스
type Controller interface {
	Run()
	Close()
}

var controllerList []Controller

// RegisterController 관리할 컨트롤러를 추가한다.
func RegisterController(c Controller) {
	controllerList = append(controllerList, c)
}

// Init initialize
func Init() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// leader election
		logger.Infof("[Controller] Campaign service (%s) leader", constant.ServiceManagerName)

		l, err := sync.CampaignLeader(ctx, constant.ServiceManagerName)
		if err != nil {
			logger.Fatalf("Could not campaign leader. Cause: %+v", err)
		}

		logger.Infof("[Controller] Elected service (%s) leader", constant.ServiceManagerName)

		// resign and close leader
		defer func() {
			if err := l.Resign(context.Background()); err != nil {
				logger.Warnf("[Controller] Could not resign leader. Cause: %v", err)
			}

			if err := l.Close(); err != nil {
				logger.Warnf("[Controller] Could not close leader. Cause: %v", err)
			}
		}()

		// run controllers
		for _, c := range controllerList {
			c.Run()
		}

		// wait until context is done
		<-ctx.Done()

		// close controllers
		for _, c := range controllerList {
			c.Close()
		}
	}()

	return cancel
}
