package schedule

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

func TestCallEvent(t *testing.T) {
	s := New()
	ev := s.Call(func() {})
	if ev == nil {
		t.Fatal("Call() 返回 nil")
	}
	if ev.GetName() != "" {
		t.Errorf("新事件名称应为空, got %q", ev.GetName())
	}
}

func TestCommandEvent(t *testing.T) {
	s := New()
	ev := s.Command("test:command")
	if ev == nil {
		t.Fatal("Command() 返回 nil")
	}
	if ev.GetName() != "test:command" {
		t.Errorf("command 事件名称应为命令名, got %q", ev.GetName())
	}
}

func TestEventCronExpressions(t *testing.T) {
	tests := []struct {
		name     string
		method   func(contracts.ScheduleEvent) contracts.ScheduleEvent
		expected string
	}{
		{"EveryMinute", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryMinute() }, "* * * * *"},
		{"EveryTwoMinutes", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryTwoMinutes() }, "*/2 * * * *"},
		{"EveryFiveMinutes", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryFiveMinutes() }, "*/5 * * * *"},
		{"EveryTenMinutes", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryTenMinutes() }, "*/10 * * * *"},
		{"EveryThirtyMinutes", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryThirtyMinutes() }, "*/30 * * * *"},
		{"Hourly", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Hourly() }, "0 * * * *"},
		{"Daily", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Daily() }, "0 0 * * *"},
		{"DailyAt13:30", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.DailyAt("13:30") }, "30 13 * * *"},
		{"Weekly", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Weekly() }, "0 0 * * 0"},
		{"Monthly", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Monthly() }, "0 0 1 * *"},
		{"Yearly", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Yearly() }, "0 0 1 1 *"},
		{"Weekdays", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Weekdays() }, "0 0 * * 1-5"},
		{"Weekends", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.Weekends() }, "0 0 * * 0,6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			ev := s.Call(func() {})
			result := tt.method(ev)
			if result.GetCron() != tt.expected {
				t.Errorf("%s: got %q, want %q", tt.name, result.GetCron(), tt.expected)
			}
		})
	}
}

func TestEventEverySeconds(t *testing.T) {
	tests := []struct {
		name     string
		method   func(contracts.ScheduleEvent) contracts.ScheduleEvent
		expected string
	}{
		{"EverySecond", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EverySecond() }, "@every 1s"},
		{"EveryTwoSeconds", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryTwoSeconds() }, "@every 2s"},
		{"EveryFiveSeconds", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryFiveSeconds() }, "@every 5s"},
		{"EveryTenSeconds", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryTenSeconds() }, "@every 10s"},
		{"EveryThirtySeconds", func(e contracts.ScheduleEvent) contracts.ScheduleEvent { return e.EveryThirtySeconds() }, "@every 30s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			ev := s.Call(func() {})
			result := tt.method(ev)
			if result.GetCron() != tt.expected {
				t.Errorf("%s: got %q, want %q", tt.name, result.GetCron(), tt.expected)
			}
		})
	}
}

func TestEventName(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).Name("my-task")
	if ev.GetName() != "my-task" {
		t.Errorf("got %q, want %q", ev.GetName(), "my-task")
	}
}

func TestSkipIfStillRunning(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).SkipIfStillRunning()
	e := ev.(*event)
	if !e.skipIfRun {
		t.Error("SkipIfStillRunning 应设置 skipIfRun = true")
	}
}

func TestDelayIfStillRunning(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).DelayIfStillRunning()
	e := ev.(*event)
	if !e.delayIfRun {
		t.Error("DelayIfStillRunning 应设置 delayIfRun = true")
	}
}

func TestOnOneServer(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).OnOneServer()
	e := ev.(*event)
	if !e.onOneServer {
		t.Error("OnOneServer 应设置 onOneServer = true")
	}
}

func TestNormalizeCron(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"@every 1s", "@every 1s"},
		{"@daily", "@daily"},
		{"* * * * *", "0 * * * * *"},
		{"0 0 * * *", "0 0 0 * * *"},
		{"0 */2 * * *", "0 0 */2 * * *"},
		{"1 2 3 4 5", "0 1 2 3 4 5"},
		{"  * * * * *  ", "0 * * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeCron(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCron(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRunCallback(t *testing.T) {
	var count atomic.Int32
	s := New()
	ev := s.Call(func() { count.Add(1) })
	e := ev.(*event)
	e.run()
	if count.Load() != 1 {
		t.Errorf("回调应执行 1 次, 实际 %d 次", count.Load())
	}
}

func TestRunSkipIfStillRunning(t *testing.T) {
	var count atomic.Int32
	s := New()
	ev := s.Call(func() {
		count.Add(1)
	}).SkipIfStillRunning()
	e := ev.(*event)

	// 模拟并发：设置 running = 1 再调用 run() 应跳过
	atomic.StoreInt32(&e.running, 1)
	e.run()
	if count.Load() != 0 {
		t.Error("SkipIfStillRunning 时不应执行回调")
	}
}

func TestDelayRunBlocking(t *testing.T) {
	var count atomic.Int32
	s := New()
	ev := s.Call(func() {
		count.Add(1)
	}).DelayIfStillRunning()
	e := ev.(*event)

	// 手动设置 running，run() 应等待而非跳过
	atomic.StoreInt32(&e.running, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		atomic.StoreInt32(&e.running, 0)
	}()
	e.run()
	if count.Load() != 1 {
		t.Errorf("DelayIfStillRunning 时回调应最终执行, 实际 %d 次", count.Load())
	}
}

func TestRegisterEvents(t *testing.T) {
	s := New()
	ev1 := s.Call(func() {}).Name("task1").EveryMinute()
	ev2 := s.Call(func() {}).Name("task2").Hourly()
	s.RegisterEvents([]contracts.ScheduleEvent{ev1, ev2})

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List() 应返回 2 个任务, 实际 %d 个", len(list))
	}
}

func TestList(t *testing.T) {
	s := New()
	if len(s.List()) != 0 {
		t.Error("空调度器的 List() 应返回空切片")
	}
}

func TestSetCache(t *testing.T) {
	s := New()
	s.SetCache(nil)
	if s.cache != nil {
		t.Error("SetCache(nil) 后 cache 应为 nil")
	}
}

func TestSetKernel(t *testing.T) {
	s := New()
	s.SetKernel(nil)
	if s.kernel != nil {
		t.Error("SetKernel(nil) 后 kernel 应为 nil")
	}
}

func TestSetLogger(t *testing.T) {
	s := New()
	s.SetLogger(nil)
	if s.log != nil {
		t.Error("SetLogger(nil) 后 log 应为 nil")
	}
}

func TestCron(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).Cron("*/5 * * * *")
	if ev.GetCron() != "*/5 * * * *" {
		t.Errorf("Cron() 未正确设置, got %q", ev.GetCron())
	}
}

func TestDays(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).Days(1, 3, 5)
	// 周一=1, 周三=3, 周五=5 → "0 0 * * 1,3,5"
	if ev.GetCron() != "0 0 * * 1,3,5" {
		t.Errorf("Days(1,3,5): got %q", ev.GetCron())
	}
}

func TestHourlyAt(t *testing.T) {
	s := New()
	ev := s.Call(func() {}).HourlyAt(30)
	if ev.GetCron() != "30 * * * *" {
		t.Errorf("HourlyAt(30): got %q", ev.GetCron())
	}
}
