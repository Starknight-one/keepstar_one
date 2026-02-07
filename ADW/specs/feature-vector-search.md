# Feature: Векторный поиск (Hybrid Search)

## ADW ID: feature-vector-search

## Feature Description

Добавление векторного поиска в мета-тул `catalog_search` через OpenAI Embeddings API + pgvector в Neon Postgres. Мультиязычные эмбеддинги решают проблему перевода (кроссовки ↔ sneakers) без LLM-нормализатора. Hybrid search (keyword SQL + vector pgvector + RRF merge) даёт лучшее качество для любых запросов: от точных ("Nike Air Max") до семантических ("самые топовые ноутбуки для работы").

**Ключевое преимущество архитектуры:** замена OpenAI на собственный embedding-сервис в будущем = создать 1 файл-адаптер + изменить 1 строку в main.go. Порт, vector SQL, мета-тул — всё остаётся.

## Objective

После реализации:
- "покажи топовые ноутбуки" → embedding → vector search → релевантные ноутбуки по семантике
- "кроссы Найк" → keyword (brand=Nike) + vector ("кроссы") → точные + семантические результаты, RRF merge
- "что-нибудь для бега" → vector search ловит running shoes, sportswear — то что ILIKE никогда не найдёт
- "дешёвые наушники Sony" → keyword (brand=Sony, max_price) + vector ("наушники") → фильтры + семантика
- LLM-нормализатор (Haiku, ~300ms, $$) заменён на статическую brand-карту (0ms, бесплатно)
- Эмбеддинги генерятся через OpenAI API (~50ms на запрос), хранятся в pgvector
- Swap на свой сервис = 1 файл + 1 строка в main.go

## Expertise Context

Expertise used:
- **backend-ports**: `CatalogPort` интерфейс с `ListProducts(ctx, tenantID, filter)`, `ProductFilter` struct. Нужно добавить `VectorSearch` метод. `EmbeddingPort` — новый порт.
- **backend-adapters**: PostgreSQL через pgx/v5. Migrations в отдельных const-строках. Каталог в schema `catalog`. Master products содержат semantic content (name, description, brand). Products = tenant-specific (price, stock).
- **backend-pipeline**: `CatalogSearchTool` мета-тул в `tools/`. Текущий normalizer использует LLM — заменяем на static map + embeddings. Registry создаёт тулы в `NewRegistry()`.
- **backend-domain**: `Product` entity без embedding-поля (embedding на master_products в БД, не в domain). Price в копейках.
- **backend-usecases**: Agent1 фильтрует тулы по `catalog_*` префиксу. Pipeline: Agent1 → Agent2 → Formation.

## Relevant Files

### Existing Files (модифицируются)

- `project/backend/internal/ports/catalog_port.go` — добавить `VectorSearch` метод в `CatalogPort`
- `project/backend/internal/adapters/postgres/catalog_migrations.go` — миграция: pgvector extension, embedding column, HNSW index
- `project/backend/internal/adapters/postgres/postgres_catalog.go` — реализация `VectorSearch`
- `project/backend/internal/tools/tool_catalog_search.go` — hybrid search: keyword + vector + RRF
- `project/backend/internal/tools/normalizer.go` — заменить LLM нормализатор на static brand map
- `project/backend/internal/tools/tool_registry.go` — передать embeddingPort в CatalogSearchTool
- `project/backend/internal/config/config.go` — добавить `OpenAIAPIKey`, `EmbeddingModel`
- `project/backend/cmd/server/main.go` — wiring: embedding adapter, seed embeddings
- `project/backend/go.mod` — добавить `github.com/pgvector/pgvector-go`

### New Files

- `project/backend/internal/ports/embedding_port.go` — интерфейс `EmbeddingPort`
- `project/backend/internal/adapters/openai/embedding_client.go` — OpenAI Embeddings API adapter

### Read-Only (не менять)

