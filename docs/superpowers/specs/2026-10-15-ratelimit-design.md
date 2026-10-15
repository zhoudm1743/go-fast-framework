# ratelimit 限流器设计

日期：2026-10-15
状态：已确认，实现中
模块：`go-fast-framework`

## 1. 目标

新增 `ratelimit` 服务：固定窗口限流，计数存于现有 cache，配 HTTP 中间件。典型场景：登录防爆破、验证码防刷、接口防爬。

## 2. 关键决策

| 决策点 | 结论 |
|---|---|
| 算法 | 固定窗口（cache 原子自增 + 首中写 TTL）。不做滑动窗口/令牌桶（YAGNI，后续可加驱动层） |
| 存储 | 复用 cache 服务，`ratelimit.store` 可选指定 store 名 |
| 重置时间 | 平行 key `<key>:timer` 存窗口截止时间戳（Unix 秒），TTL=window |
| 门面 | `facades.RateLimit()`，包名 `ratelimit` |
| 中间件 | `middleware.RateLimit(limit, window, keyFn...)`，默认 key = `IP + FullPath` |
| 明确不做 | 分布式滑动窗口、按用户/租户的内置维度（keyFn 自行组合）、漏桶 |

## 3. 目录结构

```
contracts/ratelimit.go        # RateLimiter 契约（已写好）
ratelimit/
  service_provider.go         # 注册 "ratelimit" 单例
  ratelimit.go                # 固定窗口实现
  ratelimit_test.go
http/middleware/ratelimit.go  # 429 中间件
http/middleware/ratelimit_test.go
facades/ratelimit.go
```

## 4. 实现语义

- key 实际存储键：`ratelimit:<key>`；计时键：`ratelimit:<key>:timer`
- `Hit`：`store.Increment(计数键)`；若结果为 1（首次命中）→ `Put(计时键, now+window, window)`（存在微小并发竞态：两个并发首次命中都写 timer，值近似，可接受）
- `Allow`：先 `Hit` 再判断 `count <= limit`；remaining = max(0, limit-count)
- `TooManyAttempts`：读 `GetInt64(计数键) >= limit`
- `AvailableIn`：读计时键时间戳减 now；无计时键或已过期 → 0
- `Clear`：Forget 两个键
- 计数键本身 TTL：首次命中时同样 `Put` 重建以带 TTL（Increment 新建的键无 TTL，否则会永久残留）——实现时注意：**先 Get 判断存在性，不存在则 Put(key,1,window)，存在则 Increment**，避免竞态漏 TTL

## 5. 配置

```yaml
ratelimit:
  store: ""   # cache store 名，空为默认 store
```

## 6. 中间件

```go
// RateLimit 限流中间件：window 内最多 limit 次。
// keyFn 缺省为 "IP + FullPath"；超限返回 429 并带 Retry-After 头。
func RateLimit(limit int, window time.Duration, keyFn ...func(contracts.Context) string) contracts.HandlerFunc
```

## 7. 测试方案

- 固定窗口：窗口内放行/超限/剩余次数/AvailableIn/Clear/窗口过期后重置（用 memory store 或 mock cache，时间窗口用短 window + sleep 或可注入时钟）
- 并发安全：多 goroutine 并发 Hit 计数正确（memory store 的 Increment 原子性）
- 中间件：超限 429 + Retry-After 头 + 自定义 keyFn
- 全量 `go build ./... && go vet ./... && go test ./...` 绿
