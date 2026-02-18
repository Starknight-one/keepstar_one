package domain

import "time"

// EnrichmentInput is the data sent to the LLM for product enrichment.
type EnrichmentInput struct {
	SKU               string
	Name              string
	Brand             string
	Description       string
	Ingredients       string
	ActiveIngredients string
	SkinType          string
	Benefits          string
	HowToUse          string
}

// EnrichmentOutput is the enriched data returned by the LLM (v1).
type EnrichmentOutput struct {
	SKU            string   `json:"sku"`
	CategorySlug   string   `json:"category_slug"`
	ProductForm    string   `json:"product_form"`
	SkinType       []string `json:"skin_type"`
	Concern        []string `json:"concern"`
	KeyIngredients []string `json:"key_ingredients"`
}

// EnrichmentOutputV2 is the enriched data returned by the v2 PIM prompt.
type EnrichmentOutputV2 struct {
	SKU               string   `json:"sku"`
	ShortName         string   `json:"short_name"`
	OriginalName      string   `json:"original_name"`
	ProductLine       string   `json:"product_line"`
	CategorySlug      string   `json:"category_slug"`
	ProductForm       string   `json:"product_form"`
	Texture           string   `json:"texture"`
	SkinType          []string `json:"skin_type"`
	Concern           []string `json:"concern"`
	KeyIngredients    []string `json:"key_ingredients"`
	TargetArea        []string `json:"target_area"`
	FreeFrom          []string `json:"free_from"`
	RoutineStep       string   `json:"routine_step"`
	RoutineTime       string   `json:"routine_time"`
	ApplicationMethod string   `json:"application_method"`
	MarketingClaim    string   `json:"marketing_claim"`
	Benefits          []string `json:"benefits"`
	Volume            string   `json:"volume"`
}

// EnrichmentResultV2 is the return value from one LLM v2 batch call.
type EnrichmentResultV2 struct {
	Outputs      []EnrichmentOutputV2
	InputTokens  int
	OutputTokens int
}

// EnrichmentResult is the return value from one LLM batch call.
type EnrichmentResult struct {
	Outputs     []EnrichmentOutput
	InputTokens  int
	OutputTokens int
}

// EnrichmentJob tracks an in-flight or completed enrichment run.
type EnrichmentJob struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenantId"`
	Status          string    `json:"status"` // pending, processing, completed, failed
	TotalProducts   int       `json:"totalProducts"`
	TotalBatches    int       `json:"totalBatches"`
	ProcessedBatches int      `json:"processedBatches"`
	EnrichedProducts int      `json:"enrichedProducts"`
	ErrorCount      int       `json:"errorCount"`
	InputTokens     int       `json:"inputTokens"`
	OutputTokens    int       `json:"outputTokens"`
	EstimatedCostUSD float64  `json:"estimatedCostUsd"`
	Model           string    `json:"model"`
	StartedAt       time.Time `json:"startedAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
}
