// Package drivertest 驱动一致性测试套件（orm-tag-design.md §11.1）：同一套测试
// 逻辑在多个 ORM 驱动上运行，锁死 contracts.Query 的跨驱动语义。各驱动 module
// 的 conformance_test.go 以驱动工厂注入运行 RunSuite。
//
// 约束：本包只 import contracts 与标准库，禁止 import gorm/xorm/任何驱动包；
// 全部统一测试模型使用纯 orm tag（+ rel/ext），驱动差异以"差异固化断言"或
// DriverName 分支固化（§11.14/§11.16）。
//
// 模型设计要点（§9.1.1 双驱动适配矩阵）：
//   - Orders/Profile/Dept 关联字段只带 orm:"-"：子表外键 Go 字段名命中 gorm
//     约定（<父类型名><父主键名> → SuiteUserID；belongs-to → DeptID），gorm 侧
//     走原生关联 + 原生 Preload，xorm 侧走共享 Preload 引擎——双路径语义锁定；
//   - 外键偏离 gorm 约定的关联字段（Items/Children/Parent/Roles/Photos）必须
//     额外加 gorm:"-"（gorm Parse 硬边界，§9.1.1），双驱动都经共享引擎 + rel tag；
//   - many2many 中间表 suite_user_roles 两列列名不同（'id'/'code'，由
//     joinForeignKey:ID / joinReferences:Code 指向的父/子列名决定——共享引擎以
//     这两个列名直接查询中间表，同名会造成 SELECT 歧义）；
//   - 全部模型实现 TableName()：跨驱动锁定表名（gorm 复数约定 vs xorm 单数
//     SnakeMapper 推导不同），使 SQL/DDL 断言与多态缺省值（父表名）双驱动一致。
package drivertest

