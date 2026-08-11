package http

import (
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	fiberdriver "github.com/zhoudm1743/go-fast-framework/http/fiber"
	gindriver "github.com/zhoudm1743/go-fast-framework/http/gin"
	gosession "github.com/zhoudm1743/go-fast-framework/http/session"
	validation "github.com/zhoudm1743/go-fast-framework/http/validation"
	goview "github.com/zhoudm1743/go-fast-framework/http/view"
)

// viewSetter 是内部鸭子类型接口，用于向路由注入视图引擎，
// 避免在 contracts.Route 中暴露与 HTML 渲染相关的实现细节。
type viewSetter interface {
	SetViewEngine(contracts.ViewEngine)
}

// ServiceProvider HTTP 路由服务提供者。
// 通过配置 server.driver（fiber | gin）选择底层框架，默认为 fiber。
//
// 可选 HTML 模板渲染：在 config.yaml 中配置 view.dir 即可启用，
// 无需额外注册 ServiceProvider。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	// 注册验证器（在 route 之前，因为 route 依赖 validator）
	app.Singleton("validator", func(app foundation.Application) (any, error) {
		return validation.NewValidator()
	})

	// 注册 Session 管理器
	app.Singleton("session", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		lifetimeSec := cfg.GetInt("session.lifetime", 7200)
		lifetime := time.Duration(lifetimeSec) * time.Second
		cookieName := cfg.GetString("session.cookie", "")
		return gosession.NewManager(lifetime, cookieName), nil
	})

	// 注册视图引擎（仅当 view.dir 已配置时生效）
	app.Singleton("view", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		dir := cfg.GetString("view.dir", "")
		if dir == "" {
			return nil, nil
		}
		ext := cfg.GetString("view.extension", ".html")
		reload := cfg.GetBool("view.reload", false)
		return goview.New(dir,
			goview.WithExtension(ext),
			goview.WithReload(reload),
		), nil
	})

	app.Singleton("route", func(app foundation.Application) (any, error) {
		cfg := app.MustMake("config").(contracts.Config)
		validator := app.MustMake("validator").(contracts.Validation)
		storage := app.MustMake("storage").(contracts.Storage)
		log := app.MustMake("log").(contracts.Log)

		driver := cfg.GetString("server.driver", "fiber")
		var (
			r   contracts.Route
			err error
		)
		switch driver {
		case "gin":
			r, err = gindriver.NewRoute(cfg, validator, storage, log)
		default: // fiber
			r, err = fiberdriver.NewRoute(cfg, validator, storage, log)
		}
		if err != nil {
			return nil, err
		}

		// 如果已注册视图引擎，注入到路由中
		if ve, _ := app.Make("view"); ve != nil {
			if engine, ok := ve.(contracts.ViewEngine); ok {
				if vs, ok := r.(viewSetter); ok {
					vs.SetViewEngine(engine)
				}
			}
		}
		return r, nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	app.OnShutdown(func() {
		if r, err := app.Make("route"); err == nil {
			if closer, ok := r.(contracts.Route); ok {
				_ = closer.Shutdown()
			}
		}
	})
	return nil
}
