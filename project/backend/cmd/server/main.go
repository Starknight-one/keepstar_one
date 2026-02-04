package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"keepstar/internal/adapters/anthropic"
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
		seedCancel()
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
	if dbClient != nil {
		cacheAdapter = postgres.NewCacheAdapter(dbClient)
		eventAdapter = postgres.NewEventAdapter(dbClient)
		catalogAdapter = postgres.NewCatalogAdapter(dbClient)
		stateAdapter = postgres.NewStateAdapter(dbClient)
	}

	// Initialize preset registry
	presetRegistry := presets.NewPresetRegistry()
	appLog.Info("preset_registry_initialized", "presets", presetRegistry.List())

	// Initialize tool registry (requires state and catalog adapters)
	var toolRegistry *tools.Registry
	if stateAdapter != nil && catalogAdapter != nil {
		toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry)
		appLog.Info("tool_registry_initialized", "tools", "search_products, render_product_preset, render_service_preset")
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
		pipelineUC = usecases.NewPipelineExecuteUseCase(llmClient, stateAdapter, cacheAdapter, toolRegistry, appLog)
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

	handlers.SetupRoutes(mux, chatHandler, sessionHandler, healthHandler, pipelineHandler, tenantMiddleware)

	// Setup navigation routes (expand/back)
	if navigationHandler != nil {
		handlers.SetupNavigationRoutes(mux, navigationHandler)
		appLog.Info("navigation_routes_enabled", "status", "ok")
	}

	// Setup debug routes
	if debugHandler != nil {
		mux.HandleFunc("/debug/session/", debugHandler.HandleDebugPage)
		mux.HandleFunc("/debug/api", debugHandler.HandleDebugAPI)
		mux.HandleFunc("/debug/seed", debugHandler.HandleSeedState)
		appLog.Info("debug_routes_enabled", "url", "/debug/session/", "seed", "/debug/seed")
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
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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
