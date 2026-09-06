// suite_errors.go 错误映射一致性矩阵（orm-tag-design.md §11.14 全行）。

package drivertest

import (
	"testing"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

func suiteErrors(t *testing.T, f Factory) {
	t.Run("ErrRecordNotFound（First/Get 空命中）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		q := drv.Query()

		var u SuiteUser
		errIs(t, q.Model(&SuiteUser{}).Where("id = ?", "ghost").First(&u), contracts.ErrRecordNotFound, "First 空命中")

		var one SuiteUser
		errIs(t, q.Model(&SuiteUser{}).Where("name = ?", "ghost").Take(&one), contracts.ErrRecordNotFound, "Take 空命中")

		var last SuiteUser
		errIs(t, q.Model(&SuiteUser{}).Last(&last), contracts.ErrRecordNotFound, "Last 空表")
	})

	t.Run("ErrDuplicatedKey（unique 列重复插入）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		q := drv.Query()
		mustCreate(t, q, &SuiteUser{ID: "dup000000000001", Name: "first", Email: "dup@test.dev"})
		err := q.Create(&SuiteUser{ID: "dup000000000002", Name: "second", Email: "dup@test.dev"})
		errIs(t, err, contracts.ErrDuplicatedKey, "unique 列重复插入")
	})

	t.Run("ErrInvalidTransaction（重复 Commit）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		q := drv.Query()

		tx := q.Begin()
		mustCreate(t, tx, &SuiteUser{ID: "tx-commit0000001", Name: "committed", Email: "txc@test.dev"})
		if err := tx.Commit(); err != nil {
			t.Fatalf("首次 Commit: %v", err)
		}
		errIs(t, tx.Commit(), contracts.ErrInvalidTransaction, "重复 Commit")
		// 提交后数据落库
		var count int64
		if err := q.Model(&SuiteUser{}).Where("id = ?", "tx-commit0000001").Count(&count); err != nil || count != 1 {
			t.Fatalf("提交后应落库 1 行，实际 %d (err=%v)", count, err)
		}
	})

	t.Run("ErrInvalidTransaction（Commit 后 Rollback）", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteUser{})
		tx := drv.Query().Begin()
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		errIs(t, tx.Rollback(), contracts.ErrInvalidTransaction, "Commit 后 Rollback")
	})

	t.Run("ErrUnsupported（Preload 两方向均不成立）", func(t *testing.T) {
		// §11.14 "不支持操作" 的套件内通用项：rel 外键字段不存在（gorm:"-"
		// 保证双驱动统一经共享引擎前置校验报错）。xorm 链式 Row/Having 带参等
		// 驱动侧差异用例见 gofast-xorm 包内测试（§11.3 Raw 行差异固化）。
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteBrokenRel{}, &SuiteNode{})
		q := drv.Query()
		mustCreate(t, q, &SuiteBrokenRel{ID: "errunsupported01", Title: "broken"})
		var rows []SuiteBrokenRel
		err := q.Model(&SuiteBrokenRel{}).Preload("Nodes").Find(&rows)
		errIs(t, err, contracts.ErrUnsupported, "Preload 外键字段不存在")
	})

	t.Run("业务钩子自定义错误原样透传", func(t *testing.T) {
		drv := f(t)
		mustAutoMigrate(t, drv, &SuiteHook{})
		q := drv.Query()

		h := &SuiteHook{Name: "boom", FailHook: "before_create"}
		err := q.Create(h)
		errIs(t, err, errHookBoom, "BeforeCreate 错误透传")
		// 操作中断：不落库
		var count int64
		if err := q.Model(&SuiteHook{}).Where("name = ?", "boom").Count(&count); err != nil || count != 0 {
			t.Fatalf("钩子错误应中断 Create（不落库），实际 %d 行 (err=%v)", count, err)
		}

		// AfterFind 阶段错误同样透传
		ok := &SuiteHook{ID: "hookok000000001", Name: "ok"}
		mustCreate(t, q, ok)
		var fetched SuiteHook
		if err := q.Model(&SuiteHook{}).Where("id = ?", ok.ID).First(&fetched); err != nil {
			t.Fatalf("正常钩子链路不应报错: %v", err)
		}
	})
}
