package recoveryjob

import (
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
)

type recoveryJobFilter interface {
	Apply(*gorm.DB) (*gorm.DB, error)
}

// protectionGroupIDFilter 보호그룹 ID 필터
type protectionGroupIDFilter struct {
	ID uint64
}

func (f *protectionGroupIDFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("cdm_disaster_recovery_plan.protection_group_id = ?", f.ID), nil
}

type recoveryPlanNameFilter struct {
	Name string
}

func (f *recoveryPlanNameFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("cdm_disaster_recovery_plan.name LIKE ?", "%"+f.Name+"%"), nil
}

type recoveryPlanIDFilter struct {
	ID uint64
}

func (f *recoveryPlanIDFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("cdm_disaster_recovery_plan.id = ?", f.ID), nil
}

type recoveryJobTypeFilter struct {
	Type string
}

func (f *recoveryJobTypeFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("cdm_disaster_recovery_job.type_code = ?", f.Type), nil
}

type paginationFilter struct {
	Offset uint64
	Limit  uint64
}

func (f *paginationFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Offset(f.Offset).Limit(f.Limit), nil
}

func makeRecoveryJobFilter(req *drms.RecoveryJobListRequest) []recoveryJobFilter {
	var filters []recoveryJobFilter

	filters = append(filters, &protectionGroupIDFilter{ID: req.GroupId})

	if len(req.Name) != 0 {
		filters = append(filters, &recoveryPlanNameFilter{Name: req.Name})
	}

	if req.PlanId != 0 {
		filters = append(filters, &recoveryPlanIDFilter{ID: req.PlanId})
	}

	if len(req.Type) != 0 {
		filters = append(filters, &recoveryJobTypeFilter{Type: req.Type})
	}

	if req.GetOffset() != nil && (req.GetLimit() != nil && req.GetLimit().GetValue() != 0) {
		filters = append(filters, &paginationFilter{Offset: req.GetOffset().GetValue(), Limit: req.GetLimit().GetValue()})
	}

	return filters
}
