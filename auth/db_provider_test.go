package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/mock"
)

// chainQuery 记录 Table/Where 调用并支持 First 填充的 MockQuery。
type chainQuery struct {
	*mock.MockQuery
	table  string
	wheres []whereCall
	first  func(dest any) error
}

type whereCall struct {
	query any
	args  []any
}

func newChainQuery() *chainQuery {
	q := &chainQuery{MockQuery: mock.NewMockQuery()}
	q.TableFunc = func(name string) contracts.Query {
		q.table = name
		return q
	}
	q.WhereFunc = func(query any, args ...any) contracts.Query {
		q.wheres = append(q.wheres, whereCall{query: query, args: args})
		return q
	}
	q.FirstFunc = func(dest any, conds ...any) error {
		return q.first(dest)
	}
	return q
}

func TestDBProvider_RetrieveByID(t *testing.T) {
	q := newChainQuery()
	q.first = func(dest any) error {
		u := dest.(*testUser)
		u.ID = 7
		u.Name = "alice"
		return nil
	}
	db := &mock.MockDB{QueryFunc: func(ctx ...context.Context) contracts.Query { return q }}

	p := NewDBUserProvider(db, DBUserProviderConfig{Table: "users"},
		func() contracts.AuthUser { return &testUser{} })

	user, err := p.RetrieveByID(newStubCtx(), 7)
	if err != nil {
		t.Fatalf("RetrieveByID() error: %v", err)
	}
	if user.GetAuthID() != int64(7) {
		t.Fatalf("user id = %v", user.GetAuthID())
	}
	if q.table != "users" {
		t.Fatalf("table = %q", q.table)
	}
	if len(q.wheres) != 1 || q.wheres[0].query != "id = ?" || !reflect.DeepEqual(q.wheres[0].args, []any{7}) {
		t.Fatalf("wheres = %+v", q.wheres)
	}
}

func TestDBProvider_RetrieveByIdentityOR(t *testing.T) {
	q := newChainQuery()
	q.first = func(dest any) error { return nil }
	db := &mock.MockDB{QueryFunc: func(ctx ...context.Context) contracts.Query { return q }}

	p := NewDBUserProvider(db, DBUserProviderConfig{
		Table:           "users",
		IdentityColumns: []string{"username", "email", "phone"},
	}, func() contracts.AuthUser { return &testUser{} })

	if _, err := p.RetrieveByIdentity(newStubCtx(), "alice"); err != nil {
		t.Fatal(err)
	}
	if len(q.wheres) != 1 {
		t.Fatalf("wheres = %+v", q.wheres)
	}
	want := "(username = ? OR email = ? OR phone = ?)"
	if q.wheres[0].query != want {
		t.Fatalf("where = %q, want %q", q.wheres[0].query, want)
	}
	if !reflect.DeepEqual(q.wheres[0].args, []any{"alice", "alice", "alice"}) {
		t.Fatalf("args = %v", q.wheres[0].args)
	}
}

func TestDBProvider_NotFound(t *testing.T) {
	q := newChainQuery()
	q.first = func(dest any) error { return errors.New("record not found") }
	db := &mock.MockDB{QueryFunc: func(ctx ...context.Context) contracts.Query { return q }}

	p := NewDBUserProvider(db, DBUserProviderConfig{Table: "users"},
		func() contracts.AuthUser { return &testUser{} })

	if _, err := p.RetrieveByID(newStubCtx(), 99); true {
		assertErrIs(t, err, ErrUserNotFound)
	}
	if _, err := p.RetrieveByIdentity(newStubCtx(), "ghost"); true {
		assertErrIs(t, err, ErrUserNotFound)
	}
}

func TestDBProvider_CustomIDColumn(t *testing.T) {
	q := newChainQuery()
	q.first = func(dest any) error { return nil }
	db := &mock.MockDB{QueryFunc: func(ctx ...context.Context) contracts.Query { return q }}

	p := NewDBUserProvider(db, DBUserProviderConfig{Table: "admins", IDColumn: "uid"},
		func() contracts.AuthUser { return &testUser{} })

	_, _ = p.RetrieveByID(newStubCtx(), 1)
	if q.wheres[0].query != "uid = ?" {
		t.Fatalf("where = %q", q.wheres[0].query)
	}
}
