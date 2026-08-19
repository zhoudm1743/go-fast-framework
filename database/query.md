# GoFast 数据库查询抽象层设计方案

## 一、背景与现状分析

### 当前问题

目前 `contracts.Orm` 接口直接暴露了 `*gorm.DB`：

```go
// framework/contracts/orm.go
type Orm interface {
    DB() *gorm.DB        // ← 强耦合 GORM
    Ping() error
    Close() error
    AutoMigrate(models ...any) error
}
```

控制器代码直接调用 GORM 的链式 API：

```go
// 现有写法（强依赖 GORM）
facades.Orm().DB().Model(&models.User{}).Where("email LIKE ?", "%"+email+"%").Find(&users)
facades.Orm().DB().First(&user, "id = ?", req.ID).Error
facades.Orm().DB().Create(&user).Error
```

**问题核心**：业务代码、插件代码全部与 GORM 的具体类型绑定，无法无缝切换为 xorm、torm 或其他 ORM。

---

## 二、设计目标

| 目标 | 说明 |
|------|------|
| **解耦** | 业务代码不再依赖任何具体 ORM 库的类型 |
| **可插拔** | GORM / xorm / torm 通过实现同一套 `Driver` 接口接入 |
| **多连接** | 支持同时配置多个数据库连接（主库、只读副本、多租户等） |
| **链式 API** | 提供流畅的 `Query()` 链式构建器，对开发者友好 |
| **向后兼容** | 保留 `facades.Orm()` 但标记为 Deprecated，平滑迁移 |
| **可测试** | `Query` 接口可被 Mock，方便单元测试 |

---

## 三、核心接口设计

### 3.1 `Query` —— 查询构建器接口

> 文件：`framework/contracts/query.go`

```go
package contracts

import "context"

// Query 数据库无关的查询构建器。
// 所有链式方法均返回 Query 本身，终结方法返回 error。
type Query interface {
    // ── 构建条件 ────────────────────────────────────────────
    Table(name string) Query
    Model(value any) Query
    Select(query any, args ...any) Query
    Omit(columns ...string) Query
    Where(query any, args ...any) Query
    OrWhere(query any, args ...any) Query
    Not(query any, args ...any) Query
    Order(value any) Query
    Limit(limit int) Query
    Offset(offset int) Query
    Group(name string) Query
    Having(query any, args ...any) Query
    Distinct(args ...any) Query

    // ── 关联 ────────────────────────────────────────────────
    Joins(query string, args ...any) Query
    Preload(query string, args ...any) Query

    // ── 终结方法（返回 error）───────────────────────────────
    Find(dest any, conds ...any) error
    First(dest any, conds ...any) error
    Last(dest any, conds ...any) error
    Take(dest any, conds ...any) error
    Create(value any) error
    CreateInBatches(value any, batchSize int) error
    Save(value any) error
    Update(column string, value any) error
    Updates(values any) error
    Delete(value any, conds ...any) error
    Count(count *int64) error
    Scan(dest any) error
    Pluck(column string, dest any) error
    Row() Row
    Rows() (Rows, error)

    // ── 原生 SQL ───────────────────────────────────────���────
    Raw(sql string, values ...any) Query
    Exec(sql string, values ...any) error

    // ── 事务 ────────────────────────────────────────────────
    Transaction(fc func(tx Query) error, opts ...TxOption) error
    Begin(opts ...TxOption) Query
    Commit() error
    Rollback() error
    SavePoint(name string) error
    RollbackTo(name string) error

    // ── 分页辅助 ────────────────────────────────────────────
    // Paginate 等价于 Offset((page-1)*size).Limit(size)
    Paginate(page, size int) Query

    // ── 作用域 ──────────────────────────────────────────────
    Scopes(funcs ...func(Query) Query) Query

    // ── 上下文 ──────────────────────────────────────────────
    WithContext(ctx context.Context) Query

    // ── 调试 ────────────────────────────────────────────────
    Debug() Query
}

// Row 对应 *sql.Row
type Row interface {
    Scan(dest ...any) error
}

// Rows 对应 *sql.Rows
type Rows interface {
    Next() bool
    Scan(dest ...any) error
    Close() error
    Columns() ([]string, error)
}

// TxOption 事务选项（隔离级别等），各驱动自行实现
type TxOption interface{}
```

### 3.2 `Driver` —— ORM 驱动适配器接口

> 文件：`framework/contracts/query.go`（追加）