import (
	"errors"
	"strconv"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// errHookBoom 业务钩子返回的自定义错误（§11.14：原样透传，不被 Sentinel 包装吞没）。
var errHookBoom = errors.New("boom: 业务钩子自定义错误")

// ── 嵌入字段组（嵌入类型必须导出，文档 2.2/2.3） ────────────────────────

// SuiteAuditFields 匿名嵌入组：orm:"extends" 平铺展开。
type SuiteAuditFields struct {
	AuditNote string `orm:"varchar(64) 'audit_note' null"`
}

// SuiteAuthorFields 匿名嵌入组：orm:"extends('author_')" 列名前缀（DDL 断言
// author_* 列，§11.15）。叶子字段必须显式列名——前缀由 ormtag 解析器在解析期
// 应用（gorm 侧 patch 对匿名嵌入叶子按显式列名落地）。
type SuiteAuthorFields struct {
	Name string `orm:"varchar(64) 'name' null"`
	Bio  string `orm:"varchar(255) 'bio' null"`
}

// ── 基础模型：pk/类型/唯一/索引/默认值/忽略/只读/只写/关联全集 ───────────

// SuiteUser 基础模型（§11.1 全维度）。
type SuiteUser struct {
	ID        string  `orm:"pk varchar(16) 'id'"`
	Name      string  `orm:"varchar(100) 'name' notnull"`
	Email     string  `orm:"varchar(100) 'email' unique"`
	Age       int     `orm:"'age' default(0)"`
	Score     float64 `orm:"decimal(10,2) 'score'"`
	Password  string  `orm:"varchar(100) 'password' ->"` // 只写（OnlyToDB：落库不读回）
	Token     string  `orm:"varchar(100) 'token' <-"`    // 只读（OnlyFromDB：读回不写入）
	Internal  string  `orm:"-"`                          // 忽略（不建列/不读/不写）
	DeptID    string  `orm:"varchar(16) 'dept_id'"`
	CreatedAt int64   `orm:"created 'created_at'"`
	UpdatedAt int64   `orm:"updated 'updated_at'"`

	// has-many（gorm 约定外键：子表 SuiteOrder.SuiteUserID → 原生路径）
	Orders []SuiteOrder `orm:"-" rel:"foreignKey:SuiteUserID;references:ID"`
	// has-one（gorm 约定外键：子表 SuiteProfile.SuiteUserID → 原生路径）
	Profile *SuiteProfile `orm:"-" rel:"foreignKey:SuiteUserID;references:ID"`
	// belongs-to（gorm 约定外键：父行 SuiteUser.DeptID → 原生路径）
	Dept *SuiteDept `orm:"-" rel:"foreignKey:DeptID;references:ID"`
	// many2many（共享引擎两跳；外键非 gorm 关联 tag 可表达 → gorm:"-")
	Roles []SuiteRole `orm:"-" gorm:"-" rel:"many2many:suite_user_roles;joinForeignKey:ID;joinReferences:Code"`
	// 多态 has-many（显式 polymorphicValue；gorm:"-")
	Photos []SuitePhoto `orm:"-" gorm:"-" rel:"polymorphic:Owner;polymorphicValue:suite_users"`
}

func (SuiteUser) TableName() string { return "suite_users" }

// AutoGenerateID 实现 contracts.IDAutoGenerator（框架层 Create 前自动生成时序 ID）。
func (u *SuiteUser) AutoGenerateID() {
	if u.ID == "" {
		u.ID = newSuiteID()
	}
}

// SuiteOrder has-many 子表；含嵌套关联 Items 与多态 Photos。
type SuiteOrder struct {
	ID          string `orm:"pk varchar(16) 'id'"`
	SuiteUserID string `orm:"varchar(16) 'suite_user_id'"`
	Amount      int    `orm:"'amount' default(0)"`
	DeletedAt   int64  `orm:"'deleted_at' index default(0)"`
	CreatedAt   int64  `orm:"created 'created_at'"`

	Items  []SuiteOrderItem `orm:"-" rel:"foreignKey:SuiteOrderID;references:ID"`
	Photos []SuitePhoto     `orm:"-" gorm:"-" rel:"polymorphic:Owner;polymorphicValue:suite_orders"`
}

func (SuiteOrder) TableName() string { return "suite_orders" }

func (o *SuiteOrder) AutoGenerateID() {
	if o.ID == "" {
		o.ID = newSuiteID()
	}
}

// SuiteOrderItem 嵌套 Preload 目标（外键 SuiteOrderID 命中 gorm 约定
// <父类型名><父主键名> → gorm 原生嵌套 Preload "Orders.Items" 可用）。
type SuiteOrderItem struct {
	ID           string `orm:"pk varchar(16) 'id'"`
	SuiteOrderID string `orm:"varchar(16) 'suite_order_id'"`
	Sku          string `orm:"varchar(64) 'sku'"`
}

func (SuiteOrderItem) TableName() string { return "suite_order_items" }

func (i *SuiteOrderItem) AutoGenerateID() {
	if i.ID == "" {
		i.ID = newSuiteID()
	}
}

// SuiteProfile has-one 子表。
type SuiteProfile struct {
	ID          string `orm:"pk varchar(16) 'id'"`
	SuiteUserID string `orm:"varchar(16) 'suite_user_id'"`
	Bio         string `orm:"varchar(255) 'bio' null"`
}

func (SuiteProfile) TableName() string { return "suite_profiles" }

func (p *SuiteProfile) AutoGenerateID() {
	if p.ID == "" {
		p.ID = newSuiteID()
	}
}

// SuiteDept belongs-to 父表。
type SuiteDept struct {
	ID   string `orm:"pk varchar(16) 'id'"`
	Name string `orm:"varchar(64) 'name'"`
}

func (SuiteDept) TableName() string { return "suite_depts" }

func (d *SuiteDept) AutoGenerateID() {
	if d.ID == "" {
		d.ID = newSuiteID()
	}
}

// SuiteNode 自引用树（ParentID 外键，偏离 gorm 约定 → gorm:"-")。
type SuiteNode struct {
	ID       string `orm:"pk varchar(16) 'id'"`
	Name     string `orm:"varchar(64) 'name'"`
	ParentID string `orm:"varchar(16) 'parent_id' null"`

	Children []SuiteNode `orm:"-" gorm:"-" rel:"foreignKey:ParentID;references:ID"`
}

func (SuiteNode) TableName() string { return "suite_nodes" }

func (n *SuiteNode) AutoGenerateID() {
	if n.ID == "" {
		n.ID = newSuiteID()
	}
}

// SuiteCompound 复合外键父表（引用键 ID + TenantID）。
type SuiteCompound struct {
	ID       string `orm:"pk varchar(16) 'id'"`
	TenantID string `orm:"varchar(16) 'tenant_id'"`
	Name     string `orm:"varchar(64) 'name'"`

	// 复合外键 has-one：元组 (org_id, dept_id) = (id, tenant_id)；偏离 gorm
	// 约定（复合外键无 rel 时 gorm Parse 报错）→ gorm:"-"
	Parent *SuiteCompoundChild `orm:"-" gorm:"-" rel:"foreignKey:OrgID,DeptID;references:ID,TenantID"`
}

func (SuiteCompound) TableName() string { return "suite_compounds" }

func (c *SuiteCompound) AutoGenerateID() {
	if c.ID == "" {
		c.ID = newSuiteID()
	}
}

// SuiteCompoundChild 复合外键子表（外键 OrgID + DeptID 两列）。
type SuiteCompoundChild struct {
	ID     string `orm:"pk varchar(16) 'id'"`
	OrgID  string `orm:"varchar(16) 'org_id'"`
	DeptID string `orm:"varchar(16) 'dept_id'"`
	Label  string `orm:"varchar(64) 'label'"`
}

func (SuiteCompoundChild) TableName() string { return "suite_compound_children" }

func (c *SuiteCompoundChild) AutoGenerateID() {
	if c.ID == "" {
		c.ID = newSuiteID()
	}
}

// SuiteSoftDel 软删除模型（业务级：deleted_at 列仅声明 index/default，无 deleted
// 行为 token；软删语义经 OnlyTrashed/Restore/ForceDelete 表达，§11.9）。
type SuiteSoftDel struct {
	ID        string `orm:"pk varchar(16) 'id'"`
	Name      string `orm:"varchar(64) 'name'"`
	DeletedAt int64  `orm:"'deleted_at' index default(0)"`
}

func (SuiteSoftDel) TableName() string { return "suite_soft_dels" }

func (s *SuiteSoftDel) AutoGenerateID() {
	if s.ID == "" {
		s.ID = newSuiteID()
	}
}

// SuiteQuota Expr 原子更新模型（§11.7）。
type SuiteQuota struct {
	ID   string `orm:"pk varchar(16) 'id'"`
	Name string `orm:"varchar(64) 'name'"`
	Used int64  `orm:"'used' default(0)"`
}

func (SuiteQuota) TableName() string { return "suite_quotas" }

func (q *SuiteQuota) AutoGenerateID() {
	if q.ID == "" {
		q.ID = newSuiteID()
	}
}

// SuiteRole many2many 子表；Photos 缺省 polymorphicValue（回退父表名 suite_roles）。
type SuiteRole struct {
	ID   string `orm:"pk varchar(16) 'id'"`
	Code string `orm:"varchar(16) 'code' unique"`
	Name string `orm:"varchar(64) 'name'"`

	Photos []SuitePhoto `orm:"-" gorm:"-" rel:"polymorphic:Owner"`
}

func (SuiteRole) TableName() string { return "suite_roles" }

func (r *SuiteRole) AutoGenerateID() {
	if r.ID == "" {
		r.ID = newSuiteID()
	}
}

// SuitePhoto 多态子表（OwnerID/OwnerType 两列）。
type SuitePhoto struct {
	ID        string `orm:"pk varchar(16) 'id'"`
	OwnerID   string `orm:"varchar(16) 'owner_id'"`
	OwnerType string `orm:"varchar(32) 'owner_type'"`
	URL       string `orm:"varchar(255) 'url'"`
}

func (SuitePhoto) TableName() string { return "suite_photos" }

func (p *SuitePhoto) AutoGenerateID() {
	if p.ID == "" {
		p.ID = newSuiteID()
	}
}

// SuiteVersioned 乐观锁模型（orm:"version"；xorm 原生 / gormdriver 写路径仿真，
// §7.4/§11.3）。
type SuiteVersioned struct {
	ID      string `orm:"pk varchar(16) 'id'"`
	Title   string `orm:"varchar(64) 'title'"`
	Version int    `orm:"version 'version'"`
}

func (SuiteVersioned) TableName() string { return "suite_versioned" }

func (v *SuiteVersioned) AutoGenerateID() {
	if v.ID == "" {
		v.ID = newSuiteID()
	}
}

// SuiteHook 全钩子模型（7 个 On* 钩子标记位，§11.13）。标记位 orm:"-" 不落库；
// FailHook 非空时对应钩子返回 errHookBoom（错误中断用例）。
type SuiteHook struct {
	ID       string `orm:"pk varchar(16) 'id'"`
	Name     string `orm:"varchar(64) 'name'"`
	FailHook string `orm:"varchar(16) 'fail_hook' null"`

	BeforeCreateCalled bool `orm:"-"`
	AfterCreateCalled  bool `orm:"-"`
	BeforeUpdateCalled bool `orm:"-"`
	AfterUpdateCalled  bool `orm:"-"`
	BeforeDeleteCalled bool `orm:"-"`
	AfterDeleteCalled  bool `orm:"-"`
	AfterFindCalled    bool `orm:"-"`
}

func (SuiteHook) TableName() string { return "suite_hooks" }

func (h *SuiteHook) AutoGenerateID() {
	if h.ID == "" {
		h.ID = newSuiteID()
	}
}

func (h *SuiteHook) hookFails(name string) bool {
	return h.FailHook == name
}

// OnBeforeCreate 实现 contracts.BeforeCreator。
func (h *SuiteHook) OnBeforeCreate(q contracts.Query) error {
	h.BeforeCreateCalled = true
	if h.hookFails("before_create") {
		return errHookBoom
	}
	return nil
}

// OnAfterCreate 实现 contracts.AfterCreator。
func (h *SuiteHook) OnAfterCreate(q contracts.Query) error {
	h.AfterCreateCalled = true
	if h.hookFails("after_create") {
		return errHookBoom
	}
	return nil
}

// OnBeforeUpdate 实现 contracts.BeforeUpdater。
func (h *SuiteHook) OnBeforeUpdate(q contracts.Query) error {
	h.BeforeUpdateCalled = true
	if h.hookFails("before_update") {
		return errHookBoom
	}
	return nil
}

// OnAfterUpdate 实现 contracts.AfterUpdater。
func (h *SuiteHook) OnAfterUpdate(q contracts.Query) error {
	h.AfterUpdateCalled = true
	if h.hookFails("after_update") {
		return errHookBoom
	}
	return nil
}

// OnBeforeDelete 实现 contracts.BeforeDeleter。
func (h *SuiteHook) OnBeforeDelete(q contracts.Query) error {
	h.BeforeDeleteCalled = true
	if h.hookFails("before_delete") {
		return errHookBoom
	}
	return nil
}

// OnAfterDelete 实现 contracts.AfterDeleter。
func (h *SuiteHook) OnAfterDelete(q contracts.Query) error {
	h.AfterDeleteCalled = true
	if h.hookFails("after_delete") {
		return errHookBoom
	}
	return nil
}

// OnAfterFind 实现 contracts.AfterFinder。
func (h *SuiteHook) OnAfterFind(q contracts.Query) error {
	h.AfterFindCalled = true
	if h.hookFails("after_find") {
		return errHookBoom
	}
	return nil
}

// SuiteExt ext 模型（§11.15/§11.16：check/索引高级选项/自增步长/migration:false/
// timePrecision/perm/extends 前缀；SQLite 可跑部分，方言项以差异固化）。
type SuiteExt struct {
	ID    string `orm:"pk varchar(16) 'id'"`
	Title string `orm:"varchar(64) 'title' null"`

	// check 约束：gorm 侧 DDL 生成 CONSTRAINT ... CHECK（SQLite 支持并生效）；
	// xorm 侧忽略（差异固化：违反值写入成功，§11.16）
	Age int `orm:"'age' default(0)" ext:"check:age >= 0"`
	// 联合唯一 + 联合索引组（双驱动同组两字段）
	OrgID string `orm:"varchar(16) 'org_id' default('') unique(uk_suite_ext_org_dev) index(idx_suite_ext_org_dev)"`
	DevID string `orm:"varchar(16) 'dev_id' default('') unique(uk_suite_ext_org_dev) index(idx_suite_ext_org_dev)"`
	// ext 索引高级选项：gorm 双写 patch 建索引 idx_suite_ext_email；xorm 忽略 + Warn（差异固化）
	Email string `orm:"varchar(100) 'email' null" ext:"index:idx_suite_ext_email"`
	// 自增步长（MySQL 专属；SQLite 无 DDL 表现，仅验证 patch/xorm 告警不报错）
	Step int `orm:"'step' default(0)" ext:"autoIncrementIncrement:5"`
	// created 毫秒精度：gorm 填 unix 毫秒；xorm created 恒为秒（差异固化，§11.16）
	MilliCreated int64 `orm:"created 'milli_created'" ext:"timePrecision:milli"`
	// perm:create：gorm 仅 Create 写入（Updates 跳过）；xorm 驱动不拦截（差异固化）
	BornAt int64 `orm:"'born_at' default(0)" ext:"perm:create"`
	// migration:false：gorm 不建列但读写保留（套件以 Exec 补列后 CRUD）；xorm Sync2
	// 照常建列（差异固化，§11.16）
	LegacyCode string `orm:"varchar(32) 'legacy_code' null" ext:"migration:false"`

	SuiteAuditFields  `orm:"extends"`
	SuiteAuthorFields `orm:"extends('author_')"`
}

func (SuiteExt) TableName() string { return "suite_exts" }

func (e *SuiteExt) AutoGenerateID() {
	if e.ID == "" {
		e.ID = newSuiteID()
	}
}

// SuiteUserRole many2many 中间表 DDL 模型（仅用于 AutoMigrate 建表；共享引擎对
// 中间表只读，关联管理由业务显式完成，§9.2 契约 9）。列名 'id'/'code' 与 rel tag
// joinForeignKey:ID / joinReferences:Code 指向的父/子列名一致（引擎以这两名列名
// 查询中间表），两列不同名避免 SELECT 同名列歧义。
type SuiteUserRole struct {
	UserID   string `orm:"varchar(16) 'id' notnull"`
	RoleCode string `orm:"varchar(16) 'code' notnull"`
}

func (SuiteUserRole) TableName() string { return "suite_user_roles" }

// SuiteNamedCol 自定义列名模型（§11.3：列名 'user_name' 与 gorm 约定推导 name
// 不一致——Where/Order/Select/Pluck 命中 'user_name' 是 patch/mapper 生效的直接
// 证据）。
type SuiteNamedCol struct {
	ID     string `orm:"pk varchar(16) 'id'"`
	Name   string `orm:"varchar(64) 'user_name' notnull"`
	Amount int    `orm:"'amount' default(0)"`
}

func (SuiteNamedCol) TableName() string { return "suite_named_cols" }

func (n *SuiteNamedCol) AutoGenerateID() {
	if n.ID == "" {
		n.ID = newSuiteID()
	}
}

// SuiteBrokenRel 错误路径模型：rel 外键字段不存在（§11.5/§11.14：两方向均不成立
// → ErrUnsupported 包装，信息含模型名与字段名）。gorm:"- 保证 gorm Parse 可过，
// 错误统一由共享引擎前置校验产生。
type SuiteBrokenRel struct {
	ID    string      `orm:"pk varchar(16) 'id'"`
	Title string      `orm:"varchar(64) 'title'"`
	Nodes []SuiteNode `orm:"-" gorm:"-" rel:"foreignKey:NoSuchField;references:ID"`
}

func (SuiteBrokenRel) TableName() string { return "suite_broken_rels" }

// SuiteUntaggedRel 错误路径模型（差异固化，§11.5）：Orders 无任何 tag——
// gorm 侧命中约定外键（子表 SuiteUntaggedOrder.SuiteUntaggedRelID = <父类型名><父
// 主键名>）走原生 Preload（无感成功）；xorm 侧共享引擎前置校验缺忽略标记报
// ErrUnsupported（信息含字段名引导）。
type SuiteUntaggedRel struct {
	ID     string               `orm:"pk varchar(16) 'id'"`
	Title  string               `orm:"varchar(64) 'title'"`
	Orders []SuiteUntaggedOrder // 无任何 tag：gorm 约定关联 / xorm 前置校验 ErrUnsupported
}

func (SuiteUntaggedRel) TableName() string { return "suite_untagged_rels" }

func (u *SuiteUntaggedRel) AutoGenerateID() {
	if u.ID == "" {
		u.ID = newSuiteID()
	}
}

// SuiteUntaggedOrder SuiteUntaggedRel 的约定外键子表（gorm 原生路径成立的前提）。
type SuiteUntaggedOrder struct {
	ID                 string `orm:"pk varchar(16) 'id'"`
	SuiteUntaggedRelID string `orm:"varchar(16) 'suite_untagged_rel_id'"`
	Amount             int    `orm:"'amount' default(0)"`
}

func (SuiteUntaggedOrder) TableName() string { return "suite_untagged_orders" }

func (o *SuiteUntaggedOrder) AutoGenerateID() {
	if o.ID == "" {
		o.ID = newSuiteID()
	}
}

// suiteIDSeq 套件内 ID 兜底生成（保证非空且各不相同；断言"框架 16 位时序 ID"
// 时以 Create 后实际值为准）。
var suiteIDSeq int64

func newSuiteID() string {
	suiteIDSeq++
	id := "suite" + strconv.FormatInt(suiteIDSeq, 10)
	for len(id) < 16 {
		id += "0"
	}
	return id[:16]
}
