// Package config loads V5 backend configuration from environment
// variables. One Load() call at boot, no further env reads at runtime.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config carries every env-driven knob V5 reads.
//
//   - Port:            HTTP listen port (default "8082")
//   - DatabaseURL:     Postgres URL (Neon-flavoured); REQUIRED
//   - AnthropicAPIKey: Anthropic API key; REQUIRED for the LLM call
//   - LLMModel:        e.g. "claude-haiku-4-5" (default)
//   - OpenAIAPIKey:    OpenAI API key for embeddings (catalog_search
//                      vector half). Optional — when blank, catalog_search
//                      degrades to keyword-only (V4-style fallback).
//   - TenantSlug:      fallback tenant when X-Tenant-Slug header absent
//   - LogLevel:        slog level — "debug" | "info" | "warn" | "error"
//   - StaticDir:       optional dir served on `GET /` (e.g. ./static for
//                      a vite-built widget bundle); empty disables it
type Config struct {
	Port            string
	DatabaseURL     string
	AnthropicAPIKey string
	LLMModel        string
	OpenAIAPIKey    string
	TenantSlug      string
	LogLevel        slog.Level
	StaticDir       string
	// PipelineRatePerMin caps POST /api/v1/pipeline calls per client IP per
	// minute (token bucket; burst == this value). <= 0 disables the limit.
	PipelineRatePerMin int
	// PipelineDailyUSDCap is the global per-day Anthropic spend ceiling for
	// the pipeline, summed from the actual reported per-turn cost. Once the
	// day's spend reaches it, /api/v1/pipeline returns 503 until UTC
	// midnight. <= 0 disables the cap. Process-local (correct for the
	// current single-instance deploy; revisit if V5 scales horizontally).
	PipelineDailyUSDCap float64
}

// Load reads env vars, applies defaults, and validates required fields.
// Returns a non-nil error when something required is missing.
func Load() (*Config, error) {
	c := &Config{
		Port:            envOr("PORT", "8082"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		LLMModel:        envOr("LLM_MODEL", "claude-haiku-4-5"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		TenantSlug:      envOr("TENANT_SLUG", "hey-babes-cosmetics"),
		LogLevel:        parseLogLevel(envOr("LOG_LEVEL", "info")),
		StaticDir:       envOr("STATIC_DIR", "./static"),

		PipelineRatePerMin:  envInt("PIPELINE_RATE_PER_MIN", 30),
		PipelineDailyUSDCap: envFloat("PIPELINE_DAILY_USD_CAP", 10.0),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.AnthropicAPIKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	return c, nil
}

// MustLoad is the convenience wrapper used by main — panics on missing
// config so misconfigured deployments fail loud at boot.
func MustLoad() *Config {
	c, err := Load()
	if err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}
	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
