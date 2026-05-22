package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DriftClassifierClient is a one-shot LLM client for schema-drift
// classification. Given the current mapping artifact + a list of
// unmapped inbox fields (with type guess + stats), returns a
// classification per field: typo of an existing key, alias of an
// existing attribute, or genuinely new — with a suggested action
// the curator can apply via patch-discovery (B1).
//
// Default model is Sonnet 4.6 (cfg.DriftClassifierModel) for accuracy
// on borderline alias/new decisions.
type DriftClassifierClient struct {
	apiKey string
	model  string
	client *http.Client
}

func NewDriftClassifierClient(apiKey, model string) *DriftClassifierClient {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &DriftClassifierClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *DriftClassifierClient) Model() string { return c.model }

// DriftField is one unmapped field's evidence pack sent to the LLM.
type DriftField struct {
	Name          string   `json:"name"`
	TypeGuess     string   `json:"type_guess"`
	NonNullCount  int      `json:"non_null_count"`
	TotalCount    int      `json:"total_count"`
	DistinctCount int      `json:"distinct_count"`
	MinNumeric    *float64 `json:"min_numeric,omitempty"`
	MaxNumeric    *float64 `json:"max_numeric,omitempty"`
	MinLength     *int     `json:"min_length,omitempty"`
	MaxLength     *int     `json:"max_length,omitempty"`
	SampleValues  []string `json:"sample_values"`
}

// DriftArtifactSummary is the slim view of the current artifact that
// the LLM uses as context for "what's already mapped".
type DriftArtifactSummary struct {
	ClassifyingField string                  `json:"classifying_field"`
	Branches         []DriftArtifactBranch   `json:"branches"`
	ClassifyRules    []DriftClassifyRuleView `json:"classify_rules,omitempty"`
}

type DriftArtifactBranch struct {
	Vertical string                  `json:"vertical"`
	FieldMap []DriftFieldMappingView `json:"field_map"`
}

type DriftFieldMappingView struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DriftClassifyRuleView struct {
	When         string `json:"when"`
	ThenVertical string `json:"then_vertical"`
}

// DriftClassification is one row of the LLM's response.
type DriftClassification struct {
	Field           string          `json:"field"`
	Decision        string          `json:"decision"` // "typo_of_existing" | "alias_of_attribute" | "new"
	Confidence      float64         `json:"confidence"`
	SuggestedAction json.RawMessage `json:"suggested_action"`
}

// DriftClassifyResult bundles the parsed response + token accounting
// for cost tracking.
type DriftClassifyResult struct {
	Classifications []DriftClassification
	InputTokens     int
	OutputTokens    int
}

// Classify sends one batched LLM call covering all unmapped fields.
func (c *DriftClassifierClient) Classify(ctx context.Context, art *DriftArtifactSummary, fields []DriftField) (*DriftClassifyResult, error) {
	if len(fields) == 0 {
		return &DriftClassifyResult{}, nil
	}

	userPrompt, err := buildDriftPrompt(art, fields)
	if err != nil {
		return nil, fmt.Errorf("build drift prompt: %w", err)
	}

	reqBody := messagesRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    driftSystemPrompt,
		Messages:  []message{{Role: "user", Content: userPrompt}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic drift API error (status %d): %s", resp.StatusCode, string(respBody))
	}
	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("empty drift response")
	}
	text := extractJSON(msgResp.Content[0].Text)

	var out []DriftClassification
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("parse drift JSON: %w (raw: %.500s)", err, text)
	}

	// Filter out any classifications for fields we didn't ask about
	// (defensive against hallucinations).
	known := make(map[string]bool, len(fields))
	for _, f := range fields {
		known[f.Name] = true
	}
	filtered := out[:0]
	for _, r := range out {
		if known[r.Field] && isValidDecision(r.Decision) {
			filtered = append(filtered, r)
		}
	}
	return &DriftClassifyResult{
		Classifications: filtered,
		InputTokens:     msgResp.Usage.InputTokens,
		OutputTokens:    msgResp.Usage.OutputTokens,
	}, nil
}

func isValidDecision(d string) bool {
	switch d {
	case "typo_of_existing", "alias_of_attribute", "new":
		return true
	}
	return false
}

func buildDriftPrompt(art *DriftArtifactSummary, fields []DriftField) (string, error) {
	artJSON, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal artifact: %w", err)
	}
	fieldsJSON, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal fields: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("Current mapping artifact:\n```json\n")
	sb.Write(artJSON)
	sb.WriteString("\n```\n\n")
	sb.WriteString("New unmapped inbox fields, with statistics:\n```json\n")
	sb.Write(fieldsJSON)
	sb.WriteString("\n```\n\n")
	sb.WriteString("For each new field, output one object in a JSON array. ")
	sb.WriteString("Return ONLY the JSON array, no markdown fence, no commentary.")
	return sb.String(), nil
}

const driftSystemPrompt = `You are a PIM schema-drift classifier. Your job is to look at a CURRENT mapping artifact (what's already mapped from a tenant's incoming data into a master catalog schema) plus a list of NEW unmapped fields appearing in this tenant's data, and decide for each new field:

- "typo_of_existing": the field name is a likely typo / variant of a field already present in artifact.branches[].field_map[].from. Suggest renaming.
- "alias_of_attribute": semantically the same as an already-mapped attribute under a different name. Suggest mapping it to the same target column.
- "new": legitimately new attribute, not previously seen. Suggest where it should land (typically a tier3.<key> bucket on master_products, sometimes a typed master_cosmetics column if a cosmetics-vertical attribute, rarely a universal master.<col> column).

## Mapping targets you can suggest

- master.<col>   — universal columns on master_products: name, brand, description, image_url, gtin
- cosmetics.<col> — typed cosmetics columns (only valid when the branch vertical is "cosmetics"): product_form, texture, routine_step, routine_time, application_method, scent, marketing_claim, how_to_use, spf, volume_ml, weight_g, unit_count, skin_type[], concern[], key_ingredients[], target_area[], free_from[], benefits[]
- tier3.<key>   — JSONB pocket for everything else. Always a safe fallback.

## Decision rubric

- If field_name has Levenshtein distance ≤ 2 from any existing branches[].field_map[].from → "typo_of_existing", confidence high.
- If field_name's sample values overlap heavily with a known categorical column (e.g. "primary_cat" with cosmetic categories) → "alias_of_attribute".
- If sample values look numeric and stats indicate a metric (e.g. ratings, prices) and no existing field has same purpose → "new" with tier3.<key>.
- Default to "new" + tier3 when unsure.

## suggested_action format

Always include "kind":
- For "typo_of_existing": {"kind":"rename_field","vertical":"<v>","from":"<new_name>","to":"<existing_field_map.to>"}
- For "alias_of_attribute": {"kind":"add_field_mapping","vertical":"<v>","from":"<new_name>","to":"<existing_target>","transform":"<optional>"}
- For "new": {"kind":"add_field_mapping","vertical":"<v>","from":"<new_name>","to":"tier3.<snake_case_key>","transform":"<optional>"}

Confidence is 0.0-1.0. Be honest — < 0.6 means "owner should check".

## Output format

Return ONLY a JSON array (no markdown fence, no preamble):
[
  {"field":"...", "decision":"...", "confidence":0.95, "suggested_action":{...}}
]`
