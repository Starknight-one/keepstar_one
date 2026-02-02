package usecases_test

import (
	"context"
	"encoding/json"
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

// TestAgent2Execute_Integration tests Agent 2 (Template Builder) in isolation
// Requires Agent 1 to run first to populate state with products
// Run with: go test -v -run TestAgent2Execute_Integration ./internal/usecases/
func TestAgent2Execute_Integration(t *testing.T) {
	// Load .env - go test runs with cwd = package directory
	// From internal/usecases/ need ../../../.env to reach project/.env
	if err := godotenv.Load("../../../.env"); err != nil {
		// Try alternative paths
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log := logger.New("debug")

	// Connect to database
	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close()

	// Run migrations
	if err := dbClient.RunMigrations(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
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

	// Initialize tool registry and Agent 1 (to populate state)
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter)
	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	// Initialize Agent 2
	agent2UC := usecases.NewAgent2ExecuteUseCase(llmClient, stateAdapter)

	// Helper to create session
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

	t.Run("Agent2 after Agent1", func(t *testing.T) {
		sessionID := uuid.New().String()

		t.Logf("\n=== Agent 2 Test ===")
		t.Logf("Session: %s", sessionID)

		// Create session first
		if err := createSession(sessionID); err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Step 1: Run Agent 1 to populate state
		t.Logf("\n--- Step 1: Agent 1 (Tool Caller) ---")
		agent1Resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
			SessionID: sessionID,
			Query:     "покажи кроссовки Nike",
		})
		if err != nil {
			t.Fatalf("Agent 1 failed: %v", err)
		}

		t.Logf("Agent 1 completed:")
		t.Logf("  Latency: %d ms", agent1Resp.LatencyMs)
		t.Logf("  Products found: %d", agent1Resp.Delta.Result.Count)
		t.Logf("  Fields: %v", agent1Resp.Delta.Result.Fields)
		t.Logf("  Tokens: %d (in: %d, out: %d)", agent1Resp.Usage.TotalTokens, agent1Resp.Usage.InputTokens, agent1Resp.Usage.OutputTokens)
		t.Logf("  Cost: $%.6f", agent1Resp.Usage.CostUSD)

		// Step 2: Run Agent 2 to generate template
		t.Logf("\n--- Step 2: Agent 2 (Template Builder) ---")
		agent2Resp, err := agent2UC.Execute(ctx, usecases.Agent2ExecuteRequest{
			SessionID: sessionID,
		})
		if err != nil {
			t.Fatalf("Agent 2 failed: %v", err)
		}

		t.Logf("Agent 2 completed:")
		t.Logf("  Latency: %d ms", agent2Resp.LatencyMs)
		t.Logf("  Tokens: %d (in: %d, out: %d)", agent2Resp.Usage.TotalTokens, agent2Resp.Usage.InputTokens, agent2Resp.Usage.OutputTokens)
		t.Logf("  Cost: $%.6f", agent2Resp.Usage.CostUSD)

		if agent2Resp.Template != nil {
			t.Logf("  Template mode: %s", agent2Resp.Template.Mode)
			if agent2Resp.Template.Grid != nil {
				t.Logf("  Grid: %dx%d", agent2Resp.Template.Grid.Rows, agent2Resp.Template.Grid.Cols)
			}
			t.Logf("  Widget size: %s", agent2Resp.Template.WidgetTemplate.Size)
			t.Logf("  Atoms count: %d", len(agent2Resp.Template.WidgetTemplate.Atoms))

			for i, atom := range agent2Resp.Template.WidgetTemplate.Atoms {
				t.Logf("    Atom %d: type=%s, field=%s", i, atom.Type, atom.Field)
			}
		} else {
			t.Error("Expected template to be returned")
		}

		// Check state has template
		state, err := stateAdapter.GetState(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get state: %v", err)
		}

		t.Logf("\n--- State after Agent 2 ---")
		t.Logf("  Products in state: %d", len(state.Current.Data.Products))
		t.Logf("  Meta count: %d", state.Current.Meta.Count)
		t.Logf("  Meta fields: %v", state.Current.Meta.Fields)

		if state.Current.Template != nil {
			templateJSON, _ := json.MarshalIndent(state.Current.Template, "  ", "  ")
			t.Logf("  Template saved: %s", string(templateJSON))
		} else {
			t.Error("Expected template to be saved in state")
		}
	})
}

