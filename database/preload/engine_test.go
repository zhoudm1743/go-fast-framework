package preload

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// ── 无 DB 单测：fake contracts.Query + fakeMeta ─────────────────────
//
// fakeQuery 以嵌入接口方式满足 contracts.Query（未实现方法 panic），只实现
// 引擎会用到的链式方法（Table/Where/Select/Order/Limit，写时克隆）与终结
// Find/ScanMap：记录收到的 SQL 片段与参数快照，并按表名回填预置子行数据。
// fake 对 `列 = ?` 形态的条件做真实过滤（多态类型列测试需要），IN 条件仅
// 记录参数（分组回填语义由引擎负责，回填断言已覆盖）。

// ── 测试模型（纯 struct，无 DB）────────────────────────────────────

type ptUser struct {
	ID      string
	Key     string
	DeptID  string
	Note    string     `orm:"-"`
	Orders  []ptOrder  `orm:"-" rel:"foreignKey:UserID;references:ID"`
	Profile *ptProfile `orm:"-" rel:"foreignKey:UserID;references:ID"`
	Dept    *ptDept    `gorm:"-"`
	Roles   []ptRole   `orm:"-" rel:"many2many:pt_user_roles;joinForeignKey:Key;joinReferences:Code"`
	Photos  []ptPhoto  `orm:"-" rel:"polymorphic:Owner;polymorphicValue:pt_users"`
	PhotosD []ptPhoto  `orm:"-" rel:"polymorphic:Owner"` // polymorphicValue 缺省 → 父表名
	Bad     []ptOrder  // 无忽略标记（契约 6 应报 ErrUnsupported）
	Ghost   []ptGhost  `orm:"-"` // 两个方向均不成立
}

type ptOrder struct {
	ID     string
	UserID string
	Amount int
	Items  []ptItem `orm:"-" rel:"foreignKey:OrderID;references:ID"`
}

type ptItem struct {
	ID      string
	OrderID string
	Name    string
}

type ptProfile struct {
	ID     string
	UserID string
	Bio    string
}

type ptDept struct {
	ID   string
	Name string
}

type ptRole struct {
	ID   string
	Code string
	Name string
}

type ptPhoto struct {
	ID        string
	OwnerID   string
	OwnerType string
	URL       string
}

type ptGhost struct {
	ID string
}

// 约定回退（无 rel/gorm 外键 tag）：has 系约定 <父表名蛇形>_<引用列蛇形>
// = "pt_post_id"，与子表 ptTag.PtPostID（蛇形 pt_post_id）对齐。
type ptPost struct {
	ID   string
	Tags []ptTag `xorm:"-"`
}

type ptTag struct {
	ID       string
	PtPostID string
}

// 复合外键（两列元组 IN）。
type cpParent struct {
	OrgID  string
	DeptID string
	Kids   []cpChild `orm:"-" rel:"foreignKey:POrgID,PDeptID;references:OrgID,DeptID"`
}

type cpChild struct {
	ID      string
	POrgID  string
	PDeptID string
}

// ── fakeMeta：MetaAdapter 的内存实现 ────────────────────────────────

type fakeMeta struct {
	tables map[reflect.Type]string
	pks    map[reflect.Type][]string
	cols   map[reflect.Type]map[string]string // Go 字段名 → 蛇形列名
}

func newFakeMeta(models ...any) *fakeMeta {
	m := &fakeMeta{
		tables: make(map[reflect.Type]string),
		pks:    make(map[reflect.Type][]string),
		cols:   make(map[reflect.Type]map[string]string),
	}
	for _, model := range models {
		t := reflect.TypeOf(model)
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			continue
		}
		if _, dup := m.tables[t]; dup {
			continue
		}
		m.tables[t] = toSnake(t.Name())
		cols := make(map[string]string)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			ft := f.Type
			for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				continue // 关联字段不映射列
			}
			cols[f.Name] = toSnake(f.Name)
		}
		m.cols[t] = cols
		if _, hasID := cols["ID"]; hasID {
			m.pks[t] = []string{"id"}
		}
	}
	return m
}

func (m *fakeMeta) TableName(t reflect.Type) (string, error) {
	name, ok := m.tables[t]
	if !ok {
		return "", fmt.Errorf("fakeMeta: 未知模型 %s", t)
	}
	return name, nil
}

func (m *fakeMeta) PrimaryKeys(t reflect.Type) ([]string, error) {
	if _, ok := m.tables[t]; !ok {
		return nil, fmt.Errorf("fakeMeta: 未知模型 %s", t)
	}
	if pks := m.pks[t]; len(pks) > 0 {
		return append([]string(nil), pks...), nil
	}
	return nil, fmt.Errorf("fakeMeta: 模型 %s 无主键", t)
}

func (m *fakeMeta) ColumnOfField(t reflect.Type, fieldName string) (string, bool) {
	col, ok := m.cols[t][fieldName]
	return col, ok
}

func (m *fakeMeta) HasColumn(t reflect.Type, columnName string) bool {
	for _, c := range m.cols[t] {
		if c == columnName {
			return true
		}
	}
	return false
}

// ── fakeDB / fakeQuery ──────────────────────────────────────────────

type fakeWhere struct {
	sql  any
	args []any
}

