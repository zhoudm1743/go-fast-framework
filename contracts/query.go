package contracts

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ── Sentinel Errors ──────────────────────────────────────────────────

var (
	// ErrRecordNotFound 查询无结果（对应 GORM ErrRecordNotFound / sql.ErrNoRows）
	ErrRecordNotFound = errors.New("record not found")
	// ErrDuplicatedKey 唯一约束冲突（INSERT/UPDATE 时重复键）
	ErrDuplicatedKey = errors.New("duplicated key")
	// ErrInvalidTransaction 在无效事务上执行操作
	ErrInvalidTransaction = errors.New("invalid transaction")
	// ErrDeadlock 检测到死锁
	ErrDeadlock = errors.New("deadlock detected")
	// ErrQueryTimeout 查询超时
	ErrQueryTimeout = errors.New("query timeout")
	// ErrConnFailed 连接失败
	ErrConnFailed = errors.New("connection failed")
	// ErrUnsupported 当前驱动不支持该操作
	ErrUnsupported = errors.New("operation not supported by driver")
)

// ── LockMode ─────────────────────────────────────────────────────────

// LockMode 锁定模式
type LockMode int

const (
	LockNone      LockMode = iota
	LockForUpdate          // SELECT ... FOR UPDATE（悲观写锁）
	LockShareMode          // SELECT ... LOCK IN SHARE MODE（悲观读锁）
)

// ── Result ───────────────────────────────────────────────────────────

// Result 写操作执行结果
type Result struct {
	RowsAffected int64
	Error        error
}

// IsZeroRow 执行成功但未影响任何行（如 UPDATE WHERE 无命中）
func (r Result) IsZeroRow() bool {
	return r.Error == nil && r.RowsAffected == 0
}

// ── TxOption ─────────────────────────────────────────────────────────

// TxOption 事务选项（隔离级别等），各驱动自行实现
type TxOption interface{}

// StandardTxOptions 标准事务选项，封装 sql.TxOptions。
// 各驱动负责将此类型转换为底层 ORM 的事务选项。
type StandardTxOptions struct {
	// Isolation 事务隔离级别
	Isolation sql.IsolationLevel
	// ReadOnly 是否为只读事务
	ReadOnly bool
}

// TxOpts 快捷构造函数
func TxOpts(isolation sql.IsolationLevel, readOnly ...bool) TxOption {
	ro := false
	if len(readOnly) > 0 {
		ro = readOnly[0]
	}
	return &StandardTxOptions{Isolation: isolation, ReadOnly: ro}
}

// 预定义快捷选项
var (
	// TxReadCommitted READ COMMITTED 隔离级别
	TxReadCommitted = TxOpts(sql.LevelReadCommitted)
	// TxRepeatableRead REPEATABLE READ 隔离级别（MySQL 默认）
	TxRepeatableRead = TxOpts(sql.LevelRepeatableRead)
	// TxSerializable SERIALIZABLE 最高隔离级别
	TxSerializable = TxOpts(sql.LevelSerializable)
	// TxReadOnly 只读事务
	TxReadOnly = TxOpts(sql.LevelDefault, true)
)

// ── 查询缓存 ────────────────────────────────────────────────────────

// CacheConfig 查询缓存配置。
type CacheConfig struct {
	// Store 使用的缓存存储名（如 "memory"、"redis"）。空串表示框架默认存储。
	Store string
	// TTL 缓存有效期，<=0 表示永不过期。
	TTL time.Duration
	// Tags 额外缓存标签，可通过 cache.Tags(...).Flush() 手动批量失效。
	Tags []string
}

// CacheOption 查询缓存配置选项（函数式选项）。
type CacheOption func(*CacheConfig)

// CacheStoreOption 指定查询缓存使用的缓存存储名。
func CacheStoreOption(name string) CacheOption {
	return func(c *CacheConfig) { c.Store = name }
}

