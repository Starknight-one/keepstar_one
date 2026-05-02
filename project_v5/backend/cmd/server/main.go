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
	"keepstar_v5/internal/adapters/postgres"
	"keepstar_v5/internal/config"
	"keepstar_v5/internal/handlers"
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
	} {
		if err := mig.run(bootCtx); err != nil {
			log.Error("migration failed", "name", mig.name, "err", err)
			os.Exit(1)
		}
		log.Info("migration applied", "name", mig.name)
	}

	// Adapters → ports.
	catalogPort := postgres.NewCatalogAdapter(pgClient)
	statePort := postgres.NewStateAdapter(pgClient, log)
	presetPort := postgres.NewPresetAdapter(pgClient)
	componentPort := postgres.NewComponentAdapter(pgClient)
	fdPort := postgres.NewFieldDefinitionAdapter(pgClient)

	// LLM client.
	llm := anthropicAdapter.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)

	// Tools + use cases. The registry is shared across both agents — Agent1
	// filters by name prefix ("catalog_" / "_internal_") at call time so it
	// never sees Agent2's visual_assembly, and vice versa.
	registry := tools.NewRegistry()
	registry.Register(tools.NewVisualAssemblyTool(statePort, presetPort, componentPort))
	registry.Register(tools.NewCatalogSearchTool(statePort, catalogPort))
	registry.Register(tools.NewStateFilterTool(statePort))
	registry.Register(tools.NewHistoryLookupTool(statePort))

	promptCache := usecases.NewPromptCache(fdPort, "product")
	agent1Cache := usecases.NewAgent1PromptCache(catalogPort)
	agent1 := usecases.NewAgent1Execute(llm, statePort, catalogPort, registry, agent1Cache, log)
	agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)
	pipeline := usecases.NewPipelineExecute(agent1, agent2, log)

	// Handlers + routing.
	sessionH := handlers.NewSessionHandler(statePort, pgClient.Pool())
	pipelineH := handlers.NewPipelineHandler(pipeline)
	router := handlers.RegisterRoutes(log, catalogPort, cfg.TenantSlug, sessionH, pipelineH)

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