type fakeQuery struct {
	contracts.Query // 嵌入接口：未实现方法 panic（引擎不应触达）
	db              *fakeDB
	tbl             string
	wheres          []fakeWhere
	selects         []string
	orders          []string
	limits          []int
}

type fakeDB struct {
	rows     map[string][]any            // 表名 → 预置 struct 行（Find 回填）
	maps     map[string][]map[string]any // 表名 → 预置映射行（ScanMap 回填）
	executed []*fakeQuery                // 已执行查询快照（Find/ScanMap 时落账）
}

func (d *fakeDB) NewQuery() contracts.Query {
	return &fakeQuery{db: d}
}

func (q *fakeQuery) clone() *fakeQuery {
	c := *q
	c.wheres = append([]fakeWhere(nil), q.wheres...)
	c.selects = append([]string(nil), q.selects...)
	c.orders = append([]string(nil), q.orders...)
	c.limits = append([]int(nil), q.limits...)
	return &c
}

func (q *fakeQuery) Table(name string) contracts.Query {
	c := q.clone()
	c.tbl = name
	return c
}

func (q *fakeQuery) Where(query any, args ...any) contracts.Query {
	c := q.clone()
	c.wheres = append(c.wheres, fakeWhere{sql: query, args: args})
	return c
}

func (q *fakeQuery) Select(query any, args ...any) contracts.Query {
	c := q.clone()
	c.selects = append(c.selects, fmt.Sprint(query))
	for _, a := range args {
		c.selects = append(c.selects, fmt.Sprint(a))
	}
	return c
}

func (q *fakeQuery) Order(value any) contracts.Query {
	c := q.clone()
	c.orders = append(c.orders, fmt.Sprint(value))
	return c
}

func (q *fakeQuery) Limit(limit int) contracts.Query {
	c := q.clone()
	c.limits = append(c.limits, limit)
	return c
}

func (q *fakeQuery) Find(dest any, _ ...any) error {
	q.db.executed = append(q.db.executed, q.clone())
	return q.db.fill(q.tbl, dest)
}

func (q *fakeQuery) ScanMap(dest *[]map[string]any) error {
	q.db.executed = append(q.db.executed, q.clone())
	for _, row := range q.db.maps[q.tbl] {
		*dest = append(*dest, row)
	}
	return nil
}

// fill 将预置行反射写入 dest（*[]T），`列 = ?` 条件真实过滤。
func (d *fakeDB) fill(table string, dest any) error {
	rows := d.rows[table]
	for _, w := range d.executed[len(d.executed)-1].wheres {
		sql, ok := w.sql.(string)
		if !ok || len(w.args) != 1 {
			continue
		}
		if col, found := strings.CutSuffix(sql, " = ?"); found {
			rows = filterEq(rows, col, w.args[0])
		}
	}
	rv := reflect.ValueOf(dest)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("fakeDB: Find dest 须为切片指针，收到 %T", dest)
	}
	out := reflect.MakeSlice(rv.Type(), 0, len(rows))
	for _, row := range rows {
		rval := reflect.ValueOf(row)
		switch {
		case rval.Type() == rv.Type().Elem():
			out = reflect.Append(out, rval)
		case rval.Kind() == reflect.Ptr && rval.Type().Elem() == rv.Type().Elem():
			out = reflect.Append(out, rval.Elem())
		case rv.Type().Elem().Kind() == reflect.Ptr && rval.Type() == rv.Type().Elem().Elem():
			out = reflect.Append(out, rval.Addr())
		default:
			return fmt.Errorf("fakeDB: 表 %s 行类型 %s 无法写入 %s", table, rval.Type(), rv.Type())
		}
	}
	rv.Set(out)
	return nil
}

func filterEq(rows []any, col string, want any) []any {
	out := rows[:0:0]
	for _, r := range rows {
		if got, ok := fieldByColumn(r, col); ok && fmt.Sprint(got) == fmt.Sprint(want) {
			out = append(out, r)
		}
	}
	return out
}

func fieldByColumn(row any, col string) (any, bool) {
	rv := reflect.Indirect(reflect.ValueOf(row))
	if rv.Kind() != reflect.Struct {
		return nil, false
	}
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		if toSnake(t.Field(i).Name) == col {
			return rv.Field(i).Interface(), true
		}
	}
	return nil, false
}

// ── 断言辅助 ────────────────────────────────────────────────────────

func newTestEngine(models ...any) (*Engine, *fakeDB) {
	db := &fakeDB{rows: make(map[string][]any), maps: make(map[string][]map[string]any)}
	meta := newFakeMeta(models...)
	return &Engine{Meta: meta, NewQuery: db.NewQuery}, db
}

// inArgs 断言条件为 `X IN ?` 且切片参数等于期望值列表，返回条件 SQL。
func inArgs(t *testing.T, w fakeWhere, want ...string) string {
	t.Helper()
	sql, ok := w.sql.(string)
	if !ok {
		t.Fatalf("IN 条件 SQL 非字符串: %T", w.sql)
	}
	if !strings.Contains(sql, "IN ?") {
		t.Fatalf("条件 %q 不是 IN ? 形态", sql)
	}
	if len(w.args) != 1 {
		t.Fatalf("IN 条件参数数应为 1（切片），收到 %d", len(w.args))
	}
	got := argStrings(t, w.args[0])
	if strings.Join(got, "\x1f") != strings.Join(want, "\x1f") {
		t.Fatalf("IN 参数 = %v, 期望 %v", got, want)
	}
	return sql
}

