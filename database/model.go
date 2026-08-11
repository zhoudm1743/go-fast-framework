package database

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/id"
)

// Model 基础模型，所有业务模型应嵌入此结构体。
// ID 为时序 ID 字符串主键（16 字符），由框架层驱动在 Create 前自动调用 AutoGenerateID() 生成。
type Model struct {
	ID        string `gorm:"primaryKey;size:16;column:id"      xorm:"pk varchar(16) 'id'"    json:"id"`
	CreatedAt int64  `gorm:"autoCreateTime;column:created_at"  xorm:"created 'created_at'"   json:"created_at"`
	UpdatedAt int64  `gorm:"autoUpdateTime;column:updated_at"  xorm:"updated 'updated_at'"   json:"updated_at"`
}

// SoftDelete 软删除结构体，嵌入模型以启用软删除功能。
type SoftDelete struct {
	DeletedAt int64 `gorm:"column:deleted_at;index;default:0" xorm:"'deleted_at' index default(0)" json:"deleted_at"`
}

// AutoGenerateID 实现 contracts.IDAutoGenerator。
// 驱动层在 Create 前调用，方法名不与 GORM/xorm 任何内置 Hook 冲突，
// 因此不会触发 GORM 的签名不匹配警告。
func (m *Model) AutoGenerateID() {
	if m.ID == "" {
		m.ID = id.New()
	}
}

// ModelWithSoftDelete 带软删除的基础模型。
type ModelWithSoftDelete struct {
	Model
	SoftDelete
}

// ── 模型钩子接口别名（向后兼容，实际定义在 contracts 包）─────────────
// 推荐直接使用 contracts.BeforeCreator 等。

// BeforeCreator 创建前钩子（= contracts.BeforeCreator）
type BeforeCreator = contracts.BeforeCreator

// AfterCreator 创建后钩子（= contracts.AfterCreator）
type AfterCreator = contracts.AfterCreator

// BeforeUpdater 更新前钩子（= contracts.BeforeUpdater）
type BeforeUpdater = contracts.BeforeUpdater

// AfterUpdater 更新后钩子（= contracts.AfterUpdater）
type AfterUpdater = contracts.AfterUpdater

// BeforeDeleter 删除前钩子（= contracts.BeforeDeleter）
type BeforeDeleter = contracts.BeforeDeleter

// AfterDeleter 删除后钩子（= contracts.AfterDeleter）
type AfterDeleter = contracts.AfterDeleter

// AfterFinder 查询后钩子（= contracts.AfterFinder）
type AfterFinder = contracts.AfterFinder
