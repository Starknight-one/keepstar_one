package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"keepstar/internal/adapters/anthropic"
	"keepstar/internal/config"
	"keepstar/internal/handlers"
	"keepstar/internal/logger"
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
	log := logger.New(cfg.LogLevel)

	// Initialize adapters
	llmClient := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)

	// Initialize use cases
	sendMessage := usecases.NewSendMessageUseCase(llmClient)

	// Initialize handlers
	chatHandler := handlers.NewChatHandler(sendMessage)
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
