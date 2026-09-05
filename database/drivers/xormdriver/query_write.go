package xormdriver

import (
	"fmt"
	"reflect"

	"github.com/zhoudm1743/go-fast-framework/contracts"

	"xorm.io/builder"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// ── 写终结 ───────────────────────────────────────────────────────────
//
// 钩子调用点与 gorm 驱动（gormdriver query.go）逐一对应：
//   - Create/CreateInBatches/Save(插入分支)：前 invokeBeforeCreate（框架层
//     IDAutoGenerator + BeforeCreator，替代 gorm BeforeCreate 回调）、后
//     invokeAfterCreate（对应 gorm AfterCreate）。
//   - Save(更新分支)：前 invokeBeforeUpdate、后 invokeAfterUpdate。
//   - Delete：前 invokeBeforeDelete、后 invokeAfterDelete。
//   - Update/Updates：不触发模型钩子，与 gorm 驱动一致。
//
// 错误出口统一经 q.done()（契约约定 2）：链上错误优先，其余 wrapError 归一；
// 钩子产生的业务错误不匹配任何 Sentinel，wrapError 原样透传，语义不丢失。

// createCore Create 的公共内核：钩子 → Insert → 失效缓存 → 钩子。
// 返回 Insert 的受影响行数供 CreateResult 回填；Create 丢弃行数只取错误。
// 注意 gorm 驱动的 Save 空主键分支同样先落 gorm BeforeCreate 生成 ID，
// saveOne 的插入路径复用本函数即与该语义对齐。
func (q *XormQuery) createCore(value any) (int64, error) {
	if err := invokeBeforeCreate(q, value); err != nil {
		return 0, q.done(err)
	}
	s, err := q.build(value)
	if err != nil {
		return 0, q.done(err)
	}
	n, err := s.Insert(value)
	if err != nil {
		return 0, q.done(err)
	}
	// 写操作成功后统一失效查询缓存（契约约定 5）
	q.invalidateCache()
	return n, q.done(invokeAfterCreate(q, value))
}

// Create 插入单条记录。
func (q *XormQuery) Create(value any) error {
	_, err := q.createCore(value)
	return err
}

// CreateInBatches 分批插入切片。
// gorm 的 CreateInBatches 对非切片值回落为单条 Create（default 分支），
// 这里对齐：非切片直接委托 Create，钩子因此只在单条路径触发一次。
// 仅切片走分批（数组成员不可寻址时无法按元素触发钩子，同样回落 Create）。
func (q *XormQuery) CreateInBatches(value any, batchSize int) error {
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			// nil 指针无可插入元素：no-op（与 walkValues 跳过 nil 的口径一致）
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice && !(rv.Kind() == reflect.Array && rv.CanAddr()) {
		return q.Create(value)
	}
	// batchSize 非法时兜底为不分批（gorm 对非正 batch 行为无保证）
	if batchSize <= 0 {
		batchSize = rv.Len()
	}
	// 钩子对整个切片前置触发一次（walkValues 会逐元素展开），
	// 与 gorm 每批 callbacks 触发的净效果一致：每个元素各回调一次。
	if err := invokeBeforeCreate(q, value); err != nil {
		return q.done(err)
	}
	// 每块重新 build：链上条件/表名兜底/事务会话全部复用；
	// xorm session 每条语句执行后自动重置，同一 session 可连续 Insert。
	for start := 0; start < rv.Len(); start += batchSize {
		end := start + batchSize
		if end > rv.Len() {
			end = rv.Len()
		}
		chunk := rv.Slice(start, end).Interface()
		s, err := q.build(chunk)
		if err != nil {
			return q.done(err)
		}
		// 任何一块失败立即终止：已插入块不回滚（无外层事务包裹）
		if _, err := s.Insert(chunk); err != nil {
			return q.done(err)
		}
	}
	q.invalidateCache()
	return q.done(invokeAfterCreate(q, value))
}

// save Save 的公共内核：切片逐元素路由，返回累计受影响行数。
func (q *XormQuery) save(value any) (int64, error) {
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return 0, nil
		}
		rv = rv.Elem()
	}
	// 切片：gorm Save 对切片走 upsert；xorm 无可移植 upsert，退化为
	// 逐元素"空主键插入、非空主键更新"路由（净语义最接近，钩子逐元素触发）。
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		var total int64
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			if elem.Kind() == reflect.Ptr {
				if elem.IsNil() {
					continue // 跳过 nil 元素（与 walkValues 口径一致）
				}
				n, err := q.saveOne(elem.Interface())
				if err != nil {
					return total, err
				}
				total += n
			} else if elem.CanAddr() {
				n, err := q.saveOne(elem.Addr().Interface())
				if err != nil {
					return total, err
				}
				total += n
			}
		}
		return total, nil
	}
	return q.saveOne(value)
}

