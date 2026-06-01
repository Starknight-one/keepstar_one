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
	"syscall"
	"time"

	anthropicAdapter "keepstar_v5/internal/adapters/anthropic"
	"keepstar_v5/internal/adapters/openai"
	"keepstar_v5/internal/adapters/postgres"
	"keepstar_v5/internal/config"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/handlers"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
	"keepstar_v5/internal/usecases"
)

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
	} {
		if err := mig.run(bootCtx); err != nil {
			log.Error("migration failed", "name", mig.name, "err", err)
			os.Exit(1)
		}
		log.Info("migration applied", "name", mig.name)
	}

	// Adapters → ports. Preset adapter uses a system fallback registry
	// so any LLM ask for a name no tenant has authored (product_detail,
	// empty_not_found, ...) resolves via the embedded JSON shipped with
	// the binary. DB rows always win.
	systemPresetRegistry := presets.NewSystemPresetRegistry()
	systemComponentRegistry := presets.NewSystemComponentRegistry()
	catalogPort := postgres.NewCatalogAdapter(pgClient)
	statePort := postgres.NewStateAdapter(pgClient, log)
	presetPort := postgres.NewPresetAdapterWithSystem(pgClient, systemPresetRegistry)
	componentPort := postgres.NewComponentAdapterWithSystem(pgClient, systemComponentRegistry)
	fdPort := postgres.NewFieldDefinitionAdapter(pgClient)
	tracePort := postgres.NewTraceAdapter(pgClient)

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

	// Tools + use cases. The registry is shared across both agents — Agent1
	// filters by name prefix ("catalog_" / "_internal_") at call time so it
	// never sees Agent2's visual_assembly, and vice versa.
	registry := tools.NewRegistry()
	registry.Register(tools.NewVisualAssemblyTool(statePort, presetPort, componentPort))
	registry.Register(tools.NewCatalogSearchTool(statePort, catalogPort, embeddingPort))
	registry.Register(tools.NewStateFilterTool(statePort))
	registry.Register(tools.NewHistoryLookupTool(statePort))

	promptCache := usecases.NewPromptCache(fdPort, "product")
	agent1Cache := usecases.NewAgent1PromptCache(catalogPort)
	agent1 := usecases.NewAgent1Execute(llm, statePort, catalogPort, registry, agent1Cache, log)
	agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)
	prefetchBuilder := usecases.NewPrefetchBuilder(presetPort, componentPort, log)
	pipeline := usecases.NewPipelineExecute(agent1, agent2, statePort, prefetchBuilder, log)

	// Handlers + routing.
	sessionH := handlers.NewSessionHandler(statePort, pgClient.Pool())
	pipelineGuard := handlers.NewPipelineGuard(cfg.PipelineRatePerMin, cfg.PipelineDailyUSDCap, log)
	log.Info("pipeline_guard_configured", "rate_per_min", cfg.PipelineRatePerMin, "daily_usd_cap", cfg.PipelineDailyUSDCap)
	pipelineH := handlers.NewPipelineHandler(pipeline, tracePort, pipelineGuard, log)
	actionH := handlers.NewActionHandler(statePort)
	navigationH := handlers.NewNavigationHandler(statePort, presetPort, componentPort, log)
	router := handlers.RegisterRoutes(log, catalogPort, pgClient.Pool(), cfg.StaticDir, cfg.TenantSlug, sessionH, pipelineH, actionH, navigationH)

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
