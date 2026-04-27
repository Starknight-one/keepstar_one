// run-discovery — fire the discovery agent on a tenant and print the
// resulting MappingArtifact (or stop-reason if it didn't commit).
//
// Used to iterate on the agent prompt without going through the HTTP /discover
// endpoint (which needs auth + a running admin server). Talks to the same Neon
// DB and uses the same configured Sonnet model — output should match what
// curator users will see.
//
// Usage:
//   ANTHROPIC_API_KEY=... ADMIN_ENCRYPTION_KEY=... DATABASE_URL=... \
//   go run ./cmd/run-discovery -shop keepstar-neaqpan1.myshopify.com
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	anthropicAdapter "keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/adapters/shopify"
	"keepstar-admin/internal/config"
	"keepstar-admin/internal/crypto/secretbox"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

func main() {
	shop := flag.String("shop", "", "shop domain")
	saveTo := flag.String("save", "", "optional path to save the full result JSON (default: discovery-<shop>-<ts>.json)")
	flag.Parse()
	if *shop == "" {
		log.Fatal("usage: run-discovery -shop <shop>.myshopify.com")
	}
	if _, ok := shopify.ValidateShopDomain(*shop); !ok {
		log.Fatalf("invalid shop domain: %s", *shop)
	}
	if *saveTo == "" {
		*saveTo = fmt.Sprintf("discovery-%s-%d.json", *shop, time.Now().Unix())
	}

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if cfg.EncryptionKey == "" {
		log.Fatal("ADMIN_ENCRYPTION_KEY required")
	}
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY required (Sonnet 4.6 access)")
	}
	box, err := secretbox.NewFromBase64(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lg := logger.New("info")

	dbClient, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer dbClient.Close()
	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	integrationsAdapter := postgres.NewIntegrationsAdapter(dbClient, box)
	integration, err := integrationsAdapter.GetByShopDomain(ctx, *shop)
	if err != nil {
		log.Fatalf("integration lookup: %v", err)
	}
	tenantID := integration.TenantID
	log.Printf("tenant=%s shop=%s", tenantID, *shop)

	stagingAdapter := postgres.NewShopifyStagingAdapter(dbClient, lg)
	masterVariantsAdapter := postgres.NewMasterVariantsAdapter(dbClient, lg)
	mappingArtifactAdapter := postgres.NewMappingArtifactAdapter(dbClient, lg)

	agentClient := anthropicAdapter.NewAgentClient(cfg.AnthropicAPIKey, "claude-sonnet-4-6")
	embedder := newNoOpEmbedder() // discovery agent rarely calls embeddings; see comment

	agent := usecases.NewDiscoveryAgent(agentClient, stagingAdapter, masterVariantsAdapter, embedder, lg)
	uc := usecases.NewDiscoveryUseCase(stagingAdapter, masterVariantsAdapter, mappingArtifactAdapter, nil, agent, lg)

	log.Println("=== running discovery ===")
	t0 := time.Now()
	res, err := uc.Run(ctx, tenantID)
	if err != nil {
		log.Fatalf("discovery: %v", err)
	}
	log.Printf("=== discovery done in %s ===", time.Since(t0))

	// Pretty-print summary.
	fmt.Println()
	fmt.Println("===== DISCOVERY RESULT =====")
	fmt.Printf("status:           %s\n", res.Status)
	fmt.Printf("stop_reason:      %s\n", res.StopReason)
	fmt.Printf("turns:            %d\n", res.TurnsUsed)
	fmt.Printf("input_tokens:     %d\n", res.InputTokens)
	fmt.Printf("output_tokens:    %d\n", res.OutputTokens)
	fmt.Printf("duration_ms:      %d\n", res.DurationMs)
	fmt.Printf("field_mappings:   %d\n", res.FieldMappingSize)
	fmt.Printf("category_mappings:%d\n", res.CategorySize)
	fmt.Printf("master_templates: %d\n", res.TemplateCount)

	// Persist full transcript + result for offline inspection.
	out, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(*saveTo, out, 0o600); err != nil {
		log.Printf("warn: save failed: %v", err)
	} else {
		log.Printf("full result saved to %s", *saveTo)
	}
}

// noOpEmbedder returns a zero-vector. The discovery agent rarely needs real
// embeddings — its main embedding-using tool (find_master_products_by_embedding)
// is optional. Saves the OpenAI round-trip on every CLI run.
type noOpEmbedder struct{}

func newNoOpEmbedder() *noOpEmbedder { return &noOpEmbedder{} }

func (noOpEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 384)
	}
	return out, nil
}