```go
// Driver 数据库驱动适配器，由各 ORM 插件实现。
type Driver interface {
    // Query 创建新的查询构建器实例
    Query(ctx ...context.Context) Query
    // Ping 检查连接可用性
    Ping() error
    // Close 关闭底层连接池
    Close() error
    // AutoMigrate 根据 struct 自动建表/迁移
    AutoMigrate(models ...any) error
    // DriverName 返回驱动标识，如 "gormdriver"、"xorm"、"torm"
    DriverName() string
}
```

### 3.3 `DB` —— 数据库管理器接口

> 文件：`framework/contracts/query.go`（追加）

```go
// DB 数据库管理器，管理多个命名连接。
// 通过 facades.DB() 获取。
type DB interface {
    // Query 在默认连接上返回查询构建器
    Query(ctx ...context.Context) Query
    // Connection 切换到指定命名连接，返回该连接的查询构建器
    Connection(name string) Query
    // Driver 获取默认（或指定）连接的底层驱动
    Driver(name ...string) Driver
    // Transaction 在默认连接上执行事务
    Transaction(fc func(tx Query) error, opts ...TxOption) error
    // AutoMigrate 在默认连接上迁移
    AutoMigrate(models ...any) error
    // Ping 检查默认连接
    Ping() error
    // Close 关闭所有连接
    Close() error
}
```

---

## 四、目录结构变更

```
framework/
├── contracts/
│   ├── orm.go               # 保留（Deprecated），兼容旧代码
│   └── query.go             # ★ 新增：Query / Driver / DB 接口
│
├── database/
│   ├── model.go             # 微调：补充 xorm/torm struct tag
│   ├── service_provider.go  # ★ 修改：同时注册 "db" 和兼容 "orm"
│   ├── manager.go           # ★ 新增：DB 接口的实现（多连接管理）
│   └── drivers/
│       └── gorm/
│           ├── driver.go    # ★ 新增：GORM Driver 实现
│           └── query.go     # ★ 新增：GORM Query 构建器包装
│
├── facades/
│   ├── orm.go               # 保留（Deprecated 注释）
│   └── db.go                # ★ 新增：facades.DB()
│
plugins/
├── gofast-xorm/             # ★ 独立 Go module：xorm 插件
│   ├── go.mod
│   ├── driver.go
│   ├── query.go
│   └── service_provider.go
└── gofast-torm/             # ★ 独立 Go module：torm 插件
    ├── go.mod
    ├── driver.go
    ├── query.go
    └── service_provider.go
```

---

## 五、配置格式变更

### 新配置（`config/config.yaml`）

```yaml
database:
  default: main          # 默认连接名

  connections:
    main:
      driver: gormdriver       # ORM 驱动：gormdriver | xorm | torm
      engine: sqlite     # 数据库引擎：mysql | postgres | sqlite | mssql
      database: database/gofast.db
      max_idle_conns: 10
      max_open_conns: 100
      conn_max_lifetime: 60   # 分钟
      conn_max_idle_time: 30  # 分钟
      log_level: ""           # error | warn | info | silent
      slow_threshold: 200     # ms

    read_replica:
      driver: gormdriver
      engine: mysql
      host: 10.0.0.2
      port: 3306
      username: reader
      password: secret
      database: myapp
```

### 向后兼容

旧的扁平化配置（`database.driver`、`database.host` 等）在 `manager.go` 中自动检测：若不存在 `database.connections` 节点，则自动将旧配置适配为 `connections.main`，**零改动升级**。

---

## 六、实现细节

### 6.1 `manager.go` —— DB 接口实现

```go
// framework/database/manager.go
package database

type dbManager struct {
    cfg         contracts.Config
    log         contracts.Log
    defaultConn string
    connections map[string]contracts.Driver  // 懒加载缓存
    mu          sync.RWMutex
}

// Query 获取默认连接的查询构建器
func (m *dbManager) Query(ctx ...context.Context) contracts.Query {
    return m.Connection(m.defaultConn)
}

// Connection 懒加载并返回指定连接的 Query
func (m *dbManager) Connection(name string) contracts.Query {
    driver := m.getOrCreateDriver(name)
    return driver.Query()
}

// Driver 获取底层驱动（可选名称，默认返回默认连接驱动）
func (m *dbManager) Driver(name ...string) contracts.Driver {
    connName := m.defaultConn
    if len(name) > 0 {
        connName = name[0]
    }
    return m.getOrCreateDriver(connName)
}
```

**驱动工厂注册机制（全局注册表）**：

