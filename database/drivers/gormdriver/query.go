package gormdriver

import (
	"context"
	"database/sql"
	"reflect"
	"strings"

	"github.com/zhoudm1743/go-fast-framework/contracts"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormQuery 将 contracts.Query 的每个方法代理到 *gorm.DB。
// 所有链式方法返回新的 GormQuery 实例，保持不可变。
type GormQuery struct {
	db     *gorm.DB
	schema string // 动态 schema 前缀，主要用于 PostgreSQL 多 schema 场景
}

var _ contracts.Query = (*GormQuery)(nil)

// wrap 创建新的 GormQuery，传入新的 *gorm.DB，并保留当前 schema。
func (q *GormQuery) wrap(db *gorm.DB) *GormQuery {
	return &GormQuery{db: db, schema: q.schema}
}

// schemaTable 在 schema 非空且 name 中不含 "." 时自动加上 "schema." 前缀。
func (q *GormQuery) schemaTable(name string) string {
	if q.schema != "" && !strings.Contains(name, ".") {
		return q.schema + "." + name
	}
	return name
}

// ── 调试／Schema ─────────────────────────────────────────────────────

// Schema 在当前查询链上设置动态 schema（主要用于 PostgreSQL）。
// 后续的 Model()/Table() 调用将自动在表名前加上 "schema." 前缀。
// 示例：facades.DB().Connection("pg").Schema("analytics").Model(&Event{}).Find(&events)
func (q *GormQuery) Schema(name string) contracts.Query {
	return &GormQuery{db: q.db, schema: name}
}

// GetSchema 返回当前查询上下文的 schema 名称（PostgreSQL 多 schema 场景）。
// 无 schema 上下文时返回空字符串。
// 业务代码需要在原生 SQL 中拼接 schema 限定的表名时使用此方法。
func (q *GormQuery) GetSchema() string {
	return q.schema
}

// applySchema 在终结方法（First/Find/Create 等）执行前，
// 如果设置了 schema：
//  1. 通过 SET search_path 让 PostgreSQL 自动将裸表名解析到租户 schema，
//     确保所有子查询（Joins、Preloads、关联等）无需单独处理就能找到正确的表。
//  2. 如果 dest 是可解析的 GORM model，额外设置 Table(schema.tableName)
//     作为双重保险（对主表显式指定 schema）。
func (q *GormQuery) applySchema(dest any) *gorm.DB {
	if q.schema != "" {
		db := q.db.Exec("SET search_path TO " + q.schema + ", public")
		if dest != nil {
			stmt := &gorm.Statement{DB: db}
			if err := stmt.Parse(dest); err == nil && stmt.Table != "" && !strings.Contains(stmt.Table, ".") {
				return db.Table(q.schema + "." + stmt.Table)
			}
		}
		return db
	}
	if dest != nil {
		return q.db
	}
	return q.db
}

// ── 构建条件 ─────────────────────────────────────────────────────────

func (q *GormQuery) Table(name string) contracts.Query {
	return q.wrap(q.db.Table(q.schemaTable(name)))
}

func (q *GormQuery) Model(value any) contracts.Query {
	if q.schema != "" {
		// 通过 gorm.Statement.Parse 解析 model 对应的裸表名，再加上 schema 前缀。
		stmt := &gorm.Statement{DB: q.db}
		if err := stmt.Parse(value); err == nil && stmt.Table != "" && !strings.Contains(stmt.Table, ".") {
			return q.wrap(q.db.Table(q.schema + "." + stmt.Table).Model(value))
		}
	}
	return q.wrap(q.db.Model(value))
}

func (q *GormQuery) Select(query any, args ...any) contracts.Query {
	return q.wrap(q.db.Select(query, args...))
}

func (q *GormQuery) Omit(columns ...string) contracts.Query {
	return q.wrap(q.db.Omit(columns...))
}

func (q *GormQuery) Where(query any, args ...any) contracts.Query {
	return q.wrap(q.db.Where(query, args...))
}

func (q *GormQuery) OrWhere(query any, args ...any) contracts.Query {
	return q.wrap(q.db.Or(query, args...))
}

func (q *GormQuery) Not(query any, args ...any) contracts.Query {
	return q.wrap(q.db.Not(query, args...))
}

func (q *GormQuery) Order(value any) contracts.Query {
	return q.wrap(q.db.Order(value))
}

func (q *GormQuery) Limit(limit int) contracts.Query {
	return q.wrap(q.db.Limit(limit))
}

func (q *GormQuery) Offset(offset int) contracts.Query {
	return q.wrap(q.db.Offset(offset))
}

func (q *GormQuery) Group(name string) contracts.Query {
	return q.wrap(q.db.Group(name))
}

func (q *GormQuery) Having(query any, args ...any) contracts.Query {
	return q.wrap(q.db.Having(query, args...))
}

func (q *GormQuery) Distinct(args ...any) contracts.Query {
	return q.wrap(q.db.Distinct(args...))
}

// ── 关联 ─────────────────────────────────────────────────────────────

func (q *GormQuery) Joins(query string, args ...any) contracts.Query {
	return q.wrap(q.db.Joins(query, args...))
}

func (q *GormQuery) Preload(query string, args ...any) contracts.Query {
	if q.schema != "" {
		schema := q.schema
		// Preload 生成的子查询不会继承 Tenant() 设置的 schema，
		// 需要在回调中手动设置 Table(schema.table)。
		schemaCb := func(db *gorm.DB) *gorm.DB {
			if db.Statement.Table != "" && !strings.Contains(db.Statement.Table, ".") {
				return db.Table(schema + "." + db.Statement.Table)
			}
			return db
		}
		hasCallback := false
		for i, a := range args {
			if existing, ok := a.(func(*gorm.DB) *gorm.DB); ok {
				args[i] = func(db *gorm.DB) *gorm.DB {
					return existing(schemaCb(db))
				}
				hasCallback = true
				break
			}
		}
		if !hasCallback {
			args = append(args, schemaCb)
		}
	}
	return q.wrap(q.db.Preload(query, args...))
}

// ── 终结方法 ─────────────────────────────────────────────────────────

func (q *GormQuery) Find(dest any, conds ...any) error {
	if err := wrapError(q.applySchema(dest).Find(dest, conds...).Error); err != nil {
		return err
	}
	return invokeAfterFind(q, dest)
}

func (q *GormQuery) First(dest any, conds ...any) error {
	if err := wrapError(q.applySchema(dest).First(dest, conds...).Error); err != nil {
		return err
	}
	return invokeAfterFind(q, dest)
}

func (q *GormQuery) Last(dest any, conds ...any) error {
	if err := wrapError(q.applySchema(dest).Last(dest, conds...).Error); err != nil {
		return err
	}
	return invokeAfterFind(q, dest)
}

func (q *GormQuery) Take(dest any, conds ...any) error {
	if err := wrapError(q.applySchema(dest).Take(dest, conds...).Error); err != nil {
		return err
	}
	return invokeAfterFind(q, dest)
}

func (q *GormQuery) Create(value any) error {
	if err := invokeBeforeCreate(q, value); err != nil {
		return err
	}
	if err := wrapError(q.applySchema(value).Create(value).Error); err != nil {
		return err
	}
	return invokeAfterCreate(q, value)
}

func (q *GormQuery) CreateInBatches(value any, batchSize int) error {
	if err := invokeBeforeCreate(q, value); err != nil {
		return err
	}
	return wrapError(q.applySchema(value).CreateInBatches(value, batchSize).Error)
}

func (q *GormQuery) Save(value any) error {
	if err := invokeBeforeUpdate(q, value); err != nil {
		return err
	}
	if err := wrapError(q.applySchema(value).Save(value).Error); err != nil {
		return err
	}
	return invokeAfterUpdate(q, value)
}

func (q *GormQuery) Update(column string, value any) error {
	return wrapError(q.db.Update(column, value).Error)
}

func (q *GormQuery) Updates(values any) error {
	return wrapError(q.db.Updates(values).Error)
}

func (q *GormQuery) Delete(value any, conds ...any) error {
	if err := invokeBeforeDelete(q, value); err != nil {
		return err
	}
	if err := wrapError(q.applySchema(value).Delete(value, conds...).Error); err != nil {
		return err
	}
	return invokeAfterDelete(q, value)
}

func (q *GormQuery) Count(count *int64) error {
	return wrapError(q.db.Count(count).Error)
}

func (q *GormQuery) Scan(dest any) error {
	return wrapError(q.db.Scan(dest).Error)
}

func (q *GormQuery) Pluck(column string, dest any) error {
	return wrapError(q.db.Pluck(column, dest).Error)
}

func (q *GormQuery) Row() contracts.Row {
	return q.db.Row()
}

func (q *GormQuery) Rows() (contracts.Rows, error) {
	rows, err := q.db.Rows()
	if err != nil {
		return nil, wrapError(err)
	}
	return rows, nil
}

// ── 写操作 Result 变体 ──────────────────────────────────────────────

func (q *GormQuery) CreateResult(value any) contracts.Result {
	if err := invokeBeforeCreate(q, value); err != nil {
		return contracts.Result{Error: err}
	}
	tx := q.applySchema(value).Create(value)
	if tx.Error != nil {
		return contracts.Result{RowsAffected: tx.RowsAffected, Error: wrapError(tx.Error)}
	}
	if err := invokeAfterCreate(q, value); err != nil {
		return contracts.Result{Error: err}
	}
	return contracts.Result{RowsAffected: tx.RowsAffected}
}

func (q *GormQuery) UpdateResult(column string, value any) contracts.Result {
	tx := q.db.Update(column, value)
	return contracts.Result{RowsAffected: tx.RowsAffected, Error: wrapError(tx.Error)}
}

func (q *GormQuery) UpdatesResult(values any) contracts.Result {
	tx := q.db.Updates(values)
	return contracts.Result{RowsAffected: tx.RowsAffected, Error: wrapError(tx.Error)}
}

func (q *GormQuery) DeleteResult(value any, conds ...any) contracts.Result {
	if err := invokeBeforeDelete(q, value); err != nil {
		return contracts.Result{Error: err}
	}
	tx := q.applySchema(value).Delete(value, conds...)
	if tx.Error != nil {
		return contracts.Result{RowsAffected: tx.RowsAffected, Error: wrapError(tx.Error)}
	}
	if err := invokeAfterDelete(q, value); err != nil {
		return contracts.Result{Error: err}
	}
	return contracts.Result{RowsAffected: tx.RowsAffected}
}

func (q *GormQuery) SaveResult(value any) contracts.Result {
	if err := invokeBeforeUpdate(q, value); err != nil {
		return contracts.Result{Error: err}
	}
	tx := q.applySchema(value).Save(value)
	if tx.Error != nil {
		return contracts.Result{RowsAffected: tx.RowsAffected, Error: wrapError(tx.Error)}
	}
	if err := invokeAfterUpdate(q, value); err != nil {
		return contracts.Result{Error: err}
	}
	return contracts.Result{RowsAffected: tx.RowsAffected}
}

// ── 原生 SQL ─────────────────────────────────────────────────────────

func (q *GormQuery) Raw(sql string, values ...any) contracts.Query {
	return q.wrap(q.db.Raw(sql, values...))
}

func (q *GormQuery) Exec(sql string, values ...any) error {
	return wrapError(q.db.Exec(sql, values...).Error)
}

// ── 事务 ─────────────────────────────────────────────────────────────

func (q *GormQuery) Transaction(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error {
	txOpts := parseTxOptions(opts...)
	return wrapError(q.db.Transaction(func(tx *gorm.DB) error {
		return fc(q.wrap(tx))
	}, txOpts))
}

func (q *GormQuery) Begin(opts ...contracts.TxOption) contracts.Query {
	txOpts := parseTxOptions(opts...)
	var tx *gorm.DB
	if txOpts != nil {
		tx = q.db.Begin(txOpts)
	} else {
		tx = q.db.Begin()
	}
	return q.wrap(tx)
}

func (q *GormQuery) Commit() error {
	return wrapError(q.db.Commit().Error)
}

func (q *GormQuery) Rollback() error {
	return wrapError(q.db.Rollback().Error)
}

func (q *GormQuery) SavePoint(name string) error {
	return wrapError(q.db.SavePoint(name).Error)
}

func (q *GormQuery) RollbackTo(name string) error {
	return wrapError(q.db.RollbackTo(name).Error)
}

// ── 分页 ─────────────────────────────────────────────────────────────

func (q *GormQuery) Paginate(page, size int) contracts.Query {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	return q.wrap(q.db.Offset((page - 1) * size).Limit(size))
}

// ── 作用域 ───────────────────────────────────────────────────────────

func (q *GormQuery) Scopes(funcs ...func(contracts.Query) contracts.Query) contracts.Query {
	gormScopes := make([]func(*gorm.DB) *gorm.DB, 0, len(funcs))
	for _, fn := range funcs {
		fn := fn
		gormScopes = append(gormScopes, func(db *gorm.DB) *gorm.DB {
			result := fn(q.wrap(db))
			if gq, ok := result.(*GormQuery); ok {
				return gq.db
			}
			return db
		})
	}
	return q.wrap(q.db.Scopes(gormScopes...))
}

// ── 上下文 ───────────────────────────────────────────────────────────

func (q *GormQuery) WithContext(ctx context.Context) contracts.Query {
	return q.wrap(q.db.WithContext(ctx))
}

// ── 调试 ─────────────────────────────────────────────────────

func (q *GormQuery) Debug() contracts.Query {
	return q.wrap(q.db.Debug())
}

// ── 悲观锁 ───────────────────────────────────────────────────────────

func (q *GormQuery) Lock(mode contracts.LockMode) contracts.Query {
	switch mode {
	case contracts.LockForUpdate:
		return q.wrap(q.db.Clauses(clause.Locking{Strength: "UPDATE"}))
	case contracts.LockShareMode:
		return q.wrap(q.db.Clauses(clause.Locking{Strength: "SHARE"}))
	default:
		return q
	}
}

// ── 软删除扩展 ───────────────────────────────────────────────────

func (q *GormQuery) Unscoped() contracts.Query {
	return q.wrap(q.db.Unscoped())
}

// OnlyTrashed 仅查询已软删除的记录（deleted_at != 0）。
// 注意：列名 "deleted_at" 与 database.SoftDelete.DeletedAt 字段绑定，
// 若自定义软删除列名需自行实现此逻辑。
func (q *GormQuery) OnlyTrashed() contracts.Query {
	return q.wrap(q.db.Unscoped().Where("deleted_at != 0"))
}

func (q *GormQuery) Restore() error {
	return wrapError(q.db.Unscoped().Update("deleted_at", 0).Error)
}

func (q *GormQuery) ForceDelete(value any, conds ...any) error {
	return wrapError(q.db.Unscoped().Delete(value, conds...).Error)
}

// ── 高级查询 ─────────────────────────────────────────────────────────

func (q *GormQuery) FirstOrCreate(dest any, conds ...any) error {
	return wrapError(q.applySchema(dest).FirstOrCreate(dest, conds...).Error)
}

func (q *GormQuery) FirstOrInit(dest any, conds ...any) error {
	return wrapError(q.applySchema(dest).FirstOrInit(dest, conds...).Error)
}

func (q *GormQuery) FindInBatches(dest any, batchSize int, fc func(tx contracts.Query, batch int) error) error {
	return wrapError(q.applySchema(dest).FindInBatches(dest, batchSize, func(tx *gorm.DB, batch int) error {
		return fc(q.wrap(tx), batch)
	}).Error)
}

func (q *GormQuery) ScanMap(dest *[]map[string]any) error {
	rows, err := q.db.Rows()
	if err != nil {
		return wrapError(err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return wrapError(err)
	}

	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return wrapError(err)
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		*dest = append(*dest, row)
	}
	if err := rows.Err(); err != nil {
		return wrapError(err)
	}
	return nil
}

func (q *GormQuery) Exists(dest any, conds ...any) (bool, error) {
	var count int64
	tx := q.applySchema(dest).Model(dest)
	if len(conds) > 0 {
		tx = tx.Where(conds[0], conds[1:]...)
	}
	if err := tx.Limit(1).Count(&count).Error; err != nil {
		return false, wrapError(err)
	}
	return count > 0, nil
}

// ── 辅助 ─────────────────────────────────────────────────────────────

func parseTxOptions(opts ...contracts.TxOption) *sql.TxOptions {
	for _, opt := range opts {
		if std, ok := opt.(*contracts.StandardTxOptions); ok {
			return &sql.TxOptions{
				Isolation: std.Isolation,
				ReadOnly:  std.ReadOnly,
			}
		}
	}
	return nil
}

// ── 模型钩子调用框架 ─────────────────────────────────────────────────

// walkValues 将 value 展开为可寻址对象指针，对每个元素调用 fn。
// 支持 *T、T、[]T、*[]T、[]*T、*[]*T；切片逐元素执行，非切片单次执行。
// 若 fn 返回错误，立即终止遍历并返回该错误。
func walkValues(value any, fn func(iface any) error) error {
	rv := reflect.ValueOf(value)
	// 解引用外层指针
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			var iface any
			if elem.Kind() == reflect.Ptr {
				if elem.IsNil() {
					continue
				}
				iface = elem.Interface()
			} else if elem.CanAddr() {
				iface = elem.Addr().Interface()
			} else {
				continue
			}
			if err := fn(iface); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		if !rv.CanAddr() {
			return nil
		}
		return fn(rv.Addr().Interface())
	default:
		return nil
	}
}

// invokeBeforeCreate 对 value 依次调用 IDAutoGenerator 和 BeforeCreator。
func invokeBeforeCreate(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if ag, ok := iface.(contracts.IDAutoGenerator); ok {
			ag.AutoGenerateID()
		}
		if bc, ok := iface.(contracts.BeforeCreator); ok {
			return bc.OnBeforeCreate(q)
		}
		return nil
	})
}

