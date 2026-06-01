package handlers

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PipelineGuard protects POST /api/v1/pipeline — the only public endpoint
// that triggers paid LLM calls — from runaway spend and abuse. It enforces
// two independent limits:
//
//   - a per-client-IP token-bucket rate limit (refills at ratePerMin tokens
//     per minute, bursts up to ratePerMin), and
//   - a global daily spend ceiling in USD, accumulated from the ACTUAL
//     per-turn Anthropic cost the pipeline reports (LLMUsage.CostUSD).
//
// Both counters are in-memory and process-local. That is correct for the
// current single-instance Railway deployment; if V5 ever scales out, these
// must move to a shared store (e.g. Redis). Until then a per-instance cap is
// a safe over-approximation — each instance caps itself, so total spend is
// bounded by capUSD × instances.
//
// A limit of <= 0 disables that check (used by tests and for explicit opt-out
// via env). A nil *PipelineGuard is also valid and disables all checks, so
// callers (e.g. live tests) may pass nil.
type PipelineGuard struct {
	mu  sync.Mutex
	now func() time.Time // injectable clock for tests
	log *slog.Logger

	// per-IP token bucket
	ratePerMin float64
	burst      float64
	buckets    map[string]*ipBucket

	// global daily spend ceiling
	capUSD   float64
	day      string // current UTC date key, "2006-01-02"
	spentUSD float64
}

type ipBucket struct {
	tokens   float64
	last     time.Time // last refill
	lastSeen time.Time // for eviction
}

// guardMaxBuckets bounds the per-IP map so an attacker rotating source IPs
// can't grow memory without limit. Past this size, idle buckets are evicted
// on insert.
const guardMaxBuckets = 50000

// NewPipelineGuard builds a guard. ratePerMin <= 0 disables rate limiting;
// dailyUSDCap <= 0 disables the spend ceiling.
func NewPipelineGuard(ratePerMin int, dailyUSDCap float64, log *slog.Logger) *PipelineGuard {
	return &PipelineGuard{
		now:        time.Now,
		log:        log,
		ratePerMin: float64(ratePerMin),
		burst:      float64(ratePerMin),
		buckets:    make(map[string]*ipBucket),
		capUSD:     dailyUSDCap,
	}
}

// Allow reports whether a pipeline request from ip may proceed. When it
// returns false, reason is "budget" (→ 503) or "rate" (→ 429).
func (g *PipelineGuard) Allow(ip string) (ok bool, reason string) {
	if g == nil {
		return true, ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()

	// Daily spend ceiling first: an exhausted budget short-circuits before
	// we hand out a rate token.
	if g.capUSD > 0 {
		g.rollDayLocked(now)
		if g.spentUSD >= g.capUSD {
			return false, "budget"
		}
	}

	// Per-IP token bucket.
	if g.ratePerMin > 0 {
		b := g.buckets[ip]
		if b == nil {
			g.evictLocked(now)
			b = &ipBucket{tokens: g.burst, last: now}
			g.buckets[ip] = b
		} else {
			elapsed := now.Sub(b.last).Seconds()
			if elapsed > 0 {
				b.tokens += elapsed * (g.ratePerMin / 60.0)
				if b.tokens > g.burst {
					b.tokens = g.burst
				}
				b.last = now
			}
		}
		b.lastSeen = now
		if b.tokens < 1 {
			return false, "rate"
		}
		b.tokens--
	}
	return true, ""
}

// RecordCost adds an executed turn's USD cost to today's running total.
// Call exactly once per successful pipeline turn.
func (g *PipelineGuard) RecordCost(usd float64) {
	if g == nil || g.capUSD <= 0 || usd <= 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rollDayLocked(g.now())
	g.spentUSD += usd
}

// rollDayLocked resets the spend total when the UTC date changes. Caller
// holds g.mu.
func (g *PipelineGuard) rollDayLocked(now time.Time) {
	d := now.UTC().Format("2006-01-02")
	if d != g.day {
		g.day = d
		g.spentUSD = 0
	}
}

// evictLocked drops idle IP buckets when the map grows past guardMaxBuckets.
// Called only on new-IP insertion, so the O(n) sweep is rare. Caller holds
// g.mu.
func (g *PipelineGuard) evictLocked(now time.Time) {
	if len(g.buckets) < guardMaxBuckets {
		return
	}
	cutoff := now.Add(-10 * time.Minute)
	for ip, b := range g.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(g.buckets, ip)
		}
	}
}

// clientIP extracts the best-effort client IP for rate limiting. Behind
// Railway's proxy the real client is the first entry of X-Forwarded-For;
// otherwise fall back to the connection's RemoteAddr (host part).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