```go
// DriverFactory 根据连接配置创建 Driver
type DriverFactory func(cfg ConnectionConfig, log contracts.Log) (contracts.Driver, error)

var (
    driverFactoriesMu sync.RWMutex
    driverFactories   = map[string]DriverFactory{}
)

// RegisterDriver 由插件的 ServiceProvider 在启动时调用
func RegisterDriver(name string, f DriverFactory) {
    driverFactoriesMu.Lock()
    defer driverFactoriesMu.Unlock()
    driverFactories[name] = f
}
```

### 6.2 `drivers/gorm/driver.go` —— GORM 适配器

```go
package gormdriver

import "gorm.io/gorm"

// GormDriver 实现 contracts.Driver
type GormDriver struct {
    db *gorm.DB
}

func (d *GormDriver) Query(ctx ...context.Context) contracts.Query {
    db := d.db
    if len(ctx) > 0 {
        db = db.WithContext(ctx[0])
    }
    return &GormQuery{db: db}
}

func (d *GormDriver) DriverName() string          { return "gormdriver" }
func (d *GormDriver) Ping() error                 { /* sqlDB.Ping() */ }
func (d *GormDriver) Close() error                { /* sqlDB.Close() */ }
func (d *GormDriver) AutoMigrate(m ...any) error  { return d.db.AutoMigrate(m...) }

// RawDB 逃生口：允许高级用户直接获取 *gormdriver.DB（不推荐常规使用）
func (d *GormDriver) RawDB() *gorm.DB { return d.db }
```

### 6.3 `drivers/gorm/query.go` —— GORM Query 包装

```go
package gormdriver

// GormQuery 将 contracts.Query 的每个方法代理到 *gormdriver.DB
type GormQuery struct {
    db *gorm.DB
}

// 链式方法：每次返回新的 GormQuery，保持不可变
func (q *GormQuery) Where(query any, args ...any) contracts.Query {
    return &GormQuery{db: q.db.Where(query, args...)}
}

// 终结方法：直接返回 error
func (q *GormQuery) Find(dest any, conds ...any) error {
    return q.db.Find(dest, conds...).Error
}

func (q *GormQuery) Create(value any) error {
    return q.db.Create(value).Error
}

// 事务：将 *gormdriver.DB 事务转换为 contracts.Query
func (q *GormQuery) Transaction(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error {
    return q.db.Transaction(func(tx *gorm.DB) error {
        return fc(&GormQuery{db: tx})
    })
}

// Paginate 辅助方法
func (q *GormQuery) Paginate(page, size int) contracts.Query {
    if page < 1 {
        page = 1
    }
    return &GormQuery{db: q.db.Offset((page - 1) * size).Limit(size)}
}

// ... 其余方法均以相同模式实现
```

### 6.4 xorm / torm 插件实现模式

插件作为独立 Go module 存放，只需：

1. 实现 `contracts.Driver` 和 `contracts.Query` 两个接口
2. 在 `ServiceProvider.Register` 中调用 `database.RegisterDriver`

```go
// plugins/gofast-xorm/service_provider.go
package gofast_xorm

type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
    // 向框架注册 "xorm" 驱动工厂
    database.RegisterDriver("xorm", func(cfg database.ConnectionConfig, log contracts.Log) (contracts.Driver, error) {
        return NewXormDriver(cfg, log)
    })
}
```

用户在 `bootstrap/app.go` 中注册该插件：

```go
app.Register(&gofast_xorm.ServiceProvider{})
```

再修改配置 `driver: xorm`，业务代码**零改动**。

---

## 七、Facade 变更

### 新增 `facades/db.go`

```go
package facades

import "github.com/zhoudm1743/go-fast-framework/contracts"

// DB 获取数据库管理器（推荐使用）
func DB() contracts.DB {
    return App().MustMake("db").(contracts.DB)
}
```

### 保留（Deprecated）`facades/orm.go`

```go
// Deprecated: 请使用 facades.DB().Query()，此方法将在下一主版本移除。
func Orm() contracts.Orm {
    return App().MustMake("orm").(contracts.Orm)
}
```

---

## 八、业务代码迁移对照表

