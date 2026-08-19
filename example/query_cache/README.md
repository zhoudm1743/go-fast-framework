# 查询缓存 Example（SQLite）

演示 `Query().Cache()` 查询缓存的命中与失效，使用 SQLite 文件库，零外部依赖。

## 运行

```bash
cd example/query_cache
go mod tidy
go run .
```

## 演示内容

| 步骤 | 说明 |
|------|------|
| [1] | 写入测试数据 alice |
| [2] | 首次 `Cache()` 查询，走数据库并写入缓存 |
| [3] | 用原生 SQL 直接改库（绕过缓存失效回调），模拟外部修改 |
| [3.5] | 普通查询验证 UPDATE 对当前连接池已生效 |
| [4] | 再次 `Cache()` 查询 → **命中缓存返回旧值 alice**（库中已是 alice_v2） |
| [5] | 普通查询 → 读到最新库状态（不缓存） |
| [6] | 框架写操作（`Updates`）→ 自动失效全部查询缓存 |
| [7] | 失效后 `Cache()` 查询 → 重新走数据库 |
| [8][9] | `Count` 场景缓存（需先 `Model` 指定表） |
| [10] | 列表查询（`Find` 切片）缓存 |

## 已知注意事项

1. **命中缓存时的 SELECT 日志是假象**：命中缓存不执行 SQL，但 gorm execute 层仍打印
   `logger.Trace` 日志（SQL 来自缓存 key 生成、`rows` 来自缓存恢复的 RowsAffected），
   并非真正查询了数据库。

2. **`Updates(map)` / `Count` 需先指定表**：这是 GORM 语义要求，
   `db.Query().Updates(map)` 会报 `Table not set`，需写成
   `db.Query().Model(&User{}).Where(...).Updates(map)`。

3. **缓存 key 由 SQL + 参数自动生成**，不同查询条件自动隔离；
   相同查询条件命中同一缓存。

4. **写操作失效是全局的**：任何 Create/Update/Delete/Save 会清除全部查询缓存
   （通过框架 Tags 机制，不影响其他业务缓存）。

5. **`Row()`/`Rows()`/`Lock()` 自动绕过缓存**（框架已处理）：
   - 游标类查询命中缓存会返回空游标导致 panic，框架会自动剥离缓存标记；
   - 悲观锁（`LockForUpdate`）命中缓存会跳过 DB 执行导致锁失效，框架会自动剥离缓存标记。

6. **`FirstOrCreate`/`FirstOrInit` 用法**：框架已修复 ID 自动生成（`database.Model` 实现
   gorm `BeforeCreate` 钩子）。注意 `Where("name = ?", "dave")` 只是查询条件，
   不会自动写入 dest 的字段，创建时需预填：
   `u := &User{Name: "dave"}; db.Query().Cache().Where("name = ?", "dave").FirstOrCreate(u)`。

