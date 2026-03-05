package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceCheck_AllHealthy(t *testing.T) {
	svc := NewService(
		time.Second,
		Check{Name: "valkey", Func: func(context.Context) error { return nil }},
		Check{Name: "postgres", Func: func(context.Context) error { return nil }},
	)

	report := svc.Check(context.Background())
	if report.Status != "ok" {
		t.Fatalf("expected overall status ok, got %q", report.Status)
	}
	if report.Dependencies["valkey"].Status != "ok" {
		t.Fatalf("expected valkey ok")
	}
	if report.Dependencies["postgres"].Status != "ok" {
		t.Fatalf("expected postgres ok")
	}
}

func TestServiceCheck_DegradedWhenDependencyFails(t *testing.T) {
	svc := NewService(
		time.Second,
		Check{Name: "valkey", Func: func(context.Context) error { return errors.New("ping failed") }},
		Check{Name: "postgres", Func: func(context.Context) error { return nil }},
	)

	report := svc.Check(context.Background())
	if report.Status != "degraded" {
		t.Fatalf("expected overall status degraded, got %q", report.Status)
	}
	if report.Dependencies["valkey"].Status != "down" {
		t.Fatalf("expected valkey down")
	}
	if report.Dependencies["postgres"].Status != "ok" {
		t.Fatalf("expected postgres ok")
	}
}
