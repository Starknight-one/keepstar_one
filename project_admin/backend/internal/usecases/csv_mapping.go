package usecases

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"keepstar-admin/internal/logger"
)

// allowedMappingFields whitelists the only field names Agent-suggested or
// user-chosen column mappings may refer to. Anything else collapses to
// "ignore" before reaching importer code.
var allowedMappingFields = map[string]bool{
	"sku":           true,
	"name":          true,
	"brand":         true,
	"category":      true,
	"category_slug": true,
	"price":         true,
	"currency":      true,
	"stock":         true,
	"rating":        true,
	"description":   true,
	"images":        true,
	"tags":          true,
	"ignore":        true,
	// service-specific
	"duration":     true,
	"provider":     true,
	"availability": true,
}

// CSVMappingUseCase asks Haiku to propose a column-name → internal-field map
// based on the first few rows of a user-uploaded CSV. The admin UI shows the
// suggestion with confidence badges and lets the user override before
// confirming — this usecase only proposes, it never imports.
type CSVMappingUseCase struct {
	apiKey string
	model  string
	client *http.Client
	log    *logger.Logger
}

func NewCSVMappingUseCase(apiKey, model string, log *logger.Logger) *CSVMappingUseCase {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &CSVMappingUseCase{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
		log:    log,
	}
}

type MappingSuggestion struct {
	Mapping    map[string]string  `json:"mapping"`    // CSV column → internal field
	Confidence map[string]float64 `json:"confidence"` // CSV column → 0..1
}

