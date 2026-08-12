package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/spf13/viper"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// 编译期保证 configImpl 实现了 contracts.Config 接口。
var _ contracts.Config = (*configImpl)(nil)

// ── 全局缓冲区（供 init 阶段注册 Go 配置使用）────────────────

// pendingAdd 保存一次 init 阶段的 Add 调用。
type pendingAdd struct {
	namespace string
	config    map[string]any
}

var (
	addMu       sync.RWMutex
	pendingAdds []pendingAdd
)

// Add 注册命名空间配置。
// 供项目根 config/ 包的 init() 函数调用。
// init 阶段 Config 实例尚未创建，配置暂存到 pendingAdds 缓冲区，
// 在 ServiceProvider.Register 中统一写入实例。
func Add(namespace string, config map[string]any) {
	addMu.Lock()
	defer addMu.Unlock()
	pendingAdds = append(pendingAdds, pendingAdd{namespace, cloneConfigMap(config)})
}

// GetRegistry 返回当前已注册的命名空间配置副本（用于测试）。
func GetRegistry() map[string]map[string]any {
	addMu.RLock()
	defer addMu.RUnlock()
	result := make(map[string]map[string]any, len(pendingAdds))
	for _, pa := range pendingAdds {
		result[pa.namespace] = cloneConfigMap(pa.config)
	}
	return result
}

// applyPendingAdds 将 init 阶段注册的 Go 配置写入实例。
// 缓冲区只追加、不清空——重复应用是幂等的（SetDefault 同值覆盖）。
func applyPendingAdds(c contracts.Config) {
	addMu.RLock()
	adds := append([]pendingAdd(nil), pendingAdds...)
	addMu.RUnlock()
	for _, pa := range adds {
		c.Add(pa.namespace, pa.config)
	}
}

// configImpl 实现 contracts.Config 接口，包装 viper。
type configImpl struct {
	viper *viper.Viper
	mu    sync.RWMutex // 保护 Set/SetDefaults/Add 与 Get 系列之间的并发读写
}

// NewConfig 创建 Config 实例。
// path 指向的 YAML 为可选覆盖层：文件不存在时跳过加载，仅依赖 Go 配置默认值；
// 文件存在但无法读取/解析时仍返回错误。
func NewConfig(path string) (contracts.Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		if isConfigFileMissing(err) {
			return &configImpl{viper: v}, nil
		}
		return nil, fmt.Errorf("[GoFast] 读取配置文件失败: %w", err)
	}
	return &configImpl{viper: v}, nil
}

// isConfigFileMissing 判断是否为「配置文件不存在」（可选加载场景）。
// SetConfigFile 时 viper 通常返回 os.ErrNotExist；未指定绝对路径时可能是 ConfigFileNotFoundError。
func isConfigFileMissing(err error) bool {
	if err == nil {
		return false
	}
	var notFound viper.ConfigFileNotFoundError
	if errors.As(err, &notFound) {
		return true
	}
	return errors.Is(err, os.ErrNotExist)
}

// Env 读取操作系统环境变量，支持默认值。
// 与 Get 系列不同，Env 直接读取 os.Getenv，不经过配置文件。
func (c *configImpl) Env(key string, defaultValue ...any) any {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

func (c *configImpl) Get(key string, defaultValue ...any) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return c.viper.Get(key)
}

func (c *configImpl) GetString(key string, defaultValue ...string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return c.viper.GetString(key)
}

func (c *configImpl) GetInt(key string, defaultValue ...int) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return c.viper.GetInt(key)
}

func (c *configImpl) GetBool(key string, defaultValue ...bool) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return c.viper.GetBool(key)
}

func (c *configImpl) GetFloat64(key string, defaultValue ...float64) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return c.viper.GetFloat64(key)
}

func (c *configImpl) GetStringSlice(key string, defaultValue ...[]string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return c.viper.GetStringSlice(key)
}

func (c *configImpl) GetStringMap(key string, defaultValue ...map[string]any) map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.viper.IsSet(key) {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}
	val := c.viper.Get(key)
	if m, ok := toStringAnyMap(val); ok {
		return m
	}
	return nil
}

// Set 运行时设置配置值（不持久化到文件）。
func (c *configImpl) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.viper.Set(key, value)
}

// SetDefaults 批量设置默认值，底层调用 viper.SetDefault。
// 仅在用户未通过配置文件或 Set() 明确设置时生效，不会覆盖已有配置。
func (c *configImpl) SetDefaults(defaults map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, val := range defaults {
		c.viper.SetDefault(key, val)
	}
}

// Add 以命名空间注册配置，递归展开嵌套 map 为点号键逐个写入 viper 默认值层。
// 必须逐叶键展开：v.SetDefault("ns", map) 会整体替换，同命名空间多次 Add 时
// 前一次的键将丢失。展开后每个叶键独立 SetDefault，保证逐键合并。
func (c *configImpl) Add(namespace string, config map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	setDefaultMap(c.viper, namespace, config)
}

// setDefaultMap 递归展开嵌套 map 为 "prefix.key" 点号键写入 viper 默认值层。
func setDefaultMap(v *viper.Viper, prefix string, m map[string]any) {
	for k, val := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		setDefaultValue(v, key, val)
	}
}

// setDefaultValue 写入单个配置值；map 类型递归展开为点号键。
func setDefaultValue(v *viper.Viper, key string, val any) {
	if nested, ok := toStringAnyMap(val); ok {
		for k, subVal := range nested {
			setDefaultValue(v, key+"."+k, subVal)
		}
		return
	}
	v.SetDefault(key, val)
}

// toStringAnyMap 将任意 map 类型转为 map[string]any；非 map 返回 false。
func toStringAnyMap(val any) (map[string]any, bool) {
	if val == nil {
		return nil, false
	}
	rv := reflect.ValueOf(val)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Map {
		return nil, false
	}
	out := make(map[string]any, rv.Len())
	for _, key := range rv.MapKeys() {
		var k string
		if key.Kind() == reflect.String {
			k = key.String()
		} else {
			k = fmt.Sprint(key.Interface())
		}
		out[k] = rv.MapIndex(key).Interface()
	}
	return out, true
}

// cloneConfigMap 深拷贝配置 map（map 递归克隆，其余值按引用复制）。
func cloneConfigMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneConfigValue(v)
	}
	return out
}

func cloneConfigValue(v any) any {
	if nested, ok := toStringAnyMap(v); ok {
		return cloneConfigMap(nested)
	}
	return v
}
