# Feature: Векторный поиск (Hybrid Search)

## ADW ID: feature-vector-search

## Суть

`catalog_search` становится мета-тулом с двумя явными входами:
- **`filters`** — структурированные атрибуты для жёсткого SQL поиска (brand, price, color, material, ...)
- **`vector_query`** — семантический запрос на языке пользователя для vector search

Agent1 (Haiku) видит в tool definition **все доступные поля фильтрации с типами** (brand: string, color: string, min_price: number, ...). Сам решает что пойдёт в keyword-фильтры (точное), а что в vector (семантику). Нормализация происходит естественно — Haiku это LLM с мировыми знаниями, она знает что Найк = Nike. А если напишет чуть не так — ILIKE (нечёткий поиск) + vector search подстрахуют.

LLM-нормализатор удаляется полностью. Статическая brand-карта не нужна.

## Data Flow

```
User: "чёрные кроссы Найк подешевле"
  → Agent1 (Haiku) видит tool definition с полями фильтрации:
      brand: string, category: string, color: string, min_price: number, ...
    Agent заполняет:
      filters: {brand: "Nike", color: "Black"}
      vector_query: "кроссы"
      sort_by: "price", sort_order: "asc"
     │
     ▼
  catalog_search meta-tool:
     ├─ Keyword: brand='Nike' AND category=Sneakers AND color='Black' ORDER BY price ASC
     ├─ Vector:  embed("кроссы Nike") → pgvector cosine search
     └─ RRF merge (k=60) → combined ranked results
     │
     ▼
  State write: products = merged results
  Return: "ok: found N products"
```

## Relevant Files

### Existing Files (модифицируются)

- `project/backend/internal/prompts/prompt_analyze_query.go` — промпт Agent1: объяснить filters vs vector_query
- `project/backend/internal/tools/tool_catalog_search.go` — static definition с filters/vector_query, hybrid search, RRF
- `project/backend/internal/tools/tool_registry.go` — `EmbeddingPort` вместо `LLMPort`
- `project/backend/internal/ports/catalog_port.go` — `VectorSearch`, `SeedEmbedding`, `GetMasterProductsWithoutEmbedding`, `Attributes` в `ProductFilter`
- `project/backend/internal/adapters/postgres/catalog_migrations.go` — pgvector extension, embedding column, HNSW index
- `project/backend/internal/adapters/postgres/postgres_catalog.go` — `VectorSearch`, `SeedEmbedding`, `GetMasterProductsWithoutEmbedding`, JSONB filtering в `ListProducts`
- `project/backend/internal/config/config.go` — `OpenAIAPIKey`, `EmbeddingModel`, `HasEmbeddings()`
- `project/backend/cmd/server/main.go` — wiring: embedding adapter, startup embed, admin endpoint

### New Files

- `project/backend/internal/ports/embedding_port.go` — интерфейс `EmbeddingPort`
- `project/backend/internal/adapters/openai/embedding_client.go` — OpenAI Embeddings API adapter

### Удалить

- `project/backend/internal/tools/normalizer.go` — полностью
- `project/backend/internal/prompts/prompt_normalize_query.go` — полностью

### Read-Only

- `project/backend/internal/domain/` — embedding НЕ в domain, только в БД
- `project/backend/internal/usecases/agent1_execute.go` — не меняется
- `project/backend/internal/tools/tool_search_products.go` — содержит `removeSubstringIgnoreCase`

## Step by Step Tasks

IMPORTANT: Execute strictly in order.

### 1. Добавить зависимость pgvector-go

```bash
cd project/backend && go get github.com/pgvector/pgvector-go
```

### 2. Создать EmbeddingPort

**Файл:** `project/backend/internal/ports/embedding_port.go`

```go
package ports

import "context"

// EmbeddingPort generates vector embeddings for text.
// Implementations: OpenAI API, local model server, etc.
type EmbeddingPort interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

### 3. Создать OpenAI Embedding Adapter

**Файл:** `project/backend/internal/adapters/openai/embedding_client.go`

Паттерн: как `adapters/anthropic/anthropic_client.go` — чистый `net/http` + `encoding/json`, БЕЗ SDK.

```go
type EmbeddingClient struct {
	apiKey string
	model  string
	dims   int
	client *http.Client
}
```

**Конструктор:** `NewEmbeddingClient(apiKey, model string, dims int) *EmbeddingClient`
- Default model: `"text-embedding-3-small"`
- Default dims: `384`
- HTTP client с timeout 10s

**Метод `Embed(ctx, texts)`:**
- POST `https://api.openai.com/v1/embeddings`
- Headers: `Authorization: Bearer {apiKey}`, `Content-Type: application/json`
- Body:
```json
{
  "model": "text-embedding-3-small",
  "input": ["text1", "text2"],
  "dimensions": 384
}
```
- Response: parse `data[].embedding` → `[][]float32`
- Error: HTTP status != 200 → return error с телом ответа

