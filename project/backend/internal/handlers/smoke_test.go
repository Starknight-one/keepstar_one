package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/handlers"
	"keepstar/internal/logger"
	"keepstar/internal/presets"
	"keepstar/internal/usecases"
)

// smokeServer builds a real HTTP test server with DB-backed handlers.
func smokeServer(t *testing.T) *httptest.Server {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping smoke test")
	}

	ctx := t.Context()
	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("state migrations: %v", err)
	}

	log := logger.New("error")
	cacheAdapter := postgres.NewCacheAdapter(client)
	stateAdapter := postgres.NewStateAdapter(client, log)
	presetRegistry := presets.NewPresetRegistry()
	metricsStore := handlers.NewMetricsStore()

	sessionHandler := handlers.NewSessionHandler(cacheAdapter, stateAdapter, nil, log)
	healthHandler := handlers.NewHealthHandler()
	debugHandler := handlers.NewDebugHandler(stateAdapter, cacheAdapter, metricsStore)

	expandUC := usecases.NewExpandUseCase(stateAdapter, presetRegistry)
	backUC := usecases.NewBackUseCase(stateAdapter, presetRegistry)
	navHandler := handlers.NewNavigationHandler(expandUC, backUC, log)

	mux := http.NewServeMux()

	// Core routes
	handlers.SetupRoutes(mux, nil, sessionHandler, healthHandler, nil, nil, "")
	handlers.SetupNavigationRoutes(mux, navHandler)
	mux.HandleFunc("/debug/seed", debugHandler.HandleSeedState)

	return httptest.NewServer(handlers.CORSMiddleware(mux))
}

func TestSmoke_HealthEndpoint(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health: want 200, got %d", resp.StatusCode)
	}
}

func TestSmoke_SeedAndGetSession(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	// Seed creates a session with 4 products
	resp, err := http.Post(ts.URL+"/debug/seed", "application/json", nil)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("seed: want 200, got %d: %s", resp.StatusCode, body)
	}

	var seedResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&seedResp); err != nil {
		t.Fatalf("decode seed response: %v", err)
	}

	sessionID, ok := seedResp["sessionId"].(string)
	if !ok || sessionID == "" {
		t.Fatal("seed response missing sessionId")
	}

	// Get session state
	resp2, err := http.Get(ts.URL + "/api/v1/session/" + sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("get session: want 200, got %d: %s", resp2.StatusCode, body)
	}
}

func TestSmoke_SeedExpandBack(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	// Seed
	resp, _ := http.Post(ts.URL+"/debug/seed", "application/json", nil)
	var seedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&seedResp)
	resp.Body.Close()
	sessionID := seedResp["sessionId"].(string)

	// Expand first product
	expandBody, _ := json.Marshal(map[string]interface{}{
		"sessionId":  sessionID,
		"entityType": "product",
		"entityId":   "prod-1",
	})
	resp2, err := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader(expandBody))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expand: want 200, got %d: %s", resp2.StatusCode, body)
	}

	var expandResp map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&expandResp)

	if expandResp["viewMode"] != "detail" {
		t.Errorf("expand: want viewMode=detail, got %v", expandResp["viewMode"])
	}

	// Back
	backBody, _ := json.Marshal(map[string]interface{}{
		"sessionId": sessionID,
	})
	resp3, err := http.Post(ts.URL+"/api/v1/navigation/back", "application/json", bytes.NewReader(backBody))
	if err != nil {
		t.Fatalf("back: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp3.Body)
		t.Fatalf("back: want 200, got %d: %s", resp3.StatusCode, body)
	}

	var backResp map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&backResp)

	if backResp["viewMode"] != "grid" {
		t.Errorf("back: want viewMode=grid, got %v", backResp["viewMode"])
	}
}

func TestSmoke_ExpandInvalidSession(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"sessionId":  "nonexistent-session",
		"entityType": "product",
		"entityId":   "prod-1",
	})
	resp, err := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("expand invalid session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expand with invalid session should not return 200")
	}
}

func TestSmoke_BackEmptyStack(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	// Seed session
	resp, _ := http.Post(ts.URL+"/debug/seed", "application/json", nil)
	var seedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&seedResp)
	resp.Body.Close()
	sessionID := seedResp["sessionId"].(string)

	// Back on empty stack should succeed (no-op)
	body, _ := json.Marshal(map[string]interface{}{"sessionId": sessionID})
	resp2, err := http.Post(ts.URL+"/api/v1/navigation/back", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("back empty: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("back empty stack: want 200, got %d", resp2.StatusCode)
	}
}

func TestSmoke_SeedMethodNotAllowed(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/debug/seed")
	if err != nil {
		t.Fatalf("seed GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("seed GET: want 405, got %d", resp.StatusCode)
	}
}

func TestSmoke_ExpandMissingFields(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	// Empty body
	resp, err := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("expand missing fields: %v", err)
	}
	defer resp.Body.Close()

	// Should fail (missing sessionId)
	if resp.StatusCode == http.StatusOK {
		t.Error("expand with missing fields should not return 200")
	}
}

func TestSmoke_ExpandThenExpandAgain(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	// Seed
	resp, _ := http.Post(ts.URL+"/debug/seed", "application/json", nil)
	var seedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&seedResp)
	resp.Body.Close()
	sessionID := seedResp["sessionId"].(string)

	// Expand prod-1
	expandBody, _ := json.Marshal(map[string]interface{}{
		"sessionId": sessionID, "entityType": "product", "entityId": "prod-1",
	})
	resp2, _ := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader(expandBody))
	resp2.Body.Close()

	// Expand prod-2 (stacks on top)
	expandBody2, _ := json.Marshal(map[string]interface{}{
		"sessionId": sessionID, "entityType": "product", "entityId": "prod-2",
	})
	resp3, err := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader(expandBody2))
	if err != nil {
		t.Fatalf("expand again: %v", err)
	}
	defer resp3.Body.Close()

	var expandResp map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&expandResp)

	stackSize, _ := expandResp["stackSize"].(float64)
	if stackSize < 2 {
		t.Errorf("after 2 expands, want stackSize >= 2, got %v", expandResp["stackSize"])
	}
}

func TestSmoke_BackThenExpandAgain(t *testing.T) {
	ts := smokeServer(t)
	defer ts.Close()

	// Seed
	resp, _ := http.Post(ts.URL+"/debug/seed", "application/json", nil)
	var seedResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&seedResp)
	resp.Body.Close()
	sessionID := seedResp["sessionId"].(string)

	// Expand
	expandBody, _ := json.Marshal(map[string]interface{}{
		"sessionId": sessionID, "entityType": "product", "entityId": "prod-1",
	})
	resp2, _ := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader(expandBody))
	resp2.Body.Close()

	// Back
	backBody, _ := json.Marshal(map[string]interface{}{"sessionId": sessionID})
	resp3, _ := http.Post(ts.URL+"/api/v1/navigation/back", "application/json", bytes.NewReader(backBody))
	resp3.Body.Close()

	// Expand different product
	expandBody2, _ := json.Marshal(map[string]interface{}{
		"sessionId": sessionID, "entityType": "product", "entityId": "prod-3",
	})
	resp4, err := http.Post(ts.URL+"/api/v1/navigation/expand", "application/json", bytes.NewReader(expandBody2))
	if err != nil {
		t.Fatalf("expand after back: %v", err)
	}
	defer resp4.Body.Close()

	if resp4.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp4.Body)
		t.Fatalf("expand after back: want 200, got %d: %s", resp4.StatusCode, body)
	}
}
