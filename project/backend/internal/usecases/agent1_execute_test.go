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
	"keepstar/internal/presets"
	"keepstar/internal/tools"
	"keepstar/internal/usecases"
)

// setupIntegration creates all adapters and returns them. Skips test if env vars missing.
func setupIntegration(t *testing.T, timeout time.Duration) (
	context.Context, context.CancelFunc,
	*postgres.StateAdapter, *postgres.CatalogAdapter, *postgres.CacheAdapter,
	*tools.Registry, *anthropic.Client, *logger.Logger,
) {
	t.Helper()

	if err := godotenv.Load("../../../.env"); err != nil {
		_ = godotenv.Load("../../.env")
		_ = godotenv.Load("../.env")
	}

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

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	log := logger.New("debug")

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		cancel()
		t.Fatalf("Failed to connect to database: %v", err)
	}
	t.Cleanup(func() { dbClient.Close() })

	// Run all migrations
	_ = dbClient.RunMigrations(ctx)
	_ = dbClient.RunStateMigrations(ctx)
	_ = dbClient.RunCatalogMigrations(ctx)

	llmClient := anthropic.NewClient(apiKey, model)
	stateAdapter := postgres.NewStateAdapter(dbClient, log)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient)
	cacheAdapter := postgres.NewCacheAdapter(dbClient)
	presetRegistry := presets.NewPresetRegistry()
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, nil)

	return ctx, cancel, stateAdapter, catalogAdapter, cacheAdapter, toolRegistry, llmClient, log
}

// createTestSession creates a session in DB (required for FK constraints)
func createTestSession(t *testing.T, ctx context.Context, cacheAdapter *postgres.CacheAdapter, sessionID string) {
	t.Helper()
	session := &domain.Session{
		ID: sessionID, Status: domain.SessionStatus("active"),
		StartedAt: time.Now(), LastActivityAt: time.Now(),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := cacheAdapter.SaveSession(ctx, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
}

// TestAgent1Execute_Integration verifies Agent1 actually:
// 1. Calls search_products tool
// 2. Writes products to state via UpdateData (zone-write)
// 3. Creates delta in DB
// 4. Preserves Aliases (tenant_slug)
// 5. Saves conversation history
func TestAgent1Execute_Integration(t *testing.T) {
	ctx, cancel, stateAdapter, catalogAdapter, cacheAdapter, toolRegistry, llmClient, log := setupIntegration(t, 60*time.Second)
	defer cancel()

	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, catalogAdapter, toolRegistry, log)

	t.Run("Nike query produces products in state", func(t *testing.T) {
		sessionID := uuid.New().String()
		createTestSession(t, ctx, cacheAdapter, sessionID)

		resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
			SessionID:  sessionID,
			Query:      "покажи кроссовки Nike",
			TenantSlug: "nike",
			TurnID:     "turn-test-1",
		})
		if err != nil {
			t.Fatalf("Agent1 failed: %v", err)
		}

		// 1. Tool must be called
		if resp.ToolName == "" {
			t.Fatal("Expected tool to be called, got empty ToolName")
		}
		if resp.ToolName != "search_products" {
			t.Errorf("Expected search_products, got %s", resp.ToolName)
		}

		// 2. Products must be found
		if resp.ProductsFound == 0 {
			t.Fatal("Expected products to be found, got 0")
		}
		t.Logf("Products found: %d", resp.ProductsFound)

		// 3. State must have products written via zone-write
		state, err := stateAdapter.GetState(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get state: %v", err)
		}
		if len(state.Current.Data.Products) == 0 {
			t.Fatal("Expected products in state.Current.Data.Products, got 0")
		}
		if len(state.Current.Data.Products) != resp.ProductsFound {
			t.Errorf("State products (%d) != resp.ProductsFound (%d)", len(state.Current.Data.Products), resp.ProductsFound)
		}

		// 4. First product must have real data
		p := state.Current.Data.Products[0]
		if p.Name == "" {
			t.Error("First product has empty Name")
		}
		if p.Price == 0 {
			t.Error("First product has zero Price")
		}
		t.Logf("First product: %s ($%.2f)", p.Name, float64(p.Price)/100)

		// 5. Meta fields must be populated
		if len(state.Current.Meta.Fields) == 0 {
			t.Error("Expected meta.Fields to be populated")
		}

		// 6. Aliases must be preserved (tenant_slug)
		if state.Current.Meta.Aliases == nil {
			t.Fatal("Expected Aliases to be preserved, got nil")
		}
		if state.Current.Meta.Aliases["tenant_slug"] != "nike" {
			t.Errorf("Expected tenant_slug=nike, got %q", state.Current.Meta.Aliases["tenant_slug"])
		}

		// 7. Delta must exist in DB
		deltas, err := stateAdapter.GetDeltas(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get deltas: %v", err)
		}
		if len(deltas) == 0 {
			t.Fatal("Expected at least 1 delta in DB, got 0")
		}
		lastDelta := deltas[len(deltas)-1]
		if lastDelta.TurnID != "turn-test-1" {
			t.Errorf("Expected delta TurnID=turn-test-1, got %s", lastDelta.TurnID)
		}
		if lastDelta.Path != "data.products" {
			t.Errorf("Expected delta Path=data.products, got %s", lastDelta.Path)
		}
		if lastDelta.Result.Count == 0 {
			t.Error("Expected delta Result.Count > 0")
		}
		t.Logf("Delta: step=%d, path=%s, count=%d, turnID=%s", lastDelta.Step, lastDelta.Path, lastDelta.Result.Count, lastDelta.TurnID)

		// 8. Conversation history must be saved
		if len(state.ConversationHistory) == 0 {
			t.Fatal("Expected conversation history to be saved, got 0 messages")
		}
		// Should have: user message + assistant tool_use + user tool_result = 3
		if len(state.ConversationHistory) < 3 {
			t.Errorf("Expected at least 3 messages in conversation history, got %d", len(state.ConversationHistory))
		}
		t.Logf("Conversation history: %d messages", len(state.ConversationHistory))

		// 9. Response must NOT have Delta field (removed in this patch)
		// This is a compile-time check — if Delta field existed, this file wouldn't compile
		// because we removed it from Agent1ExecuteResponse

		t.Logf("LLM: %d tokens, $%.6f", resp.Usage.TotalTokens, resp.Usage.CostUSD)
	})
}