**Response struct (internal):**
```go
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}
```

### 4. Добавить конфигурацию

**Файл:** `project/backend/internal/config/config.go`

Добавить 2 поля в `Config`:
```go
OpenAIAPIKey   string
EmbeddingModel string
```

В `Load()`:
```go
OpenAIAPIKey:   getEnv("OPENAI_API_KEY", ""),
EmbeddingModel: getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
```

Добавить метод:
```go
func (c *Config) HasEmbeddings() bool {
	return c.OpenAIAPIKey != ""
}
```

### 5. SQL миграция: pgvector + embedding column + index

**Файл:** `project/backend/internal/adapters/postgres/catalog_migrations.go`

Добавить в `RunCatalogMigrations`:
```go
migrations := []string{
	migrationCatalogSchema,
	migrationCatalogTenants,
	migrationCatalogCategories,
	migrationCatalogMasterProducts,
	migrationCatalogProducts,
	migrationCatalogIndexes,
	migrationCatalogCategorySlugUnique,
	migrationCatalogPgvector,  // NEW
}
```

Новая константа:
```go
const migrationCatalogPgvector = `
CREATE EXTENSION IF NOT EXISTS vector;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'catalog'
        AND table_name = 'master_products'
        AND column_name = 'embedding'
    ) THEN
        ALTER TABLE catalog.master_products ADD COLUMN embedding vector(384);
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_master_products_embedding
    ON catalog.master_products USING hnsw (embedding vector_cosine_ops);
`
```

### 6. Добавить VectorSearch + SeedEmbedding + ProductFilter.Attributes

**Файл:** `project/backend/internal/ports/catalog_port.go`

Добавить `Attributes` в `ProductFilter`:
```go
type ProductFilter struct {
	CategoryID   string
	CategoryName string            // NEW: category name/slug for ILIKE matching (agent passes name, not UUID)
	Brand        string
	MinPrice     int
	MaxPrice     int
	Search       string
	SortField    string
	SortOrder    string
	Limit        int
	Offset       int
	Attributes   map[string]string // NEW: JSONB attribute filters (key → ILIKE value)
}
```

Добавить методы в `CatalogPort`:
```go
type CatalogPort interface {
	// ... existing methods ...

	// VectorSearch finds products by semantic similarity via pgvector.
	VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int) ([]domain.Product, error)

	// SeedEmbedding saves embedding for a master product.
	SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error

	// GetMasterProductsWithoutEmbedding returns master products that need embeddings.
	GetMasterProductsWithoutEmbedding(ctx context.Context) ([]domain.MasterProduct, error)
}
```

### 7. Реализовать в Postgres адаптере

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

**7a. JSONB filtering в `ListProducts`:**

Добавить после существующих conditions (после `filter.Search` блока, ~строка 201):

**CategoryName ILIKE (NEW — agent передаёт имя, не UUID):**
```go
if filter.CategoryName != "" {
	conditions = append(conditions, fmt.Sprintf("(c.name ILIKE $%d OR c.slug ILIKE $%d)", argNum, argNum))
	args = append(args, "%"+filter.CategoryName+"%")
	argNum++
}
```

**JSONB attribute filters (ILIKE для fuzzy matching):**
```go
for key, value := range filter.Attributes {
	conditions = append(conditions, fmt.Sprintf("mp.attributes->>$%d ILIKE $%d", argNum, argNum+1))
	args = append(args, key, "%"+value+"%")
	argNum += 2
}
```

Оба параметра (key и value) параметризованы — SQL injection невозможен.
ILIKE вместо exact match — подстраховка если агент напишет "black" а в БД "Black".

**7b. VectorSearch:**
```go
func (a *CatalogAdapter) VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int) ([]domain.Product, error)
```

SQL:
```sql
SELECT
    p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
    COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
    p.price, p.currency, p.stock_quantity, COALESCE(p.rating, 0) as rating, COALESCE(p.images, '[]') as images,
    mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
    mp.brand, mp.category_id, mp.images as mp_images, mp.attributes,
    c.name as category_name
FROM catalog.products p
JOIN catalog.master_products mp ON p.master_product_id = mp.id
LEFT JOIN catalog.categories c ON mp.category_id = c.id
WHERE p.tenant_id = $1
  AND mp.embedding IS NOT NULL
ORDER BY mp.embedding <=> $2
LIMIT $3
```

