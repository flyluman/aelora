package system

import (
	"testing"
	"time"
)

func TestIDGeneratorProducesULIDLikeIDs(t *testing.T) {
	gen := NewIDGenerator()
	a := gen.New()
	b := gen.New()
	if len(a) != 26 || len(b) != 26 {
		t.Fatalf("expected 26-char ids, got %d and %d", len(a), len(b))
	}
	if a == b {
		t.Fatal("expected unique ids")
	}
}

func TestClockNow(t *testing.T) {
	clk := NewClock()
	now := clk.Now()
	if now.IsZero() {
		t.Fatal("expected non-zero time")
	}
	if now.After(time.Now().Add(1 * time.Second)) {
		t.Fatal("unexpected future time")
	}
}
