// keepstar_v5 server: V5 chat-runtime backend.
//
// Wires config → DB → migrations → adapters → ports → tools → use cases
// → handlers → http.Server with graceful shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"keepstar_v5/internal/adapters"
	anthropicAdapter "keepstar_v5/internal/adapters/anthropic"
	"keepstar_v5/internal/adapters/openai"
	"keepstar_v5/internal/adapters/postgres"
	"keepstar_v5/internal/config"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/handlers"
	"keepstar_v5/internal/operations"
	"keepstar_v5/internal/operations/seed"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
	"keepstar_v5/internal/tools"
	"keepstar_v5/internal/usecases"
)

// cacheTTLFromEnv reads the §6.1 TTL safety net (CACHE_TTL_SECONDS, default
// 600s) — the same env the prompt caches read at construction.
func cacheTTLFromEnv() time.Duration {
	if s := os.Getenv("CACHE_TTL_SECONDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 600 * time.Second
}

func main() {
	cfg := config.MustLoad()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// DB.
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelBoot()
	pgClient, err := postgres.NewClient(bootCtx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connect", "err", err)
		os.Exit(1)
	}
	defer pgClient.Close()

	// Migrations — state, presets, components. Run sequentially; each is
	// idempotent (CREATE TABLE IF NOT EXISTS).
	for _, mig := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"state", pgClient.RunStateMigrations},
		{"preset", pgClient.RunPresetMigrations},
		{"component", pgClient.RunComponentMigrations},
		{"trace", pgClient.RunTraceMigrations},
		{"theme", pgClient.RunThemeMigrations},
		// Runtime v1 (RUNTIME_SPEC.md §3): operation ALTERs v5_chat_sessions
		// (mode column, R17) so it must stay ordered after state.
		{"operation", pgClient.RunOperationMigrations},
		{"entity", pgClient.RunEntityMigrations},
	} {
		if err := mig.run(bootCtx); err != nil {
			log.Error("migration failed", "name", mig.name, "err", err)
			os.Exit(1)
		}
		log.Info("migration applied", "name", mig.name)
	}

	// Catalog schema-version gate. Admin owns catalog.* migrations and stamps
	// catalog.schema_version after each successful run. Fail LOUD if the DB schema
	// is behind what this build requires (a deploy-order / drift error); fail OPEN
	// if the table is absent (bootstrap, or admin not yet deployed) so we never take
	// the shopper-facing engine down over a missing version row.
	const minCatalogSchemaVersion = 1
	if v, present, err := pgClient.CatalogSchemaVersion(bootCtx); err != nil {
		log.Warn("catalog schema_version check skipped", "err", err)
	} else if !present {
		log.Warn("catalog.schema_version absent — proceeding (admin may not have stamped it yet)")
	} else if v < minCatalogSchemaVersion {
		log.Error("catalog schema behind required version — refusing to boot", "have", v, "need", minCatalogSchemaVersion)
		os.Exit(1)
	} else {
		log.Info("catalog schema_version ok", "version", v)
	}

	// Adapters → ports. Preset adapter uses a system fallback registry
	// so any LLM ask for a name no tenant has authored (product_detail,
	// empty_not_found, ...) resolves via the embedded JSON shipped with
	// the binary. DB rows always win.
	systemPresetRegistry := presets.NewSystemPresetRegistry()
	systemComponentRegistry := presets.NewSystemComponentRegistry()
	catalogPort := postgres.NewCatalogAdapter(pgClient)
	statePort := postgres.NewStateAdapter(pgClient, log)
	// Preset reads pass through the operation binder (§4.8): a preset's
	// `operationKind` / `operationField` intent is bound to THIS tenant's
	// instance name + config at read time, so a library form submits the
	// operation the tenant actually enabled. Resolver is set after the
	// registry exists (the registry consumes this port) — see below.
	presetBinder := operations.NewPresetOperationBinder(
		postgres.NewPresetAdapterWithSystem(pgClient, systemPresetRegistry), log)
	var presetPort ports.PresetPort = presetBinder
	componentPort := postgres.NewComponentAdapterWithSystem(pgClient, systemComponentRegistry)
	fdPort := postgres.NewFieldDefinitionAdapter(pgClient)
	tracePort := postgres.NewTraceAdapter(pgClient)
	themePort := postgres.NewThemeStore(pgClient)

	// LLM client.
	llm := anthropicAdapter.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)

	// Embedding client. Optional — empty OPENAI_API_KEY → nil port →
	// catalog_search degrades to keyword-only (mirrors V4 behaviour).
	var embeddingPort ports.EmbeddingPort
	if cfg.OpenAIAPIKey != "" {
		embeddingPort = openai.NewEmbeddingClient(cfg.OpenAIAPIKey, "", 0)
		log.Info("embedding client configured", "model", "text-embedding-3-small", "dims", 384)
	} else {
		log.Warn("OPENAI_API_KEY not set — catalog_search will run keyword-only")
	}

	// Operation registry (RUNTIME_SPEC.md R8) — THE single execution choke
	// point, shared by both agents. The four legacy tools run as
	// passthrough wraps: wire behavior byte-identical to the pre-registry
	// path. Per-agent visibility comes from DefinitionsFor(tenant, form,
	// plane, role) — the name-prefix filter is gone. The postgres adapter
	// turns on tenant instances + the v5_operation_runs audit; SpecTTL is
	// the §6.1 TTL net (CACHE_TTL_SECONDS, same env the prompt caches read).
	operationStore := postgres.NewOperationAdapter(pgClient)
	registry := operations.NewRegistry(operations.RegistryConfig{
		Store:    operationStore,
		Runs:     operationStore,
		Tenants:  catalogPort,
		Embedder: embeddingPort,
		SpecTTL:  cacheTTLFromEnv(),
		Log:      log,
	})
	// Closes the preset-binder wiring cycle (binder → registry → preset
	// port). Until set, presets render with their authored operation names.
	presetBinder.SetResolver(registry)
	registry.RegisterExecutor(domain.KindVisual, operations.WrapVisualAssembly(tools.NewVisualAssemblyTool(statePort, presetPort, componentPort)))
	registry.RegisterExecutor(domain.KindQuery, operations.WrapCatalogSearch(tools.NewCatalogSearchTool(statePort, catalogPort, embeddingPort), catalogPort))
	registry.RegisterExecutor(domain.KindInternal, operations.WrapStateFilter(tools.NewStateFilterTool(statePort)))
	registry.RegisterExecutor(domain.KindInternal, operations.WrapHistoryLookup(tools.NewHistoryLookupTool(statePort)))

	// Entity plane (M2, §4.2/§4.4): the six native executors over the
	// EntityWrite path (validate → tx record+outbox → post-commit inline
	// automation dispatch, R10/R12). Registered BEFORE SeedTemplates so
	// their template rows ride the same seed pass. The registry keys
	// executors by template name — the `query` executor coexists with the
	// auto-enabled legacy `catalog_search` wrap.
	entityPort := postgres.NewEntityAdapter(pgClient)
	entityWrite := usecases.NewEntityWrite(entityPort, catalogPort, log)
	operations.RegisterEntityExecutors(registry, operations.EntityExecutorDeps{
		State:         statePort,
		Catalog:       catalogPort,
		Entities:      entityPort,
		Embedding:     embeddingPort,
		Writer:        entityWrite,
		Notifications: postgres.NewNotificationAdapter(pgClient),
	})
	// The automation runner closes the EntityWrite→registry cycle, so it is
	// set only after the registry exists. Until set, events would commit
	// unprocessed with a warn — never leave this out of boot wiring.
	entityWrite.SetAutomationRunner(usecases.NewOperationRunner(registry, log))

	// Turn protocol (§4.7, R9 as owner-overridden): compose_turn is the
	// visual-plane operation of the onboarding + CRM forms. Registered under
	// its §3.1 seed row BEFORE SeedTemplates, like every other executor.
	registry.RegisterExecutor(domain.KindVisual, operations.WrapComposeTurn(tools.NewComposeTurnTool(statePort, presetPort, componentPort)))

	// Onboarding plane (M3, §4.3): the deterministic ManifestApplier + the 11
	// meta executors. The StateAdapter satisfies OnboardingStatePort (manifest
	// zone); the OnboardingAdapter owns the ingest/surface tokens; the
	// AdminGateway speaks the R7 service-route family (unset env → every call
	// answers ErrAdminGatewayNotConfigured and the endpoints 503 honestly).
	onboardingTokens := postgres.NewOnboardingAdapter(pgClient)
	adminGateway := adapters.NewAdminGateway(cfg.AdminBaseURL, cfg.AdminServiceKey, log)
	manifestApplier := usecases.NewManifestApplier(usecases.ManifestApplierConfig{
		State:          statePort,
		Tokens:         onboardingTokens,
		Gateway:        adminGateway,
		Entities:       entityPort,
		Ops:            operationStore,
		Themes:         themePort,
		Registry:       registry,
		SurfaceBaseURL: cfg.PublicBaseURL,
		Log:            log,
	})
	operations.RegisterMetaExecutors(registry, operations.MetaExecutorDeps{
		Onboarding: statePort,
		State:      statePort,
		Store:      operationStore,
		Embedder:   embeddingPort,
		Applier:    manifestApplier,
		Log:        log,
	})

	// Boot seed (§3.1): wrap rows from the registered executors + the 18
	// spec templates (6 executors, compose_turn, 11 onboarding meta-ops),
	// idempotent upsert on name, descriptions embedded when the embedder is
	// configured (nil → embedding NULL → SearchLibrary degrades to FTS).
	// Fail loud: a seed failure is a DB problem, same class as migrations.
	if err := registry.SeedTemplates(bootCtx, seed.Templates()); err != nil {
		log.Error("operation template seed failed", "err", err)
		os.Exit(1)
	}
	log.Info("operation templates seeded", "extra_rows", len(seed.Templates()))

	promptCache := usecases.NewPromptCache(fdPort, presetPort, catalogPort, "product")
	agent1Cache := usecases.NewAgent1PromptCache(catalogPort)
	// Per-form prompts (R17 seam + R24 additions). Storefront stays the
	// unregistered fallback — its prompt bytes are untouched (duty C3).
	agent1Cache.SetFormPrompt(domain.ModeOnboarding, prompts.OnboardingAgent1SystemPrompt)
	promptCache.SetFormPrompt(domain.ModeOnboarding, prompts.OnboardingAgent2SystemPrompt())
	promptCache.SetFormPrompt(domain.ModeCRM, prompts.Agent2SystemPrompt+prompts.ComposeTurnAgent2Addition)
	agent1 := usecases.NewAgent1Execute(llm, statePort, catalogPort, registry, agent1Cache, log)
	agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)
	prefetchBuilder := usecases.NewPrefetchBuilder(presetPort, componentPort, log)
	pipeline := usecases.NewPipelineExecute(agent1, agent2, statePort, prefetchBuilder, log)

	// Handlers + routing.
	sessionH := handlers.NewSessionHandler(statePort, pgClient.Pool())
	pipelineGuard := handlers.NewPipelineGuard(cfg.PipelineRatePerMin, cfg.PipelineDailyUSDCap, log)
	log.Info("pipeline_guard_configured", "rate_per_min", cfg.PipelineRatePerMin, "daily_usd_cap", cfg.PipelineDailyUSDCap)
	pipelineH := handlers.NewPipelineHandler(pipeline, tracePort, pipelineGuard, themePort, pgClient.Pool(), log)
	actionH := handlers.NewActionHandler(statePort)
	navigationH := handlers.NewNavigationHandler(statePort, presetPort, componentPort, themePort, log)
	presetH := handlers.NewPresetHandler(catalogPort, presetPort, componentPort, themePort, log)
	themeH := handlers.NewThemeHandler(themePort, log)
	onboardH := handlers.NewOnboardHandler(statePort, catalogPort, pgClient.Pool(), log)
	cacheH := handlers.NewCacheHandler(agent1Cache, promptCache, registry, log)
	// §6.3 cheap bucket: ONE shared per-IP guard over the no-LLM-spend
	// routes. The operations handler checks it internally; routes.go fronts
	// /session/init, /actions and /navigation/* with the same instance.
	cheapGuard := handlers.NewCheapGuard(cfg.CheapRatePerMin, log)
	// M3 runtime deps for the onboarding endpoints (upload door, step
	// submit, resume manifest, success-plaque render).
	onboardH.SetOnboardingDeps(handlers.OnboardDeps{
		OnboardState: statePort,
		Tokens:       onboardingTokens,
		Gateway:      adminGateway,
		Applier:      manifestApplier,
		Presets:      presetPort,
		Components:   componentPort,
		Themes:       themePort,
		CheapGuard:   cheapGuard,
	})
	log.Info("cheap_guard_configured", "rate_per_min", cfg.CheapRatePerMin)
	operationsH := handlers.NewOperationsHandler(registry, statePort, presetPort, componentPort, themePort, pgClient.Pool(), cheapGuard, log)
	router := handlers.RegisterRoutes(log, catalogPort, pgClient.Pool(), cfg.StaticDir, cfg.TenantSlug, sessionH, pipelineH, actionH, navigationH, presetH, themeH, onboardH, cacheH, operationsH, cheapGuard)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Run server in a goroutine so the main goroutine can wait for the
	// shutdown signal in parallel.
	go func() {
		log.Info("v5 listening", "port", cfg.Port, "model", cfg.LLMModel, "default_tenant", cfg.TenantSlug)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			stop() // unblock main
		}
	}()

	<-rootCtx.Done()
	log.Info("shutdown signal received; draining")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "err", err)
	} else {
		log.Info("shutdown complete")
	}
}
