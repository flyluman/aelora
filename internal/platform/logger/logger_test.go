package logger

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewLoggerFallbackLevel(t *testing.T) {
	log := New("svc", "test", "not-a-level")
	if log == nil {
		t.Fatal("expected logger")
	}
}

func TestNewLoggerIncludesCallerMetadata(t *testing.T) {
	log := New("svc", "test", "info")
	defer func() { _ = log.Sync() }()

	var entry zapcore.Entry
	log = log.WithOptions(zap.Hooks(func(next zapcore.Entry) error {
		entry = next
		return nil
	}))

	log.Info("caller-test")

	if !entry.Caller.Defined {
		t.Fatal("expected caller metadata to be populated")
	}
	if entry.Caller.Line == 0 {
		t.Fatal("expected caller line number")
	}
}