// TestPipelineExecute_Integration tests the full pipeline: Agent 1 → Agent 2 → Formation
// Run with: go test -v -run TestPipelineExecute_Integration ./internal/usecases/
func TestPipelineExecute_Integration(t *testing.T) {
	// Load .env - go test runs with cwd = package directory
	// From internal/usecases/ need ../../../.env to reach project/.env
	if err := godotenv.Load("../../../.env"); err != nil {
		// Try alternative paths
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

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	log := logger.New("debug")

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close()

	// Run migrations
	_ = dbClient.RunMigrations(ctx)
	_ = dbClient.RunStateMigrations(ctx)
	_ = dbClient.RunCatalogMigrations(ctx)
	_ = postgres.SeedCatalogData(ctx, dbClient)

	// Initialize adapters
	llmClient := anthropic.NewClient(apiKey, model)
	stateAdapter := postgres.NewStateAdapter(dbClient)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient)
	cacheAdapter := postgres.NewCacheAdapter(dbClient)
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter)

	// Initialize Pipeline
	pipelineUC := usecases.NewPipelineExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	// Helper to create session
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

	testCases := []struct {
		name  string
		query string
	}{
		{"Nike shoes", "покажи кроссовки Nike"},
		{"All products", "покажи все товары"},
		{"Cheap products", "дешевые товары до 5000 рублей"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := uuid.New().String()

			t.Logf("\n%s", strings.Repeat("=", 60))
			t.Logf("PIPELINE TEST: %s", tc.name)
			t.Logf("%s", strings.Repeat("=", 60))
			t.Logf("Session: %s", sessionID)
			t.Logf("Query: %s", tc.query)

			if err := createSession(sessionID); err != nil {
				t.Fatalf("Failed to create session: %v", err)
			}

			// Execute full pipeline
			resp, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
				SessionID: sessionID,
				Query:     tc.query,
			})
			if err != nil {
				t.Fatalf("Pipeline execution failed: %v", err)
			}

			// Timing
			t.Logf("\n--- Timing ---")
			t.Logf("Agent 1:  %d ms", resp.Agent1Ms)
			t.Logf("Agent 2:  %d ms", resp.Agent2Ms)
			t.Logf("Total:    %d ms", resp.TotalMs)

			// Delta (from Agent 1)
			if resp.Delta != nil {
				t.Logf("\n--- Delta (Agent 1 result) ---")
				t.Logf("Step: %d", resp.Delta.Step)
				t.Logf("Tool: %s", resp.Delta.Action.Tool)
				t.Logf("Products found: %d", resp.Delta.Result.Count)
			}

			// Formation (final result)
			if resp.Formation != nil {
				t.Logf("\n--- Formation (Final Result) ---")
				t.Logf("Mode: %s", resp.Formation.Mode)
				if resp.Formation.Grid != nil {
					t.Logf("Grid: %dx%d", resp.Formation.Grid.Rows, resp.Formation.Grid.Cols)
				}
				t.Logf("Widgets count: %d", len(resp.Formation.Widgets))

				// Show first widget
				if len(resp.Formation.Widgets) > 0 {
					w := resp.Formation.Widgets[0]
					t.Logf("\nFirst widget:")
					t.Logf("  ID: %s", w.ID)
					t.Logf("  Type: %s", w.Type)
					t.Logf("  Size: %s", w.Size)
					t.Logf("  Atoms: %d", len(w.Atoms))
					for i, atom := range w.Atoms {
						t.Logf("    Atom %d: type=%s, value=%v", i, atom.Type, atom.Value)
					}
				}
			} else {
				t.Logf("\nNo formation (no products or template)")
			}

			// Verify state
			state, _ := stateAdapter.GetState(ctx, sessionID)
			t.Logf("\n--- Final State ---")
			t.Logf("Products: %d", len(state.Current.Data.Products))
			t.Logf("Template saved: %v", state.Current.Template != nil)
		})
	}
}