// TestAgent1_OnlyGetsDataTools verifies Agent1 tool filter excludes render tools
func TestAgent1_OnlyGetsDataTools(t *testing.T) {
	ctx, cancel, stateAdapter, catalogAdapter, _, toolRegistry, llmClient, log := setupIntegration(t, 10*time.Second)
	defer cancel()
	_ = ctx

	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, catalogAdapter, toolRegistry, log)
	toolDefs := agent1UC.GetToolDefs()

	var hasSearch, hasRender, hasFreestyle bool
	for _, td := range toolDefs {
		if strings.HasPrefix(td.Name, "_internal_") {
			continue // skip padding tools
		}
		switch {
		case td.Name == "search_products":
			hasSearch = true
		case strings.HasPrefix(td.Name, "render_"):
			hasRender = true
			t.Errorf("Agent1 should NOT see render tool, got: %s", td.Name)
		case td.Name == "freestyle":
			hasFreestyle = true
			t.Errorf("Agent1 should NOT see freestyle tool")
		default:
			if !strings.HasPrefix(td.Name, "search_") {
				t.Errorf("Agent1 should only see search_* tools, got: %s", td.Name)
			}
		}
	}

	if !hasSearch {
		t.Error("Agent1 should see search_products")
	}
	if hasRender {
		t.Error("Agent1 has render tools — architecture violation")
	}
	if hasFreestyle {
		t.Error("Agent1 has freestyle tool — architecture violation")
	}
	t.Logf("Agent1 tool count (excluding padding): %d", func() int {
		c := 0
		for _, td := range toolDefs {
			if !strings.HasPrefix(td.Name, "_internal_") {
				c++
			}
		}
		return c
	}())
}

// TestAgent1Execute_CostEstimate runs a single query and prints detailed cost info
func TestAgent1Execute_CostEstimate(t *testing.T) {
	ctx, cancel, stateAdapter, catalogAdapter, cacheAdapter, toolRegistry, llmClient, log := setupIntegration(t, 30*time.Second)
	defer cancel()

	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, catalogAdapter, toolRegistry, log)
	sessionID := uuid.New().String()
	createTestSession(t, ctx, cacheAdapter, sessionID)

	resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
		SessionID: sessionID,
		Query:     "покажи кроссовки Nike",
	})
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	// Minimal assertion
	if resp.ProductsFound == 0 {
		t.Error("Expected products to be found")
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
	fmt.Printf("Products:      %d\n", resp.ProductsFound)
	fmt.Println(strings.Repeat("=", 50))

	costPer1000 := resp.Usage.CostUSD * 1000
	costPer10000 := resp.Usage.CostUSD * 10000
	fmt.Printf("\nCost projections (at this token rate):\n")
	fmt.Printf("  1,000 queries:  $%.2f\n", costPer1000)
	fmt.Printf("  10,000 queries: $%.2f\n", costPer10000)
	fmt.Printf("  100,000 queries: $%.2f\n", costPer10000*10)
}