Использовать `pgvector.NewVector(embedding)` для `$2`:
```go
import pgvector "github.com/pgvector/pgvector-go"
args := []interface{}{tenantID, pgvector.NewVector(embedding), limit}
```

Scan и merge логику **СКОПИРОВАТЬ из `ListProducts`** — тот же набор полей, та же merge-логика master → product.

**7c. SeedEmbedding:**
```go
func (a *CatalogAdapter) SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error
```
SQL: `UPDATE catalog.master_products SET embedding = $2 WHERE id = $1`
Использовать `pgvector.NewVector(embedding)` для `$2`.

**7d. GetMasterProductsWithoutEmbedding:**
```go
func (a *CatalogAdapter) GetMasterProductsWithoutEmbedding(ctx context.Context) ([]domain.MasterProduct, error)
```
SQL:
```sql
SELECT id, sku, name, COALESCE(description, '') as description, COALESCE(brand, '') as brand,
       COALESCE(category_id::text, '') as category_id
FROM catalog.master_products
WHERE embedding IS NULL
ORDER BY created_at
```

### 8. Удалить normalizer

**Удалить файл:** `project/backend/internal/tools/normalizer.go`
**Удалить файл:** `project/backend/internal/prompts/prompt_normalize_query.go`

### 9. Переписать CatalogSearchTool — hybrid search

**Файл:** `project/backend/internal/tools/tool_catalog_search.go`

**Imports (полный список для переписанного файла):**
```go
import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)
```

**9a. Struct:**
```go
type CatalogSearchTool struct {
	statePort   ports.StatePort
	catalogPort ports.CatalogPort
	embedding   ports.EmbeddingPort // nil = keyword-only mode
}
```

**9b. Конструктор:**
```go
func NewCatalogSearchTool(statePort ports.StatePort, catalogPort ports.CatalogPort, embedding ports.EmbeddingPort) *CatalogSearchTool {
	return &CatalogSearchTool{
		statePort:   statePort,
		catalogPort: catalogPort,
		embedding:   embedding,
	}
}
```

**9c. Definition() — статическое, поля + типы (без enum-ов):**

Agent видит доступные фильтры с типами. Haiku знает мировые бренды, цвета и т.д. ILIKE + vector search подстрахуют если agent напишет чуть не так.

```go
func (t *CatalogSearchTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "catalog_search",
		Description: "Hybrid product search. Put structured/exact filters in 'filters'. Put semantic search intent in 'vector_query' in user's original language.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"vector_query": map[string]interface{}{
					"type":        "string",
					"description": "Semantic search in user's original language. Vector search handles multilingual matching. Example: 'кроссы для бега', 'lightweight laptop for work'.",
				},
				"filters": map[string]interface{}{
					"type":        "object",
					"description": "Exact keyword filters. Only include filters you're confident about.",
					"properties": map[string]interface{}{
						"brand": map[string]interface{}{
							"type":        "string",
							"description": "Brand name in English (e.g. Nike, Samsung, Apple)",
						},
						"category": map[string]interface{}{
							"type":        "string",
							"description": "Product category (e.g. Sneakers, Laptops, Headphones)",
						},
						"min_price": map[string]interface{}{
							"type":        "number",
							"description": "Minimum price in RUBLES",
						},
						"max_price": map[string]interface{}{
							"type":        "number",
							"description": "Maximum price in RUBLES",
						},
						"color": map[string]interface{}{
							"type":        "string",
							"description": "Product color in English (e.g. Black, White, Blue)",
						},
						"material": map[string]interface{}{
							"type":        "string",
							"description": "Material (e.g. Leather, Mesh, Fleece)",
						},
						"storage": map[string]interface{}{
							"type":        "string",
							"description": "Storage capacity (e.g. 128GB, 256GB, 512GB)",
						},
						"ram": map[string]interface{}{
							"type":        "string",
							"description": "RAM size (e.g. 8GB, 16GB)",
						},
						"size": map[string]interface{}{
							"type":        "string",
							"description": "Size (e.g. 11 inch, 44mm)",
						},
					},
				},
				"sort_by": map[string]interface{}{
					"type": "string",
					"enum": []string{"price", "rating", "name"},
				},
				"sort_order": map[string]interface{}{
					"type": "string",
					"enum": []string{"asc", "desc"},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max results (default 10)",
				},
			},
			"required": []string{"vector_query"},
		},
	}
}
```

