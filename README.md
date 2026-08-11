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
| **cache** | `cache/` | 缓存服务，支持内存/Redis 驱动 |
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

## License

[Apache 2.0](LICENSE)