// TestPipelineExecute_CostReport prints detailed cost report for full pipeline
// Run with: go test -v -run TestPipelineExecute_CostReport ./internal/usecases/
func TestPipelineExecute_CostReport(t *testing.T) {
	// Load .env - go test runs with cwd = package directory
	// From internal/usecases/ need ../../../.env to reach project/.env
	if err := godotenv.Load("../../../.env"); err != nil {
		// Try alternative paths
		_ = godotenv.Load("../../.env")
		_ = godotenv.Load("../.env")
	}

	dbURL := os.Getenv("DATABASE_URL")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	if dbURL == "" || apiKey == "" {
		t.Skip("DATABASE_URL or ANTHROPIC_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log := logger.New("debug")

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("DB connection failed: %v", err)
	}
	defer dbClient.Close()

	_ = dbClient.RunMigrations(ctx)
	_ = dbClient.RunStateMigrations(ctx)
	_ = dbClient.RunCatalogMigrations(ctx)
	_ = postgres.SeedCatalogData(ctx, dbClient)

	llmClient := anthropic.NewClient(apiKey, model)
	stateAdapter := postgres.NewStateAdapter(dbClient)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient)
	cacheAdapter := postgres.NewCacheAdapter(dbClient)
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter)

	// Create both use cases to get individual costs
	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)
	agent2UC := usecases.NewAgent2ExecuteUseCase(llmClient, stateAdapter)
	pipelineUC := usecases.NewPipelineExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	sessionID := uuid.New().String()

	// Create session
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

	query := "покажи кроссовки Nike"

	// Run Agent 1 separately to get its usage
	agent1Resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
		SessionID: sessionID,
		Query:     query,
	})
	if err != nil {
		t.Fatalf("Agent 1 failed: %v", err)
	}

	// Run Agent 2 separately to measure (no direct cost tracking yet)
	agent2Start := time.Now()
	agent2Resp, err := agent2UC.Execute(ctx, usecases.Agent2ExecuteRequest{
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Agent 2 failed: %v", err)
	}
	agent2Duration := time.Since(agent2Start).Milliseconds()

	// For new session, run full pipeline to get timing
	sessionID2 := uuid.New().String()
	session2 := &domain.Session{
		ID:             sessionID2,
		Status:         domain.SessionStatus("active"),
		StartedAt:      time.Now(),
		LastActivityAt: time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	_ = cacheAdapter.SaveSession(ctx, session2)

	pipelineResp, _ := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
		SessionID: sessionID2,
		Query:     query,
	})

	// Print report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TWO-AGENT PIPELINE COST REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Query: %s\n", query)
	fmt.Printf("Model: %s\n", model)
	fmt.Println(strings.Repeat("-", 60))

	fmt.Println("\nAGENT 1 (Tool Caller):")
	fmt.Printf("  Input tokens:  %d\n", agent1Resp.Usage.InputTokens)
	fmt.Printf("  Output tokens: %d\n", agent1Resp.Usage.OutputTokens)
	fmt.Printf("  Total tokens:  %d\n", agent1Resp.Usage.TotalTokens)
	fmt.Printf("  Cost:          $%.6f\n", agent1Resp.Usage.CostUSD)
	fmt.Printf("  Latency:       %d ms\n", agent1Resp.LatencyMs)
	fmt.Printf("  Products:      %d\n", agent1Resp.Delta.Result.Count)

	fmt.Println("\nAGENT 2 (Template Builder):")
	fmt.Printf("  Input tokens:  %d\n", agent2Resp.Usage.InputTokens)
	fmt.Printf("  Output tokens: %d\n", agent2Resp.Usage.OutputTokens)
	fmt.Printf("  Total tokens:  %d\n", agent2Resp.Usage.TotalTokens)
	fmt.Printf("  Cost:          $%.6f\n", agent2Resp.Usage.CostUSD)
	fmt.Printf("  Latency:       %d ms\n", agent2Duration)
	if agent2Resp.Template != nil {
		fmt.Printf("  Mode:          %s\n", agent2Resp.Template.Mode)
		fmt.Printf("  Widget size:   %s\n", agent2Resp.Template.WidgetTemplate.Size)
		fmt.Printf("  Atoms:         %d\n", len(agent2Resp.Template.WidgetTemplate.Atoms))
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("\nPIPELINE TOTAL:")
	if pipelineResp != nil {
		fmt.Printf("  Agent 1 time:  %d ms\n", pipelineResp.Agent1Ms)
		fmt.Printf("  Agent 2 time:  %d ms\n", pipelineResp.Agent2Ms)
		fmt.Printf("  Total time:    %d ms\n", pipelineResp.TotalMs)
		if pipelineResp.Formation != nil {
			fmt.Printf("  Widgets:       %d\n", len(pipelineResp.Formation.Widgets))
		}
	}

	totalCost := agent1Resp.Usage.CostUSD + agent2Resp.Usage.CostUSD
	fmt.Printf("  Total cost:    $%.6f\n", totalCost)

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nCOST PROJECTIONS (per pipeline call):")
	fmt.Printf("  1,000 queries:   $%.2f\n", totalCost*1000)
	fmt.Printf("  10,000 queries:  $%.2f\n", totalCost*10000)
	fmt.Printf("  100,000 queries: $%.2f\n", totalCost*100000)
	fmt.Println(strings.Repeat("=", 60))
}

