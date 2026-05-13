package config

import (
	"os"
	"time"
)

type Config struct {
	Port             string
	Environment      string
	DatabaseURL      string
	JWTSecret        string
	OpenAIAPIKey     string
	EmbeddingModel   string
	AnthropicAPIKey  string
	EnrichmentModel  string
	LogLevel         string
	WidgetBaseURL    string
	ChatAPIURL       string
	EncryptionKey    string // base64 of 32 random bytes — see internal/crypto/secretbox
	ShopifyAPIKey     string
	ShopifyAPISecret  string
	ShopifyAPIVersion string // e.g. "2026-04"; falls back to client default if empty
	ShopifyScopes     string // comma-separated; MUST match scopes in Shopify App config
	PublicBaseURL     string // canonical admin URL for OAuth redirects and webhook endpoints

	// SMTP — transactional email (password reset, email verify, invites, 2FA code)
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string

	// Resend HTTP API — preferred over SMTP. When ResendAPIKey is set we use
	// HTTPS to api.resend.com instead of stdlib smtp (which hangs silently
	// on Railway egress). SMTPFrom + SMTPFromName double as the From header
	// for both transports.
	ResendAPIKey string

	// Google OAuth
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	GoogleOAuthRedirectURL  string

	// Telegram — bot token used for both OIDC (client_secret) and legacy widget HMAC.
	// TelegramOAuthRedirectURL defaults to AUTH_PUBLIC_BASE_URL+"/auth/telegram/callback".
	TelegramBotToken          string
	TelegramBotUsername       string
	TelegramOAuthRedirectURL  string

	// Auth lifetimes
	AuthTOTPIssuer   string
	AuthAccessTTL    time.Duration
	AuthRefreshTTL   time.Duration
	AuthInviteTTL    time.Duration
	AuthEmailCodeTTL time.Duration
	AuthResetTTL     time.Duration
	AuthPre2FATTL    time.Duration
	AuthPublicBaseURL string
}

func Load() *Config {
	return &Config{
		Port:             getEnv("PORT", getEnv("ADMIN_PORT", "8081")),
		Environment:      getEnv("ENVIRONMENT", "development"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		JWTSecret:        getEnv("JWT_SECRET", "keepstar-admin-secret-change-me"),
		OpenAIAPIKey:     getEnv("OPENAI_API_KEY", ""),
		EmbeddingModel:   getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
		AnthropicAPIKey:  getEnv("ANTHROPIC_API_KEY", ""),
		EnrichmentModel:  getEnv("ENRICHMENT_MODEL", "claude-haiku-4-5-20251001"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		WidgetBaseURL:    getEnv("WIDGET_BASE_URL", ""),
		ChatAPIURL:       getEnv("CHAT_API_URL", ""),
		EncryptionKey:    getEnv("ADMIN_ENCRYPTION_KEY", ""),
		ShopifyAPIKey:     getEnv("SHOPIFY_API_KEY", ""),
		ShopifyAPISecret:  getEnv("SHOPIFY_API_SECRET", ""),
		ShopifyAPIVersion: getEnv("SHOPIFY_API_VERSION", ""),
		ShopifyScopes:     getEnv("SHOPIFY_SCOPES", ""),
		PublicBaseURL:     getEnv("PUBLIC_BASE_URL", ""),

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnv("SMTP_PORT", "587"),
		SMTPUser:     getEnv("SMTP_USER", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", ""),
		SMTPFromName: getEnv("SMTP_FROM_NAME", "Keepstar One"),

		ResendAPIKey: getEnv("RESEND_API_KEY", ""),

		GoogleOAuthClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleOAuthRedirectURL:  getEnv("GOOGLE_OAUTH_REDIRECT_URL", ""),

		TelegramBotToken:         getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername:      getEnv("TELEGRAM_BOT_USERNAME", ""),
		TelegramOAuthRedirectURL: getEnv("TELEGRAM_OAUTH_REDIRECT_URL", ""),

		AuthTOTPIssuer:   getEnv("AUTH_TOTP_ISSUER", "Keepstar One"),
		AuthAccessTTL:    getDurationEnv("AUTH_ACCESS_TTL", 15*time.Minute),
		AuthRefreshTTL:   getDurationEnv("AUTH_REFRESH_TTL", 30*24*time.Hour),
		AuthInviteTTL:    getDurationEnv("AUTH_INVITE_TTL", 7*24*time.Hour),
		AuthEmailCodeTTL: getDurationEnv("AUTH_EMAIL_CODE_TTL", 15*time.Minute),
		AuthResetTTL:     getDurationEnv("AUTH_RESET_TTL", 1*time.Hour),
		AuthPre2FATTL:    getDurationEnv("AUTH_PRE_2FA_TTL", 5*time.Minute),
		AuthPublicBaseURL: getEnv("AUTH_PUBLIC_BASE_URL", getEnv("PUBLIC_BASE_URL", "")),
	}
}

func (c *Config) HasDatabase() bool   { return c.DatabaseURL != "" }
func (c *Config) HasEmbeddings() bool { return c.OpenAIAPIKey != "" }
func (c *Config) HasEnrichment() bool { return c.AnthropicAPIKey != "" }
func (c *Config) HasEncryption() bool { return c.EncryptionKey != "" }
func (c *Config) HasShopify() bool    { return c.ShopifyAPIKey != "" && c.ShopifyAPISecret != "" }

func (c *Config) HasSMTP() bool   { return c.SMTPHost != "" && c.SMTPFrom != "" }
func (c *Config) HasResend() bool { return c.ResendAPIKey != "" && c.SMTPFrom != "" }
func (c *Config) HasGoogleOAuth() bool {
	return c.GoogleOAuthClientID != "" && c.GoogleOAuthClientSecret != "" && c.GoogleOAuthRedirectURL != ""
}
func (c *Config) HasTelegramLogin() bool {
	return c.TelegramBotToken != "" && c.TelegramBotUsername != ""
}
func (c *Config) TelegramRedirectURL() string {
	if c.TelegramOAuthRedirectURL != "" {
		return c.TelegramOAuthRedirectURL
	}
	if c.AuthPublicBaseURL != "" {
		return c.AuthPublicBaseURL + "/auth/telegram/callback"
	}
	return ""
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return d
}
