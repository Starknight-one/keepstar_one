// build-projection backfills catalog.tenant_search_projection for every tenant
// by calling the same RebuildSearchProjection used on the apply hot path. Run
// it once after a data seed (or anytime) to (re)populate the search slice. With
// OPENAI_API_KEY set, rows get embeddings; without it, rows are FTS-only and a
// later run fills vectors.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	openaiAdapter "keepstar-admin/internal/adapters/openai"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/logger"
)

func main() {
	for _, path := range []string{"../../project/.env", ".env"} {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	lg := logger.New("info")
	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer client.Close()

	writer := postgres.NewCatalogV2WriterAdapter(client, lg)
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		writer.SetEmbedder(openaiAdapter.NewEmbeddingClient(key, os.Getenv("EMBEDDING_MODEL"), 384))
		fmt.Println("embedder: OpenAI (vectors will be generated)")
	} else {
		fmt.Println("embedder: none (FTS-only; rows get NULL embedding)")
	}

	tenantIDs, err := allTenantIDs(ctx, client)
	if err != nil {
		log.Fatalf("list tenants: %v", err)
	}
	fmt.Printf("Rebuilding projection for %d tenants...\n", len(tenantIDs))

	total := 0
	for i, tid := range tenantIDs {
		n, err := writer.RebuildSearchProjection(ctx, tid, nil)
		if err != nil {
			log.Printf("  [%d/%d] tenant %s FAILED: %v", i+1, len(tenantIDs), tid, err)
			continue
		}
		total += n
		fmt.Printf("  [%d/%d] tenant %s: %d rows\n", i+1, len(tenantIDs), tid, n)
	}
	fmt.Printf("Done: %d projection rows across %d tenants\n", total, len(tenantIDs))
}

func allTenantIDs(ctx context.Context, client *postgres.Client) ([]string, error) {
	rows, err := client.Pool().Query(ctx, `SELECT id::text FROM catalog.tenants ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
