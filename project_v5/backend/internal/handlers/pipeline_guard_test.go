package handlers

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"
)

func guardTestLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPipelineGuard_RateLimit(t *testing.T) {
	g := NewPipelineGuard(3, 0, guardTestLogger()) // 3/min, budget disabled
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return base }

	// burst == 3 → first three pass, fourth is rate-limited.
	for i := 0; i < 3; i++ {
		if ok, reason := g.Allow("1.2.3.4"); !ok {
			t.Fatalf("request %d should pass, got reason %q", i, reason)
		}
	}
	if ok, reason := g.Allow("1.2.3.4"); ok || reason != "rate" {
		t.Fatalf("4th request should be rate-limited, got ok=%v reason=%q", ok, reason)
	}

	// A different IP gets its own bucket.
	if ok, _ := g.Allow("9.9.9.9"); !ok {
		t.Fatal("a different IP should have its own fresh bucket")
	}

	// Advance 20s → 3/min = 0.05 tok/s × 20s = exactly one token refilled.
	g.now = func() time.Time { return base.Add(20 * time.Second) }
	if ok, _ := g.Allow("1.2.3.4"); !ok {
		t.Fatal("after 20s one token should have refilled")
	}
	if ok, reason := g.Allow("1.2.3.4"); ok || reason != "rate" {
		t.Fatalf("only one token refilled; next should fail, got ok=%v reason=%q", ok, reason)
	}
}

func TestPipelineGuard_DailyBudget(t *testing.T) {
	g := NewPipelineGuard(0, 1.0, guardTestLogger()) // rate disabled, $1/day cap
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return base }

	if ok, _ := g.Allow("1.2.3.4"); !ok {
		t.Fatal("under budget should allow")
	}
	g.RecordCost(0.6)
	if ok, _ := g.Allow("1.2.3.4"); !ok {
		t.Fatal("0.6 < 1.0 — still under budget")
	}
	g.RecordCost(0.6) // running total 1.2 >= 1.0
	if ok, reason := g.Allow("1.2.3.4"); ok || reason != "budget" {
		t.Fatalf("over budget should be rejected, got ok=%v reason=%q", ok, reason)
	}

	// Next UTC day → spend resets.
	g.now = func() time.Time { return base.Add(24 * time.Hour) }
	if ok, _ := g.Allow("1.2.3.4"); !ok {
		t.Fatal("budget should reset on the next UTC day")
	}
}

func TestPipelineGuard_DisabledAndNil(t *testing.T) {
	g := NewPipelineGuard(0, 0, guardTestLogger()) // both limits off
	for i := 0; i < 100; i++ {
		if ok, _ := g.Allow("1.2.3.4"); !ok {
			t.Fatal("a fully-disabled guard should always allow")
		}
	}
	g.RecordCost(1000) // no-op when the cap is disabled

	var nilg *PipelineGuard
	if ok, reason := nilg.Allow("1.2.3.4"); !ok || reason != "" {
		t.Fatalf("nil guard should allow, got ok=%v reason=%q", ok, reason)
	}
	nilg.RecordCost(5) // must not panic
}

func TestClientIP(t *testing.T) {
	cases := []struct{ xff, remote, want string }{
		{"203.0.113.7", "10.0.0.1:5000", "203.0.113.7"},
		{"203.0.113.7, 70.0.0.1", "10.0.0.1:5000", "203.0.113.7"},
		{"", "10.0.0.1:5000", "10.0.0.1"},
		{"", "bad-remote-no-port", "bad-remote-no-port"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/api/v1/pipeline", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(r); got != c.want {
			t.Errorf("clientIP(xff=%q remote=%q) = %q, want %q", c.xff, c.remote, got, c.want)
		}
	}
}
