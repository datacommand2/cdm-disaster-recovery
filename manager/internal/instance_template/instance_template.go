package instancetemplate

import (
	"context"
	proto1 "github.com/datacommand2/cdm-center/services/cluster-manager/proto"
	commonConstant "github.com/datacommand2/cdm-cloud/common/constant"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/errors"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/metadata"
	identity "github.com/datacommand2/cdm-cloud/services/identity/proto"
	"github.com/datacommand2/cdm-disaster-recovery/common/database/model"
	"github.com/datacommand2/cdm-disaster-recovery/services/manager/internal"
	drms "github.com/datacommand2/cdm-disaster-recovery/services/manager/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jinzhu/gorm"
	"github.com/micro/go-micro/v2/client/grpc"
)

var DefaultIdentityService = identity.NewIdentityService(commonConstant.ServiceIdentity, grpc.NewClient())

func getInstanceTemplate(id uint64) (*model.InstanceTemplate, error) {
	var m model.InstanceTemplate
	err := database.Execute(func(db *gorm.DB) error {
		return db.First(&m, &model.InstanceTemplate{ID: id}).Error
	})
	switch {
	case err == gorm.ErrRecordNotFound:
		return nil, internal.NotFoundInstanceTemplate(id)
	case err != nil:
		return nil, errors.UnusableDatabase(err)
	}
	return &m, nil
}

func getInstances(id uint64) ([]*model.InstanceTemplateInstance, error) {
	var is []*model.InstanceTemplateInstance
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Find(&is, &model.InstanceTemplateInstance{InstanceTemplateID: id}).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return is, nil
}

