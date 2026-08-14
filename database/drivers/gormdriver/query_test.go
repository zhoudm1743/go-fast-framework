package gormdriver

import (
	"errors"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 测试用的内存 SQLite 驱动
func newTestDriver(t *testing.T) *GormDriver {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	return &GormDriver{db: db}
}

// 带模型的测试驱动
func newTestDriverWithTable(t *testing.T) *GormDriver {
	t.Helper()
	drv := newTestDriver(t)
	if err := drv.AutoMigrate(&TestModel{}); err != nil {
		t.Fatalf("自动迁移失败: %v", err)
	}
	return drv
}

// TestModel 测试用模型
type TestModel struct {
	ID   string `gorm:"primaryKey;size:16"`
	Name string `gorm:"size:100"`
}

func (m *TestModel) AutoGenerateID() {
	// 测试中手动设置 ID
}

// ── Exists 测试 ─────────────────────────────────────────────────────

func TestExists_EmptyConds(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	// 空表 + 空条件：不应 panic
	exists, err := q.Exists(&TestModel{})
	if err != nil {
		t.Fatalf("Exists(空表, 无条件) 返回错误: %v", err)
	}
	if exists {
		t.Error("空表 Exists 应返回 false")
	}
}

func TestExists_WithData(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	// 插入数据
	if err := q.Create(&TestModel{ID: "test001", Name: "foo"}); err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	// 无条件：应返回 true
	exists, err := q.Exists(&TestModel{})
	if err != nil {
		t.Fatalf("Exists 返回错误: %v", err)
	}
	if !exists {
		t.Error("有数据时 Exists 应返回 true")
	}
}

func TestExists_WithConds_Match(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "test002", Name: "bar"}); err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	exists, err := q.Exists(&TestModel{}, "name = ?", "bar")
	if err != nil {
		t.Fatalf("Exists 返回错误: %v", err)
	}
	if !exists {
		t.Error("匹配条件时 Exists 应返回 true")
	}
}

func TestExists_WithConds_NoMatch(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "test003", Name: "baz"}); err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	exists, err := q.Exists(&TestModel{}, "name = ?", "nonexistent")
	if err != nil {
		t.Fatalf("Exists 返回错误: %v", err)
	}
	if exists {
		t.Error("不匹配条件时 Exists 应返回 false")
	}
}

// ── ScanMap 测试 ────────────────────────────────────────────────────

