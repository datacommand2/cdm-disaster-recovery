package recoveryplan

import (
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"github.com/jinzhu/gorm"
)

type recoveryPlanFilter interface {
	Apply(*gorm.DB) (*gorm.DB, error)
}

type protectionGroupIDFilter struct {
	ID uint64
}

func (f *protectionGroupIDFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("protection_group_id = ?", f.ID), nil
}

type recoveryPlanNameFilter struct {
	Name string
}

func (f *recoveryPlanNameFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("name LIKE ?", "%"+f.Name+"%"), nil
}

type paginationFilter struct {
	Offset uint64
	Limit  uint64
}

func (f *paginationFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Offset(f.Offset).Limit(f.Limit), nil
}

func makeRecoveryPlanFilters(req *drms.RecoveryPlanListRequest) []recoveryPlanFilter {
	var filters = []recoveryPlanFilter{
		&protectionGroupIDFilter{ID: req.GroupId},
	}

	if req.Name != "" {
		filters = append(filters, &recoveryPlanNameFilter{Name: req.Name})
	}

	if req.GetOffset() != nil && (req.GetLimit() != nil && req.GetLimit().GetValue() != 0) {
		filters = append(filters, &paginationFilter{Offset: req.GetOffset().GetValue(), Limit: req.GetLimit().GetValue()})
	}

	return filters
}
