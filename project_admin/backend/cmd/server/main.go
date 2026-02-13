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
	openaiAdapter "keepstar-admin/internal/adapters/openai"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/config"
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

	// Initialize adapters
	authAdapter := postgres.NewAuthAdapter(dbClient)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient, log)
	importAdapter := postgres.NewImportAdapter(dbClient)

	// Initialize use cases
	authUC := usecases.NewAuthUseCase(authAdapter, catalogAdapter, cfg.JWTSecret)
	productsUC := usecases.NewProductsUseCase(catalogAdapter)
	importUC := usecases.NewImportUseCase(catalogAdapter, importAdapter, embeddingClient, log)
	settingsUC := usecases.NewSettingsUseCase(catalogAdapter)
	stockUC := usecases.NewStockUseCase(catalogAdapter)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authUC, log)
	productsHandler := handlers.NewProductsHandler(productsUC, log)
	importHandler := handlers.NewImportHandler(importUC, log)
	settingsHandler := handlers.NewSettingsHandler(settingsUC, log)
	stockHandler := handlers.NewStockHandler(stockUC, log)

	// Setup routes
	mux := http.NewServeMux()
	authMW := handlers.AuthMiddleware(cfg.JWTSecret)

	// Health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public auth routes
	mux.HandleFunc("/admin/api/auth/signup", authHandler.HandleSignup)
	mux.HandleFunc("/admin/api/auth/login", authHandler.HandleLogin)

	// Protected routes
	protected := http.NewServeMux()
	protected.HandleFunc("/admin/api/auth/me", authHandler.HandleMe)
	protected.HandleFunc("/admin/api/tenant", authHandler.HandleGetTenant)
	protected.HandleFunc("/admin/api/widget-config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"GET only"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"widgetUrl":"%s"}`, cfg.WidgetBaseURL)
	})
	protected.HandleFunc("/admin/api/products", productsHandler.HandleList)
	protected.HandleFunc("/admin/api/products/", func(w http.ResponseWriter, r *http.Request) {
		// Route to get or update based on method
		path := strings.TrimPrefix(r.URL.Path, "/admin/api/products/")
		if path == "" || path == "/" {
			productsHandler.HandleList(w, r)
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
	protected.HandleFunc("/admin/api/categories", productsHandler.HandleCategories)
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

	mux.Handle("/admin/api/auth/me", authMW(protected))
	mux.Handle("/admin/api/tenant", authMW(protected))
	mux.Handle("/admin/api/widget-config", authMW(protected))
	mux.Handle("/admin/api/products", authMW(protected))
	mux.Handle("/admin/api/products/", authMW(protected))
	mux.Handle("/admin/api/categories", authMW(protected))
	mux.Handle("/admin/api/catalog/import", authMW(protected))
	mux.Handle("/admin/api/catalog/import/", authMW(protected))
	mux.Handle("/admin/api/catalog/imports", authMW(protected))
	mux.Handle("/admin/api/settings", authMW(protected))
	mux.Handle("/admin/api/stock/bulk", authMW(protected))

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
