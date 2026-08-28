package utils

import (
	"fmt"
	"sync"
	"time"
)

var (
	defaultLocation     *time.Location
	defaultLocationOnce sync.Once
)

func defaultLoc() *time.Location {
	defaultLocationOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.FixedZone("CST", 8*3600)
		}
		defaultLocation = loc
	})
	return defaultLocation
}

// TimeUtil 时间工具集（链式）。
var TimeUtil = timeUtil{}

type timeUtil struct{}

// Time 链式时间对象。
type Time struct {
	value time.Time
}

func (r timeUtil) Of(t time.Time) *Time {
	return &Time{value: t.In(defaultLoc())}
}

func (r timeUtil) Now() *Time {
	return &Time{value: time.Now().In(defaultLoc())}
}

func (r timeUtil) FromUnix(sec int64) *Time {
	return &Time{value: time.Unix(sec, 0).In(defaultLoc())}
}

func (r timeUtil) FromUnixMilli(ms int64) *Time {
	return &Time{value: time.UnixMilli(ms).In(defaultLoc())}
}

func (r timeUtil) Parse(s string, layouts ...string) *Time {
	if len(layouts) == 0 {
		layouts = []string{
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02",
			"2006/01/02",
			"15:04:05",
		}
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, defaultLoc()); err == nil {
			return &Time{value: t}
		}
	}
	return &Time{value: time.Time{}}
}

func (t *Time) Time() time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.value
}

func (t *Time) Unix() int64 {
	return t.value.Unix()
}

func (t *Time) UnixMilli() int64 {
	return t.value.UnixMilli()
}

func (t *Time) Format(layout string) string {
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	return t.value.Format(layout)
}

func (t *Time) StartOfDay() *Time {
	y, m, d := t.value.Date()
	t.value = time.Date(y, m, d, 0, 0, 0, 0, t.value.Location())
	return t
}

func (t *Time) EndOfDay() *Time {
	y, m, d := t.value.Date()
	t.value = time.Date(y, m, d, 23, 59, 59, 999999999, t.value.Location())
	return t
}

func (t *Time) StartOfWeek() *Time {
	t.StartOfDay()
	weekday := int(t.value.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	t.value = t.value.AddDate(0, 0, -(weekday - 1))
	return t
}

func (t *Time) EndOfWeek() *Time {
	return t.StartOfWeek().AddDays(6).EndOfDay()
}

func (t *Time) StartOfMonth() *Time {
	y, m, _ := t.value.Date()
	t.value = time.Date(y, m, 1, 0, 0, 0, 0, t.value.Location())
	return t
}

func (t *Time) EndOfMonth() *Time {
	return t.StartOfMonth().AddMonths(1).AddDays(-1).EndOfDay()
}

func (t *Time) StartOfYear() *Time {
	y, _, _ := t.value.Date()
	t.value = time.Date(y, 1, 1, 0, 0, 0, 0, t.value.Location())
	return t
}

func (t *Time) AddDays(n int) *Time {
	t.value = t.value.AddDate(0, 0, n)
	return t
}

func (t *Time) AddMonths(n int) *Time {
	t.value = t.value.AddDate(0, n, 0)
	return t
}

func (t *Time) Timezone(name string) *Time {
	if loc, err := time.LoadLocation(name); err == nil {
		t.value = t.value.In(loc)
	}
	return t
}

func (t *Time) IsToday() bool {
	now := time.Now().In(defaultLoc())
	y1, m1, d1 := t.value.Date()
	y2, m2, d2 := now.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func (t *Time) IsYesterday() bool {
	yesterday := time.Now().In(defaultLoc()).AddDate(0, 0, -1)
	y1, m1, d1 := t.value.Date()
	y2, m2, d2 := yesterday.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func (t *Time) IsSameDay(other time.Time) bool {
	y1, m1, d1 := t.value.Date()
	y2, m2, d2 := other.In(defaultLoc()).Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func (t *Time) Between(start, end time.Time) bool {
	return !t.value.Before(start) && !t.value.After(end)
}

func (t *Time) IsLeap() bool {
	y, _, _ := t.value.Date()
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

func (t *Time) Human() string {
	now := time.Now().In(defaultLoc())
	diff := now.Sub(t.value)
	switch {
	case diff < time.Minute:
		return "刚刚"
	case diff < time.Hour:
		return fmt.Sprintf("%d分钟前", int(diff.Minutes()))
	case diff < 24*time.Hour && t.IsToday():
		return fmt.Sprintf("%d小时前", int(diff.Hours()))
	case t.IsYesterday():
		return "昨天 " + t.value.Format("15:04")
	default:
		return t.value.Format("2006-01-02 15:04")
	}
}

func (t *Time) Diff(other time.Time) time.Duration {
	return t.value.Sub(other)
}

// Today 返回今天起止 unix 秒。
func (r timeUtil) Today() (start, end int64) {
	t := r.Now().StartOfDay()
	s := t.Unix()
	e := t.EndOfDay().Unix()
	return s, e
}

func (r timeUtil) Yesterday() (start, end int64) {
	t := r.Now().AddDays(-1).StartOfDay()
	return t.Unix(), t.EndOfDay().Unix()
}

func (r timeUtil) LastNDays(n int) (start, end int64) {
	end = r.Now().EndOfDay().Unix()
	start = r.Now().AddDays(-n + 1).StartOfDay().Unix()
	return start, end
}

func (r timeUtil) ThisWeek() (start, end int64) {
	t := r.Now().StartOfWeek()
	return t.Unix(), t.EndOfWeek().Unix()
}

func (r timeUtil) ThisMonth() (start, end int64) {
	t := r.Now().StartOfMonth()
	return t.Unix(), t.EndOfMonth().Unix()
}

func (r timeUtil) LastMonth() (start, end int64) {
	t := r.Now().StartOfMonth().AddMonths(-1)
	return t.Unix(), t.EndOfMonth().Unix()
}

func (r timeUtil) Unix() int64 {
	return r.Now().Unix()
}

func (r timeUtil) UnixMilli() int64 {
	return r.Now().UnixMilli()
}
