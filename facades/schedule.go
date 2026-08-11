package facades

import "github.com/zhoudm1743/go-fast-framework/schedule"

// Schedule 获取调度器实例。
func Schedule() *schedule.Scheduler {
	return app.MustMake("schedule").(*schedule.Scheduler)
}
