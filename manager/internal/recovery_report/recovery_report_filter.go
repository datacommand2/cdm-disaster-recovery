package recoveryreport

import (
	cms "github.com/datacommand2/cdm-center/cluster-manager/proto"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
)

type recoveryReportFilter interface {
	Apply(*gorm.DB) (*gorm.DB, error)
}

type clusterMapFilter struct {
	Map map[uint64]*cms.Cluster
}

func (f *clusterMapFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	var list []uint64
	for k := range f.Map {
		list = append(list, k)
	}

	return db.Where("protection_cluster_id in (?)", list), nil
}

type betweenFinishedAtFilter struct {
	From int64
	To   int64
}

func (b *betweenFinishedAtFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	if b.From != 0 {
		db = db.Where("started_at >= ?", b.From)
	}
	if b.To != 0 {
		db = db.Where("finished_at <= ?", b.To)
	}
	return db, nil
}

type protectionGroupIDFilter struct {
	ID uint64
}

func (f *protectionGroupIDFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("protection_group_id = ?", f.ID), nil
}

type protectionGroupNameFilter struct {
	Name string
}

func (f *protectionGroupNameFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("protection_group_name = ?", f.Name), nil
}

type recoveryTypeFilter struct {
	Type string
}

func (f *recoveryTypeFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("recovery_type_code = ?", f.Type), nil
}

type recoveryResultFilter struct {
	Result string
}

func (f *recoveryResultFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("result_code = ?", f.Result), nil
}

type paginationFilter struct {
	Offset uint64
	Limit  uint64
}

func (f *paginationFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Offset(f.Offset).Limit(f.Limit), nil
}

func makeRecoveryReportFilters(req *drms.RecoveryReportListRequest) []recoveryReportFilter {
	var filters = []recoveryReportFilter{
		&protectionGroupIDFilter{ID: req.GroupId},
		&protectionGroupNameFilter{Name: req.GroupName},
	}

	if len(req.Type) > 0 {
		filters = append(filters, &recoveryTypeFilter{Type: req.Type})
	}

	if len(req.Result) > 0 {
		filters = append(filters, &recoveryResultFilter{Result: req.Result})
	}

	if req.GetOffset() != nil && (req.GetLimit() != nil && req.GetLimit().GetValue() != 0) {
		filters = append(filters, &paginationFilter{Offset: req.GetOffset().GetValue(), Limit: req.GetLimit().GetValue()})
	}

	return filters
}

func makeJobSummaryRequestFilters(req *drms.JobSummaryRequest, clusterMap map[uint64]*cms.Cluster) []recoveryReportFilter {
	return []recoveryReportFilter{
		&recoveryTypeFilter{Type: req.RecoveryType},
		&clusterMapFilter{Map: clusterMap},
		&betweenFinishedAtFilter{From: req.GetStartDate().GetValue(), To: req.GetEndDate().GetValue()},
	}
}