// CacheTTLOption 指定查询缓存有效期。
func CacheTTLOption(ttl time.Duration) CacheOption {
	return func(c *CacheConfig) { c.TTL = ttl }
}

// CacheTagsOption 指定查询缓存附加标签，用于批量手动失效。
func CacheTagsOption(tags ...string) CacheOption {
	return func(c *CacheConfig) { c.Tags = tags }
}

// NewCacheConfig 应用默认值并合并选项，生成缓存配置。
func NewCacheConfig(opts ...CacheOption) CacheConfig {
	cfg := CacheConfig{TTL: 5 * time.Minute}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// ── Row / Rows ───────────────────────────────────────────────────────

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

// ── Query 查询构建器接口 ──────────────────────────────────────────────

// Query 数据库无关的查询构建器。
// 所有链式方法均返回 Query 本身（新实例），终结方法返回 error。
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
	// Update/Updates 的列值支持 contracts.Expr 表达式（跨驱动，X-08）：
	// Update("count", Expr("count + ?", 1)) 生成 "SET count = count + ?" 原子表达式
	//
	// 跨驱动统一语义（X-09）：链上 Model(&bean) 且 bean 主键含非零值时，
	// 主键等值条件自动并入更新（与链上 Where 取 AND 交集），与 GORM v2 一致。
	// 主键全零 / 无主键 / 仅 Table() 时不附加条件，更新范围完全由链上 Where
	// 决定——链上无 Where 即为全表更新，批量写请务必显式 Where。
	Update(column string, value any) error
	Updates(values any) error
	Delete(value any, conds ...any) error
	Count(count *int64) error
	Scan(dest any) error
	Pluck(column string, dest any) error
	Row() Row
	Rows() (Rows, error)

	// ── 写操作 Result 变体（含 RowsAffected）────────────────
	CreateResult(value any) Result
	UpdateResult(column string, value any) Result
	UpdatesResult(values any) Result
	DeleteResult(value any, conds ...any) Result
	SaveResult(value any) Result

	// ── 原生 SQL ────────────────────────────────────────────
	Raw(sql string, values ...any) Query
	Exec(sql string, values ...any) error
	// ExecResult 执行原生 SQL 写操作并返回受影响行数（X-08）。
	// 需依据行数做业务判定（如配额原子扣减、存在性更新）时使用。
	ExecResult(sql string, values ...any) Result

	// ── 事务 ────────────────────────────────────────────────
	Transaction(fc func(tx Query) error, opts ...TxOption) error
	Begin(opts ...TxOption) Query
	Commit() error
	Rollback() error
	SavePoint(name string) error
	RollbackTo(name string) error

	// ── 分页辅助 ────────────────────────────────────────────
	Paginate(page, size int) Query

	// ── 作用域 ──────────────────────────────────────────────
	Scopes(funcs ...func(Query) Query) Query

	// ── 上下文 ──────────────────────────────────────────────
	WithContext(ctx context.Context) Query

	// ── 调试 ────────────────────────────────────────────────
	Debug() Query

	// ── Schema 切换（主要用于 PostgreSQL 多 schema 场景）──────
	// 在当前查询上设置 schema，后续的 Model()/Table() 会自动加上该前缀。
	// 连续调用以最后一次为准。
	// 示例：facades.DB().Connection("pg").Schema("analytics").Model(&Event{}).Find(&events)
	Schema(name string) Query

	// GetSchema 获取当前查询上下文的 schema 名称。
	// 多 schema 场景（如 PostgreSQL 多租户）下用于原生 SQL 拼接完整表名。
	// 无 schema 上下文时返回空字符串。
	GetSchema() string

	// ── 查询缓存（需连接启用缓存插件后生效）──────────────────
	// 对当前查询链开启结果缓存。缓存键由 SQL 与参数自动生成，
	// 命中时直接返回缓存结果，不再访问数据库。
	// 写操作（Create/Update/Delete/Save）会自动失效全部查询缓存。
	// 示例：facades.DB().Query().Cache(CacheTTLOption(1*time.Minute)).Where(...).Find(&list)
	Cache(opts ...CacheOption) Query

	// ── 悲观锁 ──────────────────────────────────────────────
	Lock(mode LockMode) Query

	// ── 软删除扩展 ──────────────────────────────────────────
	Unscoped() Query
	OnlyTrashed() Query
	Restore() error
	ForceDelete(value any, conds ...any) error

	// ── 高级查询 ────────────────────────────────────────────
	FirstOrCreate(dest any, conds ...any) error
	FirstOrInit(dest any, conds ...any) error
	FindInBatches(dest any, batchSize int, fc func(tx Query, batch int) error) error
	ScanMap(dest *[]map[string]any) error
	Exists(dest any, conds ...any) (bool, error)
}