| 场景 | 旧写法 | 新写法 |
|------|--------|--------|
| 列表查询 | `facades.Orm().DB().Find(&list)` | `facades.DB().Query().Find(&list)` |
| 条件查询 | `facades.Orm().DB().Where(...).Find(&list)` | `facades.DB().Query().Where(...).Find(&list)` |
| 分页 | `.Offset((p-1)*s).Limit(s).Find(...)` | `.Paginate(p,s).Find(...)` |
| 统计 | `.Count(&total)` | `.Count(&total)`（不变） |
| 按 ID 查询 | `...DB().First(&u, "id=?", id).Error` | `...Query().First(&u, "id=?", id)` |
| 创建 | `...DB().Create(&u).Error` | `...Query().Create(&u)` |
| 更新 | `...DB().Model(&u).Updates(m).Error` | `...Query().Model(&u).Updates(m)` |
| 删除 | `...DB().Delete(&u).Error` | `...Query().Delete(&u)` |
| 事务 | `...DB().Transaction(func(tx *gorm.DB) error {...})` | `...Query().Transaction(func(tx contracts.Query) error {...})` |
| 多连接 | 不支持 | `facades.DB().Connection("read_replica").Find(...)` |
| 原生 SQL | `...DB().Raw(sql).Scan(&dest).Error` | `...Query().Raw(sql).Scan(&dest)` |

### 完整示例对照（Index 控制器）

```go
// ── 旧写法 ──────────────────────────────────────────────────────────
db := facades.Orm().DB().Model(&models.User{}).Order("created_at DESC")
if req.Email != "" {
    db = db.Where("email LIKE ?", "%"+req.Email+"%")
}
var total int64
db.Count(&total)
var users []models.User
db.Offset((req.Page-1)*req.Size).Limit(req.Size).Find(&users)

// ── 新写法 ──────────────────────────────────────────────────────────
q := facades.DB().Query().Model(&models.User{}).Order("created_at DESC")
if req.Email != "" {
    q = q.Where("email LIKE ?", "%"+req.Email+"%")
}
var total int64
q.Count(&total)
var users []models.User
q.Paginate(req.Page, req.Size).Find(&users)
```

---

## 九、Model 层多 ORM Tag 适配

`database.Model` 补充 xorm/torm 所需的 struct tag，GORM tag 保持不变：

```go
// framework/database/model.go
type Model struct {
    ID        string `gormdriver:"primaryKey;size:36;column:id"   xorm:"pk varchar(36) 'id'"    json:"id"`
    CreatedAt int64  `gormdriver:"autoCreateTime;column:created_at" xorm:"created 'created_at'" json:"created_at"`
    UpdatedAt int64  `gormdriver:"autoUpdateTime;column:updated_at" xorm:"updated 'updated_at'" json:"updated_at"`
}
```

UUID 自动赋值逻辑从 GORM callback 抽取到 `manager.go` 的通用 `BeforeCreate` 钩子，或在 `Query.Create` 的各驱动包装层统一处理。

---

## 十、实现步骤（优先级排序）

| 步骤 | 文件 | 说明 |
|------|------|------|
| 1 | `framework/contracts/query.go` | 定义 `Query`、`Driver`、`DB`、`Migrator`、`Result`、`LockMode`、`TxOption` 标准实现、Sentinel Errors |
| 2 | `framework/database/config.go` | 新增完整 `ConnectionConfig` 结构体及 `BuildDSN` 方法 |
| 3 | `framework/database/model.go` | 补充多 ORM struct tag；迁移 `BeforeCreate` Hook 到框架层；定义 Hook 接口 |
| 4 | `framework/database/drivers/gorm/errors.go` | 实现 `wrapError`，映射 GORM 错误到框架 Sentinel |
| 5 | `framework/database/drivers/gorm/query.go` | 实现 GORM Query 包装器（含所有新增方法：软删除、高级查询、Lock 等） |
| 6 | `framework/database/drivers/gorm/driver.go` | 实现 GORM Driver（含 OTel 插件注入、`Migrator` 方法） |
| 7 | `framework/database/drivers/gorm/migrator.go` | 集成 goose，实现 `GormMigrator` |
| 8 | `framework/database/manager.go` | 实现 `dbManager`（多连接懒加载、驱动注册表、向后兼容旧配置） |
| 9 | `framework/database/service_provider.go` | 注册 `"db"` 服务，保留兼容 `"orm"`，Shutdown 时关闭所有连接 |
| 10 | `framework/facades/db.go` | 新增 `facades.DB()` |
| 11 | `framework/facades/orm.go` | 添加 Deprecated 注释 |
| 12 | `framework/contracts/orm.go` | 添加 Deprecated 注释 |

### Phase 5：业务代码迁移
- [x] **T14** `app/http/admin/controllers/user_controller.go` — 替换 `facades.Orm().DB().*` 为 `facades.DB().Query().*`
- [x] **T15** `app/http/app/controllers/user_controller.go` — 同上替换

