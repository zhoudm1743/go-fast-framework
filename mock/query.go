package mock

import (
	"context"
	"fmt"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// MockQuery 实现 contracts.Query 的最小子集，用于测试。
// 未实现的方法默认返回错误，可通过函数字段覆盖。
type MockQuery struct {
	FindFunc           func(dest any, conds ...any) error
	FirstFunc          func(dest any, conds ...any) error
	LastFunc           func(dest any, conds ...any) error
	TakeFunc           func(dest any, conds ...any) error
	CreateFunc         func(value any) error
	CreateInBatchesFunc func(value any, batchSize int) error
	SaveFunc           func(value any) error
	UpdateFunc         func(column string, value any) error
	UpdatesFunc        func(values any) error
	DeleteFunc         func(value any, conds ...any) error
	CountFunc          func(count *int64) error
	ScanFunc           func(dest any) error
	PluckFunc          func(column string, dest any) error
	ExecFunc           func(sql string, values ...any) error
	TransactionFunc    func(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error
	BeginFunc          func(opts ...contracts.TxOption) contracts.Query
	CommitFunc         func() error
	RollbackFunc       func() error
	ExistsFunc         func(dest any, conds ...any) (bool, error)
	RawFunc            func(sql string, values ...any) contracts.Query
	WhereFunc          func(query any, args ...any) contracts.Query
	TableFunc          func(name string) contracts.Query
	ModelFunc          func(value any) contracts.Query
	SelectFunc         func(query any, args ...any) contracts.Query
	OrderFunc          func(value any) contracts.Query
	LimitFunc          func(limit int) contracts.Query
	OffsetFunc         func(offset int) contracts.Query
	WithContextFunc    func(ctx context.Context) contracts.Query
	DebugFunc          func() contracts.Query
}

// NewMockQuery 创建 MockQuery。
func NewMockQuery() *MockQuery {
	return &MockQuery{}
}

func (q *MockQuery) clone() *MockQuery {
	return &MockQuery{
		FindFunc: q.FindFunc, FirstFunc: q.FirstFunc, LastFunc: q.LastFunc, TakeFunc: q.TakeFunc,
		CreateFunc: q.CreateFunc, CreateInBatchesFunc: q.CreateInBatchesFunc, SaveFunc: q.SaveFunc,
		UpdateFunc: q.UpdateFunc, UpdatesFunc: q.UpdatesFunc, DeleteFunc: q.DeleteFunc,
		CountFunc: q.CountFunc, ScanFunc: q.ScanFunc, PluckFunc: q.PluckFunc,
		ExecFunc: q.ExecFunc, TransactionFunc: q.TransactionFunc, BeginFunc: q.BeginFunc,
		CommitFunc: q.CommitFunc, RollbackFunc: q.RollbackFunc, ExistsFunc: q.ExistsFunc,
		RawFunc: q.RawFunc, WhereFunc: q.WhereFunc, TableFunc: q.TableFunc, ModelFunc: q.ModelFunc,
		SelectFunc: q.SelectFunc, OrderFunc: q.OrderFunc, LimitFunc: q.LimitFunc, OffsetFunc: q.OffsetFunc,
		WithContextFunc: q.WithContextFunc, DebugFunc: q.DebugFunc,
	}
}

func (q *MockQuery) err(method string) error {
	return fmt.Errorf("[GoFast] mock query: %s not configured", method)
}

func (q *MockQuery) Table(name string) contracts.Query {
	if q.TableFunc != nil { return q.TableFunc(name) }
	return q.clone()
}
func (q *MockQuery) Model(value any) contracts.Query {
	if q.ModelFunc != nil { return q.ModelFunc(value) }
	return q.clone()
}
func (q *MockQuery) Select(query any, args ...any) contracts.Query {
	if q.SelectFunc != nil { return q.SelectFunc(query, args...) }
	return q.clone()
}
func (q *MockQuery) Omit(columns ...string) contracts.Query { return q.clone() }
func (q *MockQuery) Where(query any, args ...any) contracts.Query {
	if q.WhereFunc != nil { return q.WhereFunc(query, args...) }
	return q.clone()
}
func (q *MockQuery) OrWhere(query any, args ...any) contracts.Query { return q.clone() }
func (q *MockQuery) Not(query any, args ...any) contracts.Query { return q.clone() }
func (q *MockQuery) Order(value any) contracts.Query {
	if q.OrderFunc != nil { return q.OrderFunc(value) }
	return q.clone()
}
func (q *MockQuery) Limit(limit int) contracts.Query {
	if q.LimitFunc != nil { return q.LimitFunc(limit) }
	return q.clone()
}
func (q *MockQuery) Offset(offset int) contracts.Query {
	if q.OffsetFunc != nil { return q.OffsetFunc(offset) }
	return q.clone()
}
func (q *MockQuery) Group(name string) contracts.Query { return q.clone() }
func (q *MockQuery) Having(query any, args ...any) contracts.Query { return q.clone() }
func (q *MockQuery) Distinct(args ...any) contracts.Query { return q.clone() }
func (q *MockQuery) Joins(query string, args ...any) contracts.Query { return q.clone() }
func (q *MockQuery) Preload(query string, args ...any) contracts.Query { return q.clone() }

func (q *MockQuery) Find(dest any, conds ...any) error {
	if q.FindFunc != nil { return q.FindFunc(dest, conds...) }
	return q.err("Find")
}
func (q *MockQuery) First(dest any, conds ...any) error {
	if q.FirstFunc != nil { return q.FirstFunc(dest, conds...) }
	return q.err("First")
}
func (q *MockQuery) Last(dest any, conds ...any) error {
	if q.LastFunc != nil { return q.LastFunc(dest, conds...) }
	return q.err("Last")
}
func (q *MockQuery) Take(dest any, conds ...any) error {
	if q.TakeFunc != nil { return q.TakeFunc(dest, conds...) }
	return q.err("Take")
}
func (q *MockQuery) Create(value any) error {
	if q.CreateFunc != nil { return q.CreateFunc(value) }
	return q.err("Create")
}
func (q *MockQuery) CreateInBatches(value any, batchSize int) error {
	if q.CreateInBatchesFunc != nil { return q.CreateInBatchesFunc(value, batchSize) }
	return q.err("CreateInBatches")
}
func (q *MockQuery) Save(value any) error {
	if q.SaveFunc != nil { return q.SaveFunc(value) }
	return q.err("Save")
}
func (q *MockQuery) Update(column string, value any) error {
	if q.UpdateFunc != nil { return q.UpdateFunc(column, value) }
	return q.err("Update")
}
func (q *MockQuery) Updates(values any) error {
	if q.UpdatesFunc != nil { return q.UpdatesFunc(values) }
	return q.err("Updates")
}
func (q *MockQuery) Delete(value any, conds ...any) error {
	if q.DeleteFunc != nil { return q.DeleteFunc(value, conds...) }
	return q.err("Delete")
}
func (q *MockQuery) Count(count *int64) error {
	if q.CountFunc != nil { return q.CountFunc(count) }
	return q.err("Count")
}
func (q *MockQuery) Scan(dest any) error {
	if q.ScanFunc != nil { return q.ScanFunc(dest) }
	return q.err("Scan")
}
func (q *MockQuery) Pluck(column string, dest any) error {
	if q.PluckFunc != nil { return q.PluckFunc(column, dest) }
	return q.err("Pluck")
}
func (q *MockQuery) Row() contracts.Row { return nil }
func (q *MockQuery) Rows() (contracts.Rows, error) { return nil, q.err("Rows") }

func (q *MockQuery) CreateResult(value any) contracts.Result { return contracts.Result{} }
func (q *MockQuery) UpdateResult(column string, value any) contracts.Result { return contracts.Result{} }
func (q *MockQuery) UpdatesResult(values any) contracts.Result { return contracts.Result{} }
func (q *MockQuery) DeleteResult(value any, conds ...any) contracts.Result { return contracts.Result{} }
func (q *MockQuery) SaveResult(value any) contracts.Result { return contracts.Result{} }

func (q *MockQuery) Raw(sql string, values ...any) contracts.Query {
	if q.RawFunc != nil { return q.RawFunc(sql, values...) }
	return q.clone()
}
func (q *MockQuery) Exec(sql string, values ...any) error {
	if q.ExecFunc != nil { return q.ExecFunc(sql, values...) }
	return q.err("Exec")
}

func (q *MockQuery) Transaction(fc func(tx contracts.Query) error, opts ...contracts.TxOption) error {
	if q.TransactionFunc != nil { return q.TransactionFunc(fc, opts...) }
	return fc(q)
}
func (q *MockQuery) Begin(opts ...contracts.TxOption) contracts.Query {
	if q.BeginFunc != nil { return q.BeginFunc(opts...) }
	return q.clone()
}
func (q *MockQuery) Commit() error {
	if q.CommitFunc != nil { return q.CommitFunc() }
	return nil
}
func (q *MockQuery) Rollback() error {
	if q.RollbackFunc != nil { return q.RollbackFunc() }
	return nil
}
func (q *MockQuery) SavePoint(name string) error { return nil }
func (q *MockQuery) RollbackTo(name string) error { return nil }

func (q *MockQuery) Paginate(page, size int) contracts.Query { return q.clone() }
func (q *MockQuery) Scopes(funcs ...func(contracts.Query) contracts.Query) contracts.Query { return q.clone() }
func (q *MockQuery) WithContext(ctx context.Context) contracts.Query {
	if q.WithContextFunc != nil { return q.WithContextFunc(ctx) }
	return q.clone()
}
func (q *MockQuery) Debug() contracts.Query {
	if q.DebugFunc != nil { return q.DebugFunc() }
	return q.clone()
}
func (q *MockQuery) Schema(name string) contracts.Query { return q.clone() }
func (q *MockQuery) GetSchema() string { return "" }
func (q *MockQuery) Cache(opts ...contracts.CacheOption) contracts.Query { return q.clone() }
func (q *MockQuery) Lock(mode contracts.LockMode) contracts.Query { return q.clone() }
func (q *MockQuery) Unscoped() contracts.Query { return q.clone() }
func (q *MockQuery) OnlyTrashed() contracts.Query { return q.clone() }
func (q *MockQuery) Restore() error { return nil }
func (q *MockQuery) ForceDelete(value any, conds ...any) error { return q.err("ForceDelete") }
func (q *MockQuery) FirstOrCreate(dest any, conds ...any) error { return q.err("FirstOrCreate") }
func (q *MockQuery) FirstOrInit(dest any, conds ...any) error { return q.err("FirstOrInit") }
func (q *MockQuery) FindInBatches(dest any, batchSize int, fc func(tx contracts.Query, batch int) error) error { return q.err("FindInBatches") }
func (q *MockQuery) ScanMap(dest *[]map[string]any) error { return q.err("ScanMap") }
func (q *MockQuery) Exists(dest any, conds ...any) (bool, error) {
	if q.ExistsFunc != nil { return q.ExistsFunc(dest, conds...) }
	return false, q.err("Exists")
}