// saveOne 单条 Save：按主键是否全为零值路由插入/更新路径。
// 插入路径与 Create 完全一致——gorm 驱动的 Save 空主键同样先落 gorm
// BeforeCreate 生成 ID，此处复用 createCore 对齐该语义。
// 更新路径用 AllCols() 写入所有字段（含零值），对应 gorm Save "更新全部字段"
// 的行为。差异：gorm 更新命中 0 行会回落 upsert 插入，此处保持纯更新语义，
// 0 行场景由调用方经 SaveResult().IsZeroRow() 自行判定。
func (q *XormQuery) saveOne(value any) (int64, error) {
	if pkAllZero(q.engine, value) {
		return q.createCore(value)
	}
	if err := invokeBeforeUpdate(q, value); err != nil {
		return 0, q.done(err)
	}
	s, err := q.build(value)
	if err != nil {
		return 0, q.done(err)
	}
	// xorm 的 Update 不会从被更新 bean 自动推导 WHERE（仅来自显式条件或
	// condiBean），必须显式按主键列构造等值条件，否则生成无 WHERE 的全表
	// UPDATE（多行表触发主键唯一冲突，单行表静默覆盖全部数据）。
	if err := applyPKCondition(s, q.engine, value); err != nil {
		return 0, q.done(err)
	}
	n, err := s.AllCols().Update(value)
	if err != nil {
		return 0, q.done(err)
	}
	q.invalidateCache()
	return n, q.done(invokeAfterUpdate(q, value))
}

// Save 保存记录：主键全零按插入处理，否则按主键更新。
func (q *XormQuery) Save(value any) error {
	_, err := q.save(value)
	return err
}

// updateColumnCore Update 的公共内核：单列更新，返回受影响行数。
// build(nil)：更新不按 dest 推导表名，要求链上已显式 Table()/Model()
// （显式表名由 Model/Table 的 applier 应用到 session）。
// 单列用 map 传参而非 struct 字段：xorm 对 struct 默认跳过零值字段，
// map 键无条件写入，保证 gorm Update"显式指定列（含零值）"的语义。
// 不调用模型钩子，与 gorm 驱动一致。
func (q *XormQuery) updateColumnCore(column string, value any) (int64, error) {
	s, err := q.build(nil)
	if err != nil {
		return 0, q.done(err)
	}
	// n 为受影响行数，仅成功路径回填 Result；错误路径忽略（行数无意义）
	n, err := s.Update(map[string]any{column: value})
	if err != nil {
		return 0, q.done(err)
	}
	q.invalidateCache()
	return n, nil
}

// Update 更新单列（要求链上已显式 Table()/Model）。
func (q *XormQuery) Update(column string, value any) error {
	_, err := q.updateColumnCore(column, value)
	return err
}

// updatesCore Updates 的公共内核：map 或 struct 批量字段更新。
// struct 传参时 xorm 跳过零值字段，与 gorm Updates(struct) 语义一致；
// map 传参写入全部键。不调用模型钩子，与 gorm 驱动一致。
func (q *XormQuery) updatesCore(values any) (int64, error) {
	s, err := q.build(nil)
	if err != nil {
		return 0, q.done(err)
	}
	n, err := s.Update(values)
	if err != nil {
		return 0, q.done(err)
	}
	q.invalidateCache()
	return n, nil
}

// Updates 批量更新字段（map 或 struct，要求链上已显式 Table()/Model）。
func (q *XormQuery) Updates(values any) error {
	_, err := q.updatesCore(values)
	return err
}

// deleteCore Delete 的公共内核：钩子 → 条件 → 删除 → 失效缓存 → 钩子。
// value 同时充当表定位 bean（表名兜底/主键条件来源），conds 为 gorm 语义的
// 附加 Where 条件（conds[0] 条件、其余参数，经 applyConds 应用）。
func (q *XormQuery) deleteCore(value any, conds []any) (int64, error) {
	if err := invokeBeforeDelete(q, value); err != nil {
		return 0, q.done(err)
	}
	s, err := q.build(value)
	if err != nil {
		return 0, q.done(err)
	}
	if err := applyConds(s, conds); err != nil {
		return 0, q.done(err)
	}
	n, err := s.Delete(value)
	if err != nil {
		return 0, q.done(err)
	}
	q.invalidateCache()
	return n, q.done(invokeAfterDelete(q, value))
}

// Delete 删除记录。
func (q *XormQuery) Delete(value any, conds ...any) error {
	_, err := q.deleteCore(value, conds)
	return err
}

// ── 写操作 Result 变体 ──────────────────────────────────────────────
//
// 复用与 error 变体完全相同的内核（钩子调用点、缓存失效、错误归一均一致），
// 仅额外回填 RowsAffected。错误路径行数不回填（保持零值）。

