package middleware

import (
	"context"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"github.com/zhoudm1743/go-fast-framework/facades"
	"github.com/zhoudm1743/go-fast-framework/radar"
)

// Radar HTTP 服务端可观测性中间件：
// 提取上游追踪上下文，为每个请求开启 server span 并记录请求耗时指标。
// span 与标准 ctx 分别写入 contracts.CtxKeyRadarSpan / CtxKeyRadarStdCtx，
// 业务侧用 radar.SpanFromCtx(ctx) / radar.StdCtx(ctx) 取用。
func Radar() contracts.HandlerFunc {
	return func(ctx contracts.Context) error {
		r := facades.Radar()
		std := r.Propagator().Extract(context.Background(), radarReqHeaderCarrier{ctx})
		std, span := r.Tracer("gofast/http-server").Start(std,
			ctx.Method()+" "+ctx.FullPath(),
			radar.WithSpanKind(contracts.RadarSpanKindServer),
			radar.WithAttributes(
				radar.String("http.request.method", ctx.Method()),
				radar.String("url.path", ctx.FullPath()),
			),
		)
		ctx.WithValue(string(contracts.CtxKeyRadarSpan), span)
		ctx.WithValue(string(contracts.CtxKeyRadarStdCtx), std)
		defer span.End()

		start := time.Now()
		err := ctx.Next()
		recordHTTPDuration(std, ctx, time.Since(start))
		return err
	}
}

// recordHTTPDuration 记录请求耗时直方图（每次调用重建仪器，otel SDK 按名复用，无泄漏）。
func recordHTTPDuration(std context.Context, ctx contracts.Context, d time.Duration) {
	h, err := facades.Radar().Meter("gofast/http-server").Float64Histogram(
		"http.server.request.duration", radar.WithUnit("s"))
	if err != nil {
		return
	}
	h.Record(std, d.Seconds(),
		radar.String("http.request.method", ctx.Method()),
		radar.String("url.path", ctx.FullPath()),
	)
}

// radarReqHeaderCarrier 请求头提取载体（仅读）。
type radarReqHeaderCarrier struct {
	ctx contracts.Context
}

func (c radarReqHeaderCarrier) Get(key string) string { return c.ctx.Header(key) }
func (c radarReqHeaderCarrier) Set(_, _ string)       {}
func (c radarReqHeaderCarrier) Keys() []string        { return nil }
