package schedule

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// event 调度事件实现。
type event struct {
	mu          sync.Mutex
	name        string
	cronExpr    string
	callback    func()
	command     string
	kernel      contracts.Fast // 用于执行 Fast 命令（可为 nil）
	running     int32          // 原子标志：是否正在运行
	skipIfRun   bool
	delayIfRun  bool
	onOneServer bool
	cache       contracts.Cache // 用于 OnOneServer 分布式锁（可为 nil）
	log         contracts.Log   // 日志服务
}

func newEvent(callback func(), command string) *event {
	return &event{
		callback: callback,
		command:  command,
	}
}

// run 执行任务，处理并发控制。
func (e *event) run() {
	if e.skipIfRun {
		if !atomic.CompareAndSwapInt32(&e.running, 0, 1) {
			return // 正在运行，跳过
		}
		defer atomic.StoreInt32(&e.running, 0)
	} else if e.delayIfRun {
		for !atomic.CompareAndSwapInt32(&e.running, 0, 1) {
			time.Sleep(100 * time.Millisecond)
		}
		defer atomic.StoreInt32(&e.running, 0)
	}

	if e.onOneServer && e.cache != nil {
		lockKey := "schedule:lock:" + e.name
		lock := e.cache.Lock(lockKey, 10*time.Second)
		if !lock.Acquire() {
			return // 其他服务器正在运行
		}
		defer lock.Release()
	}

	defer func() {
		if r := recover(); r != nil {
			if e.log != nil {
				e.log.Errorf("[GoFast] schedule panic in task %q: %v", e.name, r)
			} else {
				fmt.Printf("[GoFast] schedule panic in task %q: %v\n", e.name, r)
			}
		}
	}()

	if e.callback != nil {
		e.callback()
	} else if e.command != "" {
		if e.kernel == nil {
			fmt.Printf("[GoFast] schedule: kernel not injected for command task %q, skip\n", e.name)
			return
		}
		_ = e.kernel.Call(e.command)
	}
}

// ── ScheduleEvent 接口实现 ───────────────────────────────────────────

func (e *event) Cron(expression string) contracts.ScheduleEvent {
	e.cronExpr = expression
	return e
}

func (e *event) EverySecond() contracts.ScheduleEvent         { return e.Cron("@every 1s") }
func (e *event) EveryTwoSeconds() contracts.ScheduleEvent     { return e.Cron("@every 2s") }
func (e *event) EveryFiveSeconds() contracts.ScheduleEvent    { return e.Cron("@every 5s") }
func (e *event) EveryTenSeconds() contracts.ScheduleEvent     { return e.Cron("@every 10s") }
func (e *event) EveryFifteenSeconds() contracts.ScheduleEvent { return e.Cron("@every 15s") }
func (e *event) EveryTwentySeconds() contracts.ScheduleEvent  { return e.Cron("@every 20s") }
func (e *event) EveryThirtySeconds() contracts.ScheduleEvent  { return e.Cron("@every 30s") }

func (e *event) EveryMinute() contracts.ScheduleEvent         { return e.Cron("* * * * *") }
func (e *event) EveryTwoMinutes() contracts.ScheduleEvent     { return e.Cron("*/2 * * * *") }
func (e *event) EveryThreeMinutes() contracts.ScheduleEvent   { return e.Cron("*/3 * * * *") }
func (e *event) EveryFourMinutes() contracts.ScheduleEvent    { return e.Cron("*/4 * * * *") }
func (e *event) EveryFiveMinutes() contracts.ScheduleEvent    { return e.Cron("*/5 * * * *") }
func (e *event) EveryTenMinutes() contracts.ScheduleEvent     { return e.Cron("*/10 * * * *") }
func (e *event) EveryFifteenMinutes() contracts.ScheduleEvent { return e.Cron("*/15 * * * *") }
func (e *event) EveryThirtyMinutes() contracts.ScheduleEvent  { return e.Cron("*/30 * * * *") }

func (e *event) Hourly() contracts.ScheduleEvent { return e.Cron("0 * * * *") }
func (e *event) HourlyAt(minute int) contracts.ScheduleEvent {
	return e.Cron(fmt.Sprintf("%d * * * *", minute))
}
func (e *event) EveryTwoHours() contracts.ScheduleEvent   { return e.Cron("0 */2 * * *") }
func (e *event) EveryThreeHours() contracts.ScheduleEvent { return e.Cron("0 */3 * * *") }
func (e *event) EveryFourHours() contracts.ScheduleEvent  { return e.Cron("0 */4 * * *") }
func (e *event) EverySixHours() contracts.ScheduleEvent   { return e.Cron("0 */6 * * *") }

func (e *event) Daily() contracts.ScheduleEvent { return e.Cron("0 0 * * *") }
func (e *event) DailyAt(t string) contracts.ScheduleEvent {
	// t 格式 "HH:MM"
	var h, m int
	fmt.Sscanf(t, "%d:%d", &h, &m)
	return e.Cron(fmt.Sprintf("%d %d * * *", m, h))
}

func (e *event) Days(days ...int) contracts.ScheduleEvent {
	if len(days) == 0 {
		return e
	}
	expr := "0 0 * * "
	for i, d := range days {
		if i > 0 {
			expr += ","
		}
		expr += fmt.Sprintf("%d", d%7) // cron: 0=Sun,1=Mon,...,6=Sat,7=Sun
	}
	return e.Cron(expr)
}

func (e *event) Weekdays() contracts.ScheduleEvent   { return e.Cron("0 0 * * 1-5") }
func (e *event) Weekends() contracts.ScheduleEvent   { return e.Cron("0 0 * * 0,6") }
func (e *event) Mondays() contracts.ScheduleEvent    { return e.Cron("0 0 * * 1") }
func (e *event) Tuesdays() contracts.ScheduleEvent   { return e.Cron("0 0 * * 2") }
func (e *event) Wednesdays() contracts.ScheduleEvent { return e.Cron("0 0 * * 3") }
func (e *event) Thursdays() contracts.ScheduleEvent  { return e.Cron("0 0 * * 4") }
func (e *event) Fridays() contracts.ScheduleEvent    { return e.Cron("0 0 * * 5") }
func (e *event) Saturdays() contracts.ScheduleEvent  { return e.Cron("0 0 * * 6") }
func (e *event) Sundays() contracts.ScheduleEvent    { return e.Cron("0 0 * * 0") }
func (e *event) Weekly() contracts.ScheduleEvent     { return e.Cron("0 0 * * 0") }
func (e *event) Monthly() contracts.ScheduleEvent    { return e.Cron("0 0 1 * *") }
func (e *event) Quarterly() contracts.ScheduleEvent  { return e.Cron("0 0 1 1,4,7,10 *") }
func (e *event) Yearly() contracts.ScheduleEvent     { return e.Cron("0 0 1 1 *") }

func (e *event) SkipIfStillRunning() contracts.ScheduleEvent {
	e.skipIfRun = true
	return e
}

func (e *event) DelayIfStillRunning() contracts.ScheduleEvent {
	e.delayIfRun = true
	return e
}

func (e *event) OnOneServer() contracts.ScheduleEvent {
	e.onOneServer = true
	return e
}

func (e *event) Name(name string) contracts.ScheduleEvent {
	e.name = name
	return e
}

func (e *event) GetName() string { return e.name }
func (e *event) GetCron() string { return e.cronExpr }
