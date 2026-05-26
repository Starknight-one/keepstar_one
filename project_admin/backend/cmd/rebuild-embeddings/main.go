package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	pgvector "github.com/pgvector/pgvector-go"
	openaiAdapter "keepstar-admin/internal/adapters/openai"
	"keepstar-admin/internal/searchtext"
)

type productRow struct {
	id   string
	text string
}

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
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	resetAll := len(os.Args) > 1 && os.Args[1] == "--reset"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	embeddingClient := openaiAdapter.NewEmbeddingClient(openaiKey, "", 384)

	if resetAll {
		tag, err := pool.Exec(ctx, `UPDATE catalog.master_products SET embedding = NULL`)
		if err != nil {
			log.Fatalf("reset embeddings: %v", err)
		}
		fmt.Printf("Reset %d embeddings\n", tag.RowsAffected())
	}

	// Vertical-agnostic: read tier1 identity + tier2/tier3 jsonb. The legacy
	// cosmetics columns were dropped in Group D; typed attrs now live in tier2.
	query := `SELECT mp.id, mp.name, COALESCE(mp.brand,''), COALESCE(mp.description,''),
		COALESCE(mp.vertical,''), COALESCE(mp.tier2,'{}'::jsonb), COALESCE(mp.tier3,'{}'::jsonb)
		FROM catalog.master_products mp
		WHERE mp.embedding IS NULL
		ORDER BY mp.created_at`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		log.Fatalf("query products: %v", err)
	}
	defer rows.Close()

	var products []productRow
	for rows.Next() {
		var id, name, brand, description, vertical string
		var tier2Raw, tier3Raw []byte
		if err := rows.Scan(&id, &name, &brand, &description, &vertical, &tier2Raw, &tier3Raw); err != nil {
			log.Fatalf("scan product: %v", err)
		}
		products = append(products, productRow{
			id: id,
			text: searchtext.BuildProjectionTexts(searchtext.ProjectionSource{
				Name: name, Brand: brand, Description: description, Vertical: vertical,
				Tier2: unmarshalJSONB(tier2Raw), Tier3: unmarshalJSONB(tier3Raw),
			}).SearchText,
		})
	}
	rows.Close()

	if len(products) == 0 {
		fmt.Println("No products need embeddings")
		return
	}

	fmt.Printf("Building embeddings for %d products...\n", len(products))

	texts := make([]string, len(products))
	for i, p := range products {
		texts[i] = p.text
	}

	batchSize := 100
	embedded := 0
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		embeddings, err := embeddingClient.Embed(ctx, texts[i:end])
		if err != nil {
			log.Printf("embed batch %d-%d failed: %v", i, end, err)
			break
		}

		for j, emb := range embeddings {
			_, err := pool.Exec(ctx,
				`UPDATE catalog.master_products SET embedding = $1 WHERE id = $2`,
				pgvector.NewVector(emb), products[i+j].id)
			if err != nil {
				log.Printf("save embedding for %s: %v", products[i+j].id, err)
				continue
			}
			embedded++
		}

		fmt.Printf("  batch %d/%d done (%d embedded)\n", i/batchSize+1, (len(texts)+batchSize-1)/batchSize, embedded)
	}

	fmt.Printf("Done: %d/%d products embedded\n", embedded, len(products))
}

func unmarshalJSONB(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
