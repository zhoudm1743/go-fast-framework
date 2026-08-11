package contracts

// Config 配置服务契约。
type Config interface {
	// Env 读取环境变量，支持默认值。
	Env(key string, defaultValue ...any) any
	// Get 读取配置值，支持点号路径（如 "database.host"），支持默认值。
	Get(key string, defaultValue ...any) any
	// GetString 读取字符串配置。
	GetString(key string, defaultValue ...string) string
	// GetInt 读取整数配置。
	GetInt(key string, defaultValue ...int) int
	// GetBool 读取布尔配置。
	GetBool(key string, defaultValue ...bool) bool
	// GetFloat64 读取浮点数配置。
	GetFloat64(key string, defaultValue ...float64) float64
	// GetStringSlice 读取字符串切片配置。
	GetStringSlice(key string, defaultValue ...[]string) []string
	// GetStringMap 读取 map[string]any 配置，支持默认值。
	GetStringMap(key string, defaultValue ...map[string]any) map[string]any
	// Set 运行时设置配置值（不持久化到文件）。
	Set(key string, value any)
	// SetDefaults 批量设置默认配置值（仅在用户未通过配置文件/环境变量配置时生效）。
	// 插件通过实现 foundation.ConfigProvider 接口声明默认值，框架会自动调用此方法。
	// 也可在 ServiceProvider.Register 中手动调用，为插件配置项提供合理的默认值。
	SetDefaults(defaults map[string]any)
	// Add 以命名空间注册配置，写入默认值层。
	// 优先级低于 YAML 配置文件与 Set()：同名键以 config/config.yaml 与运行时 Set 为准。
	// 典型用法：项目根 config/ 包在 init() 中通过此方法注册 Go 配置。
	//
	// 示例：
	//   config.Add("app", map[string]any{"name": "GoFast", "debug": false})
	//   name := facades.Config().GetString("app.name") // "GoFast"
	Add(namespace string, config map[string]any)
}
