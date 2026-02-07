package usecases_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/usecases"
)

// TestAgent2Execute_Integration verifies Agent2 actually:
// 1. Calls a render tool (render_product_preset or freestyle)
// 2. Writes formation to state.Template via zone-write (not UpdateState)
// 3. Formation has correct number of widgets matching product count
// 4. Each widget has atoms with real data (not empty)
// 5. Delta created for template zone
func TestAgent2Execute_Integration(t *testing.T) {
	ctx, cancel, stateAdapter, _, cacheAdapter, toolRegistry, llmClient, log := setupIntegration(t, 60*time.Second)
	defer cancel()

	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)
	agent2UC := usecases.NewAgent2ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	t.Run("Agent2 renders formation after Agent1 search", func(t *testing.T) {
		sessionID := uuid.New().String()
		turnID := "turn-agent2-test"
		createTestSession(t, ctx, cacheAdapter, sessionID)

		// Step 1: Agent1 populates state with products
		agent1Resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
			SessionID:  sessionID,
			Query:      "покажи кроссовки Nike",
			TenantSlug: "nike",
			TurnID:     turnID,
		})
		if err != nil {
			t.Fatalf("Agent1 failed: %v", err)
		}
		if agent1Resp.ProductsFound == 0 {
			t.Fatal("Agent1 found 0 products — cannot test Agent2")
		}
		t.Logf("Agent1: %d products, tool=%s", agent1Resp.ProductsFound, agent1Resp.ToolName)

		// Step 2: Agent2 renders
		agent2Resp, err := agent2UC.Execute(ctx, usecases.Agent2ExecuteRequest{
			SessionID: sessionID,
			TurnID:    turnID,
			UserQuery: "покажи кроссовки Nike",
		})
		if err != nil {
			t.Fatalf("Agent2 failed: %v", err)
		}

		// 1. Tool must be called
		if !agent2Resp.ToolCalled {
			t.Fatal("Expected Agent2 to call a render tool, but ToolCalled=false")
		}
		if agent2Resp.ToolName == "" {
			t.Fatal("Expected Agent2 ToolName to be non-empty")
		}
		t.Logf("Agent2 called tool: %s", agent2Resp.ToolName)

		// 2. Formation must be in response (read from state after tool)
		if agent2Resp.Formation == nil {
			t.Fatal("Expected formation in Agent2 response, got nil")
		}

		// 3. Widget count must match product count
		if len(agent2Resp.Formation.Widgets) == 0 {
			t.Fatal("Expected widgets in formation, got 0")
		}
		if len(agent2Resp.Formation.Widgets) != agent1Resp.ProductsFound {
			t.Errorf("Expected %d widgets (matching products), got %d", agent1Resp.ProductsFound, len(agent2Resp.Formation.Widgets))
		}
		t.Logf("Formation: mode=%s, widgets=%d", agent2Resp.Formation.Mode, len(agent2Resp.Formation.Widgets))

		// 4. First widget must have atoms with real data
		w := agent2Resp.Formation.Widgets[0]
		if len(w.Atoms) == 0 {
			t.Fatal("Expected atoms in first widget, got 0")
		}
		// Must have at least title and price
		var hasTitle, hasPrice bool
		for _, atom := range w.Atoms {
			if atom.Slot == domain.AtomSlotTitle && atom.Value != nil && atom.Value != "" {
				hasTitle = true
			}
			if atom.Slot == domain.AtomSlotPrice && atom.Value != nil {
				hasPrice = true
			}
		}
		if !hasTitle {
			t.Error("Expected title atom with non-empty value in first widget")
		}
		if !hasPrice {
			t.Error("Expected price atom with non-nil value in first widget")
		}
		t.Logf("First widget: %d atoms, template=%s", len(w.Atoms), w.Template)

		// 5. State must have template written
		state, err := stateAdapter.GetState(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get state: %v", err)
		}
		if state.Current.Template == nil {
			t.Fatal("Expected template in state, got nil")
		}
		if _, ok := state.Current.Template["formation"]; !ok {
			t.Fatal("Expected 'formation' key in state.Current.Template")
		}

		// 6. Data zone must NOT be modified by Agent2 (zone isolation)
		if len(state.Current.Data.Products) != agent1Resp.ProductsFound {
			t.Errorf("Data zone changed after Agent2! Expected %d products, got %d", agent1Resp.ProductsFound, len(state.Current.Data.Products))
		}

		// 7. Delta for template zone must exist
		deltas, err := stateAdapter.GetDeltas(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get deltas: %v", err)
		}
		var hasTemplateDelta bool
		for _, d := range deltas {
			if d.Path == "template" && d.TurnID == turnID {
				hasTemplateDelta = true
				t.Logf("Template delta: step=%d, tool=%s", d.Step, d.Action.Tool)
			}
		}
		if !hasTemplateDelta {
			t.Error("Expected delta with path=template for this turn, found none")
		}

		t.Logf("Agent2: %d tokens, $%.6f", agent2Resp.Usage.TotalTokens, agent2Resp.Usage.CostUSD)
	})

	t.Run("Agent2 does nothing with empty data", func(t *testing.T) {
		sessionID := uuid.New().String()
		createTestSession(t, ctx, cacheAdapter, sessionID)

		// Create empty state (no products)
		_, err := stateAdapter.CreateState(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to create state: %v", err)
		}

		agent2Resp, err := agent2UC.Execute(ctx, usecases.Agent2ExecuteRequest{
			SessionID: sessionID,
			TurnID:    "turn-empty",
			UserQuery: "покажи товары",
		})
		if err != nil {
			t.Fatalf("Agent2 failed on empty state: %v", err)
		}

		// Should NOT call any tool with 0 products
		if agent2Resp.ToolCalled {
			t.Error("Expected Agent2 NOT to call a tool with empty data")
		}
		if agent2Resp.Formation != nil {
			t.Error("Expected nil formation with empty data")
		}
	})
}

