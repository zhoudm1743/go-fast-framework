package schedule

import (
	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/foundation"
)

// ServiceProvider 任务调度服务提供者。
type ServiceProvider struct{}

func (sp *ServiceProvider) Register(app foundation.Application) {
	app.Singleton("schedule", func(app foundation.Application) (any, error) {
		return New(), nil
	})
}

func (sp *ServiceProvider) Boot(app foundation.Application) error {
	s := app.MustMake("schedule").(*Scheduler)

	// 注入缓存（供 OnOneServer 分布式锁使用）
	s.SetCache(app.MustMake("cache").(contracts.Cache))

	// 注入 Fast 内核：Command() 调度最终通过 contracts.Fast.Call() 执行 Artisan 命令
	s.SetKernel(app.MustMake("fast").(contracts.Fast))

	return nil
}

// RegisterSchedule 在引导完成后注册调度任务并启动调度器。
// 通常在 bootstrap/app.go 的 Boot() 函数中调用。
func RegisterSchedule(app foundation.Application, events []contracts.ScheduleEvent) error {
	s := app.MustMake("schedule").(*Scheduler)
	s.RegisterEvents(events)
	return s.Start()
}
