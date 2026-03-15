package re3

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

// startSpan starts a child span when OTEL is enabled. Returns (ctx, end) where end() must be deferred.
// Logs and metrics recorded with this ctx will be associated with the span.
func startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func()) {
	if !otelEnabled {
		return ctx, func() {}
	}
	ctx, span := otel.Tracer("re3").Start(ctx, name)
	for _, a := range attrs {
		span.SetAttributes(a)
	}
	return ctx, func() {
		span.End()
	}
}

// syncWriter serializes writes to an io.Writer for concurrent use by trace and metric exporters.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

var (
	otelInitOnce sync.Once
	otelEnabled  bool
	otelShutdown func(context.Context) error
)

// isOTelEnabled returns true if OTEL_ENABLED is truthy (true, 1, yes, on).
func isOTelEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_ENABLED")))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// InitOTel initializes OpenTelemetry when OTEL_ENABLED is set (true, 1, yes, on).
// Uses OTEL_FILE_PATH (or OTEL_FILE) for the output file; if unset, defaults to
// metrics.log in the current directory. run_benchmarks.sh --enable-metrics
// passes the file path via OTEL_FILE_PATH. Safe to call multiple times.
func InitOTel() {
	otelInitOnce.Do(func() {
		if !isOTelEnabled() {
			return
		}
		path := os.Getenv("OTEL_FILE_PATH")
		if path == "" {
			path = os.Getenv("OTEL_FILE")
		}
		if path == "" {
			path = "metrics.log"
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		bw := bufio.NewWriterSize(f, 100<<20) // 100MB buffer to reduce lock contention
		sw := &syncWriter{w: bw}
		otelEnabled = true

		// Trace exporter
		traceExp, err := stdouttrace.New(
			stdouttrace.WithWriter(sw),
		)
		if err != nil {
			return
		}
		tp := trace.NewTracerProvider(
			trace.WithBatcher(traceExp),
			trace.WithSampler(trace.AlwaysSample()),
		)
		otel.SetTracerProvider(tp)

		// Metric exporter
		metricExp, err := stdoutmetric.New(
			stdoutmetric.WithWriter(sw),
		)
		if err != nil {
			return
		}
		mp := metric.NewMeterProvider(
			metric.WithReader(metric.NewPeriodicReader(metricExp)),
		)
		otel.SetMeterProvider(mp)

		otelShutdown = func(ctx context.Context) error {
			_ = tp.Shutdown(ctx)
			_ = mp.Shutdown(ctx)
			_ = bw.Flush()
			_ = f.Close()
			return nil
		}
	})
}

// OTelEnabled reports whether OpenTelemetry was successfully initialized.
func OTelEnabled() bool {
	return otelEnabled
}

// ShutdownOTel flushes and shuts down the OpenTelemetry providers. Call before
// process exit when OTelEnabled() is true.
func ShutdownOTel(ctx context.Context) {
	if otelShutdown != nil {
		_ = otelShutdown(ctx)
		otelShutdown = nil
	}
}

func init() {
	InitOTel()
}