// ── Driver 驱动适配器接口 ────────────────────────────────────────────

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

// QueryCacher 驱动可选的查询缓存启用接口。
// 由数据库管理器在框架初始化时调用，为所有连接启用查询缓存插件
// （底层使用框架 Cache 服务存储，仅 Query().Cache() 显式开启的查询生效）。
type QueryCacher interface {
	EnableCaches(cache Cache) error
}

// ── DB 数据库管理器接口 ──────────────────────────────────────────────

// DB 数据库管理器，管理多个命名连接。
// 通过 facades.DB() 获取。
type DB interface {
	// Query 在默认连接上返回查询构建器
	Query(ctx ...context.Context) Query
	// Tenant 在默认连接上返回已自动设置好 schema 的查询构建器。
	// 等价于 Query().Schema(TenantResolver.SchemaFromCtx(ctx))。
	// 业务层推荐直接使用此方法，避免重复的样板代码。
	Tenant(ctx Context) Query
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
	// Register 在运行时动态注册一个命名连接（多租户场景使用）。
	// 若同名连接已存在，则关闭旧连接并替换。
	Register(name string, cfg ConnectionConfig) error
}

// ── SQL 表达式（跨驱动，X-08）─────────────────────────────────────────

// SQLExpression SQL 表达式值的驱动识别接口。
// 由 Expr 返回的值实现；各驱动在 Update/Updates 的列值位置检测该接口，
// 将表达式交给底层 ORM 渲染（gormdriver → gorm.Expr，xormdriver → builder
// 组装），实现 "SET col = col + ?" 等数据库端原子表达式。
type SQLExpression interface {
	// ExprSQL 返回表达式 SQL 与绑定参数
	ExprSQL() (query string, args []any)
}

// sqlExpr SQL 表达式值（unexported：只能经 Expr 创建，驱动经接口识别）。
type sqlExpr struct {
	query string
	args  []any
}

var _ SQLExpression = (*sqlExpr)(nil)

func (e *sqlExpr) ExprSQL() (string, []any) { return e.query, e.args }

// Expr 创建 SQL 表达式值，用于 Update/Updates 的列值位置，由数据库执行表达式
// 而非绑定常量。签名与 gorm.Expr 对齐，业务代码零成本迁移：
//
//	q.Model(&Quota{}).Where("id = ?", id).Update("used_bytes", Expr("used_bytes + ?", n))
//	q.Table("quotas").Updates(map[string]any{"used_bytes": Expr("GREATEST(used_bytes - ?, 0)", n)})
//
// 说明：表达式 SQL 由调用方保证可信（列名/字面量拼接有注入风险，参数一律用 ?）。
// 驱动差异：gormdriver 完整支持；xormdriver 支持 Update/Updates 中的表达式列
// （经 builder 组装，链上 Where/Where 变参条件生效），链上 Limit/Offset 不参与 UPDATE。
func Expr(query string, args ...any) SQLExpression {
	return &sqlExpr{query: query, args: args}
}
