package instancetemplate

import (
	"context"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	identity "github.com/datacommand2/cdm-cloud/services/identity/proto"
	"github.com/datacommand2/cdm-disaster-recovery/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/manager/proto"
	"github.com/jinzhu/gorm"
)

type instanceTemplateFilter interface {
	Apply(*gorm.DB) (*gorm.DB, error)
}

type OwnerGroupFilter struct {
	OwnerGroupID uint64
}

func (f *OwnerGroupFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("owner_group_id = ?", f.OwnerGroupID), nil
}

type ownerGroupListFilter struct {
	OwnerGroups []*identity.Group
}

func (f *ownerGroupListFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	var idList []uint64
	for _, g := range f.OwnerGroups {
		idList = append(idList, g.Id)
	}

	return db.Where("owner_group_id in (?)", idList), nil
}

type instanceTemplateNameFilter struct {
	Name string
}

func (f *instanceTemplateNameFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Where("name LIKE ?", "%"+f.Name+"%"), nil
}

type paginationFilter struct {
	Offset uint64
	Limit  uint64
}

func (f *paginationFilter) Apply(db *gorm.DB) (*gorm.DB, error) {
	return db.Offset(f.Offset).Limit(f.Limit), nil
}

func makeInstanceTemplateFilter(ctx context.Context, req *drms.InstanceTemplateListRequest) []instanceTemplateFilter {
	var filters []instanceTemplateFilter

	u, _ := metadata.GetAuthenticatedUser(ctx)
	if req.OwnerGroupId != 0 {
		filters = append(filters, &OwnerGroupFilter{OwnerGroupID: req.OwnerGroupId})
	} else {
		if !internal.IsAdminUser(u) {
			filters = append(filters, &ownerGroupListFilter{OwnerGroups: u.Groups})
		}
	}

	if len(req.Name) != 0 {
		filters = append(filters, &instanceTemplateNameFilter{Name: req.Name})
	}

	if req.GetOffset() != nil && (req.GetLimit() != nil && req.GetLimit().GetValue() != 0) {
		filters = append(filters, &paginationFilter{Offset: req.GetOffset().GetValue(), Limit: req.GetLimit().GetValue()})
	}

	return filters
}
