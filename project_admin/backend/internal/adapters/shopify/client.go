// Package shopify is a thin HTTP client for the merchant-facing Shopify Admin
// REST API. Scope is minimal on purpose — only what the onboarding flow needs:
// OAuth token exchange, product pagination, webhook registration, and HMAC
// verification. GraphQL bulk-query for large catalogs is deferred to the
// usecase layer, which chooses REST vs bulk per catalog size.
package shopify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const apiVersion = "2024-07"

type Client struct {
	apiKey    string
	apiSecret string
	http      *http.Client
}

func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// shopDomainRE guards against open-redirect abuse: the `shop` query param
// must be a *.myshopify.com hostname. Any other shape gets rejected upstream.
var shopDomainRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.myshopify\.com$`)

// ValidateShopDomain normalizes input ("Store-Name.myshopify.com " → "store-name.myshopify.com")
// and verifies the result matches the required pattern.
func ValidateShopDomain(shop string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(shop))
	return normalized, shopDomainRE.MatchString(normalized)
}

// InstallURL builds the redirect to Shopify's OAuth consent screen. Scopes
// are hardcoded to read-only product data — we never write back to Shopify.
func (c *Client) InstallURL(shop, redirectURI, state string) string {
	q := url.Values{}
	q.Set("client_id", c.apiKey)
	q.Set("scope", "read_products,read_product_listings,read_inventory")
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	return fmt.Sprintf("https://%s/admin/oauth/authorize?%s", shop, q.Encode())
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
}

// ExchangeCodeForToken swaps the OAuth code for an offline access token.
// Returns the plaintext token — caller must seal it via secretbox before storage.
func (c *Client) ExchangeCodeForToken(ctx context.Context, shop, code string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     c.apiKey,
		"client_secret": c.apiSecret,
		"code":          code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://%s/admin/oauth/access_token", shop), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("shopify oauth exchange: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shopify oauth status %d: %s", resp.StatusCode, string(respBody))
	}
	var tr tokenResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	return tr.AccessToken, nil
}

// VerifyInstallHMAC checks the signature Shopify puts on the install/callback
// redirect query string. The HMAC covers all params except the `hmac` itself,
// joined alphabetically as key=value&… — see Shopify auth docs.
func (c *Client) VerifyInstallHMAC(q url.Values) bool {
	sig := q.Get("hmac")
	if sig == "" {
		return false
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		if k == "hmac" || k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	// alphabetical
	sortStrings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(q.Get(k))
	}
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(b.String()))
	expected := fmt.Sprintf("%x", mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

// VerifyWebhookHMAC checks the X-Shopify-Hmac-Sha256 header on a webhook
// delivery. The HMAC covers the raw request body and is base64-encoded.
// Callers MUST pass the exact bytes that arrived (no re-marshaling).
func (c *Client) VerifyWebhookHMAC(body []byte, headerSig string) bool {
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(headerSig))
}

// RegisterWebhook subscribes a topic to our admin webhook endpoint. Idempotent
// on Shopify's side: duplicate topics return 422; we treat that as success.
func (c *Client) RegisterWebhook(ctx context.Context, shop, token, topic, address string) error {
	body, _ := json.Marshal(map[string]any{
		"webhook": map[string]any{
			"topic":   topic,
			"address": address,
			"format":  "json",
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://%s/admin/api/%s/webhooks.json", shop, apiVersion),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("register webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusUnprocessableEntity {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("webhook %s: status %d: %s", topic, resp.StatusCode, string(respBody))
}

// ShopifyProduct is the REST product shape we care about. Kept narrow — this
// is intentionally not a complete mirror of Shopify's schema.
type ShopifyProduct struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	BodyHTML    string `json:"body_html"`
	Vendor      string `json:"vendor"`
	ProductType string `json:"product_type"`
	Tags        string `json:"tags"`
	Handle      string `json:"handle"`
	Images      []struct {
		Src string `json:"src"`
	} `json:"images"`
	Variants []struct {
		ID                int64  `json:"id"`
		SKU               string `json:"sku"`
		Price             string `json:"price"`
		InventoryQuantity int    `json:"inventory_quantity"`
	} `json:"variants"`
	Metafields []Metafield `json:"metafields,omitempty"`
}

type Metafield struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Type      string `json:"type"`
}

type productsPage struct {
	Products []ShopifyProduct `json:"products"`
}

// ListProducts returns one page of products plus the next page_info token
// (empty when exhausted). Shopify uses Link-header cursor pagination:
// `<url?page_info=XXX>; rel="next"`.
func (c *Client) ListProducts(ctx context.Context, shop, token, pageInfo string, limit int) ([]ShopifyProduct, string, error) {
	if limit <= 0 || limit > 250 {
		limit = 250
	}
	u := fmt.Sprintf("https://%s/admin/api/%s/products.json?limit=%d", shop, apiVersion, limit)
	if pageInfo != "" {
		u += "&page_info=" + url.QueryEscape(pageInfo)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Shopify-Access-Token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("list products: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("list products status %d: %s", resp.StatusCode, string(respBody))
	}
	var page productsPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("parse products: %w", err)
	}
	next := parseNextPageInfo(resp.Header.Get("Link"))
	return page.Products, next, nil
}

// GetProduct fetches one product (used by products/create|update webhook
// handlers when the payload is trimmed).
func (c *Client) GetProduct(ctx context.Context, shop, token string, productID int64) (*ShopifyProduct, error) {
	u := fmt.Sprintf("https://%s/admin/api/%s/products/%d.json", shop, apiVersion, productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Shopify-Access-Token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get product status %d: %s", resp.StatusCode, string(respBody))
	}
	var wrap struct {
		Product ShopifyProduct `json:"product"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return nil, fmt.Errorf("parse product: %w", err)
	}
	return &wrap.Product, nil
}

// GetProductMetafields fetches metafields for a product. Called per-product
// on initial sync for catalogs <500; larger catalogs should use GraphQL
// bulk-query (deferred — falls back to per-product REST for MVP).
func (c *Client) GetProductMetafields(ctx context.Context, shop, token string, productID int64) ([]Metafield, error) {
	u := fmt.Sprintf("https://%s/admin/api/%s/products/%d/metafields.json", shop, apiVersion, productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Shopify-Access-Token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get metafields: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil // metafields are optional — don't fail the whole sync
	}
	var wrap struct {
		Metafields []Metafield `json:"metafields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return nil, fmt.Errorf("parse metafields: %w", err)
	}
	return wrap.Metafields, nil
}

// --- helpers ---

var linkNextRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func parseNextPageInfo(link string) string {
	match := linkNextRE.FindStringSubmatch(link)
	if len(match) < 2 {
		return ""
	}
	u, err := url.Parse(match[1])
	if err != nil {
		return ""
	}
	return u.Query().Get("page_info")
}

// sortStrings avoids pulling in sort just for 4-6 element slices on the hot path.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
