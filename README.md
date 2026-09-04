# GoFast Framework

> GoFast 框架核心 -- 一个轻量、可扩展的 Go 语言 Web 框架内核。提供 IoC 容器、ServiceProvider 生命周期、Facade 门面、配置文件、结构化日志、GORM 数据库、缓存、文件存储等企业级基础设施。

[![Go Version](https://img.shields.io/badge/Go-1.25+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

---

## 安装

```bash
go get github.com/zhoudm1743/go-fast-framework@latest
```

## 快速开始

```go
package main

import (
    "os"
    "os/signal"
    "syscall"

    "github.com/zhoudm1743/go-fast-framework/cache"
    "github.com/zhoudm1743/go-fast-framework/config"
    "github.com/zhoudm1743/go-fast-framework/database"
    "github.com/zhoudm1743/go-fast-framework/facades"
    "github.com/zhoudm1743/go-fast-framework/filesystem"
    "github.com/zhoudm1743/go-fast-framework/foundation"
    gohttp "github.com/zhoudm1743/go-fast-framework/http"
    "github.com/zhoudm1743/go-fast-framework/log"
)

func main() {
    app := foundation.NewApplication(".")

    app.SetProviders([]foundation.ServiceProvider{
        &config.ServiceProvider{},
        &log.ServiceProvider{},
        &cache.ServiceProvider{},
        &database.ServiceProvider{},
        &filesystem.ServiceProvider{},
        &gohttp.ServiceProvider{},
    })
    app.Boot()
    facades.SetApp(app)

    // 使用 Facade 访问服务
    facades.Log().Info("Hello, GoFast!")

    // 启动 HTTP 服务
    go func() {
        facades.Http.Route().Run()
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    app.Shutdown()
}
```

完整项目骨架请参阅 [github.com/zhoudm1743/go-fast](https://github.com/zhoudm1743/go-fast)。

---

## 核心模块

| 模块 | 路径 | 说明 |
|------|------|------|
| **foundation** | `foundation/` | IoC 容器与 Application 生命周期管理 |
| **contracts** | `contracts/` | 所有服务接口契约定义 |
| **facades** | `facades/` | 全局静态门面，一行代码访问任意服务 |
| **config** | `config/` | 基于 Viper 的配置管理，Go 代码 + YAML 双模式 |
| **log** | `log/` | 基于 Zap 的结构化日志，控制台/文件/混合输出 |
| **database** | `database/` | 基于 GORM 的数据库服务，多连接、时序 ID |
| **cache** | `cache/` | 缓存服务，支持内存/Redis/文件驱动 |
| **http** | `http/` | HTTP 路由服务，双引擎（Gin / Fiber） |
| **filesystem** | `filesystem/` | 文件存储，本地/OSS/COS/MinIO/S3 |
| **jwt** | `jwt/` | JWT 鉴权服务 |
| **event** | `event/` | 事件系统 |
| **queue** | `queue/` | 队列系统 |
| **schedule** | `schedule/` | 基于 cron 的任务调度 |
| **fast** | `fast/` | CLI 控制台，脚手架命令 |
| **id** | `id/` | UUID v7 时序 ID 生成 |
| **utils** | `utils/` | 通用工具函数 |

---

## HTTP 参数绑定与默认值

控制器通过 `ctx.Bind(&req)` 自动解析请求参数并校验，支持三类标签：

| 标签 | 来源 | 示例 |
|------|------|------|
| `uri` | URL 路径参数 | `uri:"id"` |
| `query` | 查询字符串 | `query:"page"` |
| `json` | 请求体 | `json:"name"` |
| `binding` | 校验规则 | `binding:"required,min=1"` |

字段可附加 `default` 标签声明默认值，`Bind` 会在零值（未传入）时自动填充，无需在控制器里手写 `if` 判断：

```go
type ListReq struct {
    Page  int    `query:"page" default:"1"`
    Size  int    `query:"size" default:"20"`
    Sort  string `query:"sort" default:"desc"`
    IDs   []int  `query:"ids" default:"1,2,3"`
}

var req ListReq
if err := ctx.Bind(&req); err != nil {
    return ctx.Response().Validation(err)
}
```

`default` 支持的类型：基础类型（`string` / `int` / `uint` / `float` / `bool`，含底层为基础类型的自定义类型）、指针、切片（逗号分隔）、`time.Duration`（如 `5s`）、`time.Time`（如 `2006-01-02`）、以及实现 `encoding.TextUnmarshaler` 的自定义类型。请求中已传入的值不会被默认值覆盖。

---

## HTTP 服务配置

以下配置均为可选项，写入 `config/config.yaml` 即可生效（Gin / Fiber 双引擎行为一致）：

```yaml
server:
  driver: fiber                 # HTTP 引擎：fiber（默认）| gin
  host: 0.0.0.0
  port: 8080
  cors_allow_origins:           # 放行的跨域来源，默认 "*"
    - https://app.example.com
    - https://admin.example.com
  cors_allow_methods: "GET,POST,PUT,DELETE,PATCH,OPTIONS"       # 放行的跨域方法，默认同左
  cors_allow_headers: "Origin,Content-Type,Accept,Authorization,X-Token"  # 放行的跨域请求头（默认不含 X-Token 等自定义头）
  cors_expose_headers: "X-Request-ID,X-Total-Count"             # 允许浏览器脚本读取的响应头，默认不输出
  cors_allow_credentials: true  # 允许携带 Cookie 等凭据；未配置时仅当 origins 非 * 时自动开启
  cors_max_age: 86400           # 预检结果缓存秒数，默认 86400；<=0 不输出该头
  security_headers_enabled: true          # 基础安全响应头，默认关闭
  security_hsts_max_age: 31536000         # HSTS max-age，默认 31536000；<=0 不输出 HSTS
```

说明：

- **CORS**：`cors_allow_origins` 支持字符串数组或逗号分隔字符串。配置多个来源时按请求 `Origin` 逐请求回显命中的值；启用 `cors_allow_credentials` 后即使来源为 `*` 也会回显具体 Origin（CORS 规范禁止 `*` 搭配凭据）。
- **安全响应头**：开启 `security_headers_enabled` 后每个响应附带 `X-Frame-Options: DENY`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: strict-origin-when-cross-origin`，HTTPS 请求额外附带 HSTS。

---

## License

[Apache 2.0](LICENSE)
