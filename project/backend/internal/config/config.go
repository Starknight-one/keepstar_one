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
	OpenAIAPIKey    string
	EmbeddingModel  string
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
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		EmbeddingModel:  getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
	}
}

// HasDatabase returns true if database URL is configured
func (c *Config) HasDatabase() bool {
	return c.DatabaseURL != ""
}

// HasEmbeddings returns true if OpenAI API key is configured for embeddings
func (c *Config) HasEmbeddings() bool {
	return c.OpenAIAPIKey != ""
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
