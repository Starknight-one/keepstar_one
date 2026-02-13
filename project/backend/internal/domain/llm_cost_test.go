package domain

import (
	"math"
	"testing"
)

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestCalculateCost_HaikuBasic(t *testing.T) {
	u := &LLMUsage{
		Model:        "claude-haiku-4-5-20251001",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 1.0 + 5.0 = 6.0
	if !almostEqual(cost, 6.0, 0.001) {
		t.Errorf("Haiku basic: want 6.0, got %f", cost)
	}
}

func TestCalculateCost_SonnetBasic(t *testing.T) {
	u := &LLMUsage{
		Model:        "claude-sonnet-4-5-20251014",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 3.0 + 15.0 = 18.0
	if !almostEqual(cost, 18.0, 0.001) {
		t.Errorf("Sonnet basic: want 18.0, got %f", cost)
	}
}

func TestCalculateCost_OpusBasic(t *testing.T) {
	u := &LLMUsage{
		Model:        "claude-opus-4-5-20251101",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 5.0 + 25.0 = 30.0
	if !almostEqual(cost, 30.0, 0.001) {
		t.Errorf("Opus basic: want 30.0, got %f", cost)
	}
}

func TestCalculateCost_CacheWrite125x(t *testing.T) {
	u := &LLMUsage{
		Model:                    "claude-sonnet-4-5-20251014",
		InputTokens:              0,
		OutputTokens:             0,
		CacheCreationInputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 3.0 * 1.25 = 3.75
	if !almostEqual(cost, 3.75, 0.001) {
		t.Errorf("Cache write 1.25x: want 3.75, got %f", cost)
	}
}

func TestCalculateCost_CacheRead01x(t *testing.T) {
	u := &LLMUsage{
		Model:                "claude-sonnet-4-5-20251014",
		InputTokens:          0,
		OutputTokens:         0,
		CacheReadInputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 3.0 * 0.1 = 0.3
	if !almostEqual(cost, 0.3, 0.001) {
		t.Errorf("Cache read 0.1x: want 0.3, got %f", cost)
	}
}

func TestCalculateCost_MixedCacheAndRegular(t *testing.T) {
	u := &LLMUsage{
		Model:                    "claude-haiku-4-5-20251001",
		InputTokens:              500_000,
		OutputTokens:             200_000,
		CacheCreationInputTokens: 300_000,
		CacheReadInputTokens:     400_000,
	}
	cost := u.CalculateCost()
	// input: 500k * 1.0 / 1M = 0.5
	// cache write: 300k * 1.0 * 1.25 / 1M = 0.375
	// cache read: 400k * 1.0 * 0.1 / 1M = 0.04
	// output: 200k * 5.0 / 1M = 1.0
	want := 0.5 + 0.375 + 0.04 + 1.0
	if !almostEqual(cost, want, 0.001) {
		t.Errorf("Mixed: want %f, got %f", want, cost)
	}
}

func TestCalculateCost_UnknownModelFallback(t *testing.T) {
	u := &LLMUsage{
		Model:        "unknown-model-x",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// Fallback to Haiku pricing: 1.0 + 5.0 = 6.0
	if !almostEqual(cost, 6.0, 0.001) {
		t.Errorf("Unknown model fallback: want 6.0 (Haiku), got %f", cost)
	}
}

func TestCalculateCost_ZeroTokens(t *testing.T) {
	u := &LLMUsage{
		Model:        "claude-sonnet-4-5-20251014",
		InputTokens:  0,
		OutputTokens: 0,
	}
	cost := u.CalculateCost()
	if cost != 0 {
		t.Errorf("Zero tokens: want 0, got %f", cost)
	}
}

func TestCalculateCost_Haiku3Legacy(t *testing.T) {
	u := &LLMUsage{
		Model:        "claude-3-haiku-20240307",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 0.25 + 1.25 = 1.5
	if !almostEqual(cost, 1.5, 0.001) {
		t.Errorf("Haiku 3 legacy: want 1.5, got %f", cost)
	}
}

func TestCalculateCost_Sonnet35Legacy(t *testing.T) {
	u := &LLMUsage{
		Model:        "claude-3-5-sonnet-20241022",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := u.CalculateCost()
	// 3.0 + 15.0 = 18.0
	if !almostEqual(cost, 18.0, 0.001) {
		t.Errorf("Sonnet 3.5 legacy: want 18.0, got %f", cost)
	}
}
