package config

import "os"

// Config holds application configuration
type Config struct {
	Port            string
	Environment     string
	AnthropicAPIKey string
	LLMModel        string
	LogLevel        string
	DatabaseURL     string
	TenantSlug      string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		Environment:     getEnv("ENVIRONMENT", "development"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		LLMModel:        getEnv("LLM_MODEL", "claude-haiku-4-5-20251001"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		DatabaseURL:     getEnv("DATABASE_URL", ""),
		TenantSlug:      getEnv("TENANT_SLUG", "nike"),
	}
}

// HasDatabase returns true if database URL is configured
func (c *Config) HasDatabase() bool {
	return c.DatabaseURL != ""
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