- `project/backend/internal/domain/product_entity.go` — embedding НЕ добавляется в domain (хранится только в БД)
- `project/backend/internal/domain/master_product_entity.go` — embedding НЕ добавляется в domain
- `project/backend/internal/usecases/agent1_execute.go` — не меняется (фильтр `catalog_*` уже работает)
- `project/backend/internal/prompts/prompt_analyze_query.go` — не меняется (Agent1 промпт уже ок)

## Step by Step Tasks

IMPORTANT: Execute strictly in order.

### 1. Добавить зависимость pgvector-go

**Команда:**
```bash
cd project/backend && go get github.com/pgvector/pgvector-go
```

Это добавит `github.com/pgvector/pgvector-go` в `go.mod` и `go.sum`.

### 2. Создать EmbeddingPort

**Файл:** `project/backend/internal/ports/embedding_port.go`

```go
package ports

import "context"

// EmbeddingPort generates vector embeddings for text.
// Implementations: OpenAI API, local model server, etc.
// Swap adapter to change provider — no other code changes needed.
type EmbeddingPort interface {
	// Embed generates embeddings for one or more texts.
	// Returns one []float32 per input text.
	// Dimension depends on the configured model (e.g. 384 for text-embedding-3-small with dimensions=384).
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

**Принцип:** Минимальный интерфейс. Одна функция. Любой провайдер (OpenAI, Cohere, Voyage, self-hosted) реализует это за 1 файл.

### 3. Создать OpenAI Embedding Adapter

**Файл:** `project/backend/internal/adapters/openai/embedding_client.go`

**Struct:**
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
- Default dims: `384` (OpenAI поддерживает dimension reduction — экономит storage и compute)
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
- Batch: OpenAI принимает до 2048 inputs за раз. Для seed 130 товаров = 1 вызов.
- Error handling: HTTP status != 200 → return error с телом ответа для дебага

**CRITICAL: НЕ использовать внешние OpenAI SDK.** Простой HTTP клиент через `net/http` + `encoding/json`. Никаких лишних зависимостей. Проект уже делает HTTP вызовы к Anthropic таким же образом (см. `adapters/anthropic/anthropic_client.go`).

**Response struct (internal):**
```go
type embeddingResponse struct {
	Data  []struct {
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

Добавить новую миграцию в `RunCatalogMigrations`:
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

**Почему master_products, а не products:**
- Semantic content (name, description, brand) живёт в master_products
- Один embedding на master product, расшарен между тенантами
- Products = tenant-specific (price, stock) — не семантика

**Почему HNSW, а не IVFFlat:**
- HNSW работает сразу без training step
- Хорошо от малых (130) до больших (миллионы) датасетов
- IVFFlat требует `lists` параметр и training — лишняя сложность

**Почему 384 dimensions:**
- OpenAI `text-embedding-3-small` поддерживает dimension reduction
- 384 dims vs 1536 dims: ~4x меньше storage, ~4x быстрее search, quality drop минимальный
- pgvector индекс меньше и быстрее

### 6. Добавить VectorSearch в CatalogPort

**Файл:** `project/backend/internal/ports/catalog_port.go`

Добавить метод в интерфейс `CatalogPort`:
```go
type CatalogPort interface {
	// ... existing methods ...

	// VectorSearch finds products by semantic similarity via pgvector.
	// embedding is the query vector (384 dims).
	// Returns products sorted by similarity (most similar first).
	VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int) ([]domain.Product, error)

	// SeedEmbedding saves embedding for a master product.
	SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error

	// GetMasterProductsWithoutEmbedding returns master products that need embeddings.
	GetMasterProductsWithoutEmbedding(ctx context.Context) ([]domain.MasterProduct, error)
}
```

### 7. Реализовать VectorSearch в Postgres адаптере

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

Добавить 3 метода в `CatalogAdapter`:

**7a. VectorSearch:**
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
    c.name as category_name,
    1 - (mp.embedding <=> $2) as similarity
FROM catalog.products p
JOIN catalog.master_products mp ON p.master_product_id = mp.id
LEFT JOIN catalog.categories c ON mp.category_id = c.id
WHERE p.tenant_id = $1
  AND mp.embedding IS NOT NULL
ORDER BY mp.embedding <=> $2
LIMIT $3
```

**CRITICAL:** Использовать `pgvector.NewVector(embedding)` из `github.com/pgvector/pgvector-go` для передачи вектора как параметра `$2`. Пример:
```go
import pgvector "github.com/pgvector/pgvector-go"
// ...
args := []interface{}{tenantID, pgvector.NewVector(embedding), limit}
```

Продукт-мерж логика (master → product) — **СКОПИРОВАТЬ** из существующего `ListProducts`. Не вызывать ListProducts (разный SQL). Использовать ту же scan-логику.

**7b. SeedEmbedding:**
```go
func (a *CatalogAdapter) SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error
```

SQL:
```sql
UPDATE catalog.master_products SET embedding = $2 WHERE id = $1
```

Использовать `pgvector.NewVector(embedding)` для `$2`.

**7c. GetMasterProductsWithoutEmbedding:**
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

Возвращает только поля нужные для генерации текста эмбеддинга. Images/attributes не нужны.

### 8. Заменить LLM нормализатор на статическую brand-карту

**Файл:** `project/backend/internal/tools/normalizer.go`

**Полная замена содержимого.** LLM-нормализатор больше не нужен — мультиязычные эмбеддинги решают проблему перевода.

Новое содержимое:
```go
package tools

import "strings"

// BrandNormalizer resolves brand transliterations via static map.
// No LLM call needed — deterministic, 0ms, free.
type BrandNormalizer struct {
	brands map[string]string // lowercase alias → canonical name
}

// NewBrandNormalizer creates a normalizer with built-in brand aliases.
func NewBrandNormalizer() *BrandNormalizer {
	n := &BrandNormalizer{brands: make(map[string]string)}
	// Russian transliterations
	aliases := map[string][]string{
		"Nike":           {"найк", "найки", "найке", "нике"},
		"Adidas":         {"адидас", "адик", "адики"},
		"Puma":           {"пума"},
		"Reebok":         {"рибок", "рибоки"},
		"Samsung":        {"самсунг", "самс"},
		"Apple":          {"эпл", "апл", "эппл", "аппл"},
		"Sony":           {"сони"},
		"Lenovo":         {"леново"},
		"Dell":           {"делл"},
		"Levi's":         {"левайс", "левис", "леви"},
		"The North Face": {"норс фейс", "зе норс фейс", "тнф"},
		"Google":         {"гугл"},
	}
	for canonical, list := range aliases {
		n.brands[strings.ToLower(canonical)] = canonical
		for _, alias := range list {
			n.brands[strings.ToLower(alias)] = canonical
		}
	}
	return n
}

// NormalizeBrand resolves a brand alias to its canonical English name.
// Returns the original string if no alias found (passthrough).
func (n *BrandNormalizer) NormalizeBrand(brand string) string {
	if brand == "" {
		return ""
	}
	if canonical, ok := n.brands[strings.ToLower(strings.TrimSpace(brand))]; ok {
		return canonical
	}
	return brand
}
```

**Удалить:**
- `QueryNormalizer` struct
- `NewQueryNormalizer(llm)` constructor
- `Normalize(ctx, query, brand)` method
- `NormalizeResult` struct
- `isASCII()` function
- `stripCodeFences()` function
- Импорт `ports` и `prompts`

**Файл `project/backend/internal/prompts/prompt_normalize_query.go`** — оставить как есть (legacy, не используется после замены). Не удалять — может понадобиться.

### 9. Обновить CatalogSearchTool — hybrid search + RRF

**Файл:** `project/backend/internal/tools/tool_catalog_search.go`

**Изменения в struct:**
```go
type CatalogSearchTool struct {
	statePort    ports.StatePort
	catalogPort  ports.CatalogPort
	embedding    ports.EmbeddingPort  // NEW: replaces normalizer
	brandNorm    *BrandNormalizer     // NEW: replaces QueryNormalizer
}
```

**Конструктор:**
```go
func NewCatalogSearchTool(statePort ports.StatePort, catalogPort ports.CatalogPort, embedding ports.EmbeddingPort) *CatalogSearchTool {
	return &CatalogSearchTool{
		statePort:   statePort,
		catalogPort: catalogPort,
		embedding:   embedding,
		brandNorm:   NewBrandNormalizer(),
	}
}
```

**Execute() — новый flow:**

1. **Parse input** (без изменений: query, brand, category, min_price, max_price, sort_by, sort_order, limit)

2. **Price conversion** (без изменений: рубли → копейки ×100)

3. **Brand normalize** (NEW — static map, 0ms):
```go
normalizedBrand := t.brandNorm.NormalizeBrand(brand)
```

4. **Generate query embedding** (NEW):
```go
var queryEmbedding []float32
embedStart := time.Now()
if t.embedding != nil {
	// Build search text: query + brand for better semantic context
	searchText := query
	if normalizedBrand != "" {
		searchText = query + " " + normalizedBrand
	}
	embeddings, err := t.embedding.Embed(ctx, []string{searchText})
	if err == nil && len(embeddings) > 0 {
		queryEmbedding = embeddings[0]
	}
	// If embedding fails — fallback to keyword-only search (graceful degradation)
}
embedMs := time.Since(embedStart).Milliseconds()
meta["embed_ms"] = embedMs
```

5. **Keyword search** (существующий код — build filter, `ListProducts`):
```go
filter := ports.ProductFilter{
	Search:    query,  // RAW query for ILIKE (not normalized — embeddings handle multilingual)
	Brand:     normalizedBrand,
	CategoryID: category,
	MinPrice:  minPriceKopecks,
	MaxPrice:  maxPriceKopecks,
	SortField: sortBy,
	SortOrder: sortOrder,
	Limit:     limit * 2, // fetch more for RRF merge
}
// Strip brand from search (existing logic)
keywordProducts, keywordTotal, _ := t.catalogPort.ListProducts(ctx, tenant.ID, filter)
```

6. **Vector search** (NEW):
```go
var vectorProducts []domain.Product
if queryEmbedding != nil {
	vectorProducts, _ = t.catalogPort.VectorSearch(ctx, tenant.ID, queryEmbedding, limit*2)
}
meta["keyword_count"] = len(keywordProducts)
meta["vector_count"] = len(vectorProducts)
```

7. **RRF merge** (NEW):
```go
merged := rrfMerge(keywordProducts, vectorProducts, limit)
total := len(merged)
meta["merged_count"] = total
meta["search_type"] = "hybrid"
```

8. **State write** (существующий код — без изменений по логике, но `products = merged`)

9. **Fallback**: если `merged` пуст — вернуть "empty" (существующий код)

**Удалить из Execute():**
- Вызов `t.normalizer.Normalize(ctx, query, brand)`
- Всё что связано с `normalizeResult` (Query, Brand, SourceLang, AliasResolved)
- Трейс полей normalize_path, normalize_input, normalize_output

**Новая функция `rrfMerge`** (в том же файле):
```go
// rrfMerge combines keyword and vector search results using Reciprocal Rank Fusion.
// k=60 is the standard constant that prevents high-ranked items from dominating.
func rrfMerge(keyword, vector []domain.Product, limit int) []domain.Product {
	const k = 60
	scores := make(map[string]float64)     // product ID → RRF score
	products := make(map[string]domain.Product) // product ID → product data

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

	// Sort by RRF score descending
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

**Import:** добавить `"sort"` в imports.

### 10. Обновить Registry

**Файл:** `project/backend/internal/tools/tool_registry.go`

**Изменения:**

В struct Registry — заменить `llmPort` на `embeddingPort`:
```go
type Registry struct {
	tools          map[string]ToolExecutor
	statePort      ports.StatePort
	catalogPort    ports.CatalogPort
	presetRegistry *presets.PresetRegistry
	embeddingPort  ports.EmbeddingPort // was: llmPort ports.LLMPort
}
```

В `NewRegistry` — изменить сигнатуру и тело:
```go
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

**Удалить:** создание `normalizer` в `NewRegistry()`.

### 11. Обновить ВСЕ call sites NewRegistry (3 штуки)

**Breaking change:** `NewRegistry()` теперь принимает `EmbeddingPort` вместо `LLMPort` (4-й параметр меняет тип).

**Call site 1:** `project/backend/cmd/server/main.go` (строка ~124)
```go
// Было:
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)
// Стало:
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, embeddingClient)
```
`embeddingClient` создаётся в main.go (см. шаг 12).

**Call site 2:** `project/backend/internal/usecases/agent1_execute_test.go`
```go
// Было:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)
// Стало:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, nil)
```
В тестах embedding = nil → hybrid search gracefully degrades to keyword-only. Тесты не ломаются.

**Call site 3:** `project/backend/internal/usecases/cache_test.go`
```go
// Было:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)
// Стало:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, nil)
```

### 12. Обновить main.go — wiring

**Файл:** `project/backend/cmd/server/main.go`

**12a. Добавить import:**
```go
openaiAdapter "keepstar/internal/adapters/openai"
```

**12b. Создать embedding client** (после `llmClient` создания, ~строка 92):
```go
// Initialize embedding client (if OpenAI key configured)
var embeddingClient ports.EmbeddingPort
if cfg.HasEmbeddings() {
	embeddingClient = openaiAdapter.NewEmbeddingClient(cfg.OpenAIAPIKey, cfg.EmbeddingModel, 384)
	appLog.Info("embedding_client_initialized", "model", cfg.EmbeddingModel, "dims", 384)
}
```

**12c. Seed embeddings** (после `SeedExtendedCatalog`, ~строка 85):
```go
// Seed embeddings for master products without them
if embeddingClient != nil && catalogAdapter != nil {
	go func() {
		embedCtx, embedCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer embedCancel()
		if err := seedEmbeddings(embedCtx, catalogAdapter, embeddingClient, appLog); err != nil {
			appLog.Error("embedding_seed_failed", "error", err)
		}
	}()
}
```

**12d. seedEmbeddings function** (в том же файле main.go, внизу):
```go
func seedEmbeddings(ctx context.Context, catalog *postgres.CatalogAdapter, emb ports.EmbeddingPort, log *logger.Logger) error {
	products, err := catalog.GetMasterProductsWithoutEmbedding(ctx)
	if err != nil {
		return fmt.Errorf("get products without embedding: %w", err)
	}
	if len(products) == 0 {
		log.Info("embedding_seed_skipped", "reason", "all products have embeddings")
		return nil
	}

	// Build texts for embedding
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

	// Batch embed (OpenAI supports up to 2048 per request)
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

		log.Info("embedding_seed_progress", "done", end, "total", len(products))
	}

	log.Info("embedding_seed_completed", "count", len(products))
	return nil
}
```

**12e. Заменить `llmClient` на `embeddingClient`** в вызове `NewRegistry` (~строка 124):
```go
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, embeddingClient)
```

### 13. Validation

```bash
cd project/backend && go build ./...
cd project/frontend && npm run build
```

Frontend не меняется в этой фиче.

## Validation Commands

```bash
# Backend build (required)
cd project/backend && go build ./...

