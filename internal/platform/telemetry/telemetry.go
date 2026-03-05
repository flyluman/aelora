package telemetry

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	runtimemetrics "runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type traceKey struct{}

var (
	totalRequests atomic.Int64
	routeCounts   sync.Map

	streamAppendFailures atomic.Int64
	eventPublishFailures atomic.Int64
	outboxEnqueueTotal   atomic.Int64
	outboxEnqueueFailure atomic.Int64
	processStartedAt     = time.Now()

	meter = otel.Meter("aelora")

	otelStreamAppendFailures metric.Int64Counter
	otelEventPublishFailures metric.Int64Counter
	otelOutboxEnqueuedTotal  metric.Int64Counter
	otelOutboxEnqueueFailure metric.Int64Counter
)

func init() {
	var err error
	otelStreamAppendFailures, err = meter.Int64Counter("aelora.stream.append.failures",
		metric.WithDescription("Number of stream append failures"))
	if err != nil {
		panic(err)
	}
	otelEventPublishFailures, err = meter.Int64Counter("aelora.event.publish.failures",
		metric.WithDescription("Number of event publish failures"))
	if err != nil {
		panic(err)
	}
	otelOutboxEnqueuedTotal, err = meter.Int64Counter("aelora.outbox.enqueued.total",
		metric.WithDescription("Number of outbox enqueued messages"))
	if err != nil {
		panic(err)
	}
	otelOutboxEnqueueFailure, err = meter.Int64Counter("aelora.outbox.enqueue.failures",
		metric.WithDescription("Number of outbox enqueue failures"))
	if err != nil {
		panic(err)
	}
}

type Telemetry struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
}

func NewProvider(ctx context.Context, serviceName string, opts ...NewProviderOption) (*Telemetry, error) {
	cfg := defaultProviderConfig()
	for _, o := range opts {
		o.apply(&cfg)
	}

	res := resource.NewWithAttributes(
		"https://opentelemetry.io/schemas/1.26.0",
		attribute.String("service.name", serviceName),
	)

	var tp *sdktrace.TracerProvider
	if cfg.traceEndpoint != "" {
		traceExporter, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.traceEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create trace exporter: %w", err)
		}
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithBatcher(traceExporter),
		)
	} else {
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
	}

	var mp *sdkmetric.MeterProvider
	if cfg.metricEndpoint != "" {
		metricExporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.metricEndpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			_ = tp.Shutdown(ctx)
			return nil, fmt.Errorf("create metric exporter: %w", err)
		}
		mp = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		)
	} else {
		mp = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
		)
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Telemetry{
		tracerProvider: tp,
		meterProvider:  mp,
	}, nil
}

type newProviderConfig struct {
	traceEndpoint  string
	metricEndpoint string
}

type NewProviderOption interface {
	apply(*newProviderConfig)
}

type withTraceEndpoint string

func (e withTraceEndpoint) apply(cfg *newProviderConfig) {
	cfg.traceEndpoint = string(e)
}

func WithTraceEndpoint(endpoint string) NewProviderOption {
	return withTraceEndpoint(endpoint)
}

type withMetricEndpoint string

func (e withMetricEndpoint) apply(cfg *newProviderConfig) {
	cfg.metricEndpoint = string(e)
}

func WithMetricEndpoint(endpoint string) NewProviderOption {
	return withMetricEndpoint(endpoint)
}

func defaultProviderConfig() newProviderConfig {
	return newProviderConfig{}
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if err := t.tracerProvider.Shutdown(ctx); err != nil {
		return err
	}
	return t.meterProvider.Shutdown(ctx)
}

type runtimeMetricSnapshot struct {
	CPUSeconds    float64
	UptimeSeconds float64
	Goroutines    uint64
	Threads       uint64
	GCCycles      uint64
	HeapObjects   uint64
	TotalMemory   uint64
	GOMAXPROCS    uint64
}

type timingWriter struct {
	http.ResponseWriter
	start       time.Time
	wroteHeader bool
	statusCode  int
	hijacked    bool
}

func (w *timingWriter) WriteHeader(statusCode int) {
	if w.hijacked {
		return
	}
	if !w.wroteHeader {
		dur := time.Since(w.start).Milliseconds()
		w.Header().Set("Server-Timing", "app;dur="+strconv.FormatInt(dur, 10))
		w.wroteHeader = true
		w.statusCode = statusCode
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *timingWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *timingWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *timingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not implement hijacker")
	}
	w.hijacked = true
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = http.StatusSwitchingProtocols
	}
	return hijacker.Hijack()
}