func argStrings(t *testing.T, v any) []string {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("参数应为 []any，收到 %T", v)
	}
	out := make([]string, 0, len(s))
	for _, e := range s {
		out = append(out, fmt.Sprint(e))
	}
	return out
}

// ── 契约 1/4/5：has-many 批量 IN 非N+1 + 分组回填 + 空组回填 ────────

func TestHasManyBatchINAndBackfill(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{})
	db.rows["pt_order"] = []any{
		ptOrder{ID: "o3", UserID: "u2", Amount: 30},
		ptOrder{ID: "o1", UserID: "u1", Amount: 10},
		ptOrder{ID: "o2", UserID: "u1", Amount: 20},
	}
	users := []ptUser{{ID: "u1"}, {ID: "u2"}, {ID: "u3"}} // u3 无子行

	if err := eng.Preload(&users, "Orders", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	// 契约 1：查询次数 == 1（批量 IN 非 N+1）
	if len(db.executed) != 1 {
		t.Fatalf("查询次数 = %d, 期望 1", len(db.executed))
	}
	q := db.executed[0]
	if q.tbl != "pt_order" {
		t.Fatalf("表名 = %q, 期望 pt_orders", q.tbl)
	}
	// IN 参数为全部父键（含无子行的 u3）
	if sql := inArgs(t, q.wheres[0], "u1", "u2", "u3"); sql != "user_id IN ?" {
		t.Fatalf("IN 条件 = %q", sql)
	}
	// 分组回填不错位：数量与内容、子行查询序一致
	if len(users[0].Orders) != 2 || users[0].Orders[0].ID != "o1" || users[0].Orders[1].ID != "o2" {
		t.Fatalf("u1.Orders 回填错位: %+v", users[0].Orders)
	}
	if len(users[1].Orders) != 1 || users[1].Orders[0].ID != "o3" {
		t.Fatalf("u2.Orders 回填错位: %+v", users[1].Orders)
	}
	// 契约 5：无子行 → 非 nil 空切片
	if users[2].Orders == nil || len(users[2].Orders) != 0 {
		t.Fatalf("u3.Orders 应为非 nil 空切片: %#v", users[2].Orders)
	}
}

// ── 契约 4：父行外键零值跳过；全零值不发起查询 ─────────────────────

func TestZeroKeyRowsSkipped(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{})
	db.rows["pt_order"] = []any{ptOrder{ID: "o1", UserID: "u1"}}

	users := []ptUser{{ID: "u1"}, {ID: ""}} // ID 零值行跳过
	if err := eng.Preload(&users, "Orders", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 1 {
		t.Fatalf("查询次数 = %d, 期望 1", len(db.executed))
	}
	inArgs(t, db.executed[0].wheres[0], "u1") // 零值键不进 IN
	if users[1].Orders != nil {
		t.Fatalf("零值键行关联字段应保持 nil: %#v", users[1].Orders)
	}
	if len(users[0].Orders) != 1 {
		t.Fatalf("u1.Orders 应有 1 行: %+v", users[0].Orders)
	}

	// 全部为零值 → 不发起查询
	eng2, db2 := newTestEngine(ptUser{}, ptOrder{})
	empty := []ptUser{{ID: ""}, {ID: ""}}
	if err := eng2.Preload(&empty, "Orders", nil, nil); err != nil {
		t.Fatalf("Preload 全零值: %v", err)
	}
	if len(db2.executed) != 0 {
		t.Fatalf("全零值不应发起查询，实际 %d 次", len(db2.executed))
	}
}

// ── 契约 5：无子行 has-many 空切片 / has-one 保持 nil ──────────────

func TestHasOneAndEmptyChildRows(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{}, ptProfile{})
	db.rows["pt_order"] = nil // 无子行
	db.rows["pt_profile"] = []any{ptProfile{ID: "p1", UserID: "u1", Bio: "hi"}}

	users := []ptUser{{ID: "u1"}}
	if err := eng.Preload(&users, "Orders", nil, nil); err != nil {
		t.Fatalf("Preload Orders: %v", err)
	}
	if err := eng.Preload(&users, "Profile", nil, nil); err != nil {
		t.Fatalf("Preload Profile: %v", err)
	}
	if users[0].Orders == nil || len(users[0].Orders) != 0 {
		t.Fatalf("无子行 has-many 应为非 nil 空切片: %#v", users[0].Orders)
	}
	if users[0].Profile == nil || users[0].Profile.Bio != "hi" {
		t.Fatalf("has-one 回填错误: %+v", users[0].Profile)
	}

	// has-one 无子行保持 nil
	eng2, db2 := newTestEngine(ptUser{}, ptProfile{})
	db2.rows["pt_profile"] = []any{ptProfile{ID: "p1", UserID: "other"}}
	users2 := []ptUser{{ID: "u1"}}
	if err := eng2.Preload(&users2, "Profile", nil, nil); err != nil {
		t.Fatalf("Preload Profile2: %v", err)
	}
	if users2[0].Profile != nil {
		t.Fatalf("无子行 has-one 应保持 nil: %+v", users2[0].Profile)
	}
}

// ── 契约 2/3：嵌套递归 + conds 仅作用于首层 ────────────────────────

