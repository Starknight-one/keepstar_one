package main

import (
	"context"
	"encoding/json"
	"errors"
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
	brokerAdapters "keepstar-admin/internal/adapters/broker"
	"keepstar-admin/internal/adapters/geoip"
	googleAdapter "keepstar-admin/internal/adapters/google"
	telegramAdapter "keepstar-admin/internal/adapters/telegram"
	openaiAdapter "keepstar-admin/internal/adapters/openai"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/adapters/shopify"
	resendAdapter "keepstar-admin/internal/adapters/resend"
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

	// Mailer — Resend HTTP API preferred (Railway egress kills SMTP 587),
	// SMTP fallback for self-hosted. Email-dependent flows (reset, verify,
	// invites, email-2FA, magic-link) stay disabled if neither is configured.
	var mailer ports.MailerPort
	switch {
	case cfg.HasResend():
		m, err := resendAdapter.New(resendAdapter.Config{
			APIKey:   cfg.ResendAPIKey,
			From:     cfg.SMTPFrom,
			FromName: cfg.SMTPFromName,
		})
		if err != nil {
			log.Error("resend_init_failed", "error", err)
		} else {
			mailer = m
			log.Info("resend_initialized", "from", cfg.SMTPFrom)
		}
	case cfg.HasSMTP():
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
	default:
		log.Warn("mailer_not_configured — password reset, email verify, invitations, magic-link disabled")
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

	// GeoIP — Settings → Sessions enrichment. Optional: when MAXMIND_DB_PATH
	// is unset or the .mmdb file is missing, sessionsUC.geo stays nil and
	// the geo_country/geo_city columns sit empty. GeoLite2 license is free
	// but requires a manual download from MaxMind.
	if geoDB, err := geoip.Open(os.Getenv("MAXMIND_DB_PATH")); err != nil {
		log.Warn("geoip_open_failed", "error", err)
	} else if geoDB != nil {
		sessionsUC.SetGeoIP(geoDB)
		log.Info("geoip_initialized", "path", os.Getenv("MAXMIND_DB_PATH"))
	} else {
		log.Info("geoip_not_configured", "hint", "set MAXMIND_DB_PATH for geo enrichment")
	}

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
		var tgOIDC *telegramAdapter.OIDCClient
		if redirectURL := cfg.TelegramRedirectURL(); redirectURL != "" {
			tgOIDC = telegramAdapter.NewOIDCClient(cfg.TelegramBotToken, redirectURL)
			log.Info("telegram_oidc_enabled", "redirect", redirectURL)
		}
		telegramAuthUC = usecases.NewTelegramAuthUseCase(
			cfg.TelegramBotToken, tgOIDC, oauthLoginStatesRepo,
			authAdapter, catalogAdapter, userTenantsRepo, sessionsUC, log,
		)
		log.Info("telegram_login_enabled", "bot", cfg.TelegramBotUsername)
	} else {
		log.Warn("telegram_login_not_configured — telegram login disabled")
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

	// Magic-link: signed sign-in links delivered by email. Two callers:
	//  - Shopify install flow uses Issue + ProvisionShopOwner to email the
	//    shop owner after first OAuth.
	//  - The /admin/api/auth/magic endpoint exposes Consume so a click on the
	//    link mints a session.
	// Constructed regardless of mailer presence so Consume always works for
	// links that did manage to go out (e.g. SMTP went down after Issue).
	magicLinkUC := usecases.NewMagicLinkUseCase(
		authAdapter, userTenantsRepo, challengesRepo, mailer, sessionsUC,
		cfg.AuthPublicBaseURL, 24*time.Hour, log,
	)

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

	categoriesV2Adapter := postgres.NewCategoriesV2Adapter(dbClient, log)

	// Onboarding: integrations + CSV + Shopify
	var integrationsUC *usecases.IntegrationsUseCase
	var csvMappingUC *usecases.CSVMappingUseCase
	var shopifyV2UC *usecases.ShopifyV2UseCase
	if integrationsAdapter != nil {
		integrationsUC = usecases.NewIntegrationsUseCase(integrationsAdapter, log)
		if cfg.HasEnrichment() {
			csvMappingUC = usecases.NewCSVMappingUseCase(cfg.AnthropicAPIKey, cfg.EnrichmentModel, log)
		}
		if cfg.HasShopify() {
			shopifyClient := shopify.NewClient(cfg.ShopifyAPIKey, cfg.ShopifyAPISecret, cfg.ShopifyAPIVersion, cfg.ShopifyScopes)
			shopifyV2UC = usecases.NewShopifyV2UseCase(shopifyClient, integrationsAdapter, catalogAdapter, cfg.PublicBaseURL, log)

			// Install-flow completion hook: after a fresh Shopify install OAuth,
			// fetch the shop owner email and either (a) provision an admin_user
			// + send a magic-link, or (b) if the shop has no owner email, mint
			// a pending-shop-link token and return it so the caller can redirect
			// the merchant to a sign-in page that auto-attaches the tenant after
			// auth. Closure captures shopifyClient + magicLinkUC so shopify_v2.go
			// itself stays decoupled from auth concerns.
			//
			// Runs synchronously (per shopify_v2.CompleteOAuth contract). The
			// slow path — Resend email delivery — fires in a goroutine inside
			// ProvisionShopOwner so the OAuth callback latency stays low.
			shopifyClientLocal := shopifyClient
			magicLinkUCLocal := magicLinkUC
			shopifyV2UC.SetInstallCompletionHook(func(ctx context.Context, tenantID, shopDomain, token string) string {
				info, err := shopifyClientLocal.GetShopInfo(ctx, shopDomain, token)
				if err != nil {
					log.Warn("shopify_install_provision_shop_lookup_failed",
						"tenant_id", tenantID, "shop", shopDomain, "error", err)
					// Shop info unavailable → fall back to pending-link path so
					// the merchant can still sign in and claim their tenant.
					pendingToken, perr := magicLinkUCLocal.IssuePendingShopLink(ctx, tenantID, shopDomain)
					if perr != nil {
						log.Error("shopify_install_pending_link_fallback_failed",
							"tenant_id", tenantID, "shop", shopDomain, "error", perr)
						return ""
					}
					return pendingToken
				}
				if info.Email == "" {
					// Shopify returned shop info but the owner email field is
					// empty — issue a pending-link token instead of silently
					// dropping the merchant on an empty page.
					pendingToken, perr := magicLinkUCLocal.IssuePendingShopLink(ctx, tenantID, shopDomain)
					if perr != nil {
						log.Error("shopify_install_pending_link_create_failed",
							"tenant_id", tenantID, "shop", shopDomain, "error", perr)
						return ""
					}
					log.Info("shopify_install_pending_link_path",
						"tenant_id", tenantID, "shop", shopDomain,
						"reason", "shop_info_returned_empty_email")
					return pendingToken
				}
				// Happy path: fire user-provisioning + magic-link send async so
				// the OAuth callback isn't gated on Resend.
				go magicLinkUCLocal.ProvisionShopOwner(context.Background(), tenantID, info.Email)
				return ""
			})

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
	magicLinkHandler := handlers.NewMagicLinkHandler(magicLinkUC, log)
	twoFactorHandler := handlers.NewTwoFactorHandler(twoFactorUC, authUC, log)
	tenantsHandler := handlers.NewTenantsHandler(tenantsUC, log)
	invitationsHandler := handlers.NewInvitationsHandler(invitationsUC, cfg.JWTSecret, log)
	auditAdapter := postgres.NewAuditAdapter(dbClient, log)
	auditHandler := handlers.NewAuditHandler(auditAdapter, log)
	productsHandler := handlers.NewProductsHandler(productsUC, auditAdapter, log)

	categoriesHandler := handlers.NewCategoriesHandler(categoriesV2Adapter, auditAdapter, log)
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
	if integrationsUC != nil {
		integrationsWipeUC := usecases.NewIntegrationsWipeUseCase(dbClient.Pool(), integrationsAdapter, log)
		integrationsHandler = handlers.NewIntegrationsHandler(integrationsUC, integrationsWipeUC, log)
		csvIntegrationsHandler = handlers.NewCSVIntegrationsHandler(csvMappingUC, importUC, integrationsUC, log)
	}
	if shopifyV2UC != nil {
		shopifyHandler = handlers.NewShopifyHandler(shopifyV2UC, log)
	}

	// -------------------------------------------------------------------
	// Catalog Flow Rebuild — 6-step flow wiring (inbox → discovery_v2 →
	// apply_v2 → updates cycle). See plan file
	// /Users/starknight/.claude/plans/structured-discovering-lollipop.md
	// and 2026-05-14 update log for the architectural picture.
	//
	// Wired alongside legacy harvester_lite / discovery_agent / merge_apply
	// so old routes keep working until step 10 cleanup.
	// -------------------------------------------------------------------
	var catalogV2Handler *handlers.CatalogV2Handler
	var catalogV2Orchestrator *usecases.UpdateOrchestrator
	// B2 schema-drift wiring locals — set inside the AnthropicAPIKey branch
	// when Redis is also configured; consumed by the bg worker block and
	// by the admin drift handler below.
	var driftBrokerForWorkers ports.BrokerPort
	var schemaDriftForWorkers *usecases.SchemaDriftUseCase
	var schemaDriftHandler *handlers.SchemaDriftHandler
	_ = schemaDriftHandler // wired into routes below when non-nil
	if cfg.AnthropicAPIKey != "" {
		inboxAdapter := postgres.NewInboxAdapter(dbClient, log)
		actionLogAdapter := postgres.NewTenantActionLogAdapter(dbClient, log)
		agentRunsAdapter := postgres.NewAgentRunsAdapter(dbClient, log)
		mappingArtifactV2Adapter := postgres.NewMappingArtifactV2Adapter(dbClient, log)
		catalogV2Writer := postgres.NewCatalogV2WriterAdapter(dbClient, log)
		if embeddingClient != nil {
			// Apply-time projection rebuild embeds combined search text.
			catalogV2Writer.SetEmbedder(embeddingClient)
		}
		masterDigestAdapter := postgres.NewMasterDigestAdapter(dbClient, log)
		rateLimitAdapter := postgres.NewApplyRateLimitAdapter(dbClient, log)

		v2AgentClient := anthropicAdapter.NewAgentClient(cfg.AnthropicAPIKey, "claude-sonnet-4-6")
		inboxUC := usecases.NewInboxUseCase(inboxAdapter, actionLogAdapter, log)
		discoveryV2 := usecases.NewDiscoveryV2(v2AgentClient, inboxAdapter, masterDigestAdapter, mappingArtifactV2Adapter, actionLogAdapter, agentRunsAdapter, log)
		applyV2 := usecases.NewApplyV2(inboxAdapter, mappingArtifactV2Adapter, catalogV2Writer, actionLogAdapter, discoveryV2, log)
		orchestrator := usecases.NewUpdateOrchestrator(inboxUC, applyV2, discoveryV2, actionLogAdapter, rateLimitAdapter, log)
		catalogV2Orchestrator = orchestrator

		// Schema drift detection (B2). Producer hook in applyV2 publishes a
		// classify job to Redis after each apply; the worker pool started
		// below in the bg block consumes the stream. If Redis is not
		// configured (cfg.HasRedis() == false), schemaDrift stays nil and
		// apply runs without drift detection — graceful degradation.
		driftFindingsAdapter := postgres.NewSchemaDriftFindingsAdapter(dbClient, log)
		if cfg.HasRedis() {
			brokerAdapter, brokerErr := brokerAdapters.NewRedisBrokerAdapter(cfg.RedisURL, log)
			if brokerErr != nil {
				log.Warn("redis_broker_unavailable_drift_disabled", "error", brokerErr)
			} else {
				driftClient := anthropicAdapter.NewDriftClassifierClient(cfg.AnthropicAPIKey, cfg.DriftClassifierModel)
				schemaDriftUC := usecases.NewSchemaDrift(inboxAdapter, mappingArtifactV2Adapter, driftFindingsAdapter, brokerAdapter, driftClient, actionLogAdapter, log)
				applyV2.AttachSchemaDrift(schemaDriftUC)
				// Hand the broker + usecase out to the bg startup block via
				// closure-friendly locals.
				driftBrokerForWorkers = brokerAdapter
				schemaDriftForWorkers = schemaDriftUC
			}
		}
		schemaDriftHandler = handlers.NewSchemaDriftHandler(driftFindingsAdapter, discoveryV2, log)

		// Ingesters — Shopify is wired into ShopifyV2UseCase below so that
		// real Shopify installs route through the new flow (inbox →
		// discovery_v2 → apply_v2). CSV ingester is kept as a local for
		// future wiring into handler_integrations_csv (legacy CSV upload
		// still uses csv_mapping for now).
		shopifyIngester := usecases.NewShopifyIngester(inboxUC, orchestrator, catalogV2Writer, actionLogAdapter, log)
		_ = usecases.NewCSVIngester(inboxUC, orchestrator, log)
		if shopifyV2UC != nil {
			shopifyV2UC.SetInboxIngester(shopifyIngester)
			log.Info("shopify_v2_inbox_ingester_wired")
		}

		catalogV2Handler = handlers.NewCatalogV2Handler(orchestrator, discoveryV2, log)
		log.Info("catalog_v2_flow_enabled", "model", v2AgentClient.Model())
	} else {
		log.Warn("catalog_v2_flow_disabled — ANTHROPIC_API_KEY not set")
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
	mux.HandleFunc("/admin/api/auth/telegram/start", oauthHandler.HandleTelegramStart)
	mux.HandleFunc("/admin/api/auth/telegram/oidc/callback", oauthHandler.HandleTelegramOIDCCallback)
	mux.HandleFunc("/admin/api/auth/telegram/callback", oauthHandler.HandleTelegramCallback)
	mux.HandleFunc("/admin/api/auth/magic", magicLinkHandler.HandleConsume)

	// Invitation preview + accept are public (token in URL is the auth).
	// Resend is auth-required (re-emails an existing invite). Create is
	// protected via the separate /admin/api/auth/invitations route below.
	mux.HandleFunc("/admin/api/auth/invitations/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/accept") {
			invitationsHandler.HandleAccept(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/resend") {
			authMW(http.HandlerFunc(invitationsHandler.HandleResend)).ServeHTTP(w, r)
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
	protected.HandleFunc("/admin/api/auth/shop-pending-link/consume", magicLinkHandler.HandleConsumePendingShopLink)
	protected.HandleFunc("/admin/api/auth/sessions", sessionsHandler.HandleList)
	protected.HandleFunc("/admin/api/auth/sessions/", sessionsHandler.HandleDelete)
	protected.HandleFunc("/admin/api/auth/set-password", authHandler.HandleSetPassword)
	protected.HandleFunc("/admin/api/auth/tenants", tenantsHandler.HandleList)
	protected.HandleFunc("/admin/api/auth/tenants/select", tenantsHandler.HandleSelect)
	protected.HandleFunc("/admin/api/auth/invitations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			invitationsHandler.HandleList(w, r)
			return
		}
		invitationsHandler.HandleCreate(w, r)
	})
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

	if schemaDriftHandler != nil {
		protected.HandleFunc("/admin/api/catalog/drift", schemaDriftHandler.List)
		// /admin/api/catalog/drift/{id}/apply and /dismiss share a trailing-
		// slash mux entry; the handler routes by URL path suffix internally.
		protected.HandleFunc("/admin/api/catalog/drift/", func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/apply"):
				schemaDriftHandler.Apply(w, r)
			case strings.HasSuffix(r.URL.Path, "/dismiss"):
				schemaDriftHandler.Dismiss(w, r)
			default:
				http.NotFound(w, r)
			}
		})
	}

	if integrationsHandler != nil {
		protected.HandleFunc("/admin/api/integrations", integrationsHandler.HandleList)
		// Shopify-specific first — more specific path wins in net/http mux.
		if shopifyHandler != nil {
			protected.HandleFunc("/admin/api/integrations/shopify/install", shopifyHandler.HandleInstall)
			protected.HandleFunc("/admin/api/integrations/shopify/", func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			})
		}
		if csvIntegrationsHandler != nil {
			protected.HandleFunc("/admin/api/integrations/csv/suggest-mapping", csvIntegrationsHandler.HandleSuggestMapping)
			protected.HandleFunc("/admin/api/integrations/csv/import", csvIntegrationsHandler.HandleImport)
		}
		protected.HandleFunc("/admin/api/integrations/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/wipe") {
				integrationsHandler.HandleWipe(w, r)
				return
			}
			integrationsHandler.HandleByID(w, r)
		})
	}

	mux.Handle("/admin/api/auth/me", authMW(protected))
	mux.Handle("/admin/api/auth/sessions", authMW(protected))
	mux.Handle("/admin/api/auth/sessions/", authMW(protected))
	mux.Handle("/admin/api/auth/set-password", authMW(protected))
	mux.Handle("/admin/api/auth/shop-pending-link/consume", authMW(protected))
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
	mux.Handle("/admin/api/catalog/drift", authMW(protected))
	mux.Handle("/admin/api/catalog/drift/", authMW(protected))
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
		mux.HandleFunc("/admin/api/integrations/shopify/app", shopifyHandler.HandleAppEntry)
		mux.HandleFunc("/admin/api/integrations/shopify/callback", shopifyHandler.HandleCallback)
		mux.HandleFunc("/admin/api/webhooks/shopify", shopifyHandler.HandleWebhook)
	}

	// Internal endpoints behind X-Internal-Key (curator-backend → admin).
	// Empty env var returns 503 from the middleware, so a forgotten config
	// never silently exposes the routes.
	internalKey := os.Getenv("CURATOR_INTERNAL_KEY")
	internalMW := handlers.InternalKeyMiddleware(internalKey)

	// Catalog v2 manual triggers (sync-now + force discover). Curator's
	// "Sync now" button hits these via MergeProxy-style forwarding.
	if catalogV2Handler != nil {
		v2Mux := http.NewServeMux()
		v2Mux.HandleFunc("/admin/api/catalog/v2/sync-now/", catalogV2Handler.HandleSyncNow)
		v2Mux.HandleFunc("/admin/api/catalog/v2/discover/", catalogV2Handler.HandleDiscover)
		mux.Handle("/admin/api/catalog/v2/", internalMW(v2Mux))
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
	// Daily catalog re-sync, gated by env-var so dev runs don't burn tokens
	// every 24 hours. Reads all tenant IDs from catalog.tenants and calls
	// orchestrator.ManualSync per tenant. Source-side pull (Shopify Bulk,
	// Sheets fetch, etc.) is followup — this MVP only re-applies the inbox
	// state webhooks already populated.
	if os.Getenv("KEEPSTAR_DAILY_SOURCE_PULL") == "1" && catalogV2Orchestrator != nil {
		// Per-tenant opt-in via settings.daily_sync_enabled — default false.
		// Owners toggle this from admin UI on the tenant settings page.
		// Filter is part of the SQL so cron never wakes up tenants that
		// did not explicitly opt in (Shopify-installed merchants who want
		// once-a-day re-sync on top of their webhook subscription).
		listTenants := func(ctx context.Context) ([]string, error) {
			// JSON key matches domain.TenantSettings.DailySyncEnabled
			// json tag (camelCase, like every other field on that struct).
			rows, err := dbClient.Pool().Query(ctx, `
				SELECT id::text FROM catalog.tenants
				WHERE COALESCE(settings->>'dailySyncEnabled', 'false') = 'true'
				ORDER BY created_at
			`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var ids []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return nil, err
				}
				ids = append(ids, id)
			}
			return ids, rows.Err()
		}
		dailyPull := usecases.NewCronDailySourcePull(catalogV2Orchestrator, listTenants, log)
		go func() {
			// Stagger first tick by 1 minute so server startup isn't immediately
			// followed by a heavyweight pass.
			select {
			case <-bgCtx.Done():
				return
			case <-time.After(1 * time.Minute):
			}
			if err := dailyPull.RunOnce(bgCtx); err != nil {
				log.Warn("daily_source_pull_initial_run_failed", "error", err)
			}
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-bgCtx.Done():
					return
				case <-ticker.C:
					if err := dailyPull.RunOnce(bgCtx); err != nil {
						log.Warn("daily_source_pull_tick_failed", "error", err)
					}
				}
			}
		}()
	}

	// B2 schema-drift worker pool. Each goroutine is a unique consumer in
	// the `drift_workers` group of the `drift_classify` stream. Workers
	// run until bgCtx is cancelled on SIGTERM. Crashes inside the handler
	// don't kill the worker — recover() inside dispatch logs and lets
	// XAUTOCLAIM redeliver the message after the idle timeout.
	if driftBrokerForWorkers != nil && schemaDriftForWorkers != nil {
		defer func() { _ = driftBrokerForWorkers.Close() }()
		topic := usecases.DriftTopic()
		const group = "drift_workers"
		for i := 0; i < cfg.DriftWorkers; i++ {
			consumer := fmt.Sprintf("drift-worker-%d", i)
			go func(name string) {
				err := driftBrokerForWorkers.Subscribe(bgCtx, topic, group, name,
					func(ctx context.Context, payload []byte) (handlerErr error) {
						defer func() {
							if r := recover(); r != nil {
								log.Error("drift_worker_panic", "consumer", name, "panic", fmt.Sprintf("%v", r))
								handlerErr = fmt.Errorf("worker panic: %v", r)
							}
						}()
						var job usecases.DriftJob
						if err := json.Unmarshal(payload, &job); err != nil {
							log.Warn("drift_worker_bad_payload", "consumer", name, "error", err)
							return nil // ACK — payload is structurally bad; don't retry
						}
						return schemaDriftForWorkers.Classify(ctx, job.TenantID, job.ApplyRunID)
					})
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Error("drift_worker_subscribe_failed", "consumer", name, "error", err)
				}
			}(consumer)
		}
		log.Info("drift_worker_pool_started", "workers", cfg.DriftWorkers)
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