**9d. Execute() — новый flow:**

1. **Parse input** — новая структура с `filters` и `vector_query`:
```go
vectorQuery, _ := input["vector_query"].(string)
sortBy, _ := input["sort_by"].(string)
sortOrder, _ := input["sort_order"].(string)
limit := 10
if v, ok := input["limit"].(float64); ok {
	limit = int(v)
}

// Parse filters object
var brand, category string
var minPrice, maxPrice int
attributes := make(map[string]string)

if filters, ok := input["filters"].(map[string]interface{}); ok {
	brand, _ = filters["brand"].(string)
	category, _ = filters["category"].(string)
	if v, ok := filters["min_price"].(float64); ok {
		minPrice = int(v)
	}
	if v, ok := filters["max_price"].(float64); ok {
		maxPrice = int(v)
	}
	// Collect JSONB attributes (everything that's not a known column filter)
	knownFilters := map[string]bool{"brand": true, "category": true, "min_price": true, "max_price": true}
	for key, val := range filters {
		if !knownFilters[key] {
			if strVal, ok := val.(string); ok {
				attributes[key] = strVal
			}
		}
	}
}
```

2. **Price conversion** (рубли → копейки ×100)

3. **Generate query embedding:**
```go
var queryEmbedding []float32
if t.embedding != nil && vectorQuery != "" {
	embedStart := time.Now()
	searchText := vectorQuery
	if brand != "" {
		searchText = vectorQuery + " " + brand
	}
	embeddings, err := t.embedding.Embed(ctx, []string{searchText})
	if err == nil && len(embeddings) > 0 {
		queryEmbedding = embeddings[0]
	}
	meta["embed_ms"] = time.Since(embedStart).Milliseconds()
}
```

4. **Keyword search:**
```go
filter := ports.ProductFilter{
	Search:     vectorQuery, // also used for ILIKE fallback
	Brand:      brand,
	CategoryName: category, // agent passes category name/slug → ILIKE on c.name/c.slug
	MinPrice:   minPriceKopecks,
	MaxPrice:   maxPriceKopecks,
	SortField:  sortBy,
	SortOrder:  sortOrder,
	Limit:      limit * 2,
	Attributes: attributes, // JSONB filters
}

// Strip brand from ILIKE search to avoid AND conflict
if filter.Brand != "" && filter.Search != "" {
	cleaned := strings.TrimSpace(removeSubstringIgnoreCase(filter.Search, filter.Brand))
	if cleaned != "" {
		filter.Search = cleaned
	}
}

keywordProducts, _, _ := t.catalogPort.ListProducts(ctx, tenant.ID, filter)
```

5. **Vector search:**
```go
var vectorProducts []domain.Product
if queryEmbedding != nil {
	vectorProducts, _ = t.catalogPort.VectorSearch(ctx, tenant.ID, queryEmbedding, limit*2)
}
meta["keyword_count"] = len(keywordProducts)
meta["vector_count"] = len(vectorProducts)
```

6. **RRF merge:**
```go
merged := rrfMerge(keywordProducts, vectorProducts, limit)
total := len(merged)
meta["merged_count"] = total
if len(keywordProducts) > 0 && len(vectorProducts) > 0 {
	meta["search_type"] = "hybrid"
} else if len(vectorProducts) > 0 {
	meta["search_type"] = "vector"
} else {
	meta["search_type"] = "keyword"
}
```

7. **State write** — без изменений по логике, `products = merged`

8. **Empty** — если merged пуст → "empty: 0 results"

**9e. Удалить:** `searchWithFallback()`, всё связанное с normalizer.

**9f. Новая функция `rrfMerge`** (в том же файле):
```go
func rrfMerge(keyword, vector []domain.Product, limit int) []domain.Product {
	const k = 60
	scores := make(map[string]float64)
	products := make(map[string]domain.Product)

	for rank, p := range keyword {
		scores[p.ID] += 1.0 / float64(k+rank+1)
		products[p.ID] = p
	}
	for rank, p := range vector {
		scores[p.ID] += 1.0 / float64(k+rank+1)
		if _, exists := products[p.ID]; !exists {
			products[p.ID] = p
		}
	}

	type scored struct {
		id    string
		score float64
	}
	var sorted []scored
	for id, score := range scores {
		sorted = append(sorted, scored{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	var result []domain.Product
	for i, s := range sorted {
		if i >= limit {
			break
		}
		result = append(result, products[s.id])
	}
	return result
}
```

