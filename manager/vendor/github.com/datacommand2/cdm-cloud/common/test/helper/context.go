package helper

import (
	"context"
	"encoding/json"
	"github.com/datacommand2/cdm-cloud/common/database/model"
	meta "github.com/datacommand2/cdm-cloud/common/metadata"
	identity "github.com/datacommand2/cdm-cloud/services/identity/proto"

	"github.com/jinzhu/gorm"
	"github.com/micro/go-micro/v2/metadata"
	"strconv"
)

// ContextOption context 생성 옵션
type ContextOption func(context.Context) context.Context

// WithAuthenticatedSession context 에 AuthenticatedSession metadata 추가
func WithAuthenticatedSession(key string) ContextOption {
	return func(ctx context.Context) context.Context {
		return metadata.Set(ctx, meta.HeaderAuthenticatedSession, key)
	}
}

// WithAuthenticatedUser context 에 AuthenticatedUser metadata 추가
func WithAuthenticatedUser(user *identity.User) ContextOption {
	return func(ctx context.Context) context.Context {
		b, _ := json.Marshal(user)
		return metadata.Set(ctx, meta.HeaderAuthenticatedUser, string(b))
	}
}

// WithClientIP context 에 ClientIP metadata 추가
func WithClientIP(ip string) ContextOption {
	return func(ctx context.Context) context.Context {
		return metadata.Set(ctx, meta.HeaderClientIP, ip)
	}
}

// WithTenantID context 에 TenantID metadata 추가
func WithTenantID(id uint64) ContextOption {
	return func(ctx context.Context) context.Context {
		return metadata.Set(ctx, meta.HeaderTenantID, strconv.FormatUint(id, 10))
	}
}

// Context context 생성
func Context(opts ...ContextOption) context.Context {
	ctx := context.Background()
	for _, o := range opts {
		ctx = o(ctx)
	}

	return ctx
}

func setDefaultTenantID(ctx context.Context, db *gorm.DB) (context.Context, error) {
	var t model.Tenant
	if err := db.First(&t, model.Tenant{Name: "default"}).Error; err != nil {
		return nil, err
	}

	return metadata.Set(ctx, meta.HeaderTenantID, strconv.FormatUint(t.ID, 10)), nil
}

//func setDefaultAuthenticatedUser(ctx context.Context, db *gorm.DB) (context.Context, error) {
//	var user identity.User
//
//	var u model.User
//	if err := db.First(&u, model.User{Account: "admin"}).Error; err != nil {
//		return nil, err
//	}
//	if b, err := json.Marshal(&u); err == nil {
//		_ = json.Unmarshal(b, &user)
//	} else {
//		return nil, err
//	}
//
//	var t model.Tenant
//	if err := db.First(&t, model.Tenant{ID: u.TenantID}).Error; err != nil {
//		return nil, err
//	}
//	if b, err := json.Marshal(&t); err == nil {
//		var tenant identity.Tenant
//		_ = json.Unmarshal(b, &tenant)
//		user.Tenant = &tenant
//	} else {
//		return nil, err
//	}
//
//	if groups, err := u.Groups(db); err == nil {
//		for _, g := range groups {
//			var group identity.Group
//			if b, err := json.Marshal(&g); err == nil {
//				_ = json.Unmarshal(b, &group)
//			} else {
//				return nil, err
//			}
//			user.Groups = append(user.Groups, &group)
//		}
//	} else {
//		return nil, err
//	}
//
//	if roles, err := u.Roles(db); err == nil {
//		for _, r := range roles {
//			var role identity.Role
//			if b, err := json.Marshal(&r); err == nil {
//				_ = json.Unmarshal(b, &role)
//			} else {
//				return nil, err
//			}
//			user.Roles = append(user.Roles, &role)
//		}
//	} else {
//		return nil, err
//	}
//
//	b, err := json.Marshal(&user)
//	if err != nil {
//		return nil, err
//	}
//
//	return metadata.Set(ctx, meta.HeaderAuthenticatedUser, string(b)), nil
//}

// DefaultContext admin 사용자의 default 테넌트에 대한 요청 context 를 생성하고 반환한다.
// 일반적인 RPC handling 에서 필요하지 않기 때문에 'HeaderAuthenticatedSession' 와 'HeaderClientIP' 는 제외함
func DefaultContext(db *gorm.DB, opts ...ContextOption) (context.Context, error) {
	var err error
	var ctx = context.Background()

	ctx, err = setDefaultTenantID(context.Background(), db)
	if err != nil {
		return nil, err
	}

	//ctx, err = setDefaultAuthenticatedUser(ctx, db)
	if err != nil {
		return nil, err
	}

	for _, o := range opts {
		ctx = o(ctx)
	}

	return ctx, nil
}