func TestScanMap(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	// 插入数据
	if err := q.Create(&TestModel{ID: "sm001", Name: "alice"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	if err := q.Create(&TestModel{ID: "sm002", Name: "bob"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	var rows []map[string]any
	if err := q.Model(&TestModel{}).ScanMap(&rows); err != nil {
		t.Fatalf("ScanMap 失败: %v", err)
	}

	if len(rows) != 2 {
		t.Errorf("期望 2 行, 实际 %d", len(rows))
	}

	if rows[0]["name"] != "alice" {
		t.Errorf("第一行 name 期望 alice, 实际 %v", rows[0]["name"])
	}
}

// ── 钩子测试 ─────────────────────────────────────────────────────────

type HookTestModel struct {
	ID   string `gorm:"primaryKey;size:16"`
	Name string `gorm:"size:100"`

	BeforeCreateCalled bool
	AfterCreateCalled  bool
	BeforeUpdateCalled bool
	AfterUpdateCalled  bool
	BeforeDeleteCalled bool
	AfterDeleteCalled  bool
	AfterFindCalled    bool
}

func (m *HookTestModel) AutoGenerateID() {
	// 测试中手动设置
}

func (m *HookTestModel) OnBeforeCreate(q contracts.Query) error {
	m.BeforeCreateCalled = true
	return nil
}

func (m *HookTestModel) OnAfterCreate(q contracts.Query) error {
	m.AfterCreateCalled = true
	return nil
}

func (m *HookTestModel) OnBeforeUpdate(q contracts.Query) error {
	m.BeforeUpdateCalled = true
	return nil
}

func (m *HookTestModel) OnAfterUpdate(q contracts.Query) error {
	m.AfterUpdateCalled = true
	return nil
}

func (m *HookTestModel) OnBeforeDelete(q contracts.Query) error {
	m.BeforeDeleteCalled = true
	return nil
}

func (m *HookTestModel) OnAfterDelete(q contracts.Query) error {
	m.AfterDeleteCalled = true
	return nil
}

func (m *HookTestModel) OnAfterFind(q contracts.Query) error {
	m.AfterFindCalled = true
	return nil
}

func TestHooks_Create(t *testing.T) {
	drv := newTestDriver(t)
	if err := drv.AutoMigrate(&HookTestModel{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	m := &HookTestModel{ID: "hook001", Name: "test"}
	if err := drv.Query().Create(m); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	if !m.BeforeCreateCalled {
		t.Error("BeforeCreate 钩子未被调用")
	}
	if !m.AfterCreateCalled {
		t.Error("AfterCreate 钩子未被调用")
	}
}

func TestHooks_Save(t *testing.T) {
	drv := newTestDriver(t)
	if err := drv.AutoMigrate(&HookTestModel{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 先创建
	m := &HookTestModel{ID: "hook002", Name: "original"}
	if err := drv.Query().Create(m); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 重置钩子标志
	m.BeforeCreateCalled = false
	m.AfterCreateCalled = false

	// 再 Save（触发 Update 钩子）
	m.Name = "updated"
	if err := drv.Query().Save(m); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	if !m.BeforeUpdateCalled {
		t.Error("Save 时应调用 BeforeUpdate 钩子")
	}
	if !m.AfterUpdateCalled {
		t.Error("Save 时应调用 AfterUpdate 钩子")
	}
}

func TestHooks_Delete(t *testing.T) {
	drv := newTestDriver(t)
	if err := drv.AutoMigrate(&HookTestModel{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	m := &HookTestModel{ID: "hook003", Name: "delete_me"}
	if err := drv.Query().Create(m); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	if err := drv.Query().Delete(m); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	if !m.BeforeDeleteCalled {
		t.Error("BeforeDelete 钩子未被调用")
	}
	if !m.AfterDeleteCalled {
		t.Error("AfterDelete 钩子未被调用")
	}
}

func TestHooks_Find(t *testing.T) {
	drv := newTestDriver(t)
	if err := drv.AutoMigrate(&HookTestModel{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	m := &HookTestModel{ID: "hook004", Name: "find_me"}
	if err := drv.Query().Create(m); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	var found HookTestModel
	if err := drv.Query().First(&found, "id = ?", "hook004"); err != nil {
		t.Fatalf("First 失败: %v", err)
	}

	if !found.AfterFindCalled {
		t.Error("AfterFind 钩子未被调用")
	}
}

func TestHooks_NoHooksModel(t *testing.T) {
	drv := newTestDriverWithTable(t)

	// 使用普通 TestModel（不实现钩子），验证不会出错
	m := &TestModel{ID: "hook005", Name: "no_hooks"}
	if err := drv.Query().Create(m); err != nil {
		t.Fatalf("无钩子模型的 Create 不应失败: %v", err)
	}
}

// ── wrapError 集成测试 ──────────────────────────────────────────────

func TestCreate_DuplicateKey(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "dup001", Name: "first"}); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}

	// 重复主键
	err := q.Create(&TestModel{ID: "dup001", Name: "second"})
	if err == nil {
		t.Fatal("重复主键应返回错误")
	}
	if !errors.Is(err, contracts.ErrDuplicatedKey) {
		t.Errorf("重复主键应映射为 ErrDuplicatedKey, 实际: %v", err)
	}
}

func TestFirst_RecordNotFound(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	var m TestModel
	err := q.First(&m, "id = ?", "nonexistent")
	if err == nil {
		t.Fatal("RecordNotFound 应返回错误")
	}
	if !errors.Is(err, contracts.ErrRecordNotFound) {
		t.Errorf("RecordNotFound 应映射为 ErrRecordNotFound, 实际: %v", err)
	}
}

// ── Select("*") 回归测试 ─────────────────────────────────────────────

func TestSelectStar_Find(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "star001", Name: "alice"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	var rows []TestModel
	if err := q.Model(&TestModel{}).Select("*").Find(&rows); err != nil {
		t.Fatalf("Select(*) 后 Find 不应报错: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "alice" {
		t.Errorf("期望查到 alice 且字段完整, 实际 %+v", rows)
	}
}

func TestSelectStar_Pluck(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "star002", Name: "bob"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	var names []string
	if err := q.Model(&TestModel{}).Select("*").Pluck("name", &names); err != nil {
		t.Fatalf("Select(*) 后 Pluck 不应报错: %v", err)
	}
	if len(names) != 1 || names[0] != "bob" {
		t.Errorf("期望 [bob], 实际 %v", names)
	}
}

func TestSelectStar_Omit(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "star003", Name: "carol"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	var rows []TestModel
	if err := q.Model(&TestModel{}).Select("*").Omit("name").Find(&rows); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if rows[0].ID == "" {
		t.Error("Omit name 后 ID 仍应被查询到")
	}
	if rows[0].Name != "" {
		t.Errorf("Omit name 后 Name 应为空, 实际 %q", rows[0].Name)
	}
}

func TestSelectStar_OverridesSelect(t *testing.T) {
	drv := newTestDriverWithTable(t)
	q := drv.Query()

	if err := q.Create(&TestModel{ID: "star004", Name: "dave"}); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	var rows []TestModel
	if err := q.Model(&TestModel{}).Select("id").Select("*").Find(&rows); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if rows[0].Name != "dave" {
		t.Errorf("Select(id) 后再 Select(*) 应恢复全字段, Name 实际 %q", rows[0].Name)
	}
}
