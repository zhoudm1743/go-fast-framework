package database

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// DriverFactory 根据连接配置创建 Driver。
type DriverFactory func(cfg ConnectionConfig, log contracts.Log) (contracts.Driver, error)

var (
	driverFactoriesMu sync.RWMutex
	driverFactories   = map[string]DriverFactory{}
)

// RegisterDriver 由插件的 ServiceProvider 在启动时调用，注册 ORM 驱动工厂。
func RegisterDriver(name string, f DriverFactory) {
	driverFactoriesMu.Lock()
	defer driverFactoriesMu.Unlock()
	driverFactories[name] = f
}

func getDriverFactory(name string) (DriverFactory, bool) {
	driverFactoriesMu.RLock()
	defer driverFactoriesMu.RUnlock()
	f, ok := driverFactories[name]
	return f, ok
}

type dbManager struct {
	cfg         contracts.Config
	log         contracts.Log
	defaultConn string
	connConfigs map[string]ConnectionConfig
	connections map[string]contracts.Driver
	mu          sync.RWMutex
}

var _ contracts.DB = (*dbManager)(nil)

// NewDBManager 创建数据库管理器实例，自动检测新/旧配置格式，
// 并预先创建所有配置的数据库连接。若任一连接创建失败，返回错误，
// 确保启动时即发现连接问题。
func NewDBManager(cfg contracts.Config, log contracts.Log) (contracts.DB, error) {
	m := &dbManager{
		cfg:         cfg,
		log:         log,
		connConfigs: make(map[string]ConnectionConfig),
		connections: make(map[string]contracts.Driver),
	}
	if err := m.parseConfig(); err != nil {
		return nil, err
	}
	// 预先创建所有配置的连接，启动时即发现问题
	for name := range m.connConfigs {
		if _, err := m.getOrCreateDriver(name); err != nil {
			return nil, fmt.Errorf("[GoFast] 数据库连接 %q 初始化失败: %w", name, err)
		}
	}
	return m, nil
}

func (m *dbManager) parseConfig() error {
	connMap := m.cfg.GetStringMap("database.connections")
	if len(connMap) > 0 {
		m.defaultConn = m.cfg.GetString("database.default", "main")
		for name := range connMap {
			cc := m.readConnectionConfig("database.connections." + name)
			cc.ApplyDefaults()
			m.connConfigs[name] = cc
		}
	} else {
		m.defaultConn = "main"
		cc := m.readLegacyConfig()
		cc.ApplyDefaults()
		m.connConfigs["main"] = cc
	}
	return nil
}

func (m *dbManager) readConnectionConfig(prefix string) ConnectionConfig {
	return ConnectionConfig{
		Driver:          m.cfg.GetString(prefix+".driver", "gormdriver"),
		Engine:          m.cfg.GetString(prefix+".engine", "sqlite"),
		DSN:             m.cfg.GetString(prefix + ".dsn"),
		Host:            m.cfg.GetString(prefix+".host", "localhost"),
		Port:            m.cfg.GetInt(prefix+".port", 0),
		Username:        m.cfg.GetString(prefix + ".username"),
		Password:        m.cfg.GetString(prefix + ".password"),
		Database:        m.cfg.GetString(prefix + ".database"),
		Schema:          m.cfg.GetString(prefix + ".schema"),
		Charset:         m.cfg.GetString(prefix + ".charset"),
		Loc:             m.cfg.GetString(prefix + ".loc"),
		SSLMode:         m.cfg.GetString(prefix + ".ssl_mode"),
		TablePrefix:     m.cfg.GetString(prefix + ".table_prefix"),
		MaxIdleConns:    m.cfg.GetInt(prefix+".max_idle_conns", 0),
		MaxOpenConns:    m.cfg.GetInt(prefix+".max_open_conns", 0),
		ConnMaxLifetime: m.cfg.GetInt(prefix+".conn_max_lifetime", 0),
		ConnMaxIdleTime: m.cfg.GetInt(prefix+".conn_max_idle_time", 0),
		LogLevel:        m.cfg.GetString(prefix + ".log_level"),
		SlowThreshold:   m.cfg.GetInt(prefix+".slow_threshold", 0),
	}
}