func TestNestedOrdersItems(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{}, ptItem{})
	db.rows["pt_order"] = []any{
		ptOrder{ID: "o1", UserID: "u1", Amount: 1},
		ptOrder{ID: "o2", UserID: "u1", Amount: 2},
		ptOrder{ID: "o3", UserID: "u2", Amount: 3},
	}
	db.rows["pt_item"] = []any{
		ptItem{ID: "i1", OrderID: "o1", Name: "a"},
		ptItem{ID: "i2", OrderID: "o1", Name: "b"},
		ptItem{ID: "i3", OrderID: "o2", Name: "c"},
		ptItem{ID: "i4", OrderID: "o3", Name: "d"},
	}
	users := []ptUser{{ID: "u1"}, {ID: "u2"}}

	conds := []any{"amount > ?", 0}
	if err := eng.Preload(&users, "Orders.Items", conds, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 2 {
		t.Fatalf("嵌套两层应恰 2 次查询，实际 %d", len(db.executed))
	}
	// 首层：user_id IN；conds 附加其后（契约 3）
	if sql := inArgs(t, db.executed[0].wheres[0], "u1", "u2"); sql != "user_id IN ?" {
		t.Fatalf("首层 IN 条件 = %q", sql)
	}
	if len(db.executed[0].wheres) != 2 {
		t.Fatalf("首层应有 IN + conds 两个条件，实际 %d", len(db.executed[0].wheres))
	}
	if w := db.executed[0].wheres[1]; fmt.Sprint(w.sql) != "amount > ?" || fmt.Sprint(w.args[0]) != "0" {
		t.Fatalf("首层 conds = %+v", db.executed[0].wheres[1])
	}
	// 第二层：以本层回填的子行（o1/o2/o3 全部订单）为父行，仅 IN 无 conds
	if sql := inArgs(t, db.executed[1].wheres[0], "o1", "o2", "o3"); sql != "order_id IN ?" {
		t.Fatalf("二层 IN 条件 = %q", sql)
	}
	if len(db.executed[1].wheres) != 1 {
		t.Fatalf("嵌套层不应携带 conds，实际 %d 个", len(db.executed[1].wheres))
	}
	// 回填正确性
	if len(users[0].Orders[0].Items) != 2 || users[0].Orders[0].Items[0].Name != "a" {
		t.Fatalf("o1.Items 回填错位: %+v", users[0].Orders[0].Items)
	}
	if len(users[0].Orders[1].Items) != 1 || users[0].Orders[1].Items[0].ID != "i3" {
		t.Fatalf("o2.Items 回填错位: %+v", users[0].Orders[1].Items)
	}
	if len(users[1].Orders[0].Items) != 1 || users[1].Orders[0].Items[0].ID != "i4" {
		t.Fatalf("o3.Items 回填错位: %+v", users[1].Orders[0].Items)
	}
}

// ── 契约 3：conds 与 callbacks 应用顺序与作用对象 ──────────────────

func TestCondsAndCallbacks(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{})
	db.rows["pt_order"] = []any{ptOrder{ID: "o1", UserID: "u1"}}
	users := []ptUser{{ID: "u1"}}

	var cbSeenWheres int
	conds := []any{"amount > ?", 15}
	cbs := []func(contracts.Query) contracts.Query{
		func(q contracts.Query) contracts.Query {
			cbSeenWheres = len(q.(*fakeQuery).wheres) // 回调收到的查询应已带 IN + conds
			return q.Order("amount DESC").Limit(2)
		},
	}
	if err := eng.Preload(&users, "Orders", conds, cbs); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 1 {
		t.Fatalf("查询次数 = %d", len(db.executed))
	}
	q := db.executed[0]
	if cbSeenWheres != 2 {
		t.Fatalf("回调收到查询时应已应用 2 个 Where，实际 %d", cbSeenWheres)
	}
	if len(q.wheres) != 2 || fmt.Sprint(q.wheres[1].sql) != "amount > ?" {
		t.Fatalf("conds 未正确应用: %+v", q.wheres)
	}
	if len(q.orders) != 1 || q.orders[0] != "amount DESC" || len(q.limits) != 1 || q.limits[0] != 2 {
		t.Fatalf("callbacks 未正确应用: orders=%v limits=%v", q.orders, q.limits)
	}
}

// ── 契约 1：复合外键元组 IN 占位符 ─────────────────────────────────

