package contracts

import (
	"context"
	"database/sql"
	"errors"
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

// ── DB 数据库管理器接口 ──────────────────────────────────────────────

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
	// Register 在运行时动态注册一个命名连接（多租户场景使用）。
	// 若同名连接已存在，则关闭旧连接并替换。
	Register(name string, cfg ConnectionConfig) error
}
