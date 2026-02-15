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

	"keepstar-admin/internal/domain"
)

const anthropicAPI = "https://api.anthropic.com/v1/messages"

type EnrichmentClient struct {
	apiKey string
	model  string
	client *http.Client
}

func NewEnrichmentClient(apiKey, model string) *EnrichmentClient {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &EnrichmentClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// --- Anthropic Messages API types ---

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
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// --- Public method ---

func (c *EnrichmentClient) Model() string { return c.model }

func (c *EnrichmentClient) EnrichProducts(ctx context.Context, items []domain.EnrichmentInput) (*domain.EnrichmentResult, error) {
	prompt := buildPrompt(items)

	reqBody := messagesRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  []message{{Role: "user", Content: prompt}},
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
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic")
	}

	// Extract JSON from response text (may be wrapped in markdown code block)
	text := msgResp.Content[0].Text
	text = extractJSON(text)

	var outputs []domain.EnrichmentOutput
	if err := json.Unmarshal([]byte(text), &outputs); err != nil {
		return nil, fmt.Errorf("parse enrichment JSON: %w (raw: %.500s)", err, text)
	}

	return &domain.EnrichmentResult{
		Outputs:      outputs,
		InputTokens:  msgResp.Usage.InputTokens,
		OutputTokens: msgResp.Usage.OutputTokens,
	}, nil
}

func extractJSON(s string) string {
	// Strip markdown code fences if present
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

// --- Prompt construction ---

func buildPrompt(items []domain.EnrichmentInput) string {
	var sb strings.Builder
	sb.WriteString("Classify the following products. Return a JSON array with one object per product.\n\n")

	for i, item := range items {
		fmt.Fprintf(&sb, "### Product %d\n", i+1)
		fmt.Fprintf(&sb, "SKU: %s\n", item.SKU)
		fmt.Fprintf(&sb, "Name: %s\n", item.Name)
		if item.Brand != "" {
			fmt.Fprintf(&sb, "Brand: %s\n", item.Brand)
		}
		if item.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", item.Description)
		}
		if item.Ingredients != "" {
			fmt.Fprintf(&sb, "Ingredients: %s\n", item.Ingredients)
		}
		if item.ActiveIngredients != "" {
			fmt.Fprintf(&sb, "Active Ingredients: %s\n", item.ActiveIngredients)
		}
		if item.SkinType != "" {
			fmt.Fprintf(&sb, "Skin Type: %s\n", item.SkinType)
		}
		if item.Benefits != "" {
			fmt.Fprintf(&sb, "Benefits: %s\n", item.Benefits)
		}
		if item.HowToUse != "" {
			fmt.Fprintf(&sb, "How to Use: %s\n", item.HowToUse)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

const systemPrompt = `You are a cosmetics product classifier. For each product, determine the best-matching values from CLOSED LISTS below.

## Category tree (use leaf slug only)
face-care:
  cleansing, toning, exfoliation, serums, moisturizing, suncare, masks, spot-treatment, essences, lip-care
makeup:
  makeup-face, makeup-eyes, makeup-lips, makeup-setting
body:
  body-cleansing, body-moisturizing, body-fragrance
hair:
  hair-shampoo, hair-conditioner, hair-treatment

## product_form (pick ONE)
cream, gel, serum, toner, essence, lotion, oil, balm, foam, mousse, mist, spray, powder, stick, patch, sheet-mask, wash-off-mask, peel, scrub, soap

## skin_type (pick 1-3)
normal, dry, oily, combination, sensitive, acne-prone, mature

## concern (pick 1-4)
hydration, anti-aging, brightening, acne, pores, dark-spots, redness, sun-protection, exfoliation, firmness, dark-circles, lip-dryness, oil-control, texture, dullness

## key_ingredients (pick 1-5)
hyaluronic-acid, niacinamide, retinol, vitamin-c, salicylic-acid, glycolic-acid, centella-asiatica, ceramides, peptides, snail-mucin, tea-tree, aloe-vera, collagen, aha-bha, squalane, shea-butter, argan-oil, rice-extract, green-tea, propolis, mugwort, panthenol, zinc, turmeric, charcoal

## Output format
Return ONLY a JSON array (no markdown, no explanation):
[
  {
    "sku": "...",
    "category_slug": "...",
    "product_form": "...",
    "skin_type": ["..."],
    "concern": ["..."],
    "key_ingredients": ["..."]
  }
]`
