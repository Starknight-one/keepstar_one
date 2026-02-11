package config

import "os"

type Config struct {
	Port           string
	Environment    string
	DatabaseURL    string
	JWTSecret      string
	OpenAIAPIKey   string
	EmbeddingModel string
	LogLevel       string
	WidgetBaseURL  string
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", getEnv("ADMIN_PORT", "8081")),
		Environment:    getEnv("ENVIRONMENT", "development"),
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", "keepstar-admin-secret-change-me"),
		OpenAIAPIKey:   getEnv("OPENAI_API_KEY", ""),
		EmbeddingModel: getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		WidgetBaseURL:  getEnv("WIDGET_BASE_URL", ""),
	}
}

func (c *Config) HasDatabase() bool    { return c.DatabaseURL != "" }
func (c *Config) HasEmbeddings() bool   { return c.OpenAIAPIKey != "" }

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
