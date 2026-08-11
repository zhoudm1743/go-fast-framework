package gormdriver

import (
	"context"
	"reflect"

	"github.com/zhoudm1743/go-fast-framework/contracts"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// registerAuditCallbacks 注册 created_by / updated_by 自动填充回调。
func registerAuditCallbacks(db *gorm.DB) {
	_ = db.Callback().Create().Before("gorm:create").Register("audit:created_by", auditCreateCallback)
	_ = db.Callback().Update().Before("gorm:update").Register("audit:updated_by", auditUpdateCallback)
}

// auditCreateCallback 在 Create 前根据 ctx 设置 created_by / updated_by。
func auditCreateCallback(db *gorm.DB) {
	if db.Statement == nil || db.Statement.Schema == nil {
		return
	}
	uid := userIDFromCtx(db.Statement.Context)
	if uid == "" {
		return
	}
	createdBy := db.Statement.Schema.LookUpField("CreatedBy")
	updatedBy := db.Statement.Schema.LookUpField("UpdatedBy")
	if createdBy == nil && updatedBy == nil {
		return
	}
	rv := reflect.Indirect(db.Statement.ReflectValue)
	switch rv.Kind() {
	case reflect.Struct:
		assignAudit(db.Statement.Context, rv, createdBy, updatedBy, uid)
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			elem := reflect.Indirect(rv.Index(i))
			if elem.Kind() == reflect.Struct {
				assignAudit(db.Statement.Context, elem, createdBy, updatedBy, uid)
			}
		}
	}
}

func assignAudit(ctx context.Context, rv reflect.Value, createdBy, updatedBy *schema.Field, uid string) {
	if createdBy != nil {
		cur, _ := createdBy.ValueOf(ctx, rv)
		if s, _ := cur.(string); s == "" {
			_ = createdBy.Set(ctx, rv, uid)
		}
	}
	if updatedBy != nil {
		cur, _ := updatedBy.ValueOf(ctx, rv)
		if s, _ := cur.(string); s == "" {
			_ = updatedBy.Set(ctx, rv, uid)
		}
	}
}

// auditUpdateCallback 在 Update 前根据 ctx 设置 updated_by。
func auditUpdateCallback(db *gorm.DB) {
	if db.Statement == nil || db.Statement.Schema == nil {
		return
	}
	uid := userIDFromCtx(db.Statement.Context)
	if uid == "" {
		return
	}
	if f := db.Statement.Schema.LookUpField("UpdatedBy"); f != nil {
		db.Statement.SetColumn(f.DBName, uid)
	}
}

// userIDFromCtx 从 context 中提取 user_id。
func userIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(contracts.CtxKeyUserID).(string); ok && v != "" {
		return v
	}
	return ""
}