// CreateResult 插入并返回结果。RowsAffected 为 Insert 受影响行数
// （单条恒为 1；value 为切片时为总行数）。
func (q *XormQuery) CreateResult(value any) contracts.Result {
	n, err := q.createCore(value)
	return contracts.Result{RowsAffected: n, Error: err}
}

// UpdateResult 更新单列并返回结果。RowsAffected 为匹配 WHERE 的行数，
// 0 行（未命中）时 Error 为 nil，可经 IsZeroRow 判定。
func (q *XormQuery) UpdateResult(column string, value any) contracts.Result {
	n, err := q.updateColumnCore(column, value)
	return contracts.Result{RowsAffected: n, Error: err}
}

// UpdatesResult 批量更新字段并返回结果。RowsAffected 语义同 UpdateResult。
func (q *XormQuery) UpdatesResult(values any) contracts.Result {
	n, err := q.updatesCore(values)
	return contracts.Result{RowsAffected: n, Error: err}
}

// DeleteResult 删除并返回结果（含 Delete 前后钩子）。
// RowsAffected 为删除的行数。
func (q *XormQuery) DeleteResult(value any, conds ...any) contracts.Result {
	n, err := q.deleteCore(value, conds)
	return contracts.Result{RowsAffected: n, Error: err}
}

// SaveResult 保存并返回结果（含对应插入/更新钩子）。
// RowsAffected：插入路径为 Insert 受影响行数（1）；更新路径为匹配 WHERE
// 的行数（0 表示无匹配行，可经 IsZeroRow 判定；gorm 驱动同路径回落 upsert，
// 此处如上 saveOne 注释保持纯更新语义）。
func (q *XormQuery) SaveResult(value any) contracts.Result {
	n, err := q.save(value)
	return contracts.Result{RowsAffected: n, Error: err}
}

// ── 主键零值判定（Save 路由用）───────────────────────────────────────

// pkAllZero 判断 value（struct / 指针）的主键值是否全为零值。
// 表无主键、解析失败或 value 非 struct 时按"零值"处理回落插入路径：
// 无主键便无法构造更新条件，s.AllCols().Update 会退化为无 WHERE 的全表
// UPDATE，数据破坏不可逆，必须避免。
func pkAllZero(e *xorm.Engine, value any) bool {
	bean := tableBeanOf(value)
	if bean == nil {
		return true
	}
	t, err := e.TableInfo(bean)
	if err != nil {
		return true
	}
	// Indirect 解引用指针；nil 指针得到零 Value（Kind 非 Struct）→ 视为零值
	rv := reflect.Indirect(reflect.ValueOf(value))
	if rv.Kind() != reflect.Struct {
		return true
	}
	return pkStructZero(t, rv)
}

// pkStructZero 判定单个 struct 值的主键字段是否全为零值。
// xorm 的 PrimaryKeys 存放 mapper 映射后的数据库列名（如字段 ID → 列 id），
// 不能直接用列名查 struct 字段，须经 schemas.Table 列的 FieldName 定位。
func pkStructZero(t *schemas.Table, rv reflect.Value) bool {
	for _, name := range t.PrimaryKeys {
		col := t.GetColumn(name)
		if col == nil || col.FieldName == "" {
			continue
		}
		fv := rv.FieldByName(col.FieldName)
		// 字段不可定位（如内嵌指针为 nil 得到零 Value）时按零值处理：
		// 宁走插入路径拿到重复键错误，也不走无主键条件的全表更新。
		if fv.IsValid() && !fv.IsZero() {
			return false
		}
	}
	return true
}

// ── 主键条件构造（Save 更新路径用）─────────────────────────────────────

// applyPKCondition 按主键列的当前值构造 WHERE 等值条件。
// xorm Session.Update 的 WHERE 仅来自显式条件，不会自动附加被更新 bean 的
// 主键（与 gorm 行为不同），漏加会生成无 WHERE 的全表 UPDATE。
func applyPKCondition(s *xorm.Session, e *xorm.Engine, value any) error {
	t, err := e.TableInfo(tableBeanOf(value))
	if err != nil {
		return fmt.Errorf("xorm: Save 解析表信息失败: %w", err)
	}
	rv := reflect.Indirect(reflect.ValueOf(value))
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: Save 更新路径要求 struct，收到 %T", contracts.ErrUnsupported, value)
	}
	eq := builder.Eq{}
	for _, col := range t.PrimaryKeys {
		c := t.GetColumn(col)
		if c == nil {
			return fmt.Errorf("xorm: 主键列 %q 无法定位", col)
		}
		fv := rv.FieldByName(c.FieldName)
		if !fv.IsValid() {
			return fmt.Errorf("xorm: 主键列 %q 对应字段 %q 不存在", col, c.FieldName)
		}
		eq[col] = fv.Interface()
	}
	s.Where(eq)
	return nil
}