func TestCompositeTupleIN(t *testing.T) {
	eng, db := newTestEngine(cpParent{}, cpChild{})
	db.rows["cp_child"] = []any{cpChild{ID: "c1", POrgID: "org1", PDeptID: "d1"}}
	parents := []cpParent{
		{OrgID: "org1", DeptID: "d1"},
		{OrgID: "org2", DeptID: "d2"},
	}
	if err := eng.Preload(&parents, "Kids", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 1 {
		t.Fatalf("查询次数 = %d", len(db.executed))
	}
	w := db.executed[0].wheres[0]
	wantSQL := "(p_org_id, p_dept_id) IN ((?, ?), (?, ?))"
	if fmt.Sprint(w.sql) != wantSQL {
		t.Fatalf("元组 IN 占位符 = %q, 期望 %q", w.sql, wantSQL)
	}
	got := make([]string, 0, len(w.args))
	for _, a := range w.args {
		got = append(got, fmt.Sprint(a))
	}
	if want := []string{"org1", "d1", "org2", "d2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("元组 IN 参数 = %v, 期望 %v", got, want)
	}
	if len(parents[0].Kids) != 1 || parents[0].Kids[0].ID != "c1" {
		t.Fatalf("复合键回填错位: %+v", parents[0].Kids)
	}
	if len(parents[1].Kids) != 0 || parents[1].Kids == nil {
		t.Fatalf("p2.Kids 应为非 nil 空切片: %#v", parents[1].Kids)
	}
}

// 复合外键元组 IN 分批：600 组父键 → 500 + 100 两批（契约 1 防超参）。
func TestCompositeTupleINBatching(t *testing.T) {
	eng, db := newTestEngine(cpParent{}, cpChild{})
	var parents []cpParent
	for i := 0; i < 600; i++ {
		parents = append(parents, cpParent{OrgID: fmt.Sprintf("o%03d", i), DeptID: "d"})
	}
	if err := eng.Preload(&parents, "Kids", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 2 {
		t.Fatalf("600 组应分 2 批查询，实际 %d 批", len(db.executed))
	}
	if got := strings.Count(fmt.Sprint(db.executed[0].wheres[0].sql), "?"); got != 1000 {
		t.Fatalf("第一批占位符数 = %d, 期望 1000", got)
	}
	if got := strings.Count(fmt.Sprint(db.executed[1].wheres[0].sql), "?"); got != 200 {
		t.Fatalf("第二批占位符数 = %d, 期望 200", got)
	}
}

// ── belongs-to：DefaultResolver 两侧探测方向判定 ───────────────────

func TestBelongsToDirection(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptDept{})
	db.rows["pt_dept"] = []any{
		ptDept{ID: "d1", Name: "研发"},
		ptDept{ID: "d2", Name: "产品"},
	}
	users := []ptUser{{ID: "u1", DeptID: "d1"}, {ID: "u2", DeptID: "d2"}, {ID: "u3", DeptID: ""}}
	if err := eng.Preload(&users, "Dept", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 1 {
		t.Fatalf("查询次数 = %d", len(db.executed))
	}
	// belongs-to：IN 列为子表（被引用表）主键，父行取值列为外键 dept_id
	if sql := inArgs(t, db.executed[0].wheres[0], "d1", "d2"); sql != "id IN ?" {
		t.Fatalf("belongs-to IN 条件 = %q", sql)
	}
	if users[0].Dept == nil || users[0].Dept.Name != "研发" {
		t.Fatalf("u1.Dept 回填错误: %+v", users[0].Dept)
	}
	if users[1].Dept == nil || users[1].Dept.Name != "产品" {
		t.Fatalf("u2.Dept 回填错误: %+v", users[1].Dept)
	}
	if users[2].Dept != nil {
		t.Fatalf("外键零值行 Dept 应保持 nil: %+v", users[2].Dept)
	}
}

// ── 约定回退：无 rel/gorm 外键 tag（has 系 <父表名>_<引用列>）─────

func TestConventionFallbackHasMany(t *testing.T) {
	eng, db := newTestEngine(ptPost{}, ptTag{})
	db.rows["pt_tag"] = []any{
		ptTag{ID: "t1", PtPostID: "p1"},
		ptTag{ID: "t2", PtPostID: "p1"},
		ptTag{ID: "t3", PtPostID: "p2"},
	}
	posts := []ptPost{{ID: "p1"}, {ID: "p2"}}
	if err := eng.Preload(&posts, "Tags", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 1 {
		t.Fatalf("查询次数 = %d", len(db.executed))
	}
	if sql := inArgs(t, db.executed[0].wheres[0], "p1", "p2"); sql != "pt_post_id IN ?" {
		t.Fatalf("约定外键列错误: %q", sql)
	}
	if len(posts[0].Tags) != 2 || len(posts[1].Tags) != 1 {
		t.Fatalf("约定回填错位: %+v / %+v", posts[0].Tags, posts[1].Tags)
	}
}

// ── 契约 7/9：many2many 两跳，中间表模型无需业务定义 ───────────────

func TestMany2ManyTwoHops(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptRole{})
	db.maps["pt_user_roles"] = []map[string]any{
		{"key": "u1", "code": "r1"},
		{"key": "u1", "code": "r2"},
		{"key": "u2", "code": "r2"}, // r2 双父行共享，验证映射不错位
	}
	db.rows["pt_role"] = []any{
		ptRole{ID: "r1", Code: "r1", Name: "admin"},
		ptRole{ID: "r2", Code: "r2", Name: "dev"},
		ptRole{ID: "r3", Code: "r3", Name: "qa"}, // 未关联，不应回填
	}
	users := []ptUser{{ID: "u1", Key: "u1"}, {ID: "u2", Key: "u2"}}
	if err := eng.Preload(&users, "Roles", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 2 {
		t.Fatalf("两跳应恰 2 次查询，实际 %d", len(db.executed))
	}
	// 第一跳：中间表（表级子查询，无需模型定义），Select 两列，joinFK IN 父键集
	hop1 := db.executed[0]
	if hop1.tbl != "pt_user_roles" {
		t.Fatalf("第一跳表 = %q", hop1.tbl)
	}
	if strings.Join(hop1.selects, ",") != "key,code" {
		t.Fatalf("第一跳 Select 列 = %v", hop1.selects)
	}
	if sql := inArgs(t, hop1.wheres[0], "u1", "u2"); sql != "key IN ?" {
		t.Fatalf("第一跳 IN 条件 = %q", sql)
	}
	// 第二跳：引用键集去重 IN 查子表
	hop2 := db.executed[1]
	if hop2.tbl != "pt_role" {
		t.Fatalf("第二跳表 = %q", hop2.tbl)
	}
	if sql := inArgs(t, hop2.wheres[0], "r1", "r2"); sql != "code IN ?" {
		t.Fatalf("第二跳 IN 条件 = %q", sql)
	}
	// 按映射回填（不错位）
	if len(users[0].Roles) != 2 || users[0].Roles[0].Code != "r1" || users[0].Roles[1].Code != "r2" {
		t.Fatalf("u1.Roles 回填错位: %+v", users[0].Roles)
	}
	if len(users[1].Roles) != 1 || users[1].Roles[0].Code != "r2" {
		t.Fatalf("u2.Roles 回填错位: %+v", users[1].Roles)
	}
}

// many2many：conds/callbacks 作用于子表查询（第二跳），第一跳不带。
func TestMany2ManyCondsOnChildHop(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptRole{})
	db.maps["pt_user_roles"] = []map[string]any{
		{"key": "u1", "code": "r1"},
		{"key": "u1", "code": "r2"},
	}
	db.rows["pt_role"] = []any{
		ptRole{ID: "r1", Code: "r1", Name: "admin"},
		ptRole{ID: "r2", Code: "r2", Name: "dev"},
	}
	users := []ptUser{{ID: "u1", Key: "u1"}}
	conds := []any{"name = ?", "dev"}
	if err := eng.Preload(&users, "Roles", conds, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed[0].wheres) != 1 {
		t.Fatalf("第一跳不应携带 conds，实际 %d 个", len(db.executed[0].wheres))
	}
	hop2 := db.executed[1]
	if len(hop2.wheres) != 2 || fmt.Sprint(hop2.wheres[1].sql) != "name = ?" {
		t.Fatalf("conds 未作用于第二跳: %+v", hop2.wheres)
	}
	// conds 真实过滤后回填（fake 对 `列 = ?` 过滤）
	if len(users[0].Roles) != 1 || users[0].Roles[0].Code != "r2" {
		t.Fatalf("conds 过滤后回填错误: %+v", users[0].Roles)
	}
}

// many2many：父行键零值 → 不发起任何查询，保持零值。
func TestMany2ManyZeroKeyNoQuery(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptRole{})
	db.maps["pt_user_roles"] = []map[string]any{{"key": "u1", "code": "r1"}}
	db.rows["pt_role"] = []any{ptRole{ID: "r1", Code: "r1"}}
	users := []ptUser{{ID: "u1"}} // Key 零值
	if err := eng.Preload(&users, "Roles", nil, nil); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("零值键不应发起查询，实际 %d 次", len(db.executed))
	}
	if users[0].Roles != nil {
		t.Fatalf("零值键行 Roles 应保持 nil: %#v", users[0].Roles)
	}
}