// TestApplyTemplate_Unit tests template application without LLM
func TestApplyTemplate_Unit(t *testing.T) {
	// Create test template
	template := &domain.FormationTemplate{
		Mode: domain.FormationTypeGrid,
		Grid: &domain.GridConfig{Rows: 2, Cols: 2},
		WidgetTemplate: domain.WidgetTemplate{
			Size: domain.WidgetSizeMedium,
			Atoms: []domain.AtomTemplate{
				{Type: domain.AtomTypeImage, Field: "images", Size: "medium"},
				{Type: domain.AtomTypeText, Field: "name", Style: "heading"},
				{Type: domain.AtomTypeNumber, Field: "price", Format: "currency"},
				{Type: domain.AtomTypeRating, Field: "rating"},
			},
		},
	}

	// Create test products
	products := []domain.Product{
		{
			ID:     "prod-1",
			Name:   "Nike Air Max 90",
			Price:  12990,
			Images: []string{"https://example.com/nike1.jpg"},
			Rating: 4.5,
		},
		{
			ID:     "prod-2",
			Name:   "Nike Air Force 1",
			Price:  9990,
			Images: []string{"https://example.com/nike2.jpg"},
			Rating: 4.8,
		},
	}

	// Apply template
	formation, err := usecases.ApplyTemplate(template, products)
	if err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}

	t.Logf("\n--- ApplyTemplate Result ---")
	t.Logf("Mode: %s", formation.Mode)
	t.Logf("Grid: %dx%d", formation.Grid.Rows, formation.Grid.Cols)
	t.Logf("Widgets: %d", len(formation.Widgets))

	// Verify
	if formation.Mode != domain.FormationTypeGrid {
		t.Errorf("Expected mode=grid, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 2 {
		t.Errorf("Expected 2 widgets, got %d", len(formation.Widgets))
	}

	// Check first widget
	w := formation.Widgets[0]
	t.Logf("\nWidget 0:")
	t.Logf("  ID: %s", w.ID)
	t.Logf("  Type: %s", w.Type)
	t.Logf("  Size: %s", w.Size)

	if w.Size != domain.WidgetSizeMedium {
		t.Errorf("Expected size=medium, got %s", w.Size)
	}
	if len(w.Atoms) != 4 {
		t.Errorf("Expected 4 atoms, got %d", len(w.Atoms))
	}

	// Check atoms have correct values
	for i, atom := range w.Atoms {
		t.Logf("  Atom %d: type=%s, value=%v, meta=%v", i, atom.Type, atom.Value, atom.Meta)
	}

	// Verify first product's data is in first widget
	if w.Atoms[1].Value != "Nike Air Max 90" {
		t.Errorf("Expected name='Nike Air Max 90', got %v", w.Atoms[1].Value)
	}
	if w.Atoms[2].Value != 12990 {
		t.Errorf("Expected price=12990, got %v", w.Atoms[2].Value)
	}
}
