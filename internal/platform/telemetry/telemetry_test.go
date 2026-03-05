package telemetry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestMetricsHandlerIncludesCustomCounters(t *testing.T) {
	IncStreamAppendFailure()
	IncEventPublishFailure()
	IncOutboxEnqueued()
	IncOutboxEnqueueFailure()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, name := range []string{
		"aelora_stream_append_failures_total",
		"aelora_event_publish_failures_total",
		"aelora_outbox_enqueued_total",
		"aelora_outbox_enqueue_failures_total",
		"process_cpu_seconds_total",
		"process_uptime_seconds",
		"go_goroutines",
		"go_threads",
		"go_gc_cycles_total",
		"go_memory_heap_objects_bytes",
		"go_memory_total_bytes",
		"go_gomaxprocs",
	} {
		if !strings.Contains(body, name) {
			t.Fatalf("expected metric %s in output", name)
		}
	}
}

type noHijackWriter struct {
	http.ResponseWriter
}

func TestTimingWriterHijackUnsupported(t *testing.T) {
	tw := &timingWriter{ResponseWriter: noHijackWriter{ResponseWriter: httptest.NewRecorder()}}
	if _, _, err := tw.Hijack(); err == nil {
		t.Fatal("expected hijack error")
	}
}

func TestTimingWriterPushUnsupported(t *testing.T) {
	tw := &timingWriter{ResponseWriter: noHijackWriter{ResponseWriter: httptest.NewRecorder()}}
	if err := tw.Push("/x", nil); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("expected not supported, got %v", err)
	}
}

func TestWithRequestMetricsAndTraceFromHeader(t *testing.T) {
	initTestOTel()
	defer shutdownTestOTel()

	h := WithRequestMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if TraceIDFromContext(r.Context()) == "" {
			t.Fatal("expected trace id in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Pattern = "GET /x"
	req.Header.Set("X-Trace-ID", "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Trace-ID") == "" {
		t.Fatalf("expected trace header in response")
	}
	if rec.Header().Get("Server-Timing") == "" {
		t.Fatalf("expected server timing header")
	}
}

func TestTraceparentParsing(t *testing.T) {
	initTestOTel()
	defer shutdownTestOTel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h := WithRequestMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid := TraceIDFromContext(r.Context())
		if tid != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("expected trace id from traceparent, got %q", tid)
		}
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Trace-ID") == "" {
		t.Fatalf("expected trace header in response")
	}
}

func TestTraceparentParsingInvalid(t *testing.T) {
	initTestOTel()
	defer shutdownTestOTel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "bad")
	h := WithRequestMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid := TraceIDFromContext(r.Context())
		if tid == "" {
			t.Fatal("expected a generated trace id even with bad traceparent")
		}
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
}

func TestNewTraceIDFallbackPath(t *testing.T) {
	id := newTraceID()
	if len(id) == 0 {
		t.Fatal("expected trace id")
	}
}

type flushWriter struct {
	*httptest.ResponseRecorder
}

func (f flushWriter) Flush() {}

func TestTimingWriterFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	tw := &timingWriter{ResponseWriter: flushWriter{rec}}
	tw.Flush()
}

func initTestOTel() {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	testTP = tp
}

func shutdownTestOTel() {
	if testTP != nil {
		_ = testTP.Shutdown(context.Background())
		testTP = nil
	}
}

var testTP *sdktrace.TracerProvider

var _ = context.Background
var _ net.Conn
