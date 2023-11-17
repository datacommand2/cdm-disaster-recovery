package protectiongroup

import (
	cms "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"github.com/jinzhu/gorm"
)

type protectionGroupFilter interface {
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

type protectionClusterIDFilter struct {
	ID uint64
}

func (f *protectionClusterIDFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("protection_cluster_id = ?", f.ID), nil
}

type protectionGroupNameFilter struct {
	Name string
}

func (f *protectionGroupNameFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("name LIKE ?", "%"+f.Name+"%"), nil
}

type paginationFilter struct {
	Offset uint64
	Limit  uint64
}

func (f *paginationFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Offset(f.Offset).Limit(f.Limit), nil
}

type tenantFilter struct {
	TenantID uint64
}

func (f *tenantFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("tenant_id = ?", f.TenantID), nil
}

func makeProtectionGroupFilter(req *drms.ProtectionGroupListRequest, tid uint64, clusterMap map[uint64]*cms.Cluster) []protectionGroupFilter {
	var filters []protectionGroupFilter

	filters = append(filters, &tenantFilter{TenantID: tid})
	if len(clusterMap) > 0 {
		filters = append(filters, &clusterMapFilter{Map: clusterMap})
	}

	if req.ProtectionClusterId != 0 {
		filters = append(filters, &protectionClusterIDFilter{ID: req.ProtectionClusterId})
	}

	if len(req.Name) != 0 {
		filters = append(filters, &protectionGroupNameFilter{Name: req.Name})
	}

	if req.GetOffset() != nil && (req.GetLimit() != nil && req.GetLimit().GetValue() != 0) {
		filters = append(filters, &paginationFilter{Offset: req.GetOffset().GetValue(), Limit: req.GetLimit().GetValue()})
	}

	return filters
}
