# radar 可观测性服务设计

日期：2026-10-15
状态：已确认，实现中
模块：`go-fast-framework`

参考 Goravel telemetry 的能力模型（[文档](https://www.goravel.dev/zh_CN/digging-deeper/telemetry.html)），基于 OpenTelemetry 实现，命名为 **radar**。

## 1. 背景与目标

框架缺少可观测性能力。目标：新增 `radar` 服务，提供 OpenTelemetry 三信号（traces / metrics / logs）接入、HTTP/queue 自动埋点、日志桥接、W3C 上下文传播，导出到任意 OTLP 后端（Jaeger / Prometheus / Grafana / Datadog）。

## 2. 关键决策

| 决策点 | 结论 |
|---|---|
| 命名 | `radar`，门面 `facades.Radar()` |
| 契约形态 | **contracts 不依赖 otel 包**：定义自有薄接口（RadarTracer / RadarSpan / RadarMeter / RadarLogger / RadarPropagator），业务代码只依赖 contracts；otel 类型只在 radar 包内部 |
| 三信号 | v1 全做：traces + metrics + logs |
| 导出器 | `otlp`（http/protobuf）/ `console` / `custom`（代码注册工厂）三种 driver；每信号独立开关，空 = 禁用 |
| 禁用态 | 全部信号禁用时返回 no-op 实现，业务代码零分支 |
| logs 桥接 | zap → OTel log 的 `zapcore.Core` 实现；log 包加 `AddCore` 扩展点，radar Boot 时鸭子类型挂载，现有 `facades.Log()` 调用零改动 |
| queue 埋点 | queue manager 加 `SetTracer` 鸭子类型扩展点，radar Boot 时挂载；worker 执行 job 开 span |
| 上下文传播 | W3C tracecontext + baggage；HTTP 中间件提取/注入 |
| 明确留 v2 | DB 埋点（gofast-gorm / gofast-xorm 跨仓库）、gRPC 埋点（gofast-grpc） |
| 新增依赖 | go.opentelemetry.io/otel + sdk + otlp/stdout exporters（一组；框架已有 aws-sdk 级重依赖先例） |

## 3. 目录结构

```
go-fast-framework/
  contracts/radar.go           # Radar / RadarTracer / RadarSpan / RadarMeter / RadarLogger / RadarPropagator / Attr
  radar/
    service_provider.go        # 注册 "radar" 单例；Boot：挂 queue tracer、log bridge、OnShutdown flush
    radar.go                   # radarService：provider 装配、信号开关、门面方法
    config.go                  # 配置读取与默认值
    exporter.go                # 三种 driver 的 exporter/reader 构造 + custom 注册表
    tracer.go                  # RadarTracer / RadarSpan 的 otel 包装
    meter.go                   # Counter / UpDownCounter / Histogram / ObservableGauge 包装
    logger.go                  # RadarLogger 包装（otel log API）
    zap_bridge.go              # zapcore.Core → otel log record（带 trace/span 关联）
    http_client.go             # Transport(base) RoundTripper：注入追踪上下文 + client span
    *_test.go
  http/middleware/radar.go     # HTTP 服务端埋点中间件
  queue/manager.go             # +SetTracer（鸭子类型扩展点）与 worker span
  log/logger.go                # +AddCore（鸭子类型扩展点）
  facades/radar.go             # facades.Radar()
```

## 4. 契约 API（contracts/radar.go）

```go
// Radar 可观测性服务契约（facades.Radar() 返回此接口）。
type Radar interface {
    // Tracer 获取追踪器；name 为埋点作用域名（惯例：包路径或模块名）。
    Tracer(name string) RadarTracer
    // Meter 获取指标器。
    Meter(name string) RadarMeter
    // Logger 获取 OTel 日志记录器（绕过 log 门面直接发 OTel 日志）。
    Logger(name string) RadarLogger
    // Propagator 追踪上下文传播器（W3C tracecontext + baggage）。
    Propagator() RadarPropagator
    // ForceFlush 立即推送缓冲数据（不停止 provider）。
    ForceFlush(ctx context.Context) error
}

type RadarTracer interface {
    // Start 开启 Span；ctx 中已有活跃 Span 时自动成为其子 Span。
    Start(ctx context.Context, name string, opts ...RadarSpanOption) (context.Context, RadarSpan)
}

type RadarSpan interface {
    End()
    SetAttributes(attrs ...RadarAttr)
    AddEvent(name string, attrs ...RadarAttr)
    RecordError(err error)
    SetStatus(code RadarStatusCode, msg string)
    // TraceID / SpanID 当前标识（用于错误响应回传等）。
    TraceID() string
    SpanID() string
}

type RadarStatusCode int // RadarStatusOK / RadarStatusError

type RadarAttr struct {
    Key   string
    Value any // string / int / int64 / float64 / bool / 对应切片
}

type RadarMeter interface {
    Int64Counter(name string, opts ...RadarInstrumentOption) (RadarInt64Counter, error)
    Int64UpDownCounter(name string, opts ...RadarInstrumentOption) (RadarInt64Counter, error)
    Float64Histogram(name string, opts ...RadarInstrumentOption) (RadarFloat64Histogram, error)
    Int64ObservableGauge(name string, callback func(ctx context.Context) int64, opts ...RadarInstrumentOption) error
}

type RadarInt64Counter interface {
    Add(ctx context.Context, value int64, attrs ...RadarAttr)
}

type RadarFloat64Histogram interface {
    Record(ctx context.Context, value float64, attrs ...RadarAttr)
}

type RadarLogger interface {
    Debug(ctx context.Context, msg string, attrs ...RadarAttr)
    Info(ctx context.Context, msg string, attrs ...RadarAttr)
    Warn(ctx context.Context, msg string, attrs ...RadarAttr)
    Error(ctx context.Context, msg string, attrs ...RadarAttr)
}

// RadarPropagator 跨进程追踪上下文传播。
type RadarPropagator interface {
    Inject(ctx context.Context, carrier RadarCarrier)
    Extract(ctx context.Context, carrier RadarCarrier) context.Context
}

// RadarCarrier 文本载体（HTTP 头 / 消息头 map 适配）。
type RadarCarrier interface {
    Get(key string) string
    Set(key, value string)
    Keys() []string
}

// RadarSpanOption / RadarInstrumentOption 选项函数。
type RadarSpanOption func(*RadarSpanConfig)      // WithSpanKind / WithAttributes
type RadarInstrumentOption func(*RadarInstrumentConfig) // WithDescription / WithUnit
```

属性/选项构造函数放 radar 包：`radar.String/Int/Int64/Float64/Bool(k, v)`、`radar.WithSpanKind(...)`、`radar.WithAttributes(...)`、`radar.WithDescription(...)`、`radar.WithUnit(...)`。

## 5. 配置

```yaml
radar:
  service:
    name: my-app
    version: "1.0.0"
    environment: production
  exporters:
    otlp:
      endpoint: localhost:4318
      insecure: true
      headers: {}
    console:
      pretty: true
  traces:
    exporter: otlp        # otlp | console | <custom 名> | ""（禁用）
    sampler: always_on    # always_on | always_off | traceidratio
    ratio: 1.0
    processor: batch      # batch | simple
  metrics:
    exporter: otlp
    interval: 60s
  logs:
    exporter: ""          # 默认关；开启后 facades.Log() 输出自动桥接
  instrumentation:
    http_server: true
    queue: true
    log_bridge: true      # 依赖 logs.exporter 已启用
```

ConfigProvider 默认值：http_server/queue/log_bridge = true，sampler always_on，processor batch，metrics.interval 60s；三信号 exporter 默认 ""（全禁用 → no-op）。

## 6. 关键实现语义

- **装配**：radarService 构造时按配置建 TracerProvider / MeterProvider / LoggerProvider；禁用信号用 otel noop provider 包装成契约实现。Shutdown 挂 app.OnShutdown（flush + shutdown，超时 15s）。
- **custom 导出器**：`radar.RegisterTraceExporter(name, func(ctx) (sdktrace.SpanExporter, error))` 等三个注册函数（代码级，不进 YAML）。
- **zap bridge**：实现 `zapcore.Core`，Entry → otel log record（级别/时间/字段/调用者映射）。
- **trace 关联**：`log.WithContext(ctx)` 目前丢弃 ctx——改为 log 包提供包级钩子 `SetContextFieldExtractor(func(ctx) []zapcore.Field)`，radar Boot 时注册，抽取 trace_id/span_id 作为 zap 字段（console/file/otel 所有输出同时受益，log 包不依赖 otel）。
- **HTTP 中间件**（`middleware.Radar()`）：提取 traceparent → server span（命名 `GET /path`）→ 记录 `http.server.request.duration` 直方图。`contracts.Context` 不暴露标准 `context.Context`（已确认）：中间件把**含 span 的 std ctx** 写入 `CtxKeyRadarStdCtx`、span 本体写入 `CtxKeyRadarSpan`；业务侧用 `radar.StdCtx(ctx)` 取标准 ctx 再开子 span，`radar.SpanFromCtx(ctx)` 取当前 span。
- **queue span**：worker 执行时 `tracer.Start(context.Background(), "queue.exec "+signature)`，失败 RecordError。
- **HTTP client**：`radar.Transport(base, tracer)` 包装 RoundTripper：Inject 头 + client span。

## 7. 错误处理

- 装配失败（exporter 构造失败等）：Register 返回 error，Boot 失败 panic（与框架现有语义一致）。
- 运行期导出失败：OTel 内部重试/丢弃，radar 不另做。
- 全部禁用：no-op，零开销路径。

## 8. 测试方案（无外部网络）

- **tracer**：sdktrace `tracetest.SpanRecorder` 断言 span 树、属性、事件、错误状态、no-op 禁用态
- **meter**：sdkmetric `NewManualReader` 采集断言计数/直方图
- **logger**：sdk log 内存 exporter 断言级别/属性/时间
- **zap bridge**：fake otel log exporter 断言 zap 条目映射与 trace 关联字段
- **middleware**：stub http ctx + SpanRecorder 断言 server span 与传播提取
- **Transport**：`httptest.Server` + RoundTripper 断言 header 注入
- **queue span**：内存 queue + recorder 断言 job span
- **配置**：默认值 / 各 exporter 开关组合 / custom 注册
- **集成**：app 装配 + facades.Radar() 端到端

## 9. 验收标准

1. `go build ./...` / `go vet ./...` / `go test ./...` 全绿
2. 全禁用配置下零开销 no-op，业务代码无需判断
3. 配置 `otlp` 导出器后 traces/metrics/logs 三信号可达 Jaeger/Prometheus（测试用内存 exporter 等价验证管线）
4. `facades.Log()` 在开启 log_bridge 后自动出现在 OTel 后端
5. HTTP 请求 → queue 任务 → 业务 span 能串成一条 trace（传播链路单测覆盖）