### Phase 6：Application 层适配
- [x] **T16** `framework/foundation/application.go` — 新增 `DB()` 快捷方法
- [x] **T17** `framework/foundation/provider.go` — `Migrator` 接口适配新 `contracts.DB`（兼容旧 `contracts.Orm`）

### 暂缓（后续迭代）
- [ ] `plugins/gofast-xorm/` — xorm Driver & Query 实现
- [ ] `plugins/gofast-torm/` — torm Driver & Query 实现
- [ ] `framework/database/drivers/gorm/migrator.go` — 集成 goose 版本化迁移
- [ ] `go.work` — 创建 workspace 管理多模块
- [ ] 读写分离路由（`Read()`/`Write()`）
- [ ] `DBStats` 连接池状态接口
- [ ] OpenTelemetry 集成

---
## ✅ TodoList（实现进度跟踪）

> 以下为根据本文档拆解的具体实现任务，勾选表示已完成。

### Phase 1：接口层
- [x] **T1** `framework/contracts/query.go` — 定义 `Query`、`Row`、`Rows`、`TxOption`、`Driver`、`DB` 接口
- [x] **T2** `framework/contracts/query.go` — 定义 `Result` 结构体、`LockMode` 常量、`StandardTxOptions`、预定义快捷选项
- [x] **T3** `framework/contracts/query.go` — 定义框架级 Sentinel Errors（`ErrRecordNotFound` 等）
- [x] **T-extra** `framework/contracts/database.go` — 新增：`ConnectionConfig`（含 `BuildDSN`/`ApplyDefaults`）及模型 Hook 接口（`BeforeCreator` 等），破除 `database` ↔ `gormdriver` 循环依赖

### Phase 2：配置与模型
- [x] **T4** `framework/database/config.go` — `ConnectionConfig` 改为类型别名（实体移至 `contracts/database.go`，破除循环依赖）
- [x] **T5** `framework/database/model.go` — 补充多 ORM struct tag（`gorm`/`xorm`）；Hook 接口别名；`BeforeCreate` UUID 逻辑

### Phase 3：GORM 驱动实现
- [x] **T6** `framework/database/drivers/gormdriver/errors.go` — 实现 `wrapError`，映射 GORM 错误到框架 Sentinel
- [x] **T7** `framework/database/drivers/gormdriver/query.go` — 实现 `GormQuery`（全部链式方法 + 终结方法 + 软删除 + 高级查询 + Lock）
- [x] **T8** `framework/database/drivers/gormdriver/driver.go` — 实现 `GormDriver`；修复错误导入；改用 `contracts.ConnectionConfig` 破除循环依赖

### Phase 4：管理器与服务注册
- [x] **T9** `framework/database/manager.go` — 实现 `dbManager`（多连接懒加载、驱动注册表、向后兼容旧配置；修复文件编码损坏导致的语法错误）
- [x] **T10** `framework/database/service_provider.go` — 注册 `"db"` 服务，保留兼容 `"orm"`，Shutdown 时关闭所有连接
- [x] **T11** `framework/facades/db.go` — 新增 `facades.DB()`
- [x] **T12** `framework/facades/orm.go` — 添加 Deprecated 注释
- [x] **T13** `framework/contracts/orm.go` — 添加 Deprecated 注释

### Phase 5：业务代码迁移
- [x] **T14** `app/http/admin/controllers/user_controller.go` — 替换为 `facades.DB().Query().*`；使用 `errors.Is(contracts.ErrRecordNotFound)` 精确判断
- [x] **T15** `app/http/app/controllers/user_controller.go` — 同上替换

### Phase 6：Application 层适配
- [x] **T16** `framework/foundation/application.go` — 新增 `DB() contracts.DB` 到 `Application` 接口与实现
- [x] **T17** `framework/foundation/provider.go` — 新增 `DBMigrator` 接口（`MigrateDB(db contracts.DB) error`）；`Boot` 同时支持旧 `Migrator` 与新 `DBMigrator`

### 额外修复（实现中发现的 Bug）
- [x] `framework/contracts/storage.go` — `Storage` 接口嵌入 `StorageDriver`（原嵌入 `Driver` 与数据库 `Driver` 命名冲突）
- [x] `framework/http/response.go` — `contracts.Driver` → `contracts.StorageDriver`
- [x] `framework/database/orm.go` — 修复错误导入 `gormdriver.io/gorm/logger` → `gorm.io/gorm/logger`
- [x] `app/models/user.go` — struct tag 从 `gormdriver:"..."` 修正为 `gorm:"..."`