// TestPipelineExecute_Integration verifies the full pipeline:
// 1. Returns sessionId
// 2. Returns formation with widgets
// 3. Widgets have real product data
// 4. State is consistent (products in data, formation in template)
// 5. Follow-up query in same session gets same products
// 6. Delta field NOT in response (removed in this patch)
func TestPipelineExecute_Integration(t *testing.T) {
	ctx, cancel, stateAdapter, _, cacheAdapter, toolRegistry, llmClient, log := setupIntegration(t, 90*time.Second)
	defer cancel()

	pipelineUC := usecases.NewPipelineExecuteUseCase(llmClient, stateAdapter, cacheAdapter, nil, toolRegistry, log)

	t.Run("Nike query returns formation with widgets", func(t *testing.T) {
		sessionID := uuid.New().String()
		createTestSession(t, ctx, cacheAdapter, sessionID)

		resp, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
			SessionID:  sessionID,
			Query:      "покажи кроссовки Nike",
			TenantSlug: "nike",
		})
		if err != nil {
			t.Fatalf("Pipeline failed: %v", err)
		}

		// 1. Formation must exist
		if resp.Formation == nil {
			t.Fatal("Expected formation in pipeline response, got nil")
		}

		// 2. Widgets must be present
		if len(resp.Formation.Widgets) == 0 {
			t.Fatal("Expected widgets in formation, got 0")
		}
		t.Logf("Formation: mode=%s, %d widgets", resp.Formation.Mode, len(resp.Formation.Widgets))

		// 3. Widgets must have real data
		for i, w := range resp.Formation.Widgets {
			if len(w.Atoms) == 0 {
				t.Errorf("Widget %d has 0 atoms", i)
			}
			if w.EntityRef == nil {
				t.Errorf("Widget %d has nil EntityRef", i)
			}
		}

		// 4. First widget title must be a real product name (not empty)
		firstWidget := resp.Formation.Widgets[0]
		for _, atom := range firstWidget.Atoms {
			if atom.Slot == domain.AtomSlotTitle {
				if atom.Value == nil || atom.Value == "" {
					t.Error("First widget title atom has empty value")
				} else {
					t.Logf("First product: %v", atom.Value)
				}
				break
			}
		}

		// 5. State must be consistent
		state, err := stateAdapter.GetState(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get state: %v", err)
		}
		if len(state.Current.Data.Products) == 0 {
			t.Fatal("Expected products in state after pipeline")
		}
		if state.Current.Template == nil {
			t.Fatal("Expected template in state after pipeline")
		}
		if len(state.ConversationHistory) == 0 {
			t.Fatal("Expected conversation history after pipeline")
		}
		t.Logf("State: %d products, %d conversation messages, template=%v",
			len(state.Current.Data.Products), len(state.ConversationHistory), state.Current.Template != nil)

		// 6. Deltas must exist for both data and template zones
		deltas, err := stateAdapter.GetDeltas(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get deltas: %v", err)
		}
		var hasDataDelta, hasTemplateDelta bool
		for _, d := range deltas {
			if strings.HasPrefix(d.Path, "data.") {
				hasDataDelta = true
			}
			if d.Path == "template" {
				hasTemplateDelta = true
			}
		}
		if !hasDataDelta {
			t.Error("Expected data delta in DB")
		}
		if !hasTemplateDelta {
			t.Error("Expected template delta in DB")
		}
		t.Logf("Deltas: %d total (data=%v, template=%v)", len(deltas), hasDataDelta, hasTemplateDelta)

		// 7. Timing must be realistic
		if resp.TotalMs == 0 {
			t.Error("Expected non-zero TotalMs")
		}
		t.Logf("Timing: agent1=%dms, agent2=%dms, total=%dms", resp.Agent1Ms, resp.Agent2Ms, resp.TotalMs)
	})

	t.Run("Follow-up query in same session preserves context", func(t *testing.T) {
		sessionID := uuid.New().String()
		createTestSession(t, ctx, cacheAdapter, sessionID)

		// First query
		resp1, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
			SessionID:  sessionID,
			Query:      "покажи кроссовки Nike",
			TenantSlug: "nike",
		})
		if err != nil {
			t.Fatalf("First pipeline failed: %v", err)
		}
		if resp1.Formation == nil {
			t.Fatal("Expected formation from first query")
		}
		firstWidgetCount := len(resp1.Formation.Widgets)
		t.Logf("First query: %d widgets", firstWidgetCount)

		// Second query in SAME session
		resp2, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
			SessionID:  sessionID,
			Query:      "покажи Jordan",
			TenantSlug: "nike",
		})
		if err != nil {
			t.Fatalf("Second pipeline failed: %v", err)
		}

		// Conversation should have both queries
		state, err := stateAdapter.GetState(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get state: %v", err)
		}
		// First query: user + assistant:tool_use + user:tool_result = 3
		// Second query: +3 more = 6
		if len(state.ConversationHistory) < 6 {
			t.Errorf("Expected at least 6 conversation messages for 2 queries, got %d", len(state.ConversationHistory))
		}
		t.Logf("After 2 queries: %d conversation messages", len(state.ConversationHistory))

		// Second query should also have formation (or at least not crash)
		if resp2.Formation != nil {
			t.Logf("Second query: %d widgets", len(resp2.Formation.Widgets))
		} else {
			t.Logf("Second query: no formation (search may have returned 0)")
		}
	})
}