// SuggestMapping returns a proposed mapping. Unknown fields returned by the
// LLM are rewritten to "ignore" before the map leaves this function.
func (uc *CSVMappingUseCase) SuggestMapping(ctx context.Context, headers []string, sampleRows [][]string) (*MappingSuggestion, error) {
	if len(headers) == 0 {
		return nil, fmt.Errorf("no headers provided")
	}

	// Trim row width to match header width; cap to 5 sample rows.
	safeSample := make([][]string, 0, 5)
	for i, row := range sampleRows {
		if i >= 5 {
			break
		}
		safe := make([]string, len(headers))
		for j := range headers {
			if j < len(row) {
				safe[j] = truncate(row[j], 120)
			}
		}
		safeSample = append(safeSample, safe)
	}

	prompt := buildCSVMappingPrompt(headers, safeSample)

	reqBody := map[string]any{
		"model":      uc.model,
		"max_tokens": 1024,
		"system":     csvMappingSystemPrompt,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", uc.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := uc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	text := extractJSONFromText(apiResp.Content[0].Text)

	var raw struct {
		Columns []struct {
			Name       string  `json:"name"`
			Field      string  `json:"field"`
			Confidence float64 `json:"confidence"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse mapping JSON: %w (raw: %.500s)", err, text)
	}

	out := &MappingSuggestion{
		Mapping:    map[string]string{},
		Confidence: map[string]float64{},
	}
	for _, c := range raw.Columns {
		field := strings.ToLower(strings.TrimSpace(c.Field))
		if !allowedMappingFields[field] {
			field = "ignore"
		}
		out.Mapping[c.Name] = field
		out.Confidence[c.Name] = c.Confidence
	}
	// Ensure every header has an entry — even if the LLM dropped it.
	for _, h := range headers {
		if _, ok := out.Mapping[h]; !ok {
			out.Mapping[h] = "ignore"
			out.Confidence[h] = 0
		}
	}
	return out, nil
}

// ApplyMapping transforms CSV rows (keyed by header) into ImportItems using a
// confirmed column mapping. Unknown mapping targets are silently dropped.
// Parses numeric fields with tolerant heuristics — "$12.99" → 12, "1,299" → 1299.
func ApplyMapping(headers []string, rows [][]string, mapping map[string]string, sourceSystem, sourceIDPrefix string) []ImportItem {
	items := make([]ImportItem, 0, len(rows))
	for rowIdx, row := range rows {
		item := ImportItem{
			SourceSystem: sourceSystem,
			SourceID:     fmt.Sprintf("%s:%d", sourceIDPrefix, rowIdx),
		}
		for colIdx, header := range headers {
			if colIdx >= len(row) {
				break
			}
			target, ok := mapping[header]
			if !ok || target == "ignore" || target == "" {
				continue
			}
			val := strings.TrimSpace(row[colIdx])
			if val == "" {
				continue
			}
			switch target {
			case "sku":
				item.SKU = val
			case "name":
				item.Name = val
			case "brand":
				item.Brand = val
			case "category":
				item.Category = val
			case "category_slug":
				item.CategorySlug = val
			case "price":
				item.Price = parseIntLoose(val)
			case "currency":
				item.Currency = strings.ToUpper(val)
			case "stock":
				item.Stock = parseIntLoose(val)
			case "rating":
				item.Rating = parseFloatLoose(val)
			case "description":
				if item.Attributes == nil {
					item.Attributes = map[string]any{}
				}
				item.Attributes["description"] = val
			case "images":
				item.Images = splitList(val)
			case "tags":
				item.Tags = splitList(val)
			case "duration":
				item.Duration = val
			case "provider":
				item.Provider = val
			case "availability":
				item.Availability = val
			}
		}
		if item.SKU == "" {
			// Fall back to a row-synthesized SKU so every item is importable.
			item.SKU = fmt.Sprintf("%s-%d", sanitizeSKU(sourceIDPrefix), rowIdx)
		}
		if item.Name == "" {
			// Row lacks a name — skip. Importer will reject it anyway.
			continue
		}
		items = append(items, item)
	}
	return items
}

// --- helpers ---

const csvMappingSystemPrompt = `You are a helpful data-mapping assistant.
Given a CSV header row and a few sample data rows from an e-commerce catalog,
propose the best mapping from each CSV column to one of the internal fields:
sku, name, brand, category, category_slug, price, currency, stock, rating,
description, images, tags, duration, provider, availability, or "ignore".

Rules:
- Only pick fields from the list above. Anything else MUST be "ignore".
- Return STRICT JSON ONLY. No markdown, no prose.
- "price" is a numeric price (cents or whole currency units — just identify).
- "images" and "tags" may be comma- or pipe-separated lists.
- "duration"/"provider"/"availability" are service-specific — use only when rows look like services.
- If a column is unclear or duplicate, prefer "ignore".

Output schema (STRICT):
{
  "columns": [
    {"name": "<exact header>", "field": "<internal field>", "confidence": 0.0-1.0}
  ]
}`

func buildCSVMappingPrompt(headers []string, sampleRows [][]string) string {
	var b strings.Builder
	b.WriteString("Headers: ")
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString("\n\nSample rows:\n")
	for i, row := range sampleRows {
		fmt.Fprintf(&b, "Row %d: %s\n", i+1, strings.Join(row, " | "))
	}
	b.WriteString("\nReturn the JSON mapping now.")
	return b.String()
}

// extractJSONFromText strips markdown fences and leading prose so the JSON
// decoder sees clean input. Same idea as the enrichment client's helper,
// kept local to avoid an anthropic-adapter dep in usecases.
func extractJSONFromText(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json fences
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	// Find outer braces
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			return s[i : j+1]
		}
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func parseIntLoose(s string) int {
	var out int
	var seenDigit bool
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out = out*10 + int(r-'0')
			seenDigit = true
		} else if seenDigit && (r == '.' || r == ',') {
			// stop before decimals (e.g. "12.99" → 12); skip thousands sep only
			// if followed by three digits — simple heuristic, cheap enough.
			if r == ',' {
				continue
			}
			break
		}
	}
	return out
}

func parseFloatLoose(s string) float64 {
	var out float64
	var frac float64 = 1
	inFrac := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			if inFrac {
				frac *= 10
				out += float64(r-'0') / frac
			} else {
				out = out*10 + float64(r-'0')
			}
		} else if r == '.' {
			inFrac = true
		}
	}
	return out
}

func splitList(s string) []string {
	sep := ","
	if strings.Contains(s, "|") {
		sep = "|"
	} else if strings.Contains(s, ";") {
		sep = ";"
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sanitizeSKU(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}
