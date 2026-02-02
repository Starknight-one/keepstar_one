package usecases_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"keepstar/internal/adapters/anthropic"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/tools"
	"keepstar/internal/usecases"
)

// TestAgent1Execute_Integration is an integration test that requires:
// - DATABASE_URL set (PostgreSQL)
// - ANTHROPIC_API_KEY set
// Run with: go test -v -run TestAgent1Execute_Integration ./internal/usecases/
func TestAgent1Execute_Integration(t *testing.T) {
	// Load .env from project root
	_ = godotenv.Load("../.env")

	// Check required env vars
	dbURL := os.Getenv("DATABASE_URL")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping integration test")
	}

	// Setup
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Initialize logger
	log := logger.New("debug")

	// Connect to database
	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close()

	// Run migrations
	if err := dbClient.RunStateMigrations(ctx); err != nil {
		t.Fatalf("Failed to run state migrations: %v", err)
	}
	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		t.Fatalf("Failed to run catalog migrations: %v", err)
	}

	// Seed catalog data
	if err := postgres.SeedCatalogData(ctx, dbClient); err != nil {
		t.Logf("Warning: seed data failed (may already exist): %v", err)
	}

	// Initialize adapters
	llmClient := anthropic.NewClient(apiKey, model)
	stateAdapter := postgres.NewStateAdapter(dbClient)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient)
	cacheAdapter := postgres.NewCacheAdapter(dbClient)

	// Initialize tool registry
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter)

	// Initialize Agent 1
	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	// Helper to create session before test
	createSession := func(sessionID string) error {
		session := &domain.Session{
			ID:             sessionID,
			Status:         domain.SessionStatus("active"),
			StartedAt:      time.Now(),
			LastActivityAt: time.Now(),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		return cacheAdapter.SaveSession(ctx, session)
	}

	// Test cases
	testCases := []struct {
		name  string
		query string
	}{
		{"Russian Nike query", "покажи кроссовки Nike"},
		{"English shoes query", "show me Nike shoes"},
		{"Air Max query", "Nike Air Max"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := uuid.New().String()

			t.Logf("\n=== Test: %s ===", tc.name)
			t.Logf("Session: %s", sessionID)
			t.Logf("Query: %s", tc.query)

			// Create session first (required by foreign key)
			if err := createSession(sessionID); err != nil {
				t.Fatalf("Failed to create session: %v", err)
			}

			// Execute Agent 1
			resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
				SessionID: sessionID,
				Query:     tc.query,
			})

			if err != nil {
				t.Errorf("Agent1 execution failed: %v", err)
				return
			}

			// Log results
			t.Logf("\n--- Results ---")
			t.Logf("Latency: %d ms", resp.LatencyMs)
			t.Logf("Input tokens: %d", resp.Usage.InputTokens)
			t.Logf("Output tokens: %d", resp.Usage.OutputTokens)
			t.Logf("Total tokens: %d", resp.Usage.TotalTokens)
			t.Logf("Cost: $%.6f", resp.Usage.CostUSD)
			t.Logf("Model: %s", resp.Usage.Model)

			if resp.Delta != nil {
				t.Logf("\n--- Delta ---")
				t.Logf("Step: %d", resp.Delta.Step)
				t.Logf("Tool: %s", resp.Delta.Action.Tool)
				t.Logf("Products found: %d", resp.Delta.Result.Count)
				t.Logf("Fields: %v", resp.Delta.Result.Fields)
			} else {
				t.Logf("No delta (no tool was called)")
			}

			// Verify state was updated
			state, err := stateAdapter.GetState(ctx, sessionID)
			if err != nil {
				t.Errorf("Failed to get state: %v", err)
				return
			}

			t.Logf("\n--- State ---")
			t.Logf("Products in state: %d", len(state.Current.Data.Products))
			if len(state.Current.Data.Products) > 0 {
				t.Logf("First product: %s (%.2f)", state.Current.Data.Products[0].Name, float64(state.Current.Data.Products[0].Price)/100)
			}

			// Assertions
			if resp.Delta == nil {
				t.Error("Expected delta to be created")
			}
			if resp.Usage.TotalTokens == 0 {
				t.Error("Expected non-zero token usage")
			}
		})
	}
}

// TestAgent1Execute_CostEstimate runs a single query and prints detailed cost info
func TestAgent1Execute_CostEstimate(t *testing.T) {
	// Load .env from project root
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	if dbURL == "" || apiKey == "" {
		t.Skip("DATABASE_URL or ANTHROPIC_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log := logger.New("debug")

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("DB connection failed: %v", err)
	}
	defer dbClient.Close()

	_ = dbClient.RunStateMigrations(ctx)
	_ = dbClient.RunCatalogMigrations(ctx)
	_ = postgres.SeedCatalogData(ctx, dbClient)

	llmClient := anthropic.NewClient(apiKey, model)
	stateAdapter := postgres.NewStateAdapter(dbClient)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient)
	cacheAdapter := postgres.NewCacheAdapter(dbClient)
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter)
	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	sessionID := uuid.New().String()

	// Create session first (required by foreign key)
	session := &domain.Session{
		ID:             sessionID,
		Status:         domain.SessionStatus("active"),
		StartedAt:      time.Now(),
		LastActivityAt: time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := cacheAdapter.SaveSession(ctx, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
		SessionID: sessionID,
		Query:     "покажи кроссовки Nike",
	})
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("AGENT 1 COST REPORT")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Model:         %s\n", resp.Usage.Model)
	fmt.Printf("Input tokens:  %d\n", resp.Usage.InputTokens)
	fmt.Printf("Output tokens: %d\n", resp.Usage.OutputTokens)
	fmt.Printf("Total tokens:  %d\n", resp.Usage.TotalTokens)
	fmt.Printf("Cost:          $%.6f\n", resp.Usage.CostUSD)
	fmt.Printf("Latency:       %d ms\n", resp.LatencyMs)
	fmt.Println(strings.Repeat("=", 50))

	// Cost projections
	costPer1000 := resp.Usage.CostUSD * 1000
	costPer10000 := resp.Usage.CostUSD * 10000
	fmt.Printf("\nCost projections (at this token rate):\n")
	fmt.Printf("  1,000 queries:  $%.2f\n", costPer1000)
	fmt.Printf("  10,000 queries: $%.2f\n", costPer10000)
	fmt.Printf("  100,000 queries: $%.2f\n", costPer10000*10)
}
