package radar

import (
	"context"

	"github.com/zhoudm1743/go-fast-framework/contracts"
	"go.opentelemetry.io/otel/metric"
)

// radarMeter 包装 otel metric.Meter。
type radarMeter struct {
	m metric.Meter
}

// instrumentOpts 契约仪器选项 → otel InstrumentOption（该接口内嵌所有仪器选项类型）。
func instrumentOpts(opts []contracts.RadarInstrumentOption) []metric.InstrumentOption {
	cfg := &contracts.RadarInstrumentConfig{}
	for _, o := range opts {
		o(cfg)
	}
	var out []metric.InstrumentOption
	if cfg.Description != "" {
		out = append(out, metric.WithDescription(cfg.Description))
	}
	if cfg.Unit != "" {
		out = append(out, metric.WithUnit(cfg.Unit))
	}
	return out
}

func asInt64CounterOpts(in []metric.InstrumentOption) []metric.Int64CounterOption {
	out := make([]metric.Int64CounterOption, len(in))
	for i, o := range in {
		out[i] = o
	}
	return out
}

func asInt64UpDownCounterOpts(in []metric.InstrumentOption) []metric.Int64UpDownCounterOption {
	out := make([]metric.Int64UpDownCounterOption, len(in))
	for i, o := range in {
		out[i] = o
	}
	return out
}

func asFloat64HistogramOpts(in []metric.InstrumentOption) []metric.Float64HistogramOption {
	out := make([]metric.Float64HistogramOption, len(in))
	for i, o := range in {
		out[i] = o
	}
	return out
}

func asInt64ObservableGaugeOpts(in []metric.InstrumentOption) []metric.Int64ObservableGaugeOption {
	out := make([]metric.Int64ObservableGaugeOption, len(in))
	for i, o := range in {
		out[i] = o
	}
	return out
}

func (m *radarMeter) Int64Counter(name string, opts ...contracts.RadarInstrumentOption) (contracts.RadarInt64Counter, error) {
	c, err := m.m.Int64Counter(name, asInt64CounterOpts(instrumentOpts(opts))...)
	if err != nil {
		return nil, err
	}
	return &radarInt64Counter{add: c.Add}, nil
}

func (m *radarMeter) Int64UpDownCounter(name string, opts ...contracts.RadarInstrumentOption) (contracts.RadarInt64Counter, error) {
	c, err := m.m.Int64UpDownCounter(name, asInt64UpDownCounterOpts(instrumentOpts(opts))...)
	if err != nil {
		return nil, err
	}
	return &radarInt64Counter{add: c.Add}, nil
}

func (m *radarMeter) Float64Histogram(name string, opts ...contracts.RadarInstrumentOption) (contracts.RadarFloat64Histogram, error) {
	h, err := m.m.Float64Histogram(name, asFloat64HistogramOpts(instrumentOpts(opts))...)
	if err != nil {
		return nil, err
	}
	return &radarFloat64Histogram{record: h.Record}, nil
}

func (m *radarMeter) Int64ObservableGauge(name string, callback func(ctx context.Context) int64, opts ...contracts.RadarInstrumentOption) error {
	_, err := m.m.Int64ObservableGauge(name, append(asInt64ObservableGaugeOpts(instrumentOpts(opts)),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			o.Observe(callback(ctx))
			return nil
		}))...)
	return err
}

// radarInt64Counter 包装 Counter / UpDownCounter 的 Add。
type radarInt64Counter struct {
	add func(ctx context.Context, value int64, options ...metric.AddOption)
}

func (c *radarInt64Counter) Add(ctx context.Context, value int64, attrs ...contracts.RadarAttr) {
	c.add(ctx, value, metric.WithAttributes(toOtelAttrs(attrs)...))
}

// radarFloat64Histogram 包装直方图 Record。
type radarFloat64Histogram struct {
	record func(ctx context.Context, value float64, options ...metric.RecordOption)
}

func (h *radarFloat64Histogram) Record(ctx context.Context, value float64, attrs ...contracts.RadarAttr) {
	h.record(ctx, value, metric.WithAttributes(toOtelAttrs(attrs)...))
}
