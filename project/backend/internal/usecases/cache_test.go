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

// TestPromptCaching_Chain tests prompt caching across a chain of requests in one session.
// It sends up to 10 queries and tracks cache hit/miss, tokens, cost for each request.
//
// Run with: go test -v -run TestPromptCaching_Chain -timeout 300s ./internal/usecases/
//
// Expected behavior:
//   - Request 1: cache_creation_input_tokens > 0, cache_read = 0 (first write)
//   - Request 2+: cache_read_input_tokens > 0 (cache hit on tools + system)
//   - Conversation history grows, cache hits should increase
//   - Total cost with cache should be lower than without
func TestPromptCaching_Chain(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log := logger.New("info")

	// Connect to database
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
	presetRegistry := presets.NewPresetRegistry()
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)

	// Initialize use cases
	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	// Create session
	sessionID := uuid.New().String()
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

	// Chain of queries simulating a real user session
	queries := []string{
		"покажи кроссовки",
		"покажи Nike",
		"что-нибудь дешёвое до 5000",
		"покажи все товары",
		"найди кроссовки Jordan",
		"покажи дорогие товары",
		"есть ли Air Max",
		"покажи товары со скидкой",
		"найди белые кроссовки",
		"покажи всё что есть Nike",
	}

	type requestMetrics struct {
		Query                    string
		InputTokens              int
		OutputTokens             int
		CacheCreationInputTokens int
		CacheReadInputTokens     int
		TotalTokens              int
		CostUSD                  float64
		LatencyMs                int
		LLMCallMs                int64
		ToolName                 string
		ProductsFound            int
		CacheHitRate             float64 // percentage
		ConversationHistoryLen   int
	}

	results := make([]requestMetrics, 0, len(queries))
	var totalCostWithCache float64
	var totalCostWithoutCache float64

	// Header
	fmt.Println()
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("PROMPT CACHING TEST — Session: %s\n", sessionID)
	fmt.Printf("Model: %s | Queries: %d\n", model, len(queries))
	fmt.Println(strings.Repeat("=", 120))

	for i, query := range queries {
		t.Run(fmt.Sprintf("Request_%d", i+1), func(t *testing.T) {
			start := time.Now()

			resp, err := agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
				SessionID: sessionID,
				Query:     query,
			})
			if err != nil {
				t.Fatalf("Request %d failed: %v", i+1, err)
			}

			elapsed := time.Since(start).Milliseconds()

			// Get conversation history length from state
			state, _ := stateAdapter.GetState(ctx, sessionID)
			historyLen := 0
			if state != nil {
				historyLen = len(state.ConversationHistory)
			}

			// Calculate cache hit rate
			totalInput := resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens
			var hitRate float64
			if totalInput > 0 {
				hitRate = float64(resp.Usage.CacheReadInputTokens) / float64(totalInput) * 100
			}

			// Calculate what cost would be without cache (all tokens at base rate)
			pricing, ok := domain.LLMPricing[model]
			if !ok {
				pricing = domain.LLMPricing["claude-haiku-4-5-20251001"]
			}
			costWithoutCache := float64(totalInput) * pricing.InputPerMillion / 1_000_000
			costWithoutCache += float64(resp.Usage.OutputTokens) * pricing.OutputPerMillion / 1_000_000

			m := requestMetrics{
				Query:                    query,
				InputTokens:              resp.Usage.InputTokens,
				OutputTokens:             resp.Usage.OutputTokens,
				CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
				TotalTokens:              resp.Usage.TotalTokens,
				CostUSD:                  resp.Usage.CostUSD,
				LatencyMs:                int(elapsed),
				LLMCallMs:                resp.LLMCallMs,
				ToolName:                 resp.ToolName,
				ProductsFound:            resp.ProductsFound,
				CacheHitRate:             hitRate,
				ConversationHistoryLen:   historyLen,
			}
			results = append(results, m)

			totalCostWithCache += resp.Usage.CostUSD
			totalCostWithoutCache += costWithoutCache

			// Print per-request summary
			cacheStatus := "MISS"
			if m.CacheReadInputTokens > 0 {
				cacheStatus = "HIT"
			}

			fmt.Printf("\n[%d/%d] %s\n", i+1, len(queries), query)
			fmt.Printf("  Cache: %-4s | Write: %5d tok | Read: %5d tok | Hit rate: %5.1f%%\n",
				cacheStatus, m.CacheCreationInputTokens, m.CacheReadInputTokens, m.CacheHitRate)
			fmt.Printf("  Input: %5d tok | Output: %3d tok | Total: %5d tok\n",
				m.InputTokens, m.OutputTokens, m.TotalTokens)
			fmt.Printf("  Cost:  $%.6f (without cache: $%.6f) | Saved: $%.6f\n",
				m.CostUSD, costWithoutCache, costWithoutCache-m.CostUSD)
			fmt.Printf("  Latency: %dms (LLM: %dms) | Tool: %s | Products: %d | History: %d msgs\n",
				m.LatencyMs, m.LLMCallMs, m.ToolName, m.ProductsFound, m.ConversationHistoryLen)
		})

		// Small delay between requests to stay within rate limits
		if i < len(queries)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Summary report
	fmt.Println()
	fmt.Println(strings.Repeat("=", 120))
	fmt.Println("SUMMARY REPORT")
	fmt.Println(strings.Repeat("=", 120))

	fmt.Printf("\n%-4s | %-30s | %6s | %6s | %6s | %6s | %8s | %6s | %s\n",
		"#", "Query", "Input", "CWrite", "CRead", "Output", "Cost", "Hit%", "Cache")
	fmt.Println(strings.Repeat("-", 120))

	for i, m := range results {
		cacheStatus := "MISS"
		if m.CacheReadInputTokens > 0 {
			cacheStatus = "HIT"
		}
		queryDisplay := m.Query
		if len(queryDisplay) > 28 {
			queryDisplay = queryDisplay[:28] + ".."
		}
		fmt.Printf("%-4d | %-30s | %6d | %6d | %6d | %6d | $%7.5f | %5.1f%% | %s\n",
			i+1, queryDisplay,
			m.InputTokens, m.CacheCreationInputTokens, m.CacheReadInputTokens, m.OutputTokens,
			m.CostUSD, m.CacheHitRate, cacheStatus)
	}

	fmt.Println(strings.Repeat("-", 120))

	// Totals
	var totalInput, totalOutput, totalCacheWrite, totalCacheRead int
	var totalLatency int
	cacheHits := 0
	for _, m := range results {
		totalInput += m.InputTokens
		totalOutput += m.OutputTokens
		totalCacheWrite += m.CacheCreationInputTokens
		totalCacheRead += m.CacheReadInputTokens
		totalLatency += m.LatencyMs
		if m.CacheReadInputTokens > 0 {
			cacheHits++
		}
	}

	fmt.Printf("\nTokens total:  Input=%d  CacheWrite=%d  CacheRead=%d  Output=%d\n",
		totalInput, totalCacheWrite, totalCacheRead, totalOutput)
	fmt.Printf("Cache hits:    %d/%d requests (%.0f%%)\n",
		cacheHits, len(results), float64(cacheHits)/float64(len(results))*100)
	fmt.Printf("Cost with cache:    $%.6f\n", totalCostWithCache)
	fmt.Printf("Cost without cache: $%.6f\n", totalCostWithoutCache)

	savings := totalCostWithoutCache - totalCostWithCache
	savingsPercent := float64(0)
	if totalCostWithoutCache > 0 {
		savingsPercent = savings / totalCostWithoutCache * 100
	}
	fmt.Printf("Savings:            $%.6f (%.1f%%)\n", savings, savingsPercent)
	fmt.Printf("Avg latency:        %dms\n", totalLatency/len(results))

	fmt.Println()
	fmt.Println("COST PROJECTIONS:")
	fmt.Printf("  1,000 sessions (10 msgs each):   $%.2f (without cache: $%.2f)\n",
		totalCostWithCache*1000, totalCostWithoutCache*1000)
	fmt.Printf("  10,000 sessions (10 msgs each):  $%.2f (without cache: $%.2f)\n",
		totalCostWithCache*10000, totalCostWithoutCache*10000)

	fmt.Println(strings.Repeat("=", 120))

	// Verify session state preserved
	finalState, err := stateAdapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to get final state: %v", err)
	}

	fmt.Printf("\nSession state check:\n")
	fmt.Printf("  Conversation history: %d messages\n", len(finalState.ConversationHistory))
	fmt.Printf("  Step: %d\n", finalState.Step)
	fmt.Printf("  Products in state: %d\n", len(finalState.Current.Data.Products))

	if len(finalState.ConversationHistory) == 0 {
		t.Error("Expected conversation history to be preserved in state")
	}

	// Verify cache is working (at least some requests should have cache reads)
	if cacheHits == 0 && len(results) > 1 {
		t.Error("Expected cache hits on requests 2+, but got 0 cache reads. Check: padding tools >= 4096 tokens, conversation_history persistence, stable JSON key ordering.")
	}
}
