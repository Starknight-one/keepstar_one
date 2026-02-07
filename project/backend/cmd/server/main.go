package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"keepstar/internal/adapters/anthropic"
	openaiAdapter "keepstar/internal/adapters/openai"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/config"
	"keepstar/internal/handlers"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
	"keepstar/internal/tools"
	"keepstar/internal/usecases"
)

func main() {
	// Load .env file
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Load config
	cfg := config.Load()

	// Initialize logger
	appLog := logger.New(cfg.LogLevel)

	// Initialize embedding client (OpenAI)
	var embeddingClient ports.EmbeddingPort
	if cfg.HasEmbeddings() {
		embeddingClient = openaiAdapter.NewEmbeddingClient(cfg.OpenAIAPIKey, cfg.EmbeddingModel, 384)
		appLog.Info("embedding_client_initialized", "model", cfg.EmbeddingModel, "dims", 384)
	}

	// Initialize PostgreSQL if configured
	var dbClient *postgres.Client
	if cfg.HasDatabase() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var err error
		dbClient, err = postgres.NewClient(ctx, cfg.DatabaseURL)
		if err != nil {
			appLog.Error("database_connection_failed", "error", err)
			os.Exit(1)
		}
		appLog.Info("database_connected", "status", "ok")

		// Run chat migrations
		if err := dbClient.RunMigrations(ctx); err != nil {
			appLog.Error("migrations_failed", "error", err)
			os.Exit(1)
		}
		appLog.Info("migrations_completed", "status", "ok")

		// Run catalog migrations
		if err := dbClient.RunCatalogMigrations(ctx); err != nil {
			appLog.Error("catalog_migrations_failed", "error", err)
			os.Exit(1)
		}
		appLog.Info("catalog_migrations_completed", "status", "ok")

		// Run state migrations
		if err := dbClient.RunStateMigrations(ctx); err != nil {
			appLog.Error("state_migrations_failed", "error", err)
			os.Exit(1)
		}
		appLog.Info("state_migrations_completed", "status", "ok")

		// Seed catalog data (with extended timeout)
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := postgres.SeedCatalogData(seedCtx, dbClient); err != nil {
			appLog.Error("catalog_seed_failed", "error", err)
			// Non-fatal: continue even if seed fails
		} else {
			appLog.Info("catalog_seed_completed", "status", "ok")
		}
		if err := postgres.SeedExtendedCatalog(seedCtx, dbClient); err != nil {
			appLog.Error("extended_catalog_seed_failed", "error", err)
		} else {
			appLog.Info("extended_catalog_seed_completed", "status", "ok")
		}
		seedCancel()

		// Startup embedding: embed products that don't have embeddings yet
		if embeddingClient != nil {
			go func() {
				embedCtx, embedCancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer embedCancel()
				catalogForEmbed := postgres.NewCatalogAdapter(dbClient)
				if err := runEmbedding(embedCtx, catalogForEmbed, embeddingClient, appLog); err != nil {
					appLog.Error("embedding_failed", "error", err)
				}
			}()
		}
	} else {
		appLog.Info("database_skipped", "reason", "DATABASE_URL not configured")
	}

	// Initialize adapters
	llmClient := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)

	// Initialize database adapters (nil interface if no database)
	var cacheAdapter ports.CachePort
	var eventAdapter ports.EventPort
	var catalogAdapter ports.CatalogPort
	var stateAdapter ports.StatePort
	var traceAdapter ports.TracePort
	if dbClient != nil {
		cacheAdapter = postgres.NewCacheAdapter(dbClient)
		eventAdapter = postgres.NewEventAdapter(dbClient)
		catalogAdapter = postgres.NewCatalogAdapter(dbClient)
		stateAdapter = postgres.NewStateAdapter(dbClient)
		traceAdapter = postgres.NewTraceAdapter(dbClient)

		// Run trace migrations
		traceCtx, traceCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := dbClient.RunTraceMigrations(traceCtx); err != nil {
			appLog.Error("trace_migrations_failed", "error", err)
		} else {
			appLog.Info("trace_migrations_completed", "status", "ok")
		}
		traceCancel()
	}

	// Initialize preset registry
	presetRegistry := presets.NewPresetRegistry()
	appLog.Info("preset_registry_initialized", "presets", presetRegistry.List())

	// Initialize tool registry (requires state and catalog adapters)
	var toolRegistry *tools.Registry
	if stateAdapter != nil && catalogAdapter != nil {
		toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, embeddingClient)
		toolNames := make([]string, 0)
		for _, def := range toolRegistry.GetDefinitions() {
			if !strings.HasPrefix(def.Name, "_internal_") {
				toolNames = append(toolNames, def.Name)
			}
		}
		appLog.Info("tool_registry_initialized", "tools", strings.Join(toolNames, ", "), "count", len(toolNames))
	}

	// Initialize Agent 1 use case (Two-Agent Pipeline)
	var agent1UC *usecases.Agent1ExecuteUseCase
	if toolRegistry != nil {
		agent1UC = usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, appLog)
		appLog.Info("agent1_usecase_initialized", "status", "ok")
	}
	_ = agent1UC // Available for direct Agent 1 calls

	// Initialize Agent 2 use case (Preset Selector)
	var agent2UC *usecases.Agent2ExecuteUseCase
	if stateAdapter != nil && toolRegistry != nil {
		agent2UC = usecases.NewAgent2ExecuteUseCase(llmClient, stateAdapter, toolRegistry, appLog)
		appLog.Info("agent2_usecase_initialized", "status", "ok")
	}
	_ = agent2UC // Available for direct Agent 2 calls

	// Initialize Pipeline orchestrator (Agent 1 → Agent 2 → Formation)
	var pipelineUC *usecases.PipelineExecuteUseCase
	if toolRegistry != nil && stateAdapter != nil && cacheAdapter != nil {
		pipelineUC = usecases.NewPipelineExecuteUseCase(llmClient, stateAdapter, cacheAdapter, traceAdapter, toolRegistry, appLog)
		appLog.Info("pipeline_usecase_initialized", "status", "ok")
	}
	_ = pipelineUC // Pipeline is ready to be called from handlers

	// Initialize use cases
	sendMessage := usecases.NewSendMessageUseCase(llmClient, cacheAdapter, eventAdapter)

	// Initialize handlers
	chatHandler := handlers.NewChatHandler(sendMessage)
	sessionHandler := handlers.NewSessionHandler(cacheAdapter)
	healthHandler := handlers.NewHealthHandler()

	// Create metrics store for debug page
	metricsStore := handlers.NewMetricsStore()

	// Initialize Pipeline handler (if pipeline use case is available)
	var pipelineHandler *handlers.PipelineHandler
	if pipelineUC != nil {
		pipelineHandler = handlers.NewPipelineHandler(pipelineUC, metricsStore)
		appLog.Info("pipeline_handler_initialized", "status", "ok")
	}

	// Initialize Navigation handler (expand/back)
	var navigationHandler *handlers.NavigationHandler
	if stateAdapter != nil && presetRegistry != nil {
		expandUC := usecases.NewExpandUseCase(stateAdapter, presetRegistry)
		backUC := usecases.NewBackUseCase(stateAdapter, presetRegistry)
		navigationHandler = handlers.NewNavigationHandler(expandUC, backUC)
		appLog.Info("navigation_handler_initialized", "status", "ok")
	}

	// Initialize Debug handler
	var debugHandler *handlers.DebugHandler
	if stateAdapter != nil {
		debugHandler = handlers.NewDebugHandler(stateAdapter, cacheAdapter, metricsStore)
	}

	// Setup routes
	mux := http.NewServeMux()

	// Create tenant middleware if catalog available
	var tenantMiddleware *handlers.TenantMiddleware
	if catalogAdapter != nil {
		tenantMiddleware = handlers.NewTenantMiddleware(catalogAdapter)
	}

	handlers.SetupRoutes(mux, chatHandler, sessionHandler, healthHandler, pipelineHandler, tenantMiddleware, cfg.TenantSlug)

	// Setup navigation routes (expand/back)
	if navigationHandler != nil {
		handlers.SetupNavigationRoutes(mux, navigationHandler)
		appLog.Info("navigation_routes_enabled", "status", "ok")
	}

	// Setup debug routes
	if debugHandler != nil {
		mux.HandleFunc("/debug/seed", debugHandler.HandleSeedState)
	}

	// Admin: reindex embeddings endpoint
	if embeddingClient != nil && dbClient != nil {
		mux.HandleFunc("/admin/reindex-embeddings", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}
			catalogForEmbed := postgres.NewCatalogAdapter(dbClient)
			go func() {
				embedCtx, embedCancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer embedCancel()
				if err := runEmbedding(embedCtx, catalogForEmbed, embeddingClient, appLog); err != nil {
					appLog.Error("reindex_embedding_failed", "error", err)
				}
			}()
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, "Embedding reindex started in background")
		})
		appLog.Info("admin_reindex_route_enabled", "url", "POST /admin/reindex-embeddings")
	}

	// Setup trace routes (new debug view)
	if traceAdapter != nil {
		traceHandler := handlers.NewTraceHandler(traceAdapter, cacheAdapter)
		mux.HandleFunc("/debug/traces/", traceHandler.HandleTraces)
		mux.HandleFunc("/debug/traces", traceHandler.HandleTraces)
		mux.HandleFunc("/debug/kill-session", traceHandler.HandleKillSession)
		appLog.Info("trace_routes_enabled", "url", "/debug/traces/")
	}

	// Setup catalog routes if database available
	if catalogAdapter != nil {
		listProductsUC := usecases.NewListProductsUseCase(catalogAdapter)
		getProductUC := usecases.NewGetProductUseCase(catalogAdapter)
		catalogHandler := handlers.NewCatalogHandler(listProductsUC, getProductUC)
		handlers.SetupCatalogRoutes(mux, catalogHandler, tenantMiddleware)
		appLog.Info("catalog_routes_enabled", "status", "ok")
	}

	// Apply middleware
	handler := handlers.CORSMiddleware(mux)

	// Create server
	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start retention service (background cleanup)
	var retentionCancel context.CancelFunc
	if dbClient != nil {
		retentionCtx, cancel := context.WithCancel(context.Background())
		retentionCancel = cancel
		retention := postgres.NewRetentionService(dbClient, postgres.DefaultRetentionConfig())
		go retention.Start(retentionCtx, func(msg string, args ...interface{}) {
			appLog.Info(msg, args...)
		})
		appLog.Info("retention_service_started",
			"trace_ttl", "48h",
			"dead_session_ttl", "1h",
			"conversation_limit", 20,
			"interval", "30min",
		)
	}

	// Start server in goroutine
	go func() {
		appLog.Info("server_starting", "addr", addr, "environment", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLog.Error("server_error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Stop retention service
	if retentionCancel != nil {
		retentionCancel()
	}

	appLog.Info("server_shutting_down")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		appLog.Error("server_shutdown_error", "error", err)
	}

	// Close database connection
	if dbClient != nil {
		dbClient.Close()
		appLog.Info("database_closed")
	}

	appLog.Info("server_stopped")
}

func runEmbedding(ctx context.Context, catalog *postgres.CatalogAdapter, emb ports.EmbeddingPort, log *logger.Logger) error {
	products, err := catalog.GetMasterProductsWithoutEmbedding(ctx)
	if err != nil {
		return fmt.Errorf("get products without embedding: %w", err)
	}
	if len(products) == 0 {
		log.Info("embedding_skipped", "reason", "all products have embeddings")
		return nil
	}

	log.Info("embedding_started", "count", len(products))

	texts := make([]string, len(products))
	for i, p := range products {
		text := p.Name
		if p.Description != "" {
			text += " " + p.Description
		}
		if p.Brand != "" {
			text += " " + p.Brand
		}
		texts[i] = text
	}

	batchSize := 100
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		embeddings, err := emb.Embed(ctx, texts[i:end])
		if err != nil {
			return fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}

		for j, embedding := range embeddings {
			if err := catalog.SeedEmbedding(ctx, products[i+j].ID, embedding); err != nil {
				return fmt.Errorf("save embedding for %s: %w", products[i+j].ID, err)
			}
		}

		log.Info("embedding_progress", "done", end, "total", len(products))
	}

	log.Info("embedding_completed", "count", len(products))
	return nil
}
