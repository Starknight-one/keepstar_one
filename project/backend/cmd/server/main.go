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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		dbClient, err = postgres.NewClient(ctx, cfg.DatabaseURL)
		if err != nil {
			appLog.Error("database_connection_failed", "error", err)
			os.Exit(1)
		}
		appLog.Info("database_connected", "status", "ok")

		// Run migrations
		if err := dbClient.RunMigrations(ctx); err != nil {
			appLog.Error("migrations_failed", "error", err)
			os.Exit(1)
		}
		appLog.Info("migrations_completed", "status", "ok")
	} else {
		appLog.Info("database_skipped", "reason", "DATABASE_URL not configured")
	}

	// Initialize adapters
	llmClient := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)

	// Initialize database adapters (nil interface if no database)
	var cacheAdapter ports.CachePort
	var eventAdapter ports.EventPort
	if dbClient != nil {
		cacheAdapter = postgres.NewCacheAdapter(dbClient)
		eventAdapter = postgres.NewEventAdapter(dbClient)
	}

	// Initialize use cases
	sendMessage := usecases.NewSendMessageUseCase(llmClient, cacheAdapter, eventAdapter)

	// Initialize handlers
	chatHandler := handlers.NewChatHandler(sendMessage)
	sessionHandler := handlers.NewSessionHandler(cacheAdapter)
	healthHandler := handlers.NewHealthHandler()

	// Setup routes
	mux := http.NewServeMux()
	handlers.SetupRoutes(mux, chatHandler, sessionHandler, healthHandler)

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
