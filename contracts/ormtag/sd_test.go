package ormtag

// sd_test.go：sd tag（框架托管软删标记，第四个 tag key）解析测试。

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// sdModel 各模式字段（嵌入展开 + 顶层字段混合）。
type sdEmbedMilli struct {
	DeletedAt int64 `orm:"'deleted_at' index default(0)" sd:"milli"`
}

type sdModel struct {
	ID    string            `orm:"pk varchar(16) 'id'"`
	Embed sdEmbedMilli      `orm:"extends" xorm:"extends"`
	Flag  int64             `sd:"flag"`
	When  *time.Time        `sd:"time"`
	Sec   int64             `sd:""`   // 空值 = 缺省 sec
	Nano  int64             `sd:"nano"`
	Plain int64             `orm:"'plain'"` // 无 sd 标记
	priv  int64             `sd:"sec"`      // 未导出字段跳过
}

func TestSdTagModes(t *testing.T) {
	m, err := Parse(sdModel{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := []struct {
		field string
		mode  SdMode
	}{
		{"DeletedAt", SdModeMilli}, // 嵌入展开叶子（sd 随字段收集，与 orm tag 无关）
		{"Flag", SdModeFlag},
		{"When", SdModeTime},
		{"Sec", SdModeSec},
		{"Nano", SdModeNano},
	}
	for _, c := range cases {
		sd, ok := m.Sd[c.field]
		if !ok {
			t.Fatalf("字段 %s 的 sd 标记未收录", c.field)
		}
		if sd.Mode != c.mode {
			t.Fatalf("字段 %s 模式 = %q，期望 %q", c.field, sd.Mode, c.mode)
		}
	}
	if _, ok := m.Sd["Plain"]; ok {
		t.Fatal("无 sd 标记的字段不应收录")
	}
	if _, ok := m.Sd["priv"]; ok {
		t.Fatal("未导出字段不应收录")
	}
}

func TestSdTagInvalidMode(t *testing.T) {
	st := reflect.StructOf([]reflect.StructField{{
		Name: "DeletedAt",
		Type: reflect.TypeOf(int64(0)),
		Tag:  reflect.StructTag(`sd:"bogus"`),
	}})
	_, err := Parse(reflect.New(st).Interface())
	if err == nil {
		t.Fatal("非法 sd 模式应报错")
	}
	for _, want := range []string{"DeletedAt", "bogus"} {
		if got := err.Error(); !strings.Contains(got, want) {
			t.Fatalf("错误信息应含 %q: %s", want, got)
		}
	}
}

func TestSdTagAbsent(t *testing.T) {
	m, err := Parse(struct {
		ID string `orm:"pk 'id'"`
	}{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(m.Sd) != 0 {
		t.Fatalf("无 sd 字段的模型 Sd 应为空，实际 %v", m.Sd)
	}
}

func TestSdTagTypeMismatch(t *testing.T) {
	// time 模式必须是时间类型
	st := reflect.StructOf([]reflect.StructField{{
		Name: "DeletedAt", Type: reflect.TypeOf(int64(0)), Tag: `sd:"time"`,
	}})
	if _, err := Parse(reflect.New(st).Interface()); err == nil {
		t.Fatal("int64 字段带 sd:\"time\" 应报错")
	}
	// 整数模式必须是整数类型
	st2 := reflect.StructOf([]reflect.StructField{{
		Name: "DeletedAt", Type: reflect.TypeOf(time.Time{}), Tag: `sd:"milli"`,
	}})
	if _, err := Parse(reflect.New(st2).Interface()); err == nil {
		t.Fatal("time.Time 字段带 sd:\"milli\" 应报错")
	}
}

func TestSdMetaCondsAndValues(t *testing.T) {
	now := time.Unix(1700000000, 123456789)
	cases := []struct {
		mode      SdMode
		timeBased bool
		aliveCond string
		trashCond string
		aliveVal  any
		delVal    any
	}{
		{SdModeSec, false, "deleted_at = ?", "deleted_at <> ?", int64(0), int64(1700000000)},
		{SdModeMilli, false, "deleted_at = ?", "deleted_at <> ?", int64(0), int64(1700000000123)},
		{SdModeNano, false, "deleted_at = ?", "deleted_at <> ?", int64(0), int64(1700000000123456789)},
		{SdModeFlag, false, "deleted_at = ?", "deleted_at = ?", int64(0), int64(1)},
		{SdModeTime, true, "deleted_at IS NULL", "deleted_at IS NOT NULL", nil, now},
	}
	for _, c := range cases {
		m := SdMeta{Mode: c.mode, TimeBased: c.timeBased}
		cond, args := m.AliveCond("deleted_at")
		if cond != c.aliveCond {
			t.Fatalf("%s AliveCond = %q，期望 %q", c.mode, cond, c.aliveCond)
		}
		if !c.timeBased && (len(args) != 1 || args[0] != int64(0)) {
			t.Fatalf("%s AliveCond args 应为 [0]，实际 %v", c.mode, args)
		}
		tcond, _ := m.TrashedCond("deleted_at")
		if tcond != c.trashCond {
			t.Fatalf("%s TrashedCond = %q，期望 %q", c.mode, tcond, c.trashCond)
		}
		if got := m.AliveValue(); got != c.aliveVal {
			t.Fatalf("%s AliveValue = %v，期望 %v", c.mode, got, c.aliveVal)
		}
		if got := m.DeletedValue(now); got != c.delVal {
			t.Fatalf("%s DeletedValue = %v（%T），期望 %v", c.mode, got, got, c.delVal)
		}
	}
}
