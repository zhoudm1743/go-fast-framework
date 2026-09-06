package http

import (
	"fmt"
	"sync"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// RouteFactory 根据配置创建 HTTP 路由驱动。
type RouteFactory func(
	cfg contracts.Config,
	validator contracts.Validation,
	storage contracts.Storage,
	log contracts.Log,
) (contracts.Route, error)

var (
	routeFactoriesMu sync.RWMutex
	routeFactories   = map[string]RouteFactory{}
)

// RegisterDriver 由 HTTP 驱动插件的 ServiceProvider 在启动时调用，注册路由驱动工厂。
// name 对应配置 server.driver（如 "gin"、"fiber"）。
func RegisterDriver(name string, f RouteFactory) {
	routeFactoriesMu.Lock()
	defer routeFactoriesMu.Unlock()
	routeFactories[name] = f
}

func getRouteFactory(name string) (RouteFactory, bool) {
	routeFactoriesMu.RLock()
	defer routeFactoriesMu.RUnlock()
	f, ok := routeFactories[name]
	return f, ok
}

// mustRouteFactory 返回已注册的驱动工厂；未注册时返回明确错误。
func mustRouteFactory(name string) (RouteFactory, error) {
	f, ok := getRouteFactory(name)
	if !ok {
		return nil, fmt.Errorf("[GoFast] HTTP 驱动 %q 未注册（请安装并注册 gofast-gin 或 gofast-fiber 插件）", name)
	}
	return f, nil
}
