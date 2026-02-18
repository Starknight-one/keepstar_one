package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const (
	anthropicAPI = "https://api.anthropic.com/v1/messages"
	model        = "claude-haiku-4-5-20251001"
	batchSize    = 50
	workers      = 3
)

// Anthropic API types (same pattern as enrichment_client.go)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type enrichedIngredient struct {
	InciName string `json:"inci_name"`
	NameRU   string `json:"name_ru"`
	Function string `json:"function"`
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func makeSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	return s
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
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	pass1ParseINCI(ctx, pool)
	pass2EnrichIngredients(ctx, pool, apiKey)
}

// ---------------------------------------------------------------------------
// Pass 1: Parse INCI ingredients from master_products
// ---------------------------------------------------------------------------

func pass1ParseINCI(ctx context.Context, pool *pgxpool.Pool) {
	log.Println("=== Pass 1: Parse INCI ingredients ===")

	rows, err := pool.Query(ctx,
		`SELECT id, attributes->>'ingredients' AS ingredients
		 FROM catalog.master_products
		 WHERE attributes->>'ingredients' IS NOT NULL
		   AND attributes->>'ingredients' != ''`)
	if err != nil {
		log.Fatalf("query products: %v", err)
	}
	defer rows.Close()

	type productRow struct {
		ID          string
		Ingredients string
	}
	var products []productRow
	for rows.Next() {
		var p productRow
		if err := rows.Scan(&p.ID, &p.Ingredients); err != nil {
			log.Fatalf("scan product: %v", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows error: %v", err)
	}

	// slug → ingredient ID cache
	slugToID := make(map[string]int64)
	totalIngredients := 0
	totalLinks := 0

	for _, p := range products {
		parts := strings.Split(p.Ingredients, ",")
		for i, raw := range parts {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			slug := makeSlug(name)
			if slug == "" {
				continue
			}

			// Get or insert ingredient
			ingID, ok := slugToID[slug]
			if !ok {
				err := pool.QueryRow(ctx,
					`INSERT INTO catalog.ingredients (inci_name, slug)
					 VALUES ($1, $2)
					 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
					 RETURNING id`,
					name, slug).Scan(&ingID)
				if err != nil {
					log.Printf("insert ingredient %q (slug=%q): %v", name, slug, err)
					continue
				}
				slugToID[slug] = ingID
				totalIngredients++
			}

			// Link product ↔ ingredient
			position := i + 1
			isKey := position <= 5
			_, err := pool.Exec(ctx,
				`INSERT INTO catalog.product_ingredients (master_product_id, ingredient_id, position, is_key)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT DO NOTHING`,
				p.ID, ingID, position, isKey)
			if err != nil {
				log.Printf("link product %s → ingredient %d: %v", p.ID, ingID, err)
				continue
			}
			totalLinks++
		}
	}

	log.Printf("Parsed %d ingredients from %d products (%d links created)",
		totalIngredients, len(products), totalLinks)
}

// ---------------------------------------------------------------------------
// Pass 2: Enrich ingredients with Russian translations via LLM
// ---------------------------------------------------------------------------

func pass2EnrichIngredients(ctx context.Context, pool *pgxpool.Pool, apiKey string) {
	log.Println("=== Pass 2: Enrich ingredients via LLM ===")

	rows, err := pool.Query(ctx,
		`SELECT id, inci_name FROM catalog.ingredients WHERE name_ru IS NULL OR name_ru = ''`)
	if err != nil {
		log.Fatalf("query unenriched ingredients: %v", err)
	}
	defer rows.Close()

	var ingredients []ingRow
	for rows.Next() {
		var ing ingRow
		if err := rows.Scan(&ing.ID, &ing.InciName); err != nil {
			log.Fatalf("scan ingredient: %v", err)
		}
		ingredients = append(ingredients, ing)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows error: %v", err)
	}

	if len(ingredients) == 0 {
		log.Println("No ingredients to enrich")
		return
	}

	// Split into batches
	var batches [][]ingRow
	for i := 0; i < len(ingredients); i += batchSize {
		end := i + batchSize
		if end > len(ingredients) {
			end = len(ingredients)
		}
		batches = append(batches, ingredients[i:end])
	}

	log.Printf("Enriching %d ingredients in %d batches (%d workers)",
		len(ingredients), len(batches), workers)

	// Worker pool
	type batchJob struct {
		index int
		items []ingRow
	}

	jobs := make(chan batchJob, len(batches))
	for i, b := range batches {
		jobs <- batchJob{index: i, items: b}
	}
	close(jobs)

	client := &http.Client{Timeout: 60 * time.Second}
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				log.Printf("Enriching batch %d/%d (%d ingredients)",
					job.index+1, len(batches), len(job.items))

				results, err := callLLM(ctx, client, apiKey, job.items)
				if err != nil {
					log.Printf("LLM error batch %d: %v", job.index+1, err)
					continue
				}

				updated := 0
				for _, r := range results {
					_, err := pool.Exec(ctx,
						`UPDATE catalog.ingredients
						 SET name_ru = $1, function = $2
						 WHERE LOWER(inci_name) = LOWER($3)`,
						r.NameRU, r.Function, r.InciName)
					if err != nil {
						log.Printf("update ingredient %q: %v", r.InciName, err)
						continue
					}
					updated++
				}
				log.Printf("Batch %d/%d: updated %d/%d ingredients",
					job.index+1, len(batches), updated, len(job.items))
			}
		}()
	}

	wg.Wait()
	log.Println("Enrichment complete")
}

func callLLM(ctx context.Context, client *http.Client, apiKey string, items []ingRow) ([]enrichedIngredient, error) {
	// Build user prompt
	var names []string
	for _, it := range items {
		names = append(names, it.InciName)
	}
	userPrompt := strings.Join(names, "\n")

	reqBody := messagesRequest{
		Model:     model,
		MaxTokens: 4096,
		System: `You are a cosmetics ingredient expert. For each INCI ingredient name, provide:
- name_ru: Russian translation/transliteration
- function: primary cosmetic function (one of: humectant, emollient, antioxidant, exfoliant, anti-inflammatory, brightening, preservative, surfactant, emulsifier, thickener, fragrance, colorant, solvent, UV-filter, pH-adjuster, other)

Return ONLY a JSON array:
[{"inci_name": "...", "name_ru": "...", "function": "..."}]`,
		Messages: []message{{Role: "user", Content: userPrompt}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic")
	}

	text := extractJSON(msgResp.Content[0].Text)

	var results []enrichedIngredient
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		return nil, fmt.Errorf("parse enrichment JSON: %w (raw: %.500s)", err, text)
	}

	return results, nil
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

type ingRow struct {
	ID       int64
	InciName string
}