### 10. Обновить промпт Agent1

**Файл:** `project/backend/internal/prompts/prompt_analyze_query.go`

Промпт объясняет split: filters (структурированное) vs vector_query (семантика).

**Заменить** `Agent1SystemPrompt` на:
```go
const Agent1SystemPrompt = `You are Agent 1 - a data retrieval agent for an e-commerce chat.

Your job: call catalog_search when user needs NEW data. If the user is asking about STYLE or DISPLAY (not new data), do nothing.

Rules:
1. If user asks for products/services → call catalog_search
2. catalog_search has two inputs:
   - filters: structured keyword filters. Write values in English.
     Brand, category, color, material etc. — translate to English.
     "Найк" → brand: "Nike". "чёрный" → color: "Black".
   - vector_query: semantic search in user's ORIGINAL language. Do NOT translate.
     This handles multilingual matching automatically via embeddings.
3. Put everything you can match exactly into filters. Put the search intent into vector_query.
4. Prices are in RUBLES. "дешевле 10000" → filters.max_price: 10000
5. If user asks to CHANGE DISPLAY STYLE → DO NOT call any tool. Just stop.
6. Do NOT explain what you're doing.
7. Do NOT ask clarifying questions - make best guess.
8. After getting "ok"/"empty", stop. Do not call more tools.

Examples:
- "покажи кроссы Найк" → catalog_search(vector_query="кроссы", filters={brand:"Nike"})
- "чёрные худи Adidas" → catalog_search(vector_query="худи", filters={brand:"Adidas", color:"Black"})
- "дешевые телефоны Samsung" → catalog_search(vector_query="телефоны", filters={brand:"Samsung"}, sort_by="price", sort_order="asc")
- "ноутбуки дешевле 50000" → catalog_search(vector_query="ноутбуки", filters={max_price:50000})
- "что-нибудь для бега" → catalog_search(vector_query="что-нибудь для бега")
- "TWS наушники с шумодавом" → catalog_search(vector_query="наушники с шумодавом", filters={type:"TWS", anc:"true"})
- "покажи с большими заголовками" → DO NOT call tool (style request)
`
```

**Удалить из файла:** `NormalizeQueryPrompt`, `BuildNormalizeRequest`, все legacy промпты.

### 11. Обновить Registry

**Файл:** `project/backend/internal/tools/tool_registry.go`

```go
type Registry struct {
	tools          map[string]ToolExecutor
	statePort      ports.StatePort
	catalogPort    ports.CatalogPort
	presetRegistry *presets.PresetRegistry
	embeddingPort  ports.EmbeddingPort // was: llmPort
}

func NewRegistry(statePort ports.StatePort, catalogPort ports.CatalogPort, presetRegistry *presets.PresetRegistry, embeddingPort ports.EmbeddingPort) *Registry {
	r := &Registry{
		tools:          make(map[string]ToolExecutor),
		statePort:      statePort,
		catalogPort:    catalogPort,
		presetRegistry: presetRegistry,
		embeddingPort:  embeddingPort,
	}

	// Data tools (Agent1)
	r.Register(NewCatalogSearchTool(statePort, catalogPort, embeddingPort))

	// Render tools (Agent2)
	r.Register(NewRenderProductPresetTool(statePort, presetRegistry))
	r.Register(NewRenderServicePresetTool(statePort, presetRegistry))
	r.Register(NewFreestyleTool(statePort))

	return r
}
```

### 12. Обновить ВСЕ call sites NewRegistry

**Breaking change:** `NewRegistry()` теперь принимает `EmbeddingPort` вместо `LLMPort`.

**Call site 1:** `project/backend/cmd/server/main.go` (~строка 124)
```go
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, embeddingClient)
```

**Call site 2:** `project/backend/internal/usecases/agent1_execute_test.go` (~строка 71)
```go
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, nil)
```

**Call site 3:** `project/backend/internal/usecases/cache_test.go` (~строка 76)
```go
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, nil)
```

В тестах embedding=nil → keyword-only. Тесты не ломаются.

### 13. Обновить main.go — wiring

**Файл:** `project/backend/cmd/server/main.go`

**13a. Import:**
```go
openaiAdapter "keepstar/internal/adapters/openai"
```

