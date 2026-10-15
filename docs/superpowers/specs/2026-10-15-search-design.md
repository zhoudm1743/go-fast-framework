# search 检索服务设计（外围仓库 gofast-meilisearch）

日期：2026-10-15
状态：已确认，实现中
模块：框架契约 `go-fast-framework/contracts/search.go` + 外围仓库 `gofast-meilisearch`

## 1. 目标

为框架增加全文检索能力。契约进内核（`contracts/search.go`，已写好），引擎驱动放外围仓库（模式同 gofast-gorm / gofast-xorm）：首个驱动为 **Meilisearch**（HTTP API 简单，无重 SDK 依赖）。

## 2. 关键决策

| 决策点 | 结论 |
|---|---|
| 仓库 | 新仓库 `gofast-meilisearch`，module `github.com/zhoudm1743/gofast-meilisearch`，独立 git repo（模式同 gofast-xorm） |
| 首引擎 | Meilisearch（HTTP API，标准库 net/http 实现，零 SDK 依赖） |
| 多驱动 | 契约 `Search.Driver(name...)` 支持命名驱动，后续可加 ES |
| 注册方式 | 外围仓库提供 `ServiceProvider`，注册 "search" 单例到容器（参照 gofast-xorm/service_provider.go 模式） |
| 文档结构 | map 或结构体直传，JSON 序列化进引擎；主键字段默认 "id"（可配置） |
| 明确不做 | ES driver、聚合/高亮/分面等引擎高级特性（契约预留空间，后续加） |

## 3. 契约（已写入 go-fast-framework/contracts/search.go）

`Search`（Driver 切换）→ `SearchDriver`（Index）→ `SearchIndex`（Upsert / Search / Delete / Clear）+ `SearchResult{Hits, Total, Took}` + `SearchOption`（Limit/Offset/Filter/Sort）。

## 4. Meilisearch 驱动实现映射

| 契约方法 | Meilisearch API |
|---|---|
| Upsert(docs...) | `PUT /indexes/{uid}/documents?primaryKey={pk}` |
| Search(q, opts) | `POST /indexes/{uid}/search`，body `{q, limit, offset, filter, sort}`；响应 `hits` / `estimatedTotalHits` / `processingTimeMs` |
| Delete(ids...) | `POST /indexes/{uid}/documents/delete-batch`，body 为 id 数组 |
| Clear() | `DELETE /indexes/{uid}/documents` |

- 认证：`Authorization: Bearer <api_key>` 头
- 配置：`search.default`（默认驱动名）+ `search.drivers.<name>.{driver: "meilisearch", host, api_key, primary_key}`
- HTTP client：带超时（默认 10s）；非 2xx 响应读取 body 包成 error
- Upsert/Delete/Clear 的 task 异步语义：v1 发送成功即返回（不轮询 task 状态；Meilisearch 默认异步落库，文档注明）

## 5. 外围仓库结构

```
gofast-meilisearch/
  go.mod               # require github.com/zhoudm1743/go-fast-framework（开发期走 go.work 本地解析）
  driver.go            # meilisearch driver 实现 contracts.SearchDriver
  search.go            # Search 服务实现（多驱动调度）— 若框架侧无 search manager 则本仓库提供
  service_provider.go  # 注册 "search" 单例
  driver_test.go       # httptest 假 Meilisearch 服务器测试
  README.md            # 接入示例
  LICENSE
```

注意：框架内核本次只加契约；`Search` 的 manager（默认驱动调度）由外围仓库的 service provider 直接提供实现（contracts.Search 是接口，外围仓库 new 一个 manager 结构体即可）。

## 6. 测试方案

- `httptest.Server` 假 Meilisearch：断言请求方法/路径/头/body 与响应解析（hits/total/took 映射）
- Upsert/Search/Delete/Clear 全路径；错误路径（401/500 → error 含状态与 body 摘要）
- 多驱动配置与 Driver(name) 切换
- 结构体文档（JSON tag）与 map 文档两种 Upsert 输入

## 7. 验收标准

1. `gofast-meilisearch` 内 `go build ./... && go vet ./... && go test ./...` 绿（无外部服务依赖）
2. 业务接入：注册 `&gofastmeili.ServiceProvider{}` + 配置 host/api_key → `facades.Search().Index("posts").Search(...)` 可用
3. 内核 `go-fast-framework` 全量测试保持绿
