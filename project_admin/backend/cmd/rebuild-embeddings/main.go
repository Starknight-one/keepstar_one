package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	pgvector "github.com/pgvector/pgvector-go"
	openaiAdapter "keepstar-admin/internal/adapters/openai"
)

// buildEmbeddingText creates a compact semantic text for vector embedding.
// For PIM-enriched products (v2), uses clean structured data.
// For legacy products, falls back to name + brand + category.
func buildEmbeddingText(row productRow) string {
	if row.enrichmentVersion >= 2 && row.shortName != "" {
		parts := []string{row.shortName}
		if row.brand != "" {
			parts = append(parts, row.brand)
		}
		if row.categoryName != "" {
			parts = append(parts, row.categoryName)
		}
		if row.productForm != "" {
			parts = append(parts, row.productForm)
		}
		if row.texture != "" {
			parts = append(parts, row.texture)
		}
		if row.marketingClaim != "" {
			parts = append(parts, row.marketingClaim)
		}
		if len(row.skinType) > 0 {
			parts = append(parts, strings.Join(row.skinType, " "))
		}
		if len(row.concern) > 0 {
			parts = append(parts, strings.Join(row.concern, " "))
		}
		if len(row.keyIngredients) > 0 {
			parts = append(parts, strings.Join(row.keyIngredients, " "))
		}
		if row.routineStep != "" {
			parts = append(parts, row.routineStep)
		}
		return strings.Join(parts, " ")
	}

	// Legacy fallback
	text := row.name
	if row.brand != "" {
		text += " " + row.brand
	}
	if row.categoryName != "" {
		text += " " + row.categoryName
	}
	return text
}

type productRow struct {
	id                string
	name              string
	brand             string
	categoryName      string
	shortName         string
	productForm       string
	texture           string
	routineStep       string
	marketingClaim    string
	skinType          []string
	concern           []string
	keyIngredients    []string
	enrichmentVersion int
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

	// Step 1: Optionally reset embeddings for enriched products
	if resetAll {
		tag, err := pool.Exec(ctx, `UPDATE catalog.master_products SET embedding = NULL WHERE enrichment_version >= 2`)
		if err != nil {
			log.Fatalf("reset embeddings: %v", err)
		}
		fmt.Printf("Reset %d embeddings for enriched products\n", tag.RowsAffected())
	}

	// Step 2: Fetch products without embeddings
	query := `SELECT mp.id, mp.name, COALESCE(mp.brand, '') as brand,
		COALESCE(c.name, '') as category_name,
		COALESCE(mp.short_name, '') as short_name,
		COALESCE(mp.product_form, '') as product_form,
		COALESCE(mp.texture, '') as texture,
		COALESCE(mp.routine_step, '') as routine_step,
		COALESCE(mp.marketing_claim, '') as marketing_claim,
		mp.skin_type, mp.concern, mp.key_ingredients,
		COALESCE(mp.enrichment_version, 0) as enrichment_version
		FROM catalog.master_products mp
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE mp.embedding IS NULL
		ORDER BY mp.created_at`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		log.Fatalf("query products: %v", err)
	}
	defer rows.Close()

	var products []productRow
	for rows.Next() {
		var p productRow
		if err := rows.Scan(
			&p.id, &p.name, &p.brand, &p.categoryName,
			&p.shortName, &p.productForm, &p.texture, &p.routineStep,
			&p.marketingClaim, &p.skinType, &p.concern, &p.keyIngredients,
			&p.enrichmentVersion,
		); err != nil {
			log.Fatalf("scan product: %v", err)
		}
		products = append(products, p)
	}

	if len(products) == 0 {
		fmt.Println("No products need embeddings")
		return
	}

	fmt.Printf("Building embeddings for %d products...\n", len(products))

	// Step 3: Build texts
	texts := make([]string, len(products))
	for i, p := range products {
		texts[i] = buildEmbeddingText(p)
	}

	// Step 4: Embed in batches of 100
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