# Backend tests (optional — some integration tests may need OPENAI_API_KEY)
cd project/backend && go test ./...

# Frontend build (required — verify no breakage)
cd project/frontend && npm run build
```

## Acceptance Criteria

- [ ] `EmbeddingPort` интерфейс создан: `Embed(ctx, texts) ([][]float32, error)`
- [ ] OpenAI adapter создан: POST /v1/embeddings, model=text-embedding-3-small, dims=384
- [ ] pgvector extension enabled: `CREATE EXTENSION IF NOT EXISTS vector`
- [ ] `catalog.master_products.embedding` column vector(384) создан
- [ ] HNSW индекс на embedding column создан
- [ ] `CatalogPort.VectorSearch` метод добавлен и реализован
- [ ] `CatalogPort.SeedEmbedding` метод добавлен и реализован
- [ ] `CatalogPort.GetMasterProductsWithoutEmbedding` метод добавлен и реализован
- [ ] LLM нормализатор заменён на `BrandNormalizer` (статическая карта, 0ms)
- [ ] `CatalogSearchTool` делает hybrid search: keyword SQL + vector pgvector
- [ ] RRF merge комбинирует результаты (k=60)
- [ ] При отсутствии embedding (nil EmbeddingPort) — graceful degradation на keyword-only
- [ ] Seed embeddings при старте сервера (горутина, не блокирует)
- [ ] Trace metadata содержит: embed_ms, keyword_count, vector_count, merged_count, search_type
- [ ] Все 3 call sites `NewRegistry` обновлены
- [ ] `OPENAI_API_KEY` в config, `HasEmbeddings()` метод
- [ ] `go build ./...` — OK
- [ ] `npm run build` — OK
- [ ] Тесты с `nil` EmbeddingPort не ломаются (keyword-only fallback)

## Notes

### Gotcha: Embedding на master_products, не на products
Semantic content (name, description, brand) живёт в `catalog.master_products`. Products = tenant-specific overlay (price, stock). Vector search JOIN-ит master_products → products для tenant filtering.

### Gotcha: pgvector-go и pgx/v5
`github.com/pgvector/pgvector-go` работает с pgx/v5 (уже используется). Для передачи vector как SQL parameter: `pgvector.NewVector(embedding)`.

### Gotcha: Graceful degradation
`EmbeddingPort` может быть `nil` (нет `OPENAI_API_KEY`). В этом случае:
- Vector search пропускается
- Работает только keyword search
- Тесты без API ключа не ломаются
- `NewRegistry` принимает `nil` — это ок

### Gotcha: removeSubstringIgnoreCase
Эта функция используется в текущем `tool_catalog_search.go` для strip brand из search query. Она остаётся — не является частью normalizer.

### Gotcha: Agent1 промпт НЕ меняется
Промпт уже говорит "pass text as-is" и "normalization is automatic". Vector search — деталь реализации мета-тула, Agent1 не знает про embeddings.

### Gotcha: Price/sort filters в vector search
Vector search НЕ фильтрует по цене/сорту — это делает keyword search. RRF merge комбинирует оба. Если нужна строгая фильтрация по цене — keyword search обеспечивает, vector search добавляет semantic relevance.

### Gotcha: Embedding dimension consistency
OpenAI API + pgvector column + HNSW index — все MUST use 384 dims. Если поменять dims — нужно пересоздать column и index + re-embed все товары.

### Gotcha: Seed embeddings в горутине
Seed embeddings запускается async (не блокирует старт сервера). Первые запросы до завершения seed будут keyword-only (embedding IS NULL → vector search вернёт пусто). Это ожидаемо и безопасно.

### Gotcha: OpenAI pricing
`text-embedding-3-small`: $0.02 per 1M tokens. 130 товаров seed ≈ 5000 tokens ≈ $0.0001. Каждый search query ≈ 20 tokens ≈ $0.0000004. Практически бесплатно.

### Future: Swap embedding provider
Чтобы перейти на собственный сервис:
1. Создать `adapters/local/embedding_client.go` — HTTP POST к Python сервису, implements `EmbeddingPort`
2. В `main.go` заменить: `embeddingClient = localAdapter.NewEmbeddingClient(url)` вместо `openaiAdapter.NewEmbeddingClient(...)`
3. Re-embed товары (dimensions могут отличаться → обновить column + index)
4. Всё остальное (tools, ports, SQL) — без изменений

### Hexagonal Architecture Compliance
- **Domain layer** — без изменений (embedding НЕ в domain)
- **Ports layer** — новый `EmbeddingPort` (1 файл). `CatalogPort` += 3 метода.
- **Adapters layer** — новый `adapters/openai/` (1 файл). `postgres_catalog.go` += 3 метода. Migration += 1 const.
- **Tools layer** — `tool_catalog_search.go` модифицирован (hybrid search). `normalizer.go` упрощён (static map).
- **Usecases layer** — без изменений
- **main.go** — composition root: wiring embeddingPort + seed embeddings

Стрелки зависимостей: Tools → Ports (interfaces). Adapters → Ports (implements). Domain ← все. Циклов нет.