// ── 契约 8：polymorphic 类型列条件（显式值 / 缺省父表名）───────────

func TestPolymorphic(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptPhoto{})
	db.rows["pt_photo"] = []any{
		ptPhoto{ID: "p1", OwnerID: "u1", OwnerType: "pt_users", URL: "a"},
		ptPhoto{ID: "p2", OwnerID: "u1", OwnerType: "pt_user", URL: "b"},
		ptPhoto{ID: "p3", OwnerID: "u2", OwnerType: "pt_users", URL: "c"},
	}
	users := []ptUser{{ID: "u1"}, {ID: "u2"}}

	// 显式 polymorphicValue: pt_users
	if err := eng.Preload(&users, "Photos", nil, nil); err != nil {
		t.Fatalf("Preload Photos: %v", err)
	}
	q := db.executed[0]
	if sql := inArgs(t, q.wheres[0], "u1", "u2"); sql != "owner_id IN ?" {
		t.Fatalf("多态外键 IN 条件 = %q", sql)
	}
	if w := q.wheres[1]; fmt.Sprint(w.sql) != "owner_type = ?" || fmt.Sprint(w.args[0]) != "pt_users" {
		t.Fatalf("多态类型列条件错误: %+v", q.wheres[1])
	}
	if len(users[0].Photos) != 1 || users[0].Photos[0].ID != "p1" {
		t.Fatalf("u1.Photos 回填错误: %+v", users[0].Photos)
	}
	if len(users[1].Photos) != 1 || users[1].Photos[0].ID != "p3" {
		t.Fatalf("u2.Photos 回填错误: %+v", users[1].Photos)
	}
	if users[0].PhotosD != nil {
		t.Fatalf("PhotosD 不应被 Photos 预加载触及: %+v", users[0].PhotosD)
	}

	// 缺省 polymorphicValue → 父表名 pt_user
	if err := eng.Preload(&users, "PhotosD", nil, nil); err != nil {
		t.Fatalf("Preload PhotosD: %v", err)
	}
	w := db.executed[1].wheres[1]
	if fmt.Sprint(w.sql) != "owner_type = ?" || fmt.Sprint(w.args[0]) != "pt_user" {
		t.Fatalf("缺省多态值应回退父表名 pt_user: %+v", w)
	}
	if len(users[0].PhotosD) != 1 || users[0].PhotosD[0].ID != "p2" {
		t.Fatalf("u1.PhotosD 回填错误: %+v", users[0].PhotosD)
	}
}

