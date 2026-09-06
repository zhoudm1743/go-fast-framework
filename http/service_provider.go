package http

import (
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
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
// 本 Provider 注册 validator / session / view / route 管理逻辑，
// 不内置任何 HTTP 引擎驱动。业务需额外注册驱动插件，例如：
//
//	import gofastgin "github.com/zhoudm1743/gofast-gin"
//	app.SetProviders(append(providers, &gofastgin.ServiceProvider{}))
//
// 或：
//
//	import gofastfiber "github.com/zhoudm1743/gofast-fiber"
//	app.SetProviders(append(providers, &gofastfiber.ServiceProvider{}))
//
// 通过配置 server.driver（gin | fiber）选择已注册的驱动，默认为 fiber。
//
// 可选 HTML 模板渲染：在 config.yaml 中配置 view.dir 即可启用。
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
		factory, err := mustRouteFactory(driver)
		if err != nil {
			return nil, err
		}
		r, err := factory(cfg, validator, storage, log)
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
