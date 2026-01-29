package main

import (
	"fmt"
	"net/http"

	"keepstar/internal/adapters/anthropic"
	jsonstore "keepstar/internal/adapters/json_store"
	"keepstar/internal/adapters/memory"
	"keepstar/internal/config"
	"keepstar/internal/handlers"
	"keepstar/internal/logger"
	"keepstar/internal/usecases"
)

func main() {
	// Load config
	cfg := config.Load()

	// Initialize logger
	log := logger.New(cfg.LogLevel)

	// Initialize adapters
	llmClient := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)
	productStore := jsonstore.NewProductStore("data/products.json")
	cache := memory.NewCache()

	// Initialize use cases
	analyzeQuery := usecases.NewAnalyzeQueryUseCase(llmClient)
	composeWidgets := usecases.NewComposeWidgetsUseCase(llmClient)
	executeSearch := usecases.NewExecuteSearchUseCase(productStore, cache)

	// Initialize handlers
	chatHandler := handlers.NewChatHandler(analyzeQuery, composeWidgets, executeSearch)
	healthHandler := handlers.NewHealthHandler()

	// Setup routes
	mux := http.NewServeMux()
	handlers.SetupRoutes(mux, chatHandler, healthHandler)

	// Apply middleware
	handler := handlers.CORSMiddleware(mux)

	// Start server
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Info("server_starting", "addr", addr, "environment", cfg.Environment)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Error("server_error", "error", err)
	}
}
