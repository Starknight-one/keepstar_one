package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	anthropicAdapter "keepstar-admin/internal/adapters/anthropic"
	googleAdapter "keepstar-admin/internal/adapters/google"
	openaiAdapter "keepstar-admin/internal/adapters/openai"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/adapters/shopify"
	smtpAdapter "keepstar-admin/internal/adapters/smtp"
	"keepstar-admin/internal/config"
	"keepstar-admin/internal/crypto/secretbox"
	"keepstar-admin/internal/handlers"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/usecases"
)

func main() {
	// Load .env from project root
	for _, path := range []string{"../../project/.env", ".env"} {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}

	cfg := config.Load()
	log := logger.New(cfg.LogLevel)

	if !cfg.HasDatabase() {
		log.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	// Connect to PostgreSQL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbClient, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database_connection_failed", "error", err)
		os.Exit(1)
	}
	log.Info("database_connected")

	// Run migrations
	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		log.Error("catalog_migrations_failed", "error", err)
		os.Exit(1)
	}
	log.Info("catalog_migrations_completed")

	if err := dbClient.RunAdminMigrations(ctx); err != nil {
		log.Error("admin_migrations_failed", "error", err)
		os.Exit(1)
	}
	log.Info("admin_migrations_completed")

	if err := dbClient.RunLogMigrations(ctx); err != nil {
		log.Error("log_migrations_failed", "error", err)
	} else {
		log.Info("log_migrations_completed")
	}

	// Initialize embedding client
	var embeddingClient ports.EmbeddingPort
	if cfg.HasEmbeddings() {
		embeddingClient = openaiAdapter.NewEmbeddingClient(cfg.OpenAIAPIKey, cfg.EmbeddingModel, 384)
		log.Info("embedding_client_initialized", "model", cfg.EmbeddingModel)
	}

	// Initialize enrichment client
	var enrichmentClient ports.EnrichmentPort
	if cfg.HasEnrichment() {
		enrichmentClient = anthropicAdapter.NewEnrichmentClient(cfg.AnthropicAPIKey, cfg.EnrichmentModel)
		log.Info("enrichment_client_initialized", "model", cfg.EnrichmentModel)
	}

	// Encryption for integration credentials — fail-closed: absent/bad key
	// crashes startup. Operators get the error loud, not silent corruption.
	var secretBox *secretbox.Box
	if cfg.HasEncryption() {
		box, err := secretbox.NewFromBase64(cfg.EncryptionKey)
		if err != nil {
			log.Error("encryption_key_invalid", "error", err)
			os.Exit(1)
		}
		secretBox = box
		log.Info("encryption_initialized")
	} else {
		log.Warn("ADMIN_ENCRYPTION_KEY not set — Shopify/CSV integrations will be unavailable")
	}

	// Initialize adapters
	authAdapter := postgres.NewAuthAdapter(dbClient)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient, log)
	importAdapter := postgres.NewImportAdapter(dbClient)
	traceAdapter := postgres.NewTraceAdapter(dbClient)
	canvasAdapter := postgres.NewCanvasAdapter(dbClient)
	var integrationsAdapter ports.IntegrationsPort
	if secretBox != nil {
		integrationsAdapter = postgres.NewIntegrationsAdapter(dbClient, secretBox)
	}

	// SMTP mailer — optional. Email-dependent flows (reset, verify, invites,
	// email-2FA) stay disabled until SMTP_HOST + SMTP_FROM are present.
	var mailer ports.MailerPort
	if cfg.HasSMTP() {
		m, err := smtpAdapter.New(smtpAdapter.Config{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			User: cfg.SMTPUser, Password: cfg.SMTPPassword,
			From: cfg.SMTPFrom, FromName: cfg.SMTPFromName,
		})
		if err != nil {
			log.Error("smtp_init_failed", "error", err)
		} else {
			mailer = m
			log.Info("smtp_initialized", "host", cfg.SMTPHost, "from", cfg.SMTPFrom)
		}
	} else {
		log.Warn("smtp_not_configured — password reset, email verify, invitations disabled")
	}

	// Challenges repo (email verify / reset / TOTP setup / email 2FA).
	challengesRepo := postgres.NewChallengesRepo(dbClient)

	// Sessions repo — refresh token store.
	sessionsRepo := postgres.NewSessionsRepo(dbClient)

	// OAuth login states (Google / Telegram) ride on admin.oauth_states with
	// NULL tenant_id since the user has no workspace yet at login time.
	oauthLoginStatesRepo := postgres.NewOAuthLoginStatesRepo(dbClient)

	// Many-to-many membership of users to tenants.
	userTenantsRepo := postgres.NewUserTenantsRepo(dbClient)

	// Initialize use cases
	authUC := usecases.NewAuthUseCase(authAdapter, catalogAdapter, cfg.JWTSecret)
	sessionsUC := usecases.NewSessionsUseCase(
		sessionsRepo, authAdapter, cfg.JWTSecret,
		cfg.AuthAccessTTL, cfg.AuthRefreshTTL, log,
	)
	authUC.SetSessions(sessionsUC)
	authUC.SetMemberships(userTenantsRepo)

	// Google OAuth — optional, activates only when env has all three keys.
	var googleAuthUC *usecases.GoogleAuthUseCase
	if cfg.HasGoogleOAuth() {
		gclient := googleAdapter.NewClient(
			cfg.GoogleOAuthClientID,
			cfg.GoogleOAuthClientSecret,
			cfg.GoogleOAuthRedirectURL,
		)
		googleAuthUC = usecases.NewGoogleAuthUseCase(
			gclient, oauthLoginStatesRepo, authAdapter, catalogAdapter, userTenantsRepo, sessionsUC, log,
		)
		log.Info("google_oauth_enabled", "redirect", cfg.GoogleOAuthRedirectURL)
	} else {
		log.Warn("google_oauth_not_configured — sign-in with google disabled")
	}

	var telegramAuthUC *usecases.TelegramAuthUseCase
	if cfg.HasTelegramLogin() {
		telegramAuthUC = usecases.NewTelegramAuthUseCase(
			cfg.TelegramBotToken, authAdapter, catalogAdapter, userTenantsRepo, sessionsUC, log,
		)
		log.Info("telegram_login_enabled", "bot", cfg.TelegramBotUsername)
	} else {
		log.Warn("telegram_login_not_configured — telegram widget disabled")
	}

	var passwordResetUC *usecases.PasswordResetUseCase
	var emailVerifyUC *usecases.EmailVerifyUseCase
	if mailer != nil {
		passwordResetUC = usecases.NewPasswordResetUseCase(
			authAdapter, challengesRepo, mailer,
			cfg.AuthPublicBaseURL, cfg.AuthResetTTL, log,
		)
		emailVerifyUC = usecases.NewEmailVerifyUseCase(
			authAdapter, challengesRepo, mailer,
			cfg.AuthPublicBaseURL, 24*time.Hour, log,
		)
	}

	// 2FA — requires secretBox for TOTP secret encryption. Email-2FA path
	// additionally requires SMTP (reported via mailer being non-nil).
	var twoFactorUC *usecases.TwoFactorUseCase
	if secretBox != nil {
		twoFactorUC = usecases.NewTwoFactorUseCase(
			authAdapter, challengesRepo, mailer, sessionsUC, secretBox,
			cfg.AuthTOTPIssuer, cfg.AuthEmailCodeTTL, log,
		)
		authUC.SetTwoFactor(twoFactorUC, cfg.AuthPre2FATTL)
		log.Info("two_factor_enabled")
	} else {
		log.Warn("two_factor_disabled — ADMIN_ENCRYPTION_KEY not set")
	}
	tenantsUC := usecases.NewTenantsUseCase(userTenantsRepo, sessionsUC, authAdapter, log)

	// Invitations — requires mailer to actually deliver the link. We still
	// construct the UC without a mailer so the API shape stays consistent;
	// the usecase just skips the Send() call and the invite row sits with no
	// way to reach the invitee. In practice we gate the handler on mailer
	// presence below.
	invitationsRepo := postgres.NewInvitationsRepo(dbClient)
	invitationsUC := usecases.NewInvitationsUseCase(
		invitationsRepo, authAdapter, userTenantsRepo, catalogAdapter,
		sessionsUC, mailer, cfg.AuthPublicBaseURL, cfg.AuthInviteTTL, log,
	)

	productsUC := usecases.NewProductsUseCase(catalogAdapter)
	canvasUC := usecases.NewCanvasUseCase(canvasAdapter)
	billingAdapter := postgres.NewBillingAdapter(dbClient)
	billingUC := usecases.NewBillingUseCase(billingAdapter)

	var enrichUC *usecases.EnrichmentUseCase
	if enrichmentClient != nil {
		enrichUC = usecases.NewEnrichmentUseCase(enrichmentClient, catalogAdapter, log)
	}

	importUC := usecases.NewImportUseCase(catalogAdapter, importAdapter, embeddingClient, log)
	if enrichUC != nil {
		importUC.SetEnrichmentTrigger(enrichUC)
	}
	settingsUC := usecases.NewSettingsUseCase(catalogAdapter)
	stockUC := usecases.NewStockUseCase(catalogAdapter)

	// Onboarding: integrations + CSV + Shopify
	var integrationsUC *usecases.IntegrationsUseCase
	var csvMappingUC *usecases.CSVMappingUseCase
	var shopifyUC *usecases.ShopifyUseCase
	var shopifyV2UC *usecases.ShopifyV2UseCase
	var discoveryUC *usecases.DiscoveryUseCase
	if integrationsAdapter != nil {
		integrationsUC = usecases.NewIntegrationsUseCase(integrationsAdapter, log)
		if cfg.HasEnrichment() {
			csvMappingUC = usecases.NewCSVMappingUseCase(cfg.AnthropicAPIKey, cfg.EnrichmentModel, log)
		}
		if cfg.HasShopify() {
			shopifyClient := shopify.NewClient(cfg.ShopifyAPIKey, cfg.ShopifyAPISecret, cfg.ShopifyAPIVersion, cfg.ShopifyScopes)
			shopifyUC = usecases.NewShopifyUseCase(shopifyClient, integrationsAdapter, importUC, catalogAdapter, cfg.PublicBaseURL, log)

			// M4a: V2 pipeline (metadata-first import). Shares the Shopify
			// client with the legacy UC; gets a dedicated staging adapter.
			// Cuts over fully in 4d when the legacy UC is removed.
			shopifyStagingAdapter := postgres.NewShopifyStagingAdapter(dbClient, log)
			shopifyV2UC = usecases.NewShopifyV2UseCase(shopifyClient, integrationsAdapter, shopifyStagingAdapter, log)

			// M4c: discovery agent (Sonnet 4.6, 8 tools). Reuses staging +
			// the M2 master_variants adapter for find_similar_masters /
			// peek_master tools, plus the existing OpenAI embedder.
			//
			// Resolver=nil → units.Parse falls back to the in-code English
			// alias table, which covers all M3-seeded global aliases. Tenant-
			// specific overrides aren't needed yet; a per-tenant
			// PostgresAliasResolver can be wired later when AliasQuery has
			// a real adapter implementation.
			if cfg.AnthropicAPIKey != "" {
				agentClient := anthropicAdapter.NewAgentClient(cfg.AnthropicAPIKey, "claude-sonnet-4-6")
				masterVariantsAdapter := postgres.NewMasterVariantsAdapter(dbClient, log)
				mappingArtifactAdapter := postgres.NewMappingArtifactAdapter(dbClient, log)
				agent := usecases.NewDiscoveryAgent(agentClient, shopifyStagingAdapter, masterVariantsAdapter, embeddingClient, log)
				discoveryUC = usecases.NewDiscoveryUseCase(shopifyStagingAdapter, masterVariantsAdapter, mappingArtifactAdapter, nil, agent, log)
				log.Info("shopify_v2_discovery_enabled", "model", agentClient.Model())
			} else {
				log.Warn("shopify_v2_discovery_disabled — ANTHROPIC_API_KEY not set")
			}

			log.Info("shopify_integration_enabled")
		}
	}

	// Initialize handlers
	authFlags := handlers.AuthFeatureFlags{
		Google: cfg.HasGoogleOAuth(),
		Email:  cfg.HasSMTP(),
	}
	authFlags.Telegram.Enabled = cfg.HasTelegramLogin()
	authFlags.Telegram.BotUsername = cfg.TelegramBotUsername
	authHandler := handlers.NewAuthHandler(authUC, log, authFlags)

	passwordResetHandler := handlers.NewPasswordResetHandler(passwordResetUC, emailVerifyUC, log)
	sessionsHandler := handlers.NewSessionsHandler(sessionsUC, log)
	oauthHandler := handlers.NewOAuthHandler(googleAuthUC, telegramAuthUC, log)
	twoFactorHandler := handlers.NewTwoFactorHandler(twoFactorUC, authUC, log)
	tenantsHandler := handlers.NewTenantsHandler(tenantsUC, log)
	invitationsHandler := handlers.NewInvitationsHandler(invitationsUC, cfg.JWTSecret, log)
	auditAdapter := postgres.NewAuditAdapter(dbClient, log)
	auditHandler := handlers.NewAuditHandler(auditAdapter, log)
	productsHandler := handlers.NewProductsHandler(productsUC, auditAdapter, log)
	categoriesV2Adapter := postgres.NewCategoriesV2Adapter(dbClient, log)
	categoriesHandler := handlers.NewCategoriesHandler(categoriesV2Adapter, auditAdapter, log)
	candidatesAdapter := postgres.NewCandidatesAdapter(dbClient, log)
	junkHandler := handlers.NewJunkHandler(candidatesAdapter, auditAdapter, log)
	apiKeysAdapter := postgres.NewAPIKeysAdapter(dbClient, log)
	apiKeysHandler := handlers.NewAPIKeysHandler(apiKeysAdapter, auditAdapter, log)
	apiV1ProductsHandler := handlers.NewAPIv1ProductsHandler(productsUC, log)
	apiV1CategoriesHandler := handlers.NewAPIv1CategoriesHandler(categoriesV2Adapter, log)
	importHandler := handlers.NewImportHandler(importUC, log)
	settingsHandler := handlers.NewSettingsHandler(settingsUC, log)
	stockHandler := handlers.NewStockHandler(stockUC, log)
	tracesHandler := handlers.NewTracesHandler(traceAdapter, log)
	canvasHandler := handlers.NewCanvasHandler(canvasUC, traceAdapter, log)
	billingHandler := handlers.NewBillingHandler(billingUC, log)

	var enrichmentHandler *handlers.EnrichmentHandler
	if enrichUC != nil {
		enrichmentHandler = handlers.NewEnrichmentHandler(enrichUC, log)
	}

	var integrationsHandler *handlers.IntegrationsHandler
	var csvIntegrationsHandler *handlers.CSVIntegrationsHandler
	var shopifyHandler *handlers.ShopifyHandler
	var shopifyV2Handler *handlers.ShopifyV2Handler
	if integrationsUC != nil {
		integrationsHandler = handlers.NewIntegrationsHandler(integrationsUC, log)
		csvIntegrationsHandler = handlers.NewCSVIntegrationsHandler(csvMappingUC, importUC, integrationsUC, log)
	}
	if shopifyUC != nil {
		shopifyHandler = handlers.NewShopifyHandler(shopifyUC, log)
	}
	if shopifyV2UC != nil {
		shopifyV2Handler = handlers.NewShopifyV2Handler(shopifyV2UC, discoveryUC, log)
	}

	// Setup routes
	mux := http.NewServeMux()
	authMW := handlers.AuthMiddleware(cfg.JWTSecret)

	// Health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public auth routes
	mux.HandleFunc("/admin/api/auth/config", authHandler.HandleConfig)
	mux.HandleFunc("/admin/api/auth/signup", authHandler.HandleSignup)
	mux.HandleFunc("/admin/api/auth/login", authHandler.HandleLogin)
	mux.HandleFunc("/admin/api/auth/password/forgot", passwordResetHandler.HandleForgot)
	mux.HandleFunc("/admin/api/auth/password/reset", passwordResetHandler.HandleReset)
	mux.HandleFunc("/admin/api/auth/email/verify", passwordResetHandler.HandleVerifyEmail)
	mux.HandleFunc("/admin/api/auth/email/resend", passwordResetHandler.HandleResendVerify)
	mux.HandleFunc("/admin/api/auth/sessions/refresh", sessionsHandler.HandleRefresh)
	mux.HandleFunc("/admin/api/auth/logout", sessionsHandler.HandleLogout)
	mux.HandleFunc("/admin/api/auth/google/start", oauthHandler.HandleGoogleStart)
	mux.HandleFunc("/admin/api/auth/google/callback", oauthHandler.HandleGoogleCallback)
	mux.HandleFunc("/admin/api/auth/telegram/callback", oauthHandler.HandleTelegramCallback)

	// Invitation preview + accept are public (token in URL is the auth). The
	// create endpoint is protected below.
	mux.HandleFunc("/admin/api/auth/invitations/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/accept") {
			invitationsHandler.HandleAccept(w, r)
			return
		}
		invitationsHandler.HandlePreview(w, r)
	})

	// 2FA pre-session routes — gated by pre-2FA-scoped JWT from /login.
	pre2faMW := handlers.Pre2FAMiddleware(cfg.JWTSecret)
	pre2faMux := http.NewServeMux()
	pre2faMux.HandleFunc("/admin/api/auth/2fa/verify/totp", twoFactorHandler.HandleVerifyTOTP)
	pre2faMux.HandleFunc("/admin/api/auth/2fa/send/email", twoFactorHandler.HandleSendEmailCode)
	pre2faMux.HandleFunc("/admin/api/auth/2fa/verify/email", twoFactorHandler.HandleVerifyEmailCode)
	mux.Handle("/admin/api/auth/2fa/verify/totp", pre2faMW(pre2faMux))
	mux.Handle("/admin/api/auth/2fa/send/email", pre2faMW(pre2faMux))
	mux.Handle("/admin/api/auth/2fa/verify/email", pre2faMW(pre2faMux))

	// Protected routes
	protected := http.NewServeMux()
	protected.HandleFunc("/admin/api/auth/me", authHandler.HandleMe)
	protected.HandleFunc("/admin/api/auth/sessions", sessionsHandler.HandleList)
	protected.HandleFunc("/admin/api/auth/sessions/", sessionsHandler.HandleDelete)
	protected.HandleFunc("/admin/api/auth/tenants", tenantsHandler.HandleList)
	protected.HandleFunc("/admin/api/auth/tenants/select", tenantsHandler.HandleSelect)
	protected.HandleFunc("/admin/api/auth/invitations", invitationsHandler.HandleCreate)
	protected.HandleFunc("/admin/api/auth/2fa/setup/totp", twoFactorHandler.HandleSetupTOTP)
	protected.HandleFunc("/admin/api/auth/2fa/confirm/totp", twoFactorHandler.HandleConfirmTOTP)
	protected.HandleFunc("/admin/api/auth/2fa/disable/totp", twoFactorHandler.HandleDisableTOTP)
	protected.HandleFunc("/admin/api/tenant", authHandler.HandleGetTenant)
	protected.HandleFunc("/admin/api/widget-config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"GET only"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"widgetUrl":"%s","chatApiUrl":"%s"}`, cfg.WidgetBaseURL, cfg.ChatAPIURL)
	})
	protected.HandleFunc("/admin/api/products", productsHandler.HandleList)
	protected.HandleFunc("/admin/api/products/", func(w http.ResponseWriter, r *http.Request) {
		// Route to get or update based on method
		path := strings.TrimPrefix(r.URL.Path, "/admin/api/products/")
		if path == "" || path == "/" {
			productsHandler.HandleList(w, r)
			return
		}
		// Sub-path: /admin/api/products/{id}/categories → categoriesHandler.
		if strings.HasSuffix(strings.TrimSuffix(path, "/"), "/categories") {
			categoriesHandler.HandleListingCategories(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			productsHandler.HandleGet(w, r)
		case http.MethodPut:
			productsHandler.HandleUpdate(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Legacy categories endpoint (read-only sidebar tree from catalog.categories).
	protected.HandleFunc("/admin/api/categories", productsHandler.HandleCategories)
	// V2 categories — tenant CRUD + master read-only + M:N mapping (M8).
	protected.HandleFunc("/admin/api/categories/tenant", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			categoriesHandler.HandleListTenant(w, r)
		case http.MethodPost:
			categoriesHandler.HandleCreateTenant(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	protected.HandleFunc("/admin/api/categories/tenant/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			categoriesHandler.HandleUpdateTenant(w, r)
		case http.MethodDelete:
			categoriesHandler.HandleDeleteTenant(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	protected.HandleFunc("/admin/api/categories/master", categoriesHandler.HandleListMaster)
	protected.HandleFunc("/admin/api/categories/mapping", categoriesHandler.HandleSetMapping)
	// Junk variant triage (M9). Empty until harvester (M4d) populates the
	// queue; UI renders an empty state in the meantime.
	protected.HandleFunc("/admin/api/junk", junkHandler.HandleList)
	protected.HandleFunc("/admin/api/junk/count", junkHandler.HandleCount)
	protected.HandleFunc("/admin/api/junk/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/classify") {
			junkHandler.HandleClassify(w, r)
			return
		}
		http.NotFound(w, r)
	})
	// API keys (M10) — admin-protected CRUD.
	protected.HandleFunc("/admin/api/api-keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiKeysHandler.HandleList(w, r)
		case http.MethodPost:
			apiKeysHandler.HandleCreate(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	protected.HandleFunc("/admin/api/api-keys/", apiKeysHandler.HandleRevoke)
	// Audit log read (M12)
	protected.HandleFunc("/admin/api/audit", auditHandler.HandleList)
	protected.HandleFunc("/admin/api/catalog/import", importHandler.HandleUpload)
	protected.HandleFunc("/admin/api/catalog/import/", importHandler.HandleGetJob)
	protected.HandleFunc("/admin/api/catalog/imports", importHandler.HandleListJobs)
	protected.HandleFunc("/admin/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.HandleGet(w, r)
		case http.MethodPut:
			settingsHandler.HandleUpdate(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	protected.HandleFunc("/admin/api/stock/bulk", stockHandler.HandleBulkUpdate)
	protected.HandleFunc("/admin/api/traces", tracesHandler.HandleList)
	protected.HandleFunc("/admin/api/traces/", tracesHandler.HandleGet)
	protected.HandleFunc("/admin/api/sessions", tracesHandler.HandleSessions)
	protected.HandleFunc("/admin/api/sessions/kill", tracesHandler.HandleKillSession)
	protected.HandleFunc("/admin/api/sessions/kill-all", tracesHandler.HandleKillAllSessions)
	protected.HandleFunc("/admin/api/sessions/", tracesHandler.HandleSessionDetail)
	protected.HandleFunc("/admin/api/conversations", tracesHandler.HandleConversations)

	protected.HandleFunc("/admin/api/billing/overview", billingHandler.HandleOverview)
	protected.HandleFunc("/admin/api/billing/invoices", billingHandler.HandleInvoices)
	protected.HandleFunc("/admin/api/billing/plan", billingHandler.HandleUpdatePlan)
	protected.HandleFunc("/admin/api/billing/preferences", billingHandler.HandleUpdatePreferences)

	// KeepstarCanvas editor
	protected.HandleFunc("/admin/api/canvas/presets", canvasHandler.HandlePresets)
	protected.HandleFunc("/admin/api/canvas/presets/", canvasHandler.HandlePresetByID)
	protected.HandleFunc("/admin/api/canvas/components", canvasHandler.HandleComponents)
	protected.HandleFunc("/admin/api/canvas/components/", canvasHandler.HandleComponentByID)
	protected.HandleFunc("/admin/api/canvas/capture", canvasHandler.HandleCapture)
	protected.HandleFunc("/admin/api/canvas/tokens", canvasHandler.HandleTokens)
	protected.HandleFunc("/admin/api/canvas/tokens/", canvasHandler.HandleTokenByID)

	if enrichmentHandler != nil {
		protected.HandleFunc("/admin/api/catalog/enrich", enrichmentHandler.HandleEnrich)
		protected.HandleFunc("/admin/api/catalog/enrich-v2", enrichmentHandler.HandleEnrichV2)
	}

	if integrationsHandler != nil {
		protected.HandleFunc("/admin/api/integrations", integrationsHandler.HandleList)
		// Shopify-specific first — more specific path wins in net/http mux.
		if shopifyHandler != nil {
			protected.HandleFunc("/admin/api/integrations/shopify/install", shopifyHandler.HandleInstall)
			protected.HandleFunc("/admin/api/integrations/shopify/", func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/resync"):
					shopifyHandler.HandleResync(w, r)
				case shopifyV2Handler != nil && strings.HasSuffix(r.URL.Path, "/dump-to-staging"):
					shopifyV2Handler.HandleDumpToStaging(w, r)
				case shopifyV2Handler != nil && strings.HasSuffix(r.URL.Path, "/discover"):
					shopifyV2Handler.HandleDiscover(w, r)
				default:
					http.NotFound(w, r)
				}
			})
		}
		if csvIntegrationsHandler != nil {
			protected.HandleFunc("/admin/api/integrations/csv/suggest-mapping", csvIntegrationsHandler.HandleSuggestMapping)
			protected.HandleFunc("/admin/api/integrations/csv/import", csvIntegrationsHandler.HandleImport)
		}
		protected.HandleFunc("/admin/api/integrations/", integrationsHandler.HandleByID)
	}

	mux.Handle("/admin/api/auth/me", authMW(protected))
	mux.Handle("/admin/api/auth/sessions", authMW(protected))
	mux.Handle("/admin/api/auth/sessions/", authMW(protected))
	mux.Handle("/admin/api/auth/tenants", authMW(protected))
	mux.Handle("/admin/api/auth/tenants/select", authMW(protected))
	mux.Handle("/admin/api/auth/invitations", authMW(protected))
	mux.Handle("/admin/api/auth/2fa/setup/totp", authMW(protected))
	mux.Handle("/admin/api/auth/2fa/confirm/totp", authMW(protected))
	mux.Handle("/admin/api/auth/2fa/disable/totp", authMW(protected))
	mux.Handle("/admin/api/tenant", authMW(protected))
	mux.Handle("/admin/api/widget-config", authMW(protected))
	mux.Handle("/admin/api/products", authMW(protected))
	mux.Handle("/admin/api/products/", authMW(protected))
	mux.Handle("/admin/api/categories", authMW(protected))
	mux.Handle("/admin/api/categories/tenant", authMW(protected))
	mux.Handle("/admin/api/categories/tenant/", authMW(protected))
	mux.Handle("/admin/api/categories/master", authMW(protected))
	mux.Handle("/admin/api/categories/mapping", authMW(protected))
	mux.Handle("/admin/api/junk", authMW(protected))
	mux.Handle("/admin/api/junk/count", authMW(protected))
	mux.Handle("/admin/api/junk/", authMW(protected))
	mux.Handle("/admin/api/api-keys", authMW(protected))
	mux.Handle("/admin/api/api-keys/", authMW(protected))
	mux.Handle("/admin/api/audit", authMW(protected))
	mux.Handle("/admin/api/catalog/import", authMW(protected))
	mux.Handle("/admin/api/catalog/import/", authMW(protected))
	mux.Handle("/admin/api/catalog/imports", authMW(protected))
	mux.Handle("/admin/api/settings", authMW(protected))
	mux.Handle("/admin/api/stock/bulk", authMW(protected))
	mux.Handle("/admin/api/traces", authMW(protected))
	mux.Handle("/admin/api/traces/", authMW(protected))
	mux.Handle("/admin/api/sessions", authMW(protected))
	mux.Handle("/admin/api/sessions/kill-all", authMW(protected))
	mux.Handle("/admin/api/sessions/", authMW(protected))
	mux.Handle("/admin/api/conversations", authMW(protected))
	mux.Handle("/admin/api/billing/overview", authMW(protected))
	mux.Handle("/admin/api/billing/invoices", authMW(protected))
	mux.Handle("/admin/api/billing/plan", authMW(protected))
	mux.Handle("/admin/api/billing/preferences", authMW(protected))
	mux.Handle("/admin/api/canvas/presets", authMW(protected))
	mux.Handle("/admin/api/canvas/presets/", authMW(protected))
	mux.Handle("/admin/api/canvas/components", authMW(protected))
	mux.Handle("/admin/api/canvas/components/", authMW(protected))
	mux.Handle("/admin/api/canvas/capture", authMW(protected))
	mux.Handle("/admin/api/canvas/tokens", authMW(protected))
	mux.Handle("/admin/api/canvas/tokens/", authMW(protected))
	if enrichmentHandler != nil {
		mux.Handle("/admin/api/catalog/enrich", authMW(protected))
		mux.Handle("/admin/api/catalog/enrich-v2", authMW(protected))
	}
	if integrationsHandler != nil {
		mux.Handle("/admin/api/integrations", authMW(protected))
		mux.Handle("/admin/api/integrations/", authMW(protected))
	}
	// Shopify webhook + OAuth callback ride OUTSIDE authMW — HMAC and the
	// signed state nonce are the auth layer for these routes.
	if shopifyHandler != nil {
		mux.HandleFunc("/admin/api/integrations/shopify/callback", shopifyHandler.HandleCallback)
		mux.HandleFunc("/admin/api/webhooks/shopify", shopifyHandler.HandleWebhook)
	}

	// Public REST API v1 (M10) — X-API-Key auth instead of JWT.
	// Tenant comes from the resolved key; AuthMiddleware is NOT applied here.
	apiV1MW := handlers.APIKeyMiddleware(apiKeysAdapter)
	apiV1 := http.NewServeMux()
	apiV1.HandleFunc("/api/v1/products", apiV1ProductsHandler.HandleCollection)
	apiV1.HandleFunc("/api/v1/products/", apiV1ProductsHandler.HandleResource)
	apiV1.HandleFunc("/api/v1/categories", apiV1CategoriesHandler.HandleCollection)
	apiV1.HandleFunc("/api/v1/categories/", apiV1CategoriesHandler.HandleResource)
	mux.Handle("/api/v1/products", apiV1MW(apiV1))
	mux.Handle("/api/v1/products/", apiV1MW(apiV1))
	mux.Handle("/api/v1/categories", apiV1MW(apiV1))
	mux.Handle("/api/v1/categories/", apiV1MW(apiV1))

	// SPA file server: serve React frontend from ./static
	staticDir := "./static"
	if _, err := os.Stat(staticDir); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := filepath.Join(staticDir, r.URL.Path)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
		})
		log.Info("spa_file_server_enabled", "dir", staticDir)
	}

	// Apply CORS + Logging middleware (DB persist opt-in via PERSIST_LOGS=true)
	var logAdapter *postgres.LogAdapter
	if os.Getenv("PERSIST_LOGS") == "true" {
		logAdapter = postgres.NewLogAdapter(dbClient)
		log.Info("request_log_persist_enabled", "storage", "postgres")
	}
	handler := handlers.LoggingMiddleware(log, logAdapter)(handlers.CORSMiddleware(mux))

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Background sweepers for onboarding — stop fns are discarded because
	// SIGTERM takes the whole process down anyway.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	if integrationsUC != nil {
		_ = integrationsUC.StartOAuthStateSweeper(bgCtx)
	}
	if shopifyUC != nil {
		_ = shopifyUC.StartPeriodicResync(bgCtx)
	}

	go func() {
		log.Info("admin_server_starting", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server_error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("admin_server_shutting_down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown_error", "error", err)
	}
	dbClient.Close()
	log.Info("admin_server_stopped")
}