func (m *dbManager) readLegacyConfig() ConnectionConfig {
	engine := m.cfg.GetString("database.driver", "sqlite")
	return ConnectionConfig{
		Driver:          "gormdriver",
		Engine:          engine,
		Host:            m.cfg.GetString("database.host", "localhost"),
		Port:            m.cfg.GetInt("database.port", 0),
		Username:        m.cfg.GetString("database.username"),
		Password:        m.cfg.GetString("database.password"),
		Database:        m.cfg.GetString("database.database"),
		Loc:             m.cfg.GetString("database.loc"),
		MaxIdleConns:    m.cfg.GetInt("database.max_idle_conns", 0),
		MaxOpenConns:    m.cfg.GetInt("database.max_open_conns", 0),
		ConnMaxLifetime: m.cfg.GetInt("database.conn_max_lifetime", 0),
		ConnMaxIdleTime: m.cfg.GetInt("database.conn_max_idle_time", 0),
		LogLevel:        m.cfg.GetString("database.log_level"),
		SlowThreshold:   m.cfg.GetInt("database.slow_threshold", 0),
	}
}

func (m *dbManager) getOrCreateDriver(name string) (contracts.Driver, error) {
	m.mu.RLock()
	drv, ok := m.connections[name]
	m.mu.RUnlock()
	if ok {
		return drv, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if drv, ok = m.connections[name]; ok {
		return drv, nil
	}

	cc, exists := m.connConfigs[name]
	if !exists {
		return nil, fmt.Errorf("[GoFast] 数据库连接 %q 未配置", name)
	}

	factory, ok := getDriverFactory(cc.Driver)
	if !ok {
		return nil, fmt.Errorf("[GoFast] 数据库驱动 %q 未注册（连接 %q）", cc.Driver, name)
	}

	drv, err := factory(cc, m.log)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] 数据库连接 %q 初始化失败: %w", name, err)
	}

	m.connections[name] = drv
	return drv, nil
}

func (m *dbManager) Query(ctx ...context.Context) contracts.Query {
	return mustDriver(m.getOrCreateDriver(m.defaultConn)).Query(ctx...)
}

// Tenant 返回已按当前请求自动设置好 schema 的查询构建器。
// 等价于 Query().Schema(TenantResolver.SchemaFromCtx(ctx))。
// 业务层推荐直接使用此方法，避免重复的样板代码。
func (m *dbManager) Tenant(ctx contracts.Context) contracts.Query {
	var schema string
	if resolver := contracts.GlobalTenantResolver(); resolver != nil {
		schema = resolver.SchemaFromCtx(ctx)
	}
	return m.Query().Schema(schema)
}

func (m *dbManager) Connection(name string) contracts.Query {
	return mustDriver(m.getOrCreateDriver(name)).Query()
}

func (m *dbManager) Driver(name ...string) contracts.Driver {
	connName := m.defaultConn
	if len(name) > 0 {
		connName = name[0]
	}
	return mustDriver(m.getOrCreateDriver(connName))
}

func (m *dbManager) Transaction(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error {
	drv, err := m.getOrCreateDriver(m.defaultConn)
	if err != nil {
		return err
	}
	return drv.Query().Transaction(fc, opts...)
}

func (m *dbManager) AutoMigrate(models ...any) error {
	drv, err := m.getOrCreateDriver(m.defaultConn)
	if err != nil {
		return err
	}
	return drv.AutoMigrate(models...)
}

func (m *dbManager) Ping() error {
	drv, err := m.getOrCreateDriver(m.defaultConn)
	if err != nil {
		return err
	}
	return drv.Ping()
}

func (m *dbManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, drv := range m.connections {
		if err := drv.Close(); err != nil {
			m.log.Errorf("[GoFast] 关闭数据库连接 %q 失败: %v", name, err)
			errs = append(errs, fmt.Errorf("连接 %q: %w", name, err))
		}
	}
	m.connections = make(map[string]contracts.Driver)
	return errorsJoin(errs...)
}

// mustDriver 在驱动获取失败时 panic，仅用于预配置连接已在启动时验证通过、
// 运行时不应失败的场景。类似 regexp.MustCompile 的语义。
func mustDriver(drv contracts.Driver, err error) contracts.Driver {
	if err != nil {
		panic(fmt.Sprintf("[GoFast] 数据库驱动获取失败: %v", err))
	}
	return drv
}

// errorsJoin 聚合多个错误（兼容 Go 1.20+ 的 errors.Join 语义）。
func errorsJoin(errs ...error) error {
	var nonNil []error
	for _, e := range errs {
		if e != nil {
			nonNil = append(nonNil, e)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	return errors.Join(nonNil...)
}

// Register 在运行时动态注册一个命名连接（多租户场景）。
// 若同名连接已存在，先关闭旧连接再替换。
func (m *dbManager) Register(name string, cfg contracts.ConnectionConfig) error {
	cfg.ApplyDefaults()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭旧连接（如有）
	if old, ok := m.connections[name]; ok {
		_ = old.Close()
		delete(m.connections, name)
	}

	// 存入配置，等待首次使用时懒加载
	m.connConfigs[name] = cfg
	return nil
}