// invokeAfterCreate 对 value 调用 AfterCreator。
func invokeAfterCreate(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if ac, ok := iface.(contracts.AfterCreator); ok {
			return ac.OnAfterCreate(q)
		}
		return nil
	})
}

// invokeBeforeUpdate 对 value 调用 BeforeUpdater。
func invokeBeforeUpdate(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if bu, ok := iface.(contracts.BeforeUpdater); ok {
			return bu.OnBeforeUpdate(q)
		}
		return nil
	})
}

// invokeAfterUpdate 对 value 调用 AfterUpdater。
func invokeAfterUpdate(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if au, ok := iface.(contracts.AfterUpdater); ok {
			return au.OnAfterUpdate(q)
		}
		return nil
	})
}

// invokeBeforeDelete 对 value 调用 BeforeDeleter。
func invokeBeforeDelete(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if bd, ok := iface.(contracts.BeforeDeleter); ok {
			return bd.OnBeforeDelete(q)
		}
		return nil
	})
}

// invokeAfterDelete 对 value 调用 AfterDeleter。
func invokeAfterDelete(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if ad, ok := iface.(contracts.AfterDeleter); ok {
			return ad.OnAfterDelete(q)
		}
		return nil
	})
}

// invokeAfterFind 对 value 调用 AfterFinder。
func invokeAfterFind(q contracts.Query, value any) error {
	return walkValues(value, func(iface any) error {
		if af, ok := iface.(contracts.AfterFinder); ok {
			return af.OnAfterFind(q)
		}
		return nil
	})
}
