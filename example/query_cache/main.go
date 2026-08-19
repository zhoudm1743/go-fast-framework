// 查询缓存 Example：使用 SQLite 直接演示 Query().Cache() 的命中与失效。
//
// 运行方式（在 example/query_cache 目录下）：
//
//	go mod tidy && go run .
package main

import (
	"fmt"
	"os"

	"github.com/zhoudm1743/go-fast-framework/cache"
	"github.com/zhoudm1743/go-fast-framework/config"
	"github.com/zhoudm1743/go-fast-framework/database"
	"github.com/zhoudm1743/go-fast-framework/foundation"
	"github.com/zhoudm1743/go-fast-framework/log"
)

// 注册配置默认值。config.Add 在 init 阶段暂存，Boot 时自动写入 config 服务。
func init() {
	config.Add("database", map[string]any{
		"default": "main",
		"cache":   map[string]any{"enabled": true}, // 开启查询缓存插件
		"connections": map[string]any{
			"main": map[string]any{
				"driver":   "gormdriver",
				"engine":   "sqlite",
				"database": "query_cache.db",
			},
		},
	})
	// 调低日志级别，避免 SQL 日志干扰演示输出
	config.Add("log", map[string]any{"level": "error"})
}

// User 测试模型，嵌入框架基础模型（ID 为时序 ID 主键）。
type User struct {
	database.Model
	Name string `gorm:"size:100"`
}

func main() {
	// 清理上一次运行遗留的 sqlite 文件
	_ = os.Remove("query_cache.db")

	app := foundation.NewApplication(".")
	app.SetProviders([]foundation.ServiceProvider{
		&config.ServiceProvider{},
		&log.ServiceProvider{},
		&cache.ServiceProvider{},
		&database.ServiceProvider{},
	})
	app.Boot()
	defer app.Shutdown()

	db := app.DB()

	// 自动建表
	if err := db.AutoMigrate(&User{}); err != nil {
		panic(fmt.Errorf("自动建表失败: %w", err))
	}

	// ── 步骤 1：写入测试数据 ──────────────────────────────────────────
	if err := db.Query().Create(&User{Name: "alice"}); err != nil {
		panic(fmt.Errorf("插入数据失败: %w", err))
	}
	fmt.Println("[1] 已写入数据: alice")

	// ── 步骤 2：首次 Cache() 查询，走数据库并写入缓存 ────────────────
	var u1 User
	if err := db.Query().Cache().First(&u1, "name = ?", "alice"); err != nil {
		panic(fmt.Errorf("首次缓存查询失败: %w", err))
	}
	fmt.Printf("[2] 首次 Cache() 查询 → name=%s（来自数据库）\n", u1.Name)

	// ── 步骤 3：用原生 SQL 直接改库，绕过 caches 插件的失效回调 ──────
	// 模拟外部程序修改了数据库，但查询缓存未被清除
	if err := db.Query().Exec("UPDATE users SET name = 'alice_v2' WHERE name = 'alice'"); err != nil {
		panic(fmt.Errorf("直接改库失败: %w", err))
	}
	fmt.Println("[3] 用原生 SQL 将库中 alice 改为 alice_v2（未触发缓存失效）")

	// ── 步骤 3.5：基准——普通查询验证 UPDATE 对当前连接池可见 ─────────
	var base User
	baseErr := db.Query().First(&base, "name = ?", "alice")
	if baseErr != nil {
		fmt.Println("[3.5] 普通查询 name=alice → record not found（UPDATE 已生效）")
	} else {
		fmt.Printf("[3.5] 普通查询 name=alice → %s（UPDATE 未生效？连接可见性问题）\n", base.Name)
	}

	// ── 步骤 4：再次 Cache() 查询，应命中缓存返回旧值 alice ──────────
	// 注意：命中缓存时控制台仍可能打印 SELECT 日志，这是 gorm execute 层的
	// 日志假象（SQL 来自缓存 key 生成、rows 来自缓存恢复值），实际未查库。
	var u2 User
	if err := db.Query().Cache().First(&u2, "name = ?", "alice"); err != nil {
		panic(fmt.Errorf("缓存命中查询失败: %w", err))
	}
	fmt.Printf("[4] 再次 Cache() 查询 → name=%s（命中缓存，库中已是 alice_v2）\n", u2.Name)

	// ── 步骤 5：未调用 Cache() 的查询，直接读库，看到新值 ─────────────
	var u3 User
	err := db.Query().First(&u3, "name = ?", "alice")
	if err != nil {
		fmt.Printf("[5] 普通查询 name=alice → %v（不缓存，读到的是最新库状态）\n", err)
	} else {
		fmt.Printf("[5] 普通查询 → name=%s（不缓存，直接读库）\n", u3.Name)
	}

	// ── 步骤 6：框架写操作（Updates），应触发缓存失效 ─────────────────
	if err := db.Query().Model(&User{}).Where("name = ?", "alice_v2").Updates(map[string]any{"name": "alice_v3"}); err != nil {
		panic(fmt.Errorf("更新失败: %w", err))
	}
	fmt.Println("[6] 框架 Updates 写操作，已自动失效全部查询缓存")

	// ── 步骤 7：失效后 Cache() 查询，重新走数据库，看到 alice_v3 ─────
	var u4 User
	if err := db.Query().Cache().First(&u4, "name = ?", "alice_v3"); err != nil {
		panic(fmt.Errorf("失效后缓存查询失败: %w", err))
	}
	fmt.Printf("[7] 失效后 Cache() 查询 → name=%s（缓存已失效，重新读库）\n", u4.Name)

	// ── 步骤 8：Count 场景缓存（需先 Model 指定表）───────────────────
	var count int64
	if err := db.Query().Model(&User{}).Cache().Count(&count); err != nil {
		panic(fmt.Errorf("计数查询失败: %w", err))
	}
	fmt.Printf("[8] Cache() Count 查询 → %d 条记录\n", count)

	// 再次 Count 查询，应命中缓存
	if err := db.Query().Model(&User{}).Cache().Count(&count); err != nil {
		panic(fmt.Errorf("二次计数查询失败: %w", err))
	}
	fmt.Printf("[9] 再次 Cache() Count 查询 → %d 条记录（命中缓存）\n", count)

	// ── 步骤 9：列表查询（Find 切片）缓存 ─────────────────────────────
	var list []User
	if err := db.Query().Model(&User{}).Cache().Find(&list); err != nil {
		panic(fmt.Errorf("列表查询失败: %w", err))
	}
	fmt.Printf("[10] Cache() Find 列表 → %d 条: %v\n", len(list), userNames(list))

	// ── 步骤 10：清理验证 ─────────────────────────────────────────────
	fmt.Println("[11] 演示结束")
}

func userNames(list []User) []string {
	names := make([]string, 0, len(list))
	for _, u := range list {
		names = append(names, u.Name)
	}
	return names
}
