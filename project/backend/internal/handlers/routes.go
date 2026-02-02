package handlers

import "net/http"

// SetupRoutes configures all HTTP routes
func SetupRoutes(mux *http.ServeMux, chat *ChatHandler, session *SessionHandler, health *HealthHandler) {
	// Health checks
	mux.HandleFunc("/health", health.HandleHealth)
	mux.HandleFunc("/ready", health.HandleReady)

	// API v1
	mux.HandleFunc("/api/v1/chat", chat.HandleChat)
	mux.HandleFunc("/api/v1/session/", session.HandleGetSession)
}
