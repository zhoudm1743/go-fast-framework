package radar

import (
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// getDuration 读取 Duration 字符串配置（如 "60s"），缺省或非法返回默认值。
func getDuration(cfg contracts.Config, key string, def time.Duration) time.Duration {
	s := cfg.GetString(key)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