// ── 契约 6：无忽略标记 → ErrUnsupported ────────────────────────────

func TestIgnoreMarkerRequired(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{})
	users := []ptUser{{ID: "u1"}}
	err := eng.Preload(&users, "Bad", nil, nil)
	if err == nil {
		t.Fatal("无忽略标记字段应报错")
	}
	if !strings.Contains(err.Error(), "Bad") {
		t.Fatalf("错误信息应含字段名: %v", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("校验失败不应发起查询，实际 %d 次", len(db.executed))
	}
}

// ── 两方向均不成立 → ErrUnsupported（含模型名与字段名引导）─────────

func TestUnresolvableDirection(t *testing.T) {
	eng, _ := newTestEngine(ptUser{}, ptGhost{})
	users := []ptUser{{ID: "u1"}}
	err := eng.Preload(&users, "Ghost", nil, nil)
	if err == nil {
		t.Fatal("两方向均不成立应报错")
	}
	if !strings.Contains(err.Error(), "Ghost") || !strings.Contains(err.Error(), "ptUser") || !strings.Contains(err.Error(), "ptGhost") {
		t.Fatalf("错误信息应含字段名与模型名引导: %v", err)
	}
}

// ── parents 三形态（*T / []T / []*T）与 nil 元素 ───────────────────

func TestParentShapes(t *testing.T) {
	setup := func(eng *Engine, db *fakeDB) {
		db.rows["pt_order"] = []any{
			ptOrder{ID: "o1", UserID: "u1"},
			ptOrder{ID: "o2", UserID: "u2"},
		}
	}

	// *T 单行
	eng1, db1 := newTestEngine(ptUser{}, ptOrder{})
	setup(eng1, db1)
	u := ptUser{ID: "u1"}
	if err := eng1.Preload(&u, "Orders", nil, nil); err != nil {
		t.Fatalf("*T: %v", err)
	}
	if len(u.Orders) != 1 || u.Orders[0].ID != "o1" {
		t.Fatalf("*T 回填错误: %+v", u.Orders)
	}

	// []T 值切片
	eng2, db2 := newTestEngine(ptUser{}, ptOrder{})
	setup(eng2, db2)
	users2 := []ptUser{{ID: "u1"}, {ID: "u2"}}
	if err := eng2.Preload(&users2, "Orders", nil, nil); err != nil {
		t.Fatalf("[]T: %v", err)
	}
	if len(users2[1].Orders) != 1 {
		t.Fatalf("[]T 回填错误: %+v", users2[1].Orders)
	}

	// []*T + nil 元素跳过
	eng3, db3 := newTestEngine(ptUser{}, ptOrder{})
	setup(eng3, db3)
	users3 := []*ptUser{{ID: "u1"}, nil, {ID: "u2"}}
	if err := eng3.Preload(&users3, "Orders", nil, nil); err != nil {
		t.Fatalf("[]*T: %v", err)
	}
	if sql := inArgs(t, db3.executed[0].wheres[0], "u1", "u2"); sql != "user_id IN ?" {
		t.Fatalf("nil 元素应被跳过: %q", sql)
	}
	if len(users3[2].Orders) != 1 {
		t.Fatalf("[]*T 回填错误: %+v", users3[2].Orders)
	}
}

// ── 空父行 / 空路径 → 不发起查询且不报错 ──────────────────────────

func TestEmptyParentsNoQuery(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptOrder{})

	var nilList []*ptUser
	if err := eng.Preload(nilList, "Orders", nil, nil); err != nil {
		t.Fatalf("nil 父行: %v", err)
	}
	if err := eng.Preload([]ptUser{}, "Orders", nil, nil); err != nil {
		t.Fatalf("空切片: %v", err)
	}
	if err := eng.Preload(&[]ptUser{}, "Orders", nil, nil); err != nil {
		t.Fatalf("空切片指针: %v", err)
	}
	if err := eng.Preload(&ptUser{ID: "u1"}, "  ", nil, nil); err != nil {
		t.Fatalf("空路径: %v", err)
	}
	if len(db.executed) != 0 {
		t.Fatalf("不应发起查询，实际 %d 次", len(db.executed))
	}
}

// ── DefaultResolver 单元断言（方向判定 / 约定回退 / toSnake）───────

