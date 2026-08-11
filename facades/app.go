package facades

import "github.com/zhoudm1743/go-fast-framework/foundation"

var app foundation.Application

// SetApp 设置全局 Application 实例（框架启动时调用一次）。
func SetApp(a foundation.Application) {
	app = a
}

// App 获取全局 Application 实例。
func App() foundation.Application {
	return app
}