func getDependInstances(tid uint64, name string) ([]*model.InstanceTemplateInstanceDependency, error) {
	var dis []*model.InstanceTemplateInstanceDependency
	if err := database.Execute(func(db *gorm.DB) error {
		return db.Find(&dis, &model.InstanceTemplateInstanceDependency{InstanceTemplateID: tid, ProtectionClusterInstanceName: name}).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return dis, nil
}

func Get(ctx context.Context, req *drms.InstanceTemplateRequest) (*drms.InstanceTemplate, error) {
	var err error
	if req.GetTemplateId() == 0 {
		err = errors.RequiredParameter("template_id")
		logger.Errorf("[InstanceTemplate-Get] Errors occurred during validating the request. Cause: %v", err)
		return nil, err
	}
	var (
		it *model.InstanceTemplate
		is []*model.InstanceTemplateInstance
	)

	if it, err = getInstanceTemplate(req.TemplateId); err != nil {
		logger.Errorf("[InstanceTemplate-Get] Could not get the instance template(%v). Cause: %+v", req.TemplateId, err)
		return nil, err
	}

	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, it.OwnerGroupID) {
		return nil, errors.UnauthorizedRequest(ctx)
	}

	var template drms.InstanceTemplate
	if err = template.SetFromModel(it); err != nil {
		return nil, err
	}

	var instances []*drms.InstanceTemplateInstance
	if is, err = getInstances(it.ID); err != nil {
		return nil, err
	}

	for _, i := range is {
		var msgInstance drms.InstanceTemplateInstance
		if err = msgInstance.SetFromModel(i); err != nil {
			return nil, err
		}
		dis, err := getDependInstances(it.ID, i.ProtectionClusterInstanceName)
		if err != nil {
			return nil, err
		}
		var dependencyInstances []*drms.InstanceTemplateInstanceDependency
		for _, di := range dis {
			dependencyInstance := &drms.InstanceTemplateInstanceDependency{
				Name: di.DependProtectionClusterInstanceName,
			}

			dependencyInstances = append(dependencyInstances, dependencyInstance)
		}
		msgInstance.Dependencies = dependencyInstances
		instances = append(instances, &msgInstance)

	}
	template.OwnerGroup = &proto1.Group{Id: it.OwnerGroupID}
	template.Instances = instances
	return &template, err
}

func checkValidInstanceTemplateList(name string) error {
	if len(name) > 255 {
		return errors.LengthOverflowParameterValue("instanceTemplate.name", name, 255)
	}

	return nil
}

func getInstanceTemplateList(filters ...instanceTemplateFilter) ([]*model.InstanceTemplate, error) {
	var err error
	var m []*model.InstanceTemplate

	if err = database.Execute(func(db *gorm.DB) error {
		cond := db
		for _, f := range filters {
			if cond, err = f.Apply(cond); err != nil {
				return err
			}
		}
		return cond.Model(&m).Find(&m).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	return m, nil
}

func getInstanceTemplatePagination(filters ...instanceTemplateFilter) (*drms.Pagination, error) {
	var err error
	var offset, limit, total uint64

	if err = database.Execute(func(db *gorm.DB) error {
		conditions := db
		for _, f := range filters {
			if _, ok := f.(*paginationFilter); ok {
				offset = f.(*paginationFilter).Offset
				limit = f.(*paginationFilter).Limit
				continue
			}

			if conditions, err = f.Apply(conditions); err != nil {
				return err
			}
		}

		return conditions.Model(&model.InstanceTemplate{}).Count(&total).Error
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	if limit == 0 {
		return &drms.Pagination{
			Page:       &wrappers.UInt64Value{Value: 1},
			TotalPage:  &wrappers.UInt64Value{Value: 1},
			TotalItems: &wrappers.UInt64Value{Value: total},
		}, nil
	}

	return &drms.Pagination{
		Page:       &wrappers.UInt64Value{Value: offset/limit + 1},
		TotalPage:  &wrappers.UInt64Value{Value: (total + limit - 1) / limit},
		TotalItems: &wrappers.UInt64Value{Value: total},
	}, nil
}

func GetList(ctx context.Context, req *drms.InstanceTemplateListRequest) ([]*drms.InstanceTemplate, *drms.Pagination, error) {
	var err error
	if err = checkValidInstanceTemplateList(req.Name); err != nil {
		logger.Errorf("[InstanceTemplate-GetList] Errors occurred during validating the request. Cause: %+v", err)
		return nil, nil, err
	}

	filters := makeInstanceTemplateFilter(ctx, req)

	var its []*model.InstanceTemplate
	if its, err = getInstanceTemplateList(filters...); err != nil {
		logger.Errorf("[InstanceTemplate-GetList] Cloud not get the instance template list. Cause: %+v", err)
		return nil, nil, err
	}

	var templates []*drms.InstanceTemplate
	for _, it := range its {
		var template drms.InstanceTemplate
		if err = template.SetFromModel(it); err != nil {
			return nil, nil, err
		}

		var is []*model.InstanceTemplateInstance
		if is, err = getInstances(it.ID); err != nil {
			return nil, nil, err
		}

		var instances []*drms.InstanceTemplateInstance
		for _, i := range is {
			var msgInstance drms.InstanceTemplateInstance
			if err = msgInstance.SetFromModel(i); err != nil {
				return nil, nil, err
			}
			dis, err := getDependInstances(it.ID, i.ProtectionClusterInstanceName)
			if err != nil {
				return nil, nil, err
			}
			var dependencyInstances []*drms.InstanceTemplateInstanceDependency
			for _, di := range dis {
				var dependencyInstance drms.InstanceTemplateInstanceDependency
				if err = dependencyInstance.SetFromModel(di); err != nil {
					return nil, nil, err
				}
				dependencyInstances = append(dependencyInstances, &dependencyInstance)
			}
			msgInstance.Dependencies = dependencyInstances
			instances = append(instances, &msgInstance)

		}
		template.Instances = instances
		template.OwnerGroup = &proto1.Group{Id: it.OwnerGroupID}
		templates = append(templates, &template)
	}

	pagination, err := getInstanceTemplatePagination(filters...)
	if err != nil {
		logger.Errorf("[InstanceTemplate-GetList] Could not get the instance templates pagination. Cause: %+v", err)
		return nil, nil, err
	}
	return templates, pagination, nil
}

func checkValidInstanceTemplate(ctx context.Context, it *drms.InstanceTemplate) error {
	if it == nil {
		return errors.RequiredParameter("template")
	}

	if _, err := DefaultIdentityService.GetGroup(ctx, &identity.GetGroupRequest{GroupId: it.OwnerGroup.Id}); err != nil {
		return internal.NotFoundOwnerGroup(it.OwnerGroup.Id)
	}

	if it.OwnerGroup == nil {
		return errors.RequiredParameter("template.owner_group")
	}

	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, it.OwnerGroup.Id) {
		return errors.UnauthorizedRequest(ctx)
	}

	if len(it.Name) == 0 {
		return errors.RequiredParameter("template.name")
	}

	if len(it.Name) > 255 {
		return errors.LengthOverflowParameterValue("template.name", it.Name, 255)
	}

	if len(it.Remarks) > 300 {
		return errors.LengthOverflowParameterValue("template.remarks", it.Remarks, 300)
	}

	return nil
}

func checkValidAddInstanceTemplate(it *drms.InstanceTemplate) error {
	err := database.Execute(func(db *gorm.DB) error {
		return db.Where(&model.InstanceTemplate{Name: it.Name, OwnerGroupID: it.OwnerGroup.Id}).First(&model.InstanceTemplate{}).Error
	})
	switch {
	case err == nil:
		return errors.ConflictParameterValue("instance_template.name", it.Name)
	case err != gorm.ErrRecordNotFound:
		return errors.UnusableDatabase(err)
	}
	return nil
}

func Add(ctx context.Context, req *drms.AddInstanceTemplateRequest) (*drms.InstanceTemplate, error) {
	var err error
	if err = checkValidInstanceTemplate(ctx, req.Template); err != nil {
		logger.Errorf("[InstanceTemplate-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	if err = checkValidAddInstanceTemplate(req.Template); err != nil {
		logger.Errorf("[InstanceTemplate-Add] Errors occurred during validating the request. Cause: %+v", err)
		return nil, err
	}

	var template *model.InstanceTemplate
	if template, err = req.Template.Model(); err != nil {
		return nil, err
	}

	template.OwnerGroupID = req.Template.OwnerGroup.Id
	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = db.Create(template).Error; err != nil {
			logger.Errorf("[InstanceTemplate-Add] Could not create the instance template(%s). Cause: %+v", req.Template.Name, err)
			return err
		}

		for _, i := range req.Template.Instances {
			var instance *model.InstanceTemplateInstance
			if instance, err = i.Model(); err != nil {
				return err
			}

			instance.InstanceTemplateID = template.ID
			if err = db.Create(instance).Error; err != nil {
				logger.Errorf("[InstanceTemplate-Add] Could not create the instance template instance(%s). Cause: %+v", instance.ProtectionClusterInstanceName, err)
				return err
			}

			for _, di := range i.Dependencies {
				depInstance := &model.InstanceTemplateInstanceDependency{
					InstanceTemplateID:                  template.ID,
					ProtectionClusterInstanceName:       instance.ProtectionClusterInstanceName,
					DependProtectionClusterInstanceName: di.Name,
				}
				if err = db.Create(depInstance).Error; err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		return nil, errors.UnusableDatabase(err)
	}

	var rsp *drms.InstanceTemplate
	if rsp, err = Get(ctx, &drms.InstanceTemplateRequest{TemplateId: template.ID}); err != nil {
		logger.Errorf("[InstanceSnapshot-Add] Could not get Instance Snapshot(%d). Cause: %+v", template.ID, err)
	}

	return rsp, err
}

func deleteInstanceTemplate(db *gorm.DB, id, ownerGroupID uint64) error {
	if err := db.Where(&model.InstanceTemplate{ID: id, OwnerGroupID: ownerGroupID}).Delete(&model.InstanceTemplate{}).Error; err != nil {
		return err
	}

	return nil
}

func deleteInstances(db *gorm.DB, id uint64) error {
	if err := db.Where(&model.InstanceTemplateInstance{InstanceTemplateID: id}).Delete(&model.InstanceTemplateInstance{}).Error; err != nil {
		return err
	}
	return nil
}

func deleteDependencyInstances(db *gorm.DB, id uint64) error {
	if err := db.Where(&model.InstanceTemplateInstanceDependency{InstanceTemplateID: id}).Error; err != nil {
		return err
	}
	return nil
}

func Delete(ctx context.Context, req *drms.DeleteInstanceTemplateRequest) error {
	logger.Info("[InstanceTemplate-Delete] Start")

	var err error
	if req.GetTemplateId() == 0 {
		err = errors.RequiredParameter("template_id")
		logger.Errorf("[InstanceTemplate-Delete] Errors occurred during validate the request. Cause: %+v", err)
		return err
	}

	var it *model.InstanceTemplate
	if it, err = getInstanceTemplate(req.TemplateId); err != nil {
		logger.Errorf("[InstanceTemplate-Delete] Could not get the instance template(%v). Cause: %+v", req.TemplateId, err)
		return err
	}

	user, _ := metadata.GetAuthenticatedUser(ctx)
	if !internal.IsAdminUser(user) && !internal.IsGroupUser(user, it.OwnerGroupID) {
		logger.Errorf("[InstanceTemplate-Delete] Errors occurred during checking the authentication of the user. Cause: %+v", err)
		return errors.UnauthorizedRequest(ctx)
	}

	if err = database.GormTransaction(func(db *gorm.DB) error {
		if err = deleteDependencyInstances(db, it.ID); err != nil {
			return err
		}

		if err = deleteInstances(db, it.ID); err != nil {
			return err
		}

		if err = deleteInstanceTemplate(db, it.ID, it.OwnerGroupID); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return errors.UnusableDatabase(err)
	}

	return nil
}