func TestDefaultResolverUnit(t *testing.T) {
	meta := newFakeMeta(ptUser{}, ptOrder{}, ptDept{}, ptPost{}, ptTag{})
	r := &DefaultResolver{Meta: meta}
	userT := reflect.TypeOf(ptUser{})

	rel, err := r.Resolve(userT, "Orders", true)
	if err != nil {
		t.Fatalf("Resolve Orders: %v", err)
	}
	if rel.Kind != HasMany || strings.Join(rel.FKCols, ",") != "user_id" || strings.Join(rel.RefCols, ",") != "id" {
		t.Fatalf("Orders 解析错误: %+v", rel)
	}
	if rel.ChildType != reflect.TypeOf(ptOrder{}) {
		t.Fatalf("ChildType 错误: %s", rel.ChildType)
	}

	rel, err = r.Resolve(userT, "Dept", false)
	if err != nil {
		t.Fatalf("Resolve Dept: %v", err)
	}
	if rel.Kind != BelongsTo || strings.Join(rel.FKCols, ",") != "dept_id" || strings.Join(rel.RefCols, ",") != "id" {
		t.Fatalf("Dept（belongs-to 方向探测）解析错误: %+v", rel)
	}

	if _, err = r.Resolve(userT, "Missing", false); err == nil {
		t.Fatal("不存在的字段应报错")
	}
	if _, err = r.Resolve(userT, "Note", false); err == nil {
		t.Fatal("非关联类型字段应报错")
	}

	// 蛇形转换（约定回退基础）
	cases := map[string]string{
		"UserID":     "user_id",
		"DeptID":     "dept_id",
		"HTTPServer": "http_server",
		"CPostID":    "c_post_id",
		"pt_post_id": "pt_post_id", // 已蛇形幂等
	}
	for in, want := range cases {
		if got := toSnake(in); got != want {
			t.Fatalf("toSnake(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

// ── ORMTagResolver：ormtag 解析产物驱动（rel/gorm 兼容读取 + 回退）──

// ortNoIgnore：rel 条目存在但无忽略标记（契约 6 应报 ErrUnsupported）。
type ortNoIgnore struct {
	ID     string
	Orders []ptOrder `rel:"foreignKey:UserID;references:ID"`
}

func TestORMTagResolver(t *testing.T) {
	meta := newFakeMeta(ptUser{}, ptOrder{}, ptDept{}, ptRole{})
	r := &ORMTagResolver{Meta: meta}
	userT := reflect.TypeOf(ptUser{})

	// rel 条目（ormtag Rels）→ has-many
	rel, err := r.Resolve(userT, "Orders", true)
	if err != nil {
		t.Fatalf("Resolve Orders: %v", err)
	}
	if rel.Kind != HasMany || strings.Join(rel.FKCols, ",") != "user_id" || strings.Join(rel.RefCols, ",") != "id" {
		t.Fatalf("ORMTagResolver Orders 解析错误: %+v", rel)
	}

	// many2many：joinForeignKey/joinReferences 经 Meta 转列名
	rel, err = r.Resolve(userT, "Roles", true)
	if err != nil {
		t.Fatalf("Resolve Roles: %v", err)
	}
	if rel.Kind != Many2Many || rel.Many2Many != "pt_user_roles" ||
		strings.Join(rel.JoinFKCols, ",") != "key" || strings.Join(rel.JoinRefCols, ",") != "code" ||
		strings.Join(rel.FKCols, ",") != "code" || strings.Join(rel.RefCols, ",") != "key" {
		t.Fatalf("ORMTagResolver Roles 解析错误: %+v", rel)
	}

	// polymorphic：polymorphicValue 缺省回退父表名
	rel, err = r.Resolve(userT, "PhotosD", true)
	if err != nil {
		t.Fatalf("Resolve PhotosD: %v", err)
	}
	if rel.Kind != HasMany || strings.Join(rel.FKCols, ",") != "owner_id" ||
		rel.PolyColumn != "owner_type" || rel.PolyValue != "pt_user" {
		t.Fatalf("ORMTagResolver PhotosD 解析错误: %+v", rel)
	}

	// 无 rel 条目 → 回退 DefaultResolver（gorm:"-" + 两侧探测 → belongs-to）
	rel, err = r.Resolve(userT, "Dept", false)
	if err != nil {
		t.Fatalf("Resolve Dept（回退）: %v", err)
	}
	if rel.Kind != BelongsTo || strings.Join(rel.FKCols, ",") != "dept_id" {
		t.Fatalf("ORMTagResolver 回退 belongs-to 解析错误: %+v", rel)
	}

	// rel 条目存在但无忽略标记 → ErrUnsupported（契约 6）
	r2 := &ORMTagResolver{Meta: newFakeMeta(ortNoIgnore{}, ptOrder{})}
	if _, err = r2.Resolve(reflect.TypeOf(ortNoIgnore{}), "Orders", true); err == nil {
		t.Fatal("rel 条目存在但无忽略标记应报错")
	} else if !strings.Contains(err.Error(), "Orders") {
		t.Fatalf("错误信息应含字段名: %v", err)
	}
}

// 引擎接入 ORMTagResolver 端到端：polymorphic 缺省值回退父表名并正确回填。
func TestEngineWithORMTagResolver(t *testing.T) {
	eng, db := newTestEngine(ptUser{}, ptPhoto{})
	eng.Resolver = &ORMTagResolver{Meta: eng.Meta}
	db.rows["pt_photo"] = []any{
		ptPhoto{ID: "p1", OwnerID: "u1", OwnerType: "pt_user"},
		ptPhoto{ID: "p2", OwnerID: "u1", OwnerType: "other", URL: "x"},
	}
	users := []ptUser{{ID: "u1"}}
	if err := eng.Preload(&users, "PhotosD", nil, nil); err != nil {
		t.Fatalf("Preload PhotosD: %v", err)
	}
	w := db.executed[0].wheres[1]
	if fmt.Sprint(w.sql) != "owner_type = ?" || fmt.Sprint(w.args[0]) != "pt_user" {
		t.Fatalf("ORMTagResolver 多态类型条件错误: %+v", w)
	}
	if len(users[0].PhotosD) != 1 || users[0].PhotosD[0].ID != "p1" {
		t.Fatalf("PhotosD 回填错误: %+v", users[0].PhotosD)
	}
}
