package database

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/id"
)

// Model 基础模型，所有业务模型应嵌入此结构体。
// ID 为时序 ID 字符串主键（16 字符），由框架层驱动在 Create 前自动调用 AutoGenerateID() 生成。
// GORM 驱动另通过 Create 回调覆盖 FirstOrCreate 等内部路径（见 gofast-gorm）。
//
// 标签形态（orm-tag-design.md §10.1 三 tag 过渡期）：orm（统一标签，xorm 语法）
// + gorm + xorm 三键并存，值等价——旧连接（tag_identifier=xorm）读 xorm tag、
// gorm 连接读 gorm tag（orm tag 经 schema patch 覆盖同维度语义），新连接读 orm tag。
type Model struct {
	ID        string `orm:"pk varchar(16) 'id'"               gorm:"primaryKey;size:16;column:id"     xorm:"pk varchar(16) 'id'"        json:"id"`
	CreatedAt int64  `orm:"created 'created_at'"              gorm:"autoCreateTime;column:created_at" xorm:"created 'created_at'"       json:"created_at"`
	UpdatedAt int64  `orm:"updated 'updated_at'"              gorm:"autoUpdateTime;column:updated_at" xorm:"updated 'updated_at'"       json:"updated_at"`
}

// SoftDelete 软删除结构体，嵌入模型以启用软删除功能。
//
// 注意：不声明 xorm deleted 行为 tag——框架软删除为业务级实现
// （OnlyTrashed/Restore/ForceDelete），避免与 xorm 原生 deleted 语义冲突
// （deleted 为 orm tag 禁用 token，文档 §4.4）。
type SoftDelete struct {
	DeletedAt int64 `orm:"'deleted_at' index default(0)" gorm:"column:deleted_at;index;default:0" xorm:"'deleted_at' index default(0)" json:"deleted_at"`
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
// 嵌入字段声明 orm:"extends" + xorm:"extends"（文档 §10.1 三 tag 过渡形态）：
// xorm（identifier=xorm/orm 均原生支持 extends）与 ormtag 解析器都将两嵌入
// 展开为 id/created_at/updated_at/deleted_at 平铺列；gorm 对匿名嵌入原生平铺，
// orm:"extends" 由 schema patch 消费（extends('前缀') 时加列名前缀）。
type ModelWithSoftDelete struct {
	Model      `orm:"extends" xorm:"extends"`
	SoftDelete `orm:"extends" xorm:"extends"`
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
