package radar

import (
	"net/http"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// NewTransport 包装 HTTP RoundTripper：外呼时注入追踪上下文并开启 client span。
// base 为 nil 时使用 http.DefaultTransport。
func NewTransport(base http.RoundTripper, r contracts.Radar) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &radarTransport{base: base, radar: r}
}

// radarTransport HTTP client 埋点。
type radarTransport struct {
	base  http.RoundTripper
	radar contracts.Radar
}

func (t *radarTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := t.radar.Tracer("gofast/http-client").Start(req.Context(),
		req.Method+" "+req.URL.Host,
		func(c *contracts.RadarSpanConfig) { c.Kind = contracts.RadarSpanKindClient })
	defer span.End()

	// 注入追踪上下文，下游服务可继续同一条 trace
	t.radar.Propagator().Inject(ctx, httpHeaderCarrier{req.Header})

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(contracts.RadarStatusError, err.Error())
		return nil, err
	}
	if resp.StatusCode >= 500 {
		span.SetStatus(contracts.RadarStatusError, resp.Status)
	}
	return resp, nil
}

// httpHeaderCarrier 适配 http.Header 为 RadarCarrier。
type httpHeaderCarrier struct {
	h http.Header
}

func (c httpHeaderCarrier) Get(key string) string { return c.h.Get(key) }
func (c httpHeaderCarrier) Set(key, value string) { c.h.Set(key, value) }
func (c httpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.h))
	for k := range c.h {
		keys = append(keys, k)
	}
	return keys
}
