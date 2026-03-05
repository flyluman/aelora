package config

import (
	"os"
	"testing"
)

func TestGetenvHelpersFallback(t *testing.T) {
	if got := getenv("__AELORA_NOT_SET__", "x"); got != "x" {
		t.Fatalf("expected fallback, got %q", got)
	}
	if got := getenvInt("__AELORA_NOT_SET_INT__", 7); got != 7 {
		t.Fatalf("expected int fallback, got %d", got)
	}
	if got := getenvInt64("__AELORA_NOT_SET_INT64__", 9); got != 9 {
		t.Fatalf("expected int64 fallback, got %d", got)
	}
	if got := getenvBool("__AELORA_NOT_SET_BOOL__", true); !got {
		t.Fatalf("expected bool fallback true")
	}
}

func TestLoadReadsConfiguredValues(t *testing.T) {
	t.Setenv("AELORA_HTTP_ADDR", ":9999")
	t.Setenv("AELORA_AUTH_ENABLED", "false")
	t.Setenv("AELORA_FRONTEND_SIDECAR_ENABLED", "true")
	t.Setenv("AELORA_FRONTEND_ADDR", ":18081")
	t.Setenv("AELORA_VALKEY_ROOM_STREAM_MAXLEN", "123")
	t.Setenv("AELORA_VALKEY_EVENT_STREAM_MAXLEN", "456")
	t.Setenv("AELORA_HTTP_REQUEST_LOG_HEALTH_SAMPLE_EVERY", "9")
	t.Setenv("AELORA_HTTP_REQUEST_LOG_MESSAGE_LEVEL", "debug")
	cfg := Load()
	if cfg.HTTPAddr != ":9999" {
		t.Fatalf("unexpected http addr: %s", cfg.HTTPAddr)
	}
	if cfg.AuthEnabled {
		t.Fatalf("expected auth disabled")
	}
	if !cfg.FrontendSidecarEnabled {
		t.Fatalf("expected frontend sidecar enabled")
	}
	if cfg.FrontendAddr != ":18081" {
		t.Fatalf("unexpected frontend addr: %s", cfg.FrontendAddr)
	}
	if cfg.ValkeyRoomStreamMaxLen != 123 || cfg.ValkeyEventStreamMaxLen != 456 {
		t.Fatalf("unexpected stream maxlens: %d %d", cfg.ValkeyRoomStreamMaxLen, cfg.ValkeyEventStreamMaxLen)
	}
	if cfg.RequestLogHealthSampleEvery != 9 {
		t.Fatalf("unexpected health log sample every: %d", cfg.RequestLogHealthSampleEvery)
	}
	if cfg.RequestLogMessageSuccessLevel != "debug" {
		t.Fatalf("unexpected message log level: %s", cfg.RequestLogMessageSuccessLevel)
	}
	_ = os.Getenv("AELORA_HTTP_ADDR")
}