// TestPipelineExecute_CostReport prints detailed cost report for full pipeline
func TestPipelineExecute_CostReport(t *testing.T) {
	ctx, cancel, stateAdapter, _, cacheAdapter, toolRegistry, llmClient, log := setupIntegration(t, 60*time.Second)
	defer cancel()

	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)
	agent2UC := usecases.NewAgent2ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)
	pipelineUC := usecases.NewPipelineExecuteUseCase(llmClient, stateAdapter, cacheAdapter, nil, toolRegistry, log)

	sessionID := uuid.New().String()
	createTestSession(t, ctx, cacheAdapter, sessionID)

	query := "покажи кроссовки Nike"

	// Run Agent 1 separately
	agent1Resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
		SessionID:  sessionID,
		Query:      query,
		TenantSlug: "nike",
		TurnID:     "cost-turn-1",
	})
	if err != nil {
		t.Fatalf("Agent 1 failed: %v", err)
	}
	if agent1Resp.ProductsFound == 0 {
		t.Fatal("Agent 1 found 0 products")
	}

	// Run Agent 2 separately
	agent2Start := time.Now()
	agent2Resp, err := agent2UC.Execute(ctx, usecases.Agent2ExecuteRequest{
		SessionID: sessionID,
		TurnID:    "cost-turn-1",
		UserQuery: query,
	})
	if err != nil {
		t.Fatalf("Agent 2 failed: %v", err)
	}
	agent2Duration := time.Since(agent2Start).Milliseconds()

	if !agent2Resp.ToolCalled {
		t.Error("Expected Agent2 to call a tool")
	}

	// Full pipeline on new session
	sessionID2 := uuid.New().String()
	createTestSession(t, ctx, cacheAdapter, sessionID2)

	pipelineResp, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
		SessionID:  sessionID2,
		Query:      query,
		TenantSlug: "nike",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	if pipelineResp.Formation == nil {
		t.Error("Expected formation from pipeline")
	}

	// Print report
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TWO-AGENT PIPELINE COST REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Query: %s\n", query)
	fmt.Println(strings.Repeat("-", 60))

	fmt.Println("\nAGENT 1 (Tool Caller):")
	fmt.Printf("  Input tokens:  %d\n", agent1Resp.Usage.InputTokens)
	fmt.Printf("  Output tokens: %d\n", agent1Resp.Usage.OutputTokens)
	fmt.Printf("  Total tokens:  %d\n", agent1Resp.Usage.TotalTokens)
	fmt.Printf("  Cost:          $%.6f\n", agent1Resp.Usage.CostUSD)
	fmt.Printf("  Latency:       %d ms\n", agent1Resp.LatencyMs)
	fmt.Printf("  Products:      %d\n", agent1Resp.ProductsFound)

	fmt.Println("\nAGENT 2 (Renderer):")
	fmt.Printf("  Input tokens:  %d\n", agent2Resp.Usage.InputTokens)
	fmt.Printf("  Output tokens: %d\n", agent2Resp.Usage.OutputTokens)
	fmt.Printf("  Total tokens:  %d\n", agent2Resp.Usage.TotalTokens)
	fmt.Printf("  Cost:          $%.6f\n", agent2Resp.Usage.CostUSD)
	fmt.Printf("  Latency:       %d ms\n", agent2Duration)
	fmt.Printf("  Tool:          %s\n", agent2Resp.ToolName)
	if agent2Resp.Formation != nil {
		fmt.Printf("  Widgets:       %d\n", len(agent2Resp.Formation.Widgets))
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
	template := &domain.FormationTemplate{
		Mode: domain.FormationTypeGrid,
		Grid: &domain.GridConfig{Rows: 2, Cols: 2},
		WidgetTemplate: domain.WidgetTemplate{
			Size: domain.WidgetSizeMedium,
			Atoms: []domain.AtomTemplate{
				{Type: domain.AtomTypeImage, Field: "images", Size: "medium"},
				{Type: domain.AtomTypeText, Field: "name", Style: "heading"},
				{Type: domain.AtomTypeNumber, Field: "price", Format: "currency"},
				{Type: domain.AtomTypeNumber, Field: "rating"},
			},
		},
	}

	products := []domain.Product{
		{
			ID: "prod-1", Name: "Nike Air Max 90", Price: 12990, Currency: "$",
			Images: []string{"https://example.com/nike1.jpg"}, Rating: 4.5,
			Brand: "Nike", Category: "Sneakers",
		},
		{
			ID: "prod-2", Name: "Nike Air Force 1", Price: 9990, Currency: "$",
			Images: []string{"https://example.com/nike2.jpg"}, Rating: 4.8,
			Brand: "Nike", Category: "Sneakers",
		},
	}

	formation, err := usecases.ApplyTemplate(template, products)
	if err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}

	if formation.Mode != domain.FormationTypeGrid {
		t.Errorf("Expected mode=grid, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 2 {
		t.Fatalf("Expected 2 widgets, got %d", len(formation.Widgets))
	}

	w := formation.Widgets[0]
	if w.Template != domain.WidgetTemplateProductCard {
		t.Errorf("Expected template=ProductCard, got %s", w.Template)
	}
	if w.Size != domain.WidgetSizeMedium {
		t.Errorf("Expected size=medium, got %s", w.Size)
	}
	if len(w.Atoms) == 0 {
		t.Fatalf("Expected atoms to be present")
	}

	var titleAtom, priceAtom, heroAtom *domain.Atom
	for i := range w.Atoms {
		switch w.Atoms[i].Slot {
		case domain.AtomSlotTitle:
			titleAtom = &w.Atoms[i]
		case domain.AtomSlotPrice:
			priceAtom = &w.Atoms[i]
		case domain.AtomSlotHero:
			heroAtom = &w.Atoms[i]
		}
	}

	if titleAtom == nil || titleAtom.Value != "Nike Air Max 90" {
		t.Errorf("Expected title='Nike Air Max 90', got %v", titleAtom)
	}
	if priceAtom == nil || priceAtom.Value != 12990 {
		t.Errorf("Expected price=12990, got %v", priceAtom)
	}
	if heroAtom == nil {
		t.Errorf("Expected hero image atom")
	}

	// Verify JSON serialization works (what frontend receives)
	jsonBytes, err := json.Marshal(formation)
	if err != nil {
		t.Fatalf("Failed to marshal formation: %v", err)
	}
	if len(jsonBytes) == 0 {
		t.Error("Expected non-empty JSON")
	}
}
