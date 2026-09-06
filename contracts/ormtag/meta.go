// Package ormtag 实现 GoFast 统一模型标签（orm + rel + ext 三键体系）的解析器，
// 将 struct tag 解析为驱动无关的 ModelMeta，供 gormdriver（schema patch/乐观锁仿真）、
// xormdriver（SetTagIdentifier/Preload 元数据）与共享 Preload 引擎复用。
//
// 设计文档：docs/md/orm-tag-design.md（v1.2）第四、五、六章。
//
// 关键约束：
//   - orm tag 采用 xorm 语法（token 序列），本包在其上施加框架级白名单校验：
//     未加引号的未知裸 token、禁用 token（cache/nocache/deleted）、非法语法
//     （未闭合引号/括号、逗号在引号/括号外）一律解析期报错——因为 xorm 会把
//     未知裸 token 静默当作列名建垃圾列（文档 2.2 实验结论）；
//   - 列名强制单引号书写：'column_name'；
//   - 本包仅依赖标准库，禁止 import xorm（SQL 类型白名单为硬编码等价清单，
//     来源见 parser.go sqlTypeWhitelist 注释）。
package ormtag

import "reflect"

// FieldMeta 单个字段的统一元数据（orm tag 解析产物，覆盖 xorm 全量 token）。
//
// 字段集与设计文档 §6.2 逐字段一致；FieldIndex 与 RawTag 为实现层追加字段
// （文档 §6.3 第 5 点：嵌入展开路径；RawTag 用于错误信息定位）。
type FieldMeta struct {
	FieldName string // Go 字段名
	// ColumnName 列名（orm tag 中 '...' 指定）；空表示未显式指定，走驱动命名约定。
	// 注意：在 extends('前缀') 展开下，叶子字段的显式列名已加前缀（见 parser.go）。
	ColumnName string
	Ignore     bool // "-"：不建列/不读/不写

	PrimaryKey    bool   // pk
	AutoIncrement bool   // autoincr
	Type          string // 类型 token 原文（含括号参数），如 "varchar(100)"、"decimal(10,2)"

	NotNull bool // notnull / not null
	Null    bool // null（显式可空）

	Unique     bool   // unique
	UniqueName string // unique(name) 联合唯一组名
	Index      bool   // index
	IndexName  string // index(name) 联合索引组名

	Default    string // default 值原文（引号去除后；default 0 / default 'x' / default(x) 三形态）
	HasDefault bool
	Comment    string // comment('...')

	Created bool // created（插入时填当前时间）
	Updated bool // updated（插入/更新时填当前时间）
	Version bool // version（乐观锁；gormdriver 写路径仿真，文档 7.4）

	Extends       bool   // extends（嵌入展开标记；展开后的叶子字段见 Fields 中 FieldIndex 路径更长的条目）
	ExtendsPrefix string // extends('前缀')：嵌入列名前缀（文档 2.2 实测 xorm 原生支持）

	Unsigned bool   // unsigned（无符号数值，MySQL）
	Collate  string // collate(...)（gormdriver Warn 忽略）
	JSON     bool   // json / jsonb（jsonb 同样置位，对齐 xorm JSONBTagHandler；gormdriver 见文档 4.3 引导）

	UTC   bool // utc（gormdriver Warn 忽略）
	Local bool // local（gormdriver Warn 忽略）

	ReadOnly  bool // <-（OnlyFromDB：只读——读回填充、写入忽略）
	WriteOnly bool // ->（OnlyToDB：只写——写入落库、读回不填）
	// 方向以 xorm 语义为准（文档 4.2 注）：-> 只写、<- 只读，
	// 与 gorm 的 <-/-> 权限 tag 方向相反，驱动侧 patch 时负责翻译。

	FieldIndex []int // 反射索引路径：顶层叶子字段为 [i]，嵌入展开的叶子字段为 [嵌入下标, ..., 叶子下标]

	RawTag string // orm tag 原文（去首尾空白；错误信息定位用；未打 tag 时为空）
}

// RelMeta 关联元数据（rel tag 解析产物，覆盖 gorm 关联 tag 全集，文档第五章）。
type RelMeta struct {
	ForeignKeys []string // 外键 Go 字段名列表（foreignKey，逗号分隔复合键拆分）
	References  []string // 引用 Go 字段名列表（references，逗号分隔复合键拆分；缺省=对方主键）

	Many2Many        string // many2many：中间表表名
	JoinForeignKey   string // joinForeignKey：中间表指向父模型的键（父模型 Go 字段名）
	JoinReferences   string // joinReferences：中间表指向子模型的键（子模型 Go 字段名）
	Polymorphic      string // polymorphic：子表多态类型列的 Go 字段名前缀（如 Owner → OwnerID/OwnerType）
	PolymorphicValue string // polymorphicValue：多态类型值（缺省=父表名，由消费方回退）
	Constraint       string // constraint 原文（仅元数据，框架已禁用自动 FK 约束，不建约束）
}

// ExtMeta 驱动扩展元数据（ext tag 解析产物，gorm 专属能力的统一表达，文档 4.3）。
type ExtMeta struct {
	Check                  string // check:<expr>：CHECK 约束表达式
	IndexOptions           string // index:<完整选项> 原文（覆盖 orm index token 的同维度语义）
	AutoIncrementIncrement int64  // autoIncrementIncrement:N：自增步长（MySQL）
	IgnoreMigration        bool   // migration:false：读写但不参与建表
	TimePrecision          string // timePrecision:milli|nano：created/updated 精度
	PermCreateOnly         bool   // perm:create：仅 Create 写入
	PermUpdateOnly         bool   // perm:update：仅 Update 写入

	// UnknownKeys 未知 ext key 原文（大小写保留，键名去重）。
	// 本包不报错（文档 11.16），由消费驱动侧输出 Warn。
	UnknownKeys []string
}

// ModelMeta 模型级元数据（Parse 产物，按 reflect.Type 缓存）。
type ModelMeta struct {
	Type   reflect.Type
	Fields []FieldMeta        // 仅含打了 orm tag 的字段（含匿名嵌入/extends 展开后的叶子字段与嵌入标记条目）
	Rels   map[string]RelMeta // 关联字段名 → 关联元数据（rel tag 或 gorm tag 兼容读取，文档 5.3）
	Exts   map[string]ExtMeta // 字段名 → 扩展元数据（ext tag）
}