**13b. Создать embedding client** (после `llmClient`, ~строка 92):
```go
var embeddingClient ports.EmbeddingPort
if cfg.HasEmbeddings() {
	embeddingClient = openaiAdapter.NewEmbeddingClient(cfg.OpenAIAPIKey, cfg.EmbeddingModel, 384)
	appLog.Info("embedding_client_initialized", "model", cfg.EmbeddingModel, "dims", 384)
}
```

**13c. Startup embed** (после seed):
```go
if embeddingClient != nil && dbClient != nil {
	go func() {
		embedCtx, embedCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer embedCancel()
		catalogForEmbed := postgres.NewCatalogAdapter(dbClient)
		if err := runEmbedding(embedCtx, catalogForEmbed, embeddingClient, appLog); err != nil {
			appLog.Error("embedding_failed", "error", err)
		}
	}()
}
```

**13d. Функция `runEmbedding`** (в том же файле, внизу):
```go
func runEmbedding(ctx context.Context, catalog *postgres.CatalogAdapter, emb ports.EmbeddingPort, log *logger.Logger) error {
	products, err := catalog.GetMasterProductsWithoutEmbedding(ctx)
	if err != nil {
		return fmt.Errorf("get products without embedding: %w", err)
	}
	if len(products) == 0 {
		log.Info("embedding_skipped", "reason", "all products have embeddings")
		return nil
	}

	log.Info("embedding_started", "count", len(products))

	texts := make([]string, len(products))
	for i, p := range products {
		text := p.Name
		if p.Description != "" {
			text += " " + p.Description
		}
		if p.Brand != "" {
			text += " " + p.Brand
		}
		texts[i] = text
	}

	batchSize := 100
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		embeddings, err := emb.Embed(ctx, texts[i:end])
		if err != nil {
			return fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}

		for j, embedding := range embeddings {
			if err := catalog.SeedEmbedding(ctx, products[i+j].ID, embedding); err != nil {
				return fmt.Errorf("save embedding for %s: %w", products[i+j].ID, err)
			}
		}

		log.Info("embedding_progress", "done", end, "total", len(products))
	}

	log.Info("embedding_completed", "count", len(products))
	return nil
}
```

**13e. NewRegistry** (~строка 124):
```go
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, embeddingClient)
```

**13f. Admin endpoint** (после debug routes):
```go
if embeddingClient != nil && dbClient != nil {
	mux.HandleFunc("/admin/reindex-embeddings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		catalogForEmbed := postgres.NewCatalogAdapter(dbClient)
		go func() {
			embedCtx, embedCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer embedCancel()
			if err := runEmbedding(embedCtx, catalogForEmbed, embeddingClient, appLog); err != nil {
				appLog.Error("reindex_embedding_failed", "error", err)
			}
		}()
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, "Embedding reindex started in background")
	})
	appLog.Info("admin_reindex_route_enabled", "url", "POST /admin/reindex-embeddings")
}
```

### 14. Validation

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
```

## Acceptance Criteria

- [ ] Tool input: явный split `vector_query` + `filters` object (поля + типы, без enum-ов)
- [ ] Agent1 промпт: объясняет filters (English) vs vector_query (user language)
- [ ] `normalizer.go` и `prompt_normalize_query.go` удалены
- [ ] `ProductFilter.Attributes` — JSONB фильтрация в SQL (`mp.attributes->>$N = $M`)
- [ ] `EmbeddingPort` + OpenAI adapter (net/http, dims=384)
- [ ] pgvector: extension + column vector(384) + HNSW index на master_products
- [ ] `CatalogPort`: VectorSearch, SeedEmbedding, GetMasterProductsWithoutEmbedding
- [ ] Hybrid search: keyword SQL + vector pgvector + RRF merge (k=60)
- [ ] Graceful degradation: nil embedding → keyword-only
- [ ] Startup embed + POST `/admin/reindex-embeddings`
- [ ] Все call sites NewRegistry обновлены
- [ ] `go build && go test && npm run build` — OK

## Hexagonal Architecture Compliance

- **Domain** — без изменений
- **Ports** — `EmbeddingPort` (new), `CatalogPort` += 3 метода, `ProductFilter` += Attributes
- **Adapters** — `adapters/openai/` (new), `postgres_catalog.go` += 3 метода + JSONB filtering, migration += 1 const
- **Tools** — `tool_catalog_search.go` переписан (static def + hybrid), `normalizer.go` удалён
- **Prompts** — `prompt_analyze_query.go` обновлён, `prompt_normalize_query.go` удалён
- **Usecases** — без изменений
- **main.go** — wiring + startup embed + admin endpoint
