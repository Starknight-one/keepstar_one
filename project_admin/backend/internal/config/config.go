package config

import "os"

type Config struct {
	Port            string
	Environment     string
	DatabaseURL     string
	JWTSecret       string
	OpenAIAPIKey    string
	EmbeddingModel  string
	AnthropicAPIKey string
	EnrichmentModel string
	LogLevel        string
	WidgetBaseURL   string
	ChatAPIURL      string
	EncryptionKey   string // base64 of 32 random bytes — see internal/crypto/secretbox
	ShopifyAPIKey   string
	ShopifyAPISecret string
	PublicBaseURL   string // canonical admin URL for OAuth redirects and webhook endpoints
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", getEnv("ADMIN_PORT", "8081")),
		Environment:    getEnv("ENVIRONMENT", "development"),
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", "keepstar-admin-secret-change-me"),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		EmbeddingModel:  getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		EnrichmentModel: getEnv("ENRICHMENT_MODEL", "claude-haiku-4-5-20251001"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		WidgetBaseURL:  getEnv("WIDGET_BASE_URL", ""),
		ChatAPIURL:     getEnv("CHAT_API_URL", ""),
		EncryptionKey:  getEnv("ADMIN_ENCRYPTION_KEY", ""),
		ShopifyAPIKey:    getEnv("SHOPIFY_API_KEY", ""),
		ShopifyAPISecret: getEnv("SHOPIFY_API_SECRET", ""),
		PublicBaseURL:  getEnv("PUBLIC_BASE_URL", ""),
	}
}

func (c *Config) HasDatabase() bool    { return c.DatabaseURL != "" }
func (c *Config) HasEmbeddings() bool   { return c.OpenAIAPIKey != "" }
func (c *Config) HasEnrichment() bool   { return c.AnthropicAPIKey != "" }
func (c *Config) HasEncryption() bool   { return c.EncryptionKey != "" }
func (c *Config) HasShopify() bool      { return c.ShopifyAPIKey != "" && c.ShopifyAPISecret != "" }

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
