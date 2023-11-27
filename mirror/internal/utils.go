package internal

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/event"
	"github.com/datacommand2/cdm-cloud/common/logger"
	cloudMeta "github.com/datacommand2/cdm-cloud/common/metadata"
	"github.com/datacommand2/cdm-cloud/common/test/helper"
	"github.com/jinzhu/gorm"
)

// GetDefaultContext default context 를 가져온다.
func GetDefaultContext() (context.Context, error) {
	var err error
	var ctx context.Context
	if err = database.Transaction(func(db *gorm.DB) error {
		ctx, err = helper.DefaultContext(db)
		return nil
	}); err != nil {
		return nil, errors.Unknown(err)
	}

	return ctx, nil
}

// ReportEvent cdm-cloud-event service 로 event 전송 함수
func ReportEvent(eventCode, errorCode string, eventContents interface{}) {
	ctx, err := GetDefaultContext()
	if err != nil {
		logger.Warnf("Could not report event. cause: %+v", err)
		return
	}

	var tid uint64
	if tid, err = cloudMeta.GetTenantID(ctx); err != nil {
		logger.Warnf("Could not report event. cause: %+v", errors.Unknown(err))
		return
	}

	err = event.ReportEvent(tid, eventCode, errorCode, event.WithContents(eventContents))
	if err != nil {
		logger.Warnf("Could not report event. cause: %+v", errors.Unknown(err))
	}
}

// GetVolumeName 볼륨의 uuid 에 prefix 추가 함수
func GetVolumeName(uuid string) string {
	return "volume-" + uuid
}

// GetDifferenceList  A - B
func GetDifferenceList(diffA, diffB []string) (diff []string) {
	m := make(map[string]bool)

	for _, item := range diffB {
		m[item] = true
	}

	for _, item := range diffA {
		if _, ok := m[item]; !ok {
			diff = append(diff, item)
		}
	}

	return diff
}