### 查询缓存（Query().Cache()）
结合 [go-gorm/caches](https://github.com/go-gorm/caches) 与框架 Cache 服务，实现按需查询缓存：

```go
// 按需开启：仅当前查询链生效，命中缓存时不再访问数据库
facades.DB().Query().
    Cache(contracts.CacheTTLOption(1*time.Minute), contracts.CacheTagsOption("users")).
    Where("status = ?", 1).Find(&list)
```

设计要点：
- **按需启用**：`GormQuery.Cache()` 通过 context 注入缓存配置，`gormCacher`（`caches.Cacher` 实现）据此判断——未调用 `Cache()` 的查询不读写缓存，零副作用
- **存储复用框架 Cache**：底层使用 `contracts.Cache`（memory/redis store），缓存值以 `caches.Query[any]` 序列化（JSON）存储，key 由 SQL + 参数自动生成
- **写操作自动失效**：插件注册于 Query/Create/Update/Delete callback，写操作调用 `Invalidate`，通过框架 Tags 机制（`gorm:query:cache` 标签）批量清除，不影响其他业务缓存
- **自定义标签**：`CacheTagsOption` 附加业务标签，可用 `cache.Tags("users").Flush()` 手动失效

实现清单：
- [x] `contracts/query.go` — 新增 `CacheOption`/`CacheConfig`/`NewCacheConfig` 及 `Query.Cache()` 接口、`QueryCacher` 可选接口
- [x] `database/drivers/gormdriver/cacher.go` — 实现 `gormCacher` 与 `GormDriver.EnableCaches()`（注册 go-gorm/caches 插件）
- [x] `database/drivers/gormdriver/query.go` — 实现 `GormQuery.Cache()`（context 标记按需启用）
- [x] `database/manager.go` — 新增 `dbManager.UseQueryCache()` 遍历连接启用
- [x] `database/service_provider.go` — Boot 阶段按配置 `database.cache.enabled: true` 注入 cache 服务
- [x] `database/drivers/gormdriver/cacher_test.go` — 命中/未命中、写操作失效、TTL 与标签、未启用插件无副作用

### 查询缓存 · 兼容性实测结论（2026-08-19）

使用 SQLite 实测 `Query().Cache()` 与各类查询的兼容性：

| 场景 | 结果 | 说明 |
|------|------|------|
| `First/Find/Count/Pluck/Exists` | ✅ | 缓存命中返回旧值，写操作失效正常 |
| `Preload/Joins` 关联查询 | ✅ | 关联子查询继承缓存标记，同样被缓存，行为一致 |
| `Transaction` 事务内缓存 | ✅ | 事务内 `Cache()` 正常 |
| `FindInBatches` | ✅ | 分批查询各自缓存 |
| `ScanMap` | ✅ | 需先 `Model`/`Table`（GORM 语义） |
| `Row()` / `Rows()` 游标 | 🔧 已修复 | 命中缓存短路会返回空游标导致 **panic / "unsupported data type"**，现已内部剥离缓存标记（`withoutCache`） |
| `Lock(FOR UPDATE/SHARE)` | 🔧 已处理 | 悲观锁命中缓存会跳过 DB 执行、**锁失效**，现已自动剥离缓存标记 |
| `FirstOrCreate` / `FirstOrInit` | ✅ 已修复 | 原不触发 `AutoGenerateID`、ID 为空，已为 `database.Model` 实现 gorm `BeforeCreate` 钩子修复。注意：`Where` 条件不会自动写入 dest，创建时需在 dest 预填字段值 |

补充：`FirstOrCreate` 需 dest 预填创建值（或 gorm `Attrs`），否则插入空字段——这是 GORM 语义，与缓存无关。

配置示例（`config/config.yaml`）：

```yaml
database:
  cache:
    enabled: true    # 启用查询缓存插件（默认 false）
```

### 暂缓（后续迭代）
- [ ] `plugins/gofast-xorm/` — xorm Driver & Query 实现
- [ ] `plugins/gofast-torm/` — torm Driver & Query 实现
- [ ] `framework/database/drivers/gorm/migrator.go` — 集成 goose 版本化迁移
- [ ] `go.work` — 创建 workspace 管理多模块
- [ ] 读写分离路由（`Read()`/`Write()`）
- [ ] `DBStats` 连接池状态接口
- [ ] OpenTelemetry 集成
