package contracts

import "fmt"

// ── ConnectionConfig ─────────────────────────────────────────────────

// ConnectionConfig 单个数据库连接的完整配置。
// 定义在 contracts 包中，以便 database 包和各 ORM 驱动包均可引用，
// 避免 database ↔ gormdriver 之间的循环依赖。
type ConnectionConfig struct {
	// ── 驱动与引擎 ──────────────────────────────────────────
	Driver string `mapstructure:"driver"` // ORM 驱动：gormdriver | xorm | torm
	Engine string `mapstructure:"engine"` // 数据库引擎：mysql | postgres | sqlite | mssql

	// ── 直连 DSN（优先级高于以下分项配置）──────────────────
	DSN string `mapstructure:"dsn"`

	// ── 分项连接配置 ────────────────────────────────────────
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	Charset  string `mapstructure:"charset"`  // 默认 utf8mb4（MySQL）
	Loc      string `mapstructure:"loc"`      // 时区，默认 Local
	SSLMode  string `mapstructure:"ssl_mode"` // postgres: disable|require|verify-full

	// ── Schema（主要用于 PostgreSQL）──────────────────────────
	// Schema 设置 PostgreSQL 的 search_path（同时作为 GORM NamingStrategy.TablePrefix 的 schema 前缀）。
	// 其他引擎忽略此字段。
	// 示例："public"、"analytics"、"tenant_001"
	Schema string `mapstructure:"schema"`

	// ── 表前缀 ──────────────────────────────────────────────
	TablePrefix string `mapstructure:"table_prefix"`

	// ── 连接池 ──────────────────────────────────────────────
	MaxIdleConns    int `mapstructure:"max_idle_conns"`     // 默认 10
	MaxOpenConns    int `mapstructure:"max_open_conns"`     // 默认 100
	ConnMaxLifetime int `mapstructure:"conn_max_lifetime"`  // 分钟，默认 60
	ConnMaxIdleTime int `mapstructure:"conn_max_idle_time"` // 分钟，默认 30

	// ── 日志与监控 ──────────────────────────────────────────
	LogLevel      string `mapstructure:"log_level"`      // error|warn|info|silent
	SlowThreshold int    `mapstructure:"slow_threshold"` // ms，默认 200
}

// BuildDSN 根据分项配置生成 DSN 字符串（DSN 字段非空时直接返回）。
func (c *ConnectionConfig) BuildDSN() string {
	if c.DSN != "" {
		return c.DSN
	}

	charset := c.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	loc := c.Loc
	if loc == "" {
		loc = "Local"
	}

	switch c.Engine {
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=%s",
			c.Username, c.Password, c.Host, c.Port, c.Database, charset, loc)
	case "postgres":
		sslMode := c.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		// PostgreSQL 不支持 "Local" 作为 TimeZone 值，使用真实时区名
		pgTimeZone := c.Loc
		if pgTimeZone == "" || pgTimeZone == "Local" {
			pgTimeZone = "Asia/Shanghai"
		}
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			c.Host, c.Port, c.Username, c.Password, c.Database, sslMode, pgTimeZone)
		if c.Schema != "" {
			dsn += " search_path=" + c.Schema
		}
		return dsn
	case "sqlite", "sqlite3":
		return fmt.Sprintf("file:%s?cache=shared&_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000",
			c.Database)
	case "mssql":
		return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
			c.Username, c.Password, c.Host, c.Port, c.Database)
	default:
		return ""
	}
}

// ApplyDefaults 为未设置的配置项填充默认值。
func (c *ConnectionConfig) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "gormdriver"
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 10
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 100
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = 60
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 30
	}
	if c.SlowThreshold == 0 {
		c.SlowThreshold = 200
	}
}

// ── Model Hook 接口 ──────────────────────────────────────────────────
// 各 ORM 驱动在执行相应操作前后，检测目标模型是否实现这些接口并调用。
// 方法名统一使用 On 前缀（OnBeforeCreate/OnAfterCreate 等），避免与 GORM/xorm 等
// ORM 框架的内置 Hook 方法名冲突，消除签名不匹配警告。

// IDAutoGenerator 主键自动生成接口。
// database.Model 实现此接口以生成 UUID v7 主键。
// 驱动层在 Create 前调用 AutoGenerateID()，独立于 ORM 框架，方法名不与任何 ORM 的
// 内置 Hook 冲突（GORM 只识别 BeforeCreate(*gorm.DB)，不会扫描此方法）。
type IDAutoGenerator interface {
	AutoGenerateID()
}

// BeforeCreator 创建前钩子
type BeforeCreator interface {
	OnBeforeCreate(q Query) error
}

// AfterCreator 创建后钩子
type AfterCreator interface {
	OnAfterCreate(q Query) error
}

// BeforeUpdater 更新前钩子
type BeforeUpdater interface {
	OnBeforeUpdate(q Query) error
}

// AfterUpdater 更新后钩子
type AfterUpdater interface {
	OnAfterUpdate(q Query) error
}

// BeforeDeleter 删除前钩子
type BeforeDeleter interface {
	OnBeforeDelete(q Query) error
}

// AfterDeleter 删除后钩子
type AfterDeleter interface {
	OnAfterDelete(q Query) error
}

// AfterFinder 查询后钩子
type AfterFinder interface {
	OnAfterFind(q Query) error
}