func (w *timingWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		runtime := readRuntimeMetricSnapshot()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "aelora_http_requests_total %d\n", totalRequests.Load())
		routeCounts.Range(func(key, value any) bool {
			k, _ := key.(string)
			v, _ := value.(*atomic.Int64)
			parts := strings.Split(k, "|")
			if len(parts) != 3 {
				return true
			}
			fmt.Fprintf(w, "aelora_http_route_requests_total{method=%q,route=%q,status=%q} %d\n", parts[0], parts[1], parts[2], v.Load())
			return true
		})
		fmt.Fprintf(w, "aelora_stream_append_failures_total %d\n", streamAppendFailures.Load())
		fmt.Fprintf(w, "aelora_event_publish_failures_total %d\n", eventPublishFailures.Load())
		fmt.Fprintf(w, "aelora_outbox_enqueued_total %d\n", outboxEnqueueTotal.Load())
		fmt.Fprintf(w, "aelora_outbox_enqueue_failures_total %d\n", outboxEnqueueFailure.Load())
		fmt.Fprintf(w, "process_cpu_seconds_total %g\n", runtime.CPUSeconds)
		fmt.Fprintf(w, "process_uptime_seconds %g\n", runtime.UptimeSeconds)
		fmt.Fprintf(w, "go_goroutines %d\n", runtime.Goroutines)
		fmt.Fprintf(w, "go_threads %d\n", runtime.Threads)
		fmt.Fprintf(w, "go_gc_cycles_total %d\n", runtime.GCCycles)
		fmt.Fprintf(w, "go_memory_heap_objects_bytes %d\n", runtime.HeapObjects)
		fmt.Fprintf(w, "go_memory_total_bytes %d\n", runtime.TotalMemory)
		fmt.Fprintf(w, "go_gomaxprocs %d\n", runtime.GOMAXPROCS)
	})
}

func readRuntimeMetricSnapshot() runtimeMetricSnapshot {
	samples := []runtimemetrics.Sample{
		{Name: "/cpu/classes/total:cpu-seconds"},
		{Name: "/cpu/classes/idle:cpu-seconds"},
		{Name: "/sched/goroutines:goroutines"},
		{Name: "/sched/threads/total:threads"},
		{Name: "/gc/cycles/total:gc-cycles"},
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/total:bytes"},
		{Name: "/sched/gomaxprocs:threads"},
	}
	runtimemetrics.Read(samples)

	snapshot := runtimeMetricSnapshot{
		UptimeSeconds: time.Since(processStartedAt).Seconds(),
	}
	var totalCPU float64
	var idleCPU float64
	for _, sample := range samples {
		switch sample.Name {
		case "/cpu/classes/total:cpu-seconds":
			totalCPU = runtimeMetricFloat64(sample.Value)
		case "/cpu/classes/idle:cpu-seconds":
			idleCPU = runtimeMetricFloat64(sample.Value)
		case "/sched/goroutines:goroutines":
			snapshot.Goroutines = runtimeMetricUint64(sample.Value)
		case "/sched/threads/total:threads":
			snapshot.Threads = runtimeMetricUint64(sample.Value)
		case "/gc/cycles/total:gc-cycles":
			snapshot.GCCycles = runtimeMetricUint64(sample.Value)
		case "/memory/classes/heap/objects:bytes":
			snapshot.HeapObjects = runtimeMetricUint64(sample.Value)
		case "/memory/classes/total:bytes":
			snapshot.TotalMemory = runtimeMetricUint64(sample.Value)
		case "/sched/gomaxprocs:threads":
			snapshot.GOMAXPROCS = runtimeMetricUint64(sample.Value)
		}
	}
	if totalCPU > idleCPU {
		snapshot.CPUSeconds = totalCPU - idleCPU
	}
	return snapshot
}

func runtimeMetricFloat64(value runtimemetrics.Value) float64 {
	if value.Kind() != runtimemetrics.KindFloat64 {
		return 0
	}
	return value.Float64()
}

func runtimeMetricUint64(value runtimemetrics.Value) uint64 {
	if value.Kind() != runtimemetrics.KindUint64 {
		return 0
	}
	return value.Uint64()
}

func IncStreamAppendFailure() {
	streamAppendFailures.Add(1)
	otelStreamAppendFailures.Add(context.Background(), 1)
}

func IncEventPublishFailure() {
	eventPublishFailures.Add(1)
	otelEventPublishFailures.Add(context.Background(), 1)
}

func IncOutboxEnqueued() {
	outboxEnqueueTotal.Add(1)
	otelOutboxEnqueuedTotal.Add(context.Background(), 1)
}

func IncOutboxEnqueueFailure() {
	outboxEnqueueFailure.Add(1)
	otelOutboxEnqueueFailure.Add(context.Background(), 1)
}

func WithRequestMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))

		tracer := otel.Tracer("aelora")
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path)
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
		)

		traceID := span.SpanContext().TraceID().String()
		if traceID == "" {
			traceID = newTraceID()
		}
		ctx = context.WithValue(ctx, traceKey{}, traceID)

		tw := &timingWriter{ResponseWriter: w, start: time.Now(), statusCode: http.StatusOK}
		tw.Header().Set("X-Trace-ID", traceID)
		next.ServeHTTP(tw, r.WithContext(ctx))
		if !tw.wroteHeader && !tw.hijacked {
			tw.WriteHeader(http.StatusOK)
		}

		span.SetAttributes(attribute.Int("http.status_code", tw.statusCode))

		totalRequests.Add(1)
		status := strconv.Itoa(tw.statusCode)
		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}
		key := r.Method + "|" + route + "|" + status
		counter, _ := routeCounts.LoadOrStore(key, &atomic.Int64{})
		counter.(*atomic.Int64).Add(1)
	})
}

func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	v, _ := ctx.Value(traceKey{}).(string)
	return v
}

func newTraceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}
