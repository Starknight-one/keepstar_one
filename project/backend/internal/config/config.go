package config

import "os"

// Config holds application configuration
type Config struct {
	Port            string
	Environment     string
	AnthropicAPIKey string
	LLMModel        string
	LogLevel        string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		Environment:     getEnv("ENVIRONMENT", "development"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		LLMModel:        getEnv("LLM_MODEL", "claude-3-haiku-20240307"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
