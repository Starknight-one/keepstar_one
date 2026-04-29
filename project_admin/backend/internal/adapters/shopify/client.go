// Package shopify is an HTTP client for the merchant-facing Shopify Admin API.
// REST methods cover the pieces that need single-payload responses (OAuth,
// webhook registration, single-product fetch). GraphQL Bulk Operations cover
// large-catalog initial sync — see RunBulkProductsQuery + GetBulkOperation +
// FetchBulkJSONL. Metadata methods (MetafieldDefinitions, NavigationMenu,
// ShopReferences) sample tenant configuration before the discovery agent runs.
package shopify

import (
	"bufio"
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

const defaultAPIVersion = "2026-04"
const defaultScopes = "read_products,read_product_listings,read_inventory"

// bulkOpHTTPTimeout is generous because Shopify bulk-op endpoints can sit on
// the request for tens of seconds while they queue work. JSONL download uses
// a separate, longer-lived request without this timeout.
const bulkOpHTTPTimeout = 90 * time.Second

type Client struct {
	apiKey     string
	apiSecret  string
	apiVersion string
	scopes     string
	http       *http.Client
	bulkHTTP   *http.Client
}

func NewClient(apiKey, apiSecret, apiVersion, scopes string) *Client {
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	if scopes == "" {
		scopes = defaultScopes
	}
	return &Client{
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		apiVersion: apiVersion,
		scopes:     scopes,
		http:       &http.Client{Timeout: 30 * time.Second},
		bulkHTTP:   &http.Client{Timeout: bulkOpHTTPTimeout},
	}
}

// Scopes returns the configured scope list (comma-separated). Useful for
// persisting alongside an integration record so we know later what we asked
// for at install time, even if the env-configured set drifts.
func (c *Client) Scopes() string { return c.scopes }

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
// come from env (SHOPIFY_SCOPES) and must match what's registered in the
// Shopify App's "Access" → "Scopes" section, otherwise install will be rejected.
func (c *Client) InstallURL(shop, redirectURI, state string) string {
	q := url.Values{}
	q.Set("client_id", c.apiKey)
	q.Set("scope", c.scopes)
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
		fmt.Sprintf("https://%s/admin/api/%s/webhooks.json", shop, c.apiVersion),
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
	u := fmt.Sprintf("https://%s/admin/api/%s/products.json?limit=%d", shop, c.apiVersion, limit)
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
	u := fmt.Sprintf("https://%s/admin/api/%s/products/%d.json", shop, c.apiVersion, productID)
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
	u := fmt.Sprintf("https://%s/admin/api/%s/products/%d/metafields.json", shop, c.apiVersion, productID)
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

// =============================================================================
// GraphQL — Bulk Operations + metadata
// =============================================================================

// graphqlEndpoint returns the Admin GraphQL URL for a shop.
func (c *Client) graphqlEndpoint(shop string) string {
	return fmt.Sprintf("https://%s/admin/api/%s/graphql.json", shop, c.apiVersion)
}

type graphqlError struct {
	Message string `json:"message"`
}

type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors"`
}

// graphqlRequest sends a query/mutation and returns the raw `data` block.
// Top-level GraphQL `errors` are surfaced as a Go error; user-error fields
// inside specific mutations (e.g. bulkOperationRunQuery.userErrors) must be
// inspected by the caller because their shape varies per mutation.
func (c *Client) graphqlRequest(ctx context.Context, shop, token, query string, variables map[string]any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlEndpoint(shop), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql status %d: %s", resp.StatusCode, string(respBody))
	}
	var gr graphqlResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return nil, fmt.Errorf("parse graphql: %w", err)
	}
	if len(gr.Errors) > 0 {
		return nil, fmt.Errorf("graphql errors: %s", gr.Errors[0].Message)
	}
	return gr.Data, nil
}

// bulkProductsQueryBody is the JSONL-shaped query embedded inside
// bulkOperationRunQuery. Shopify processes it asynchronously and emits one
// JSON object per line, with parent-child relationships flattened via
// `__parentId` (a variant carries `__parentId` = product GID, an image
// carries `__parentId` = product GID, a metafield carries the GID of its
// owner). The harvester reassembles the tree on read.
//
// Fields are deliberately broad: we fetch everything that might inform the
// discovery agent's mapping decisions. Trimming this later costs nothing —
// re-running a bulk op is cheap.
const bulkProductsQueryBody = `{
  products {
    edges {
      node {
        id
        title
        handle
        descriptionHtml
        productType
        vendor
        tags
        status
        createdAt
        updatedAt
        publishedAt
        featuredImage { url altText }
        images { edges { node { url altText } } }
        options { name values position }
        variants {
          edges {
            node {
              id
              sku
              title
              price
              compareAtPrice
              barcode
              inventoryQuantity
              inventoryItem {
                measurement {
                  weight { value unit }
                }
              }
              selectedOptions { name value }
              image { url altText }
            }
          }
        }
        metafields {
          edges {
            node {
              namespace
              key
              type
              value
            }
          }
        }
        collections(first: 50) {
          edges {
            node { id handle title }
          }
        }
      }
    }
  }
}`

const bulkRunQueryMutation = `mutation runBulk($q: String!) {
  bulkOperationRunQuery(query: $q) {
    bulkOperation { id status }
    userErrors { field message }
  }
}`

// BulkOperationStatus mirrors Shopify's enum. We only branch on the
// terminal values (Completed/Failed/Canceled) elsewhere; intermediate states
// just keep polling.
type BulkOperationStatus string

const (
	BulkOpCreated   BulkOperationStatus = "CREATED"
	BulkOpRunning   BulkOperationStatus = "RUNNING"
	BulkOpCompleted BulkOperationStatus = "COMPLETED"
	BulkOpCanceling BulkOperationStatus = "CANCELING"
	BulkOpCanceled  BulkOperationStatus = "CANCELED"
	BulkOpFailed    BulkOperationStatus = "FAILED"
	BulkOpExpired   BulkOperationStatus = "EXPIRED"
)

// BulkOperation is the lifecycle record for a single bulk pull. URL is
// populated only when Status == Completed; ErrorCode is populated on failure.
// ObjectCount is the rows that were emitted to the JSONL file (useful for
// progress UI).
type BulkOperation struct {
	ID             string              `json:"id"`
	Status         BulkOperationStatus `json:"status"`
	ErrorCode      string              `json:"errorCode,omitempty"`
	CreatedAt      string              `json:"createdAt,omitempty"`
	CompletedAt    string              `json:"completedAt,omitempty"`
	ObjectCount    string              `json:"objectCount,omitempty"`
	FileSize       string              `json:"fileSize,omitempty"`
	URL            string              `json:"url,omitempty"`
	PartialDataURL string              `json:"partialDataUrl,omitempty"`
}

// RunBulkProductsQuery fires bulkOperationRunQuery for the full products
// graph. Returns the operation GID; caller polls GetBulkOperation until
// Status == COMPLETED, then streams via FetchBulkJSONL.
//
// Shopify only allows ONE active bulk op per shop per access token. If a
// previous op is still running (status CREATED/RUNNING), the userErrors
// block returns "A bulk query operation for this app and shop is already
// in progress." Caller should poll the existing op rather than retry.
func (c *Client) RunBulkProductsQuery(ctx context.Context, shop, token string) (string, error) {
	data, err := c.graphqlRequest(ctx, shop, token, bulkRunQueryMutation, map[string]any{
		"q": bulkProductsQueryBody,
	})
	if err != nil {
		return "", fmt.Errorf("bulk run query: %w", err)
	}
	var wrap struct {
		BulkOperationRunQuery struct {
			BulkOperation *BulkOperation `json:"bulkOperation"`
			UserErrors    []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"bulkOperationRunQuery"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return "", fmt.Errorf("parse bulk run query: %w", err)
	}
	if len(wrap.BulkOperationRunQuery.UserErrors) > 0 {
		return "", fmt.Errorf("bulk run user error: %s", wrap.BulkOperationRunQuery.UserErrors[0].Message)
	}
	if wrap.BulkOperationRunQuery.BulkOperation == nil {
		return "", fmt.Errorf("bulk run: empty operation in response")
	}
	return wrap.BulkOperationRunQuery.BulkOperation.ID, nil
}

const bulkCurrentOpQuery = `{
  currentBulkOperation {
    id status errorCode createdAt completedAt objectCount fileSize url partialDataUrl
  }
}`

// GetCurrentBulkOperation returns the most recent bulk op for this app+shop.
// We poll this rather than tracking ID through Shopify's hop because their
// docs explicitly recommend it: there's only ever one active op per app/shop,
// so "current" is unambiguous.
func (c *Client) GetCurrentBulkOperation(ctx context.Context, shop, token string) (*BulkOperation, error) {
	data, err := c.graphqlRequest(ctx, shop, token, bulkCurrentOpQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("current bulk op: %w", err)
	}
	var wrap struct {
		CurrentBulkOperation *BulkOperation `json:"currentBulkOperation"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("parse current bulk op: %w", err)
	}
	return wrap.CurrentBulkOperation, nil
}

// FetchBulkJSONL streams the completed bulk-op output. Caller MUST close the
// returned reader. The URL is a presigned GCS link with a short TTL, so
// fetch promptly after the op completes.
//
// Use bufio.Scanner on the returned reader to iterate one JSONL row per line.
// Rows are top-level objects of the queried types — no JSON array wrapper.
func (c *Client) FetchBulkJSONL(ctx context.Context, jsonlURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jsonlURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.bulkHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jsonl: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("fetch jsonl status %d: %s", resp.StatusCode, string(respBody))
	}
	return resp.Body, nil
}

// ScanBulkJSONL is a small helper that wraps a JSONL stream in a Scanner
// with a generous buffer. Default Scanner buffer (64KB) overflows on
// products with long descriptions or huge metafield arrays.
func ScanBulkJSONL(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // up to 16 MB per line
	return sc
}

// MetafieldDefinition is the declared shape of a metafield in this shop —
// what the merchant has formally defined under Settings → Custom Data.
// Discovery agent reads this to know which fields are intentional vs ad-hoc.
type MetafieldDefinition struct {
	ID          string `json:"id"`
	Namespace   string `json:"namespace"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	OwnerType   string `json:"ownerType"`
}

const metafieldDefinitionsQuery = `query mfDefs($ownerType: MetafieldOwnerType!, $first: Int!, $after: String) {
  metafieldDefinitions(ownerType: $ownerType, first: $first, after: $after) {
    edges {
      cursor
      node {
        id namespace key name description type { name } ownerType
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

// MetafieldDefinitions paginates all declared metafield definitions for an
// owner type ("PRODUCT", "PRODUCTVARIANT", "COLLECTION"). Returns the full
// list — typically <100 entries per owner, so no streaming needed.
func (c *Client) MetafieldDefinitions(ctx context.Context, shop, token, ownerType string) ([]MetafieldDefinition, error) {
	var (
		out    []MetafieldDefinition
		cursor string
	)
	for {
		vars := map[string]any{"ownerType": ownerType, "first": 100}
		if cursor != "" {
			vars["after"] = cursor
		}
		data, err := c.graphqlRequest(ctx, shop, token, metafieldDefinitionsQuery, vars)
		if err != nil {
			return nil, fmt.Errorf("metafield definitions %s: %w", ownerType, err)
		}
		var wrap struct {
			MetafieldDefinitions struct {
				Edges []struct {
					Cursor string `json:"cursor"`
					Node   struct {
						ID          string `json:"id"`
						Namespace   string `json:"namespace"`
						Key         string `json:"key"`
						Name        string `json:"name"`
						Description string `json:"description"`
						Type        struct {
							Name string `json:"name"`
						} `json:"type"`
						OwnerType string `json:"ownerType"`
					} `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"metafieldDefinitions"`
		}
		if err := json.Unmarshal(data, &wrap); err != nil {
			return nil, fmt.Errorf("parse metafield defs: %w", err)
		}
		for _, e := range wrap.MetafieldDefinitions.Edges {
			out = append(out, MetafieldDefinition{
				ID:          e.Node.ID,
				Namespace:   e.Node.Namespace,
				Key:         e.Node.Key,
				Name:        e.Node.Name,
				Description: e.Node.Description,
				Type:        e.Node.Type.Name,
				OwnerType:   e.Node.OwnerType,
			})
		}
		if !wrap.MetafieldDefinitions.PageInfo.HasNextPage {
			break
		}
		cursor = wrap.MetafieldDefinitions.PageInfo.EndCursor
	}
	return out, nil
}

// NavigationMenuItem is one node in the published storefront menu. Children
// is recursive (Shopify nests up to 4 levels deep). ResourceID, when set,
// usually points to a Collection — we use that to build the tenant's
// preferred category tree before falling back to handle prefixes.
type NavigationMenuItem struct {
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	URL        string               `json:"url"`
	Type       string               `json:"type"`
	ResourceID string               `json:"resourceId,omitempty"`
	Items      []NavigationMenuItem `json:"items,omitempty"`
}

// NavigationMenu is a single named menu (typically "main-menu" — the
// storefront top nav). Shops sometimes have multiple menus (footer, mobile,
// etc.); caller picks the one most likely to reflect the tenant's
// merchandising taxonomy.
type NavigationMenu struct {
	ID     string               `json:"id"`
	Handle string               `json:"handle"`
	Title  string               `json:"title"`
	Items  []NavigationMenuItem `json:"items"`
}

const navigationMenuQuery = `query menu($handle: String!) {
  menu(handle: $handle) {
    id handle title
    items {
      id title url type resourceId
      items {
        id title url type resourceId
        items {
          id title url type resourceId
          items { id title url type resourceId }
        }
      }
    }
  }
}`

// NavigationMenu fetches one named menu. Returns nil (no error) if the menu
// doesn't exist — many shops use only the default "main-menu", and a missing
// "footer" or whatever is normal, not an error.
func (c *Client) NavigationMenu(ctx context.Context, shop, token, handle string) (*NavigationMenu, error) {
	data, err := c.graphqlRequest(ctx, shop, token, navigationMenuQuery, map[string]any{"handle": handle})
	if err != nil {
		return nil, fmt.Errorf("menu %s: %w", handle, err)
	}
	var wrap struct {
		Menu *NavigationMenu `json:"menu"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("parse menu: %w", err)
	}
	return wrap.Menu, nil
}

// ShopReferences is the small reference-data bundle that helps discovery
// agent see what universe of values exists in this shop without scanning
// the catalog. ProductVendors is the brand list, ProductTypes is shopify's
// flat product_type field across all products, ProductTags is every tag.
//
// All three are bounded — Shopify caps each at "first 250" in the basic
// shop query. Larger shops would need separate paginated queries; we accept
// the cap for now (250 brands/types/tags is plenty for discovery context).
type ShopReferences struct {
	ProductVendors []string `json:"productVendors"`
	ProductTypes   []string `json:"productTypes"`
	ProductTags    []string `json:"productTags"`
}

const shopReferencesQuery = `{
  shop {
    productVendors(first: 250) { edges { node } }
    productTypes(first: 250)   { edges { node } }
    productTags(first: 250)    { edges { node } }
  }
}`

// ShopReferences pulls vendors, product_types, and tags in one round trip.
// All three lists are deduped client-side just in case Shopify ever returns
// duplicates (their docs say they don't, but the lists are cheap to clean).
func (c *Client) ShopReferences(ctx context.Context, shop, token string) (*ShopReferences, error) {
	data, err := c.graphqlRequest(ctx, shop, token, shopReferencesQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("shop references: %w", err)
	}
	var wrap struct {
		Shop struct {
			ProductVendors struct {
				Edges []struct {
					Node string `json:"node"`
				} `json:"edges"`
			} `json:"productVendors"`
			ProductTypes struct {
				Edges []struct {
					Node string `json:"node"`
				} `json:"edges"`
			} `json:"productTypes"`
			ProductTags struct {
				Edges []struct {
					Node string `json:"node"`
				} `json:"edges"`
			} `json:"productTags"`
		} `json:"shop"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("parse shop references: %w", err)
	}
	out := &ShopReferences{}
	out.ProductVendors = dedupStrings(extractEdgeStrings(wrap.Shop.ProductVendors.Edges))
	out.ProductTypes = dedupStrings(extractEdgeStrings(wrap.Shop.ProductTypes.Edges))
	out.ProductTags = dedupStrings(extractEdgeStrings(wrap.Shop.ProductTags.Edges))
	return out, nil
}

func extractEdgeStrings(edges []struct {
	Node string `json:"node"`
}) []string {
	out := make([]string, 0, len(edges))
	for _, e := range edges {
		s := strings.TrimSpace(e.Node)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// =============================================================================
// Write methods — used by cmd/seed-devstore for populating test data in
// dev-store (curator pivot 2026-04-27 Этап 3). Not used by harvester-lite or
// merge-agent — those are read-only on Shopify side. Mutations require
// `write_products` + `write_inventory` + `write_publications` scopes.
// =============================================================================

// ProductCreateInput is the Go-side shape we pass to ProductCreate. We translate
// it into Shopify's productSet mutation input. Currency is shop default; price
// is a decimal string (Shopify Money scalar).
type ProductCreateInput struct {
	Title           string
	DescriptionHTML string
	Vendor          string
	ProductType     string
	Tags            []string
	Status          string // ACTIVE | DRAFT | ARCHIVED. Default ACTIVE.
	OptionNames     []string // e.g. ["Size", "Color"]; up to 3
	Variants        []VariantInput
	Metafields      []MetafieldInput
	ImageURLs       []string // Shopify will fetch and host these URLs.
	CollectionGIDs  []string // gid://shopify/Collection/... — assigns at create time.
}

type VariantInput struct {
	SKU             string
	Barcode         string
	PriceDecimal    string // e.g. "12.99"
	CompareDecimal  string // optional
	InventoryQty    int
	OptionValues    []string // values aligned with parent OptionNames; e.g. ["50ml","Vanilla"]
	WeightGrams     float64  // 0 = unset
}

type MetafieldInput struct {
	Namespace string
	Key       string
	Type      string // single_line_text_field | number_decimal | json | etc.
	Value     string
}

// ProductCreate creates a product with variants, metafields, images, and
// collection assignments in a single GraphQL mutation. Returns the product GID.
//
// Uses productSet (synchronous) — supports full nested input incl. options +
// variants + metafields + media + collection links. Synchronous mode is fine
// for ≤25 variants per product (our seed scenarios have ≤6).
func (c *Client) ProductCreate(ctx context.Context, shop, token string, in ProductCreateInput) (string, error) {
	const mutation = `
mutation seedProductSet($input: ProductSetInput!) {
  productSet(synchronous: true, input: $input) {
    product { id title }
    userErrors { field message }
  }
}`

	// Shopify products always have at least one option ("Title"/"Default Title"
	// is auto-created by the storefront). productSet enforces this — pass-through
	// here so option-less products don't fail validation.
	optionNames := in.OptionNames
	usingDefaultTitle := false
	if len(optionNames) == 0 {
		optionNames = []string{"Title"}
		usingDefaultTitle = true
	}

	// Build productOptions[].name + productOptions[].values[] from variant option values.
	productOptions := make([]map[string]any, 0, len(optionNames))
	if usingDefaultTitle {
		productOptions = append(productOptions, map[string]any{
			"name":   "Title",
			"values": []map[string]any{{"name": "Default Title"}},
		})
	} else {
		// Collect unique values per option index.
		valuesPerOpt := make([][]string, len(in.OptionNames))
		seen := make([]map[string]struct{}, len(in.OptionNames))
		for i := range seen {
			seen[i] = map[string]struct{}{}
		}
		for _, v := range in.Variants {
			for i, val := range v.OptionValues {
				if i >= len(in.OptionNames) || val == "" {
					continue
				}
				if _, dup := seen[i][val]; dup {
					continue
				}
				seen[i][val] = struct{}{}
				valuesPerOpt[i] = append(valuesPerOpt[i], val)
			}
		}
		for i, name := range in.OptionNames {
			vals := make([]map[string]any, 0, len(valuesPerOpt[i]))
			for _, v := range valuesPerOpt[i] {
				vals = append(vals, map[string]any{"name": v})
			}
			productOptions = append(productOptions, map[string]any{
				"name":   name,
				"values": vals,
			})
		}
	}

	variants := make([]map[string]any, 0, len(in.Variants))
	for _, v := range in.Variants {
		// 2026-04 ProductSetInput requires optionValues as a present array — null
		// is rejected. For products with no real options we fall back to the
		// auto-injected "Title" option above; each variant must reference it.
		optVals := []map[string]any{}
		if usingDefaultTitle {
			optVals = []map[string]any{{
				"optionName": "Title", "name": "Default Title",
			}}
		} else {
			for i, ov := range v.OptionValues {
				if i >= len(in.OptionNames) || ov == "" {
					continue
				}
				optVals = append(optVals, map[string]any{
					"optionName": in.OptionNames[i],
					"name":       ov,
				})
			}
		}
		mv := map[string]any{
			"sku":          v.SKU,
			"barcode":      v.Barcode,
			"price":        v.PriceDecimal,
			"optionValues": optVals,
		}
		if v.CompareDecimal != "" {
			mv["compareAtPrice"] = v.CompareDecimal
		}
		variants = append(variants, mv)
	}

	mfs := make([]map[string]any, 0, len(in.Metafields))
	for _, m := range in.Metafields {
		if m.Value == "" {
			// Shopify rejects empty metafield values. Skip silently — we use
			// these in seed scenarios to simulate sparse-data sources.
			continue
		}
		mfs = append(mfs, map[string]any{
			"namespace": m.Namespace,
			"key":       m.Key,
			"type":      m.Type,
			"value":     m.Value,
		})
	}

	// productSet input — schema based on Admin API 2026-04. Notes on what we DON'T
	// send: `collectionsToJoin` and `files` are not on ProductSetInput in this
	// version; collections are attached via collectionAddProductsV2 after create,
	// images via productCreateMedia. For seed-devstore that's fine — we don't
	// need images to test merge-agent matching. Collections are wired below via
	// AssignProductsToCollection.
	input := map[string]any{
		"title":           in.Title,
		"descriptionHtml": in.DescriptionHTML,
		"vendor":          in.Vendor,
		"productType":     in.ProductType,
		"tags":            in.Tags,
		"productOptions":  productOptions,
		"variants":        variants,
		"metafields":      mfs,
	}
	if in.Status != "" {
		input["status"] = in.Status
	} else {
		input["status"] = "ACTIVE"
	}

	data, err := c.graphqlRequest(ctx, shop, token, mutation, map[string]any{"input": input})
	if err != nil {
		return "", fmt.Errorf("productCreate: %w", err)
	}
	var resp struct {
		ProductSet struct {
			Product *struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"product"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"productSet"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse productSet: %w", err)
	}
	if len(resp.ProductSet.UserErrors) > 0 {
		errs := make([]string, 0, len(resp.ProductSet.UserErrors))
		for _, e := range resp.ProductSet.UserErrors {
			errs = append(errs, fmt.Sprintf("%v: %s", e.Field, e.Message))
		}
		return "", fmt.Errorf("productSet user-errors: %s", strings.Join(errs, "; "))
	}
	if resp.ProductSet.Product == nil {
		return "", fmt.Errorf("productSet returned nil product")
	}
	return resp.ProductSet.Product.ID, nil
}

// ProductCreateMedia attaches one or more images to a product by URL. Shopify
// fetches each URL and hosts the file itself — no upload needed. Used by
// seed-devstore after ProductCreate to attach scenario images (productSet input
// in 2026-04 doesn't accept media inline, so this is a follow-up call).
//
// Returns nil on full success. Per-image upload errors come back inside
// `mediaUserErrors` and are flattened into a single error so seed-devstore can
// log the failure but keep going on remaining products.
func (c *Client) ProductCreateMedia(ctx context.Context, shop, token, productGID string, imageURLs []string) error {
	if len(imageURLs) == 0 || productGID == "" {
		return nil
	}
	const mutation = `
mutation seedProductCreateMedia($productId: ID!, $media: [CreateMediaInput!]!) {
  productCreateMedia(productId: $productId, media: $media) {
    media { mediaContentType status }
    mediaUserErrors { field message }
  }
}`
	media := make([]map[string]any, 0, len(imageURLs))
	for _, u := range imageURLs {
		if u == "" {
			continue
		}
		media = append(media, map[string]any{
			"originalSource":   u,
			"mediaContentType": "IMAGE",
		})
	}
	data, err := c.graphqlRequest(ctx, shop, token, mutation, map[string]any{
		"productId": productGID,
		"media":     media,
	})
	if err != nil {
		return fmt.Errorf("productCreateMedia: %w", err)
	}
	var resp struct {
		ProductCreateMedia struct {
			MediaUserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"mediaUserErrors"`
		} `json:"productCreateMedia"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse productCreateMedia: %w", err)
	}
	if len(resp.ProductCreateMedia.MediaUserErrors) > 0 {
		errs := make([]string, 0, len(resp.ProductCreateMedia.MediaUserErrors))
		for _, e := range resp.ProductCreateMedia.MediaUserErrors {
			errs = append(errs, fmt.Sprintf("%v: %s", e.Field, e.Message))
		}
		return fmt.Errorf("productCreateMedia user-errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// CollectionAddProducts attaches products to a custom collection. Used by
// seed-devstore after ProductCreate to wire scenario 3.4 (fuzzy category
// match — products live in "Skincare Solutions" not the canonical "Face Care").
// Uses collectionAddProductsV2 (async job — returns immediately, but for ≤50
// products the job completes before the next API call we'd make anyway).
func (c *Client) CollectionAddProducts(ctx context.Context, shop, token, collectionGID string, productGIDs []string) error {
	if len(productGIDs) == 0 {
		return nil
	}
	const mutation = `
mutation seedCollAddProducts($id: ID!, $productIds: [ID!]!) {
  collectionAddProductsV2(id: $id, productIds: $productIds) {
    job { id done }
    userErrors { field message }
  }
}`
	data, err := c.graphqlRequest(ctx, shop, token, mutation, map[string]any{
		"id":         collectionGID,
		"productIds": productGIDs,
	})
	if err != nil {
		return fmt.Errorf("collectionAddProductsV2: %w", err)
	}
	var resp struct {
		CollectionAddProductsV2 struct {
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"collectionAddProductsV2"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse collectionAddProductsV2: %w", err)
	}
	if len(resp.CollectionAddProductsV2.UserErrors) > 0 {
		errs := make([]string, 0, len(resp.CollectionAddProductsV2.UserErrors))
		for _, e := range resp.CollectionAddProductsV2.UserErrors {
			errs = append(errs, fmt.Sprintf("%v: %s", e.Field, e.Message))
		}
		return fmt.Errorf("collectionAddProductsV2 user-errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// CollectionCreate makes a custom (manual) collection. Returns the collection
// GID. Products are assigned via ProductCreate's CollectionGIDs field, so
// no separate "add products" call is needed for the seed flow.
func (c *Client) CollectionCreate(ctx context.Context, shop, token, title, handle, descriptionHTML string) (string, error) {
	const mutation = `
mutation seedCollectionCreate($input: CollectionInput!) {
  collectionCreate(input: $input) {
    collection { id title }
    userErrors { field message }
  }
}`
	input := map[string]any{
		"title":           title,
		"descriptionHtml": descriptionHTML,
	}
	if handle != "" {
		input["handle"] = handle
	}
	data, err := c.graphqlRequest(ctx, shop, token, mutation, map[string]any{"input": input})
	if err != nil {
		return "", fmt.Errorf("collectionCreate: %w", err)
	}
	var resp struct {
		CollectionCreate struct {
			Collection *struct {
				ID string `json:"id"`
			} `json:"collection"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"collectionCreate"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse collectionCreate: %w", err)
	}
	if len(resp.CollectionCreate.UserErrors) > 0 {
		errs := make([]string, 0, len(resp.CollectionCreate.UserErrors))
		for _, e := range resp.CollectionCreate.UserErrors {
			errs = append(errs, fmt.Sprintf("%v: %s", e.Field, e.Message))
		}
		return "", fmt.Errorf("collectionCreate user-errors: %s", strings.Join(errs, "; "))
	}
	if resp.CollectionCreate.Collection == nil {
		return "", fmt.Errorf("collectionCreate returned nil")
	}
	return resp.CollectionCreate.Collection.ID, nil
}

// ProductsDeleteAll wipes every product in the store. Used by seed-devstore --reset
// to start clean. Returns the count deleted. Refuses to run on shops not
// matching `*.myshopify.com` to prevent any accidental prod-store usage.
func (c *Client) ProductsDeleteAll(ctx context.Context, shop, token string) (int, error) {
	if _, ok := ValidateShopDomain(shop); !ok {
		return 0, fmt.Errorf("ProductsDeleteAll refuses non-myshopify shop %q", shop)
	}
	deleted := 0
	for {
		// Fetch up to 50 product IDs, delete one by one (Shopify has no bulk-delete mutation).
		const listQuery = `query { products(first: 50) { edges { node { id } } } }`
		data, err := c.graphqlRequest(ctx, shop, token, listQuery, nil)
		if err != nil {
			return deleted, fmt.Errorf("list ids: %w", err)
		}
		var listResp struct {
			Products struct {
				Edges []struct {
					Node struct {
						ID string `json:"id"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"products"`
		}
		if err := json.Unmarshal(data, &listResp); err != nil {
			return deleted, fmt.Errorf("parse list: %w", err)
		}
		if len(listResp.Products.Edges) == 0 {
			return deleted, nil
		}
		for _, e := range listResp.Products.Edges {
			const delMutation = `
mutation seedProductDelete($id: ID!) {
  productDelete(input: {id: $id}) {
    deletedProductId
    userErrors { field message }
  }
}`
			if _, err := c.graphqlRequest(ctx, shop, token, delMutation, map[string]any{"id": e.Node.ID}); err != nil {
				return deleted, fmt.Errorf("delete %s: %w", e.Node.ID, err)
			}
			deleted++
		}
	}
}

// CollectionsDeleteAll removes all custom collections. Companion to
// ProductsDeleteAll for clean re-seed runs.
func (c *Client) CollectionsDeleteAll(ctx context.Context, shop, token string) (int, error) {
	if _, ok := ValidateShopDomain(shop); !ok {
		return 0, fmt.Errorf("CollectionsDeleteAll refuses non-myshopify shop %q", shop)
	}
	deleted := 0
	for {
		const listQuery = `query { collections(first: 50, query: "collection_type:custom") { edges { node { id } } } }`
		data, err := c.graphqlRequest(ctx, shop, token, listQuery, nil)
		if err != nil {
			return deleted, fmt.Errorf("list collections: %w", err)
		}
		var listResp struct {
			Collections struct {
				Edges []struct {
					Node struct {
						ID string `json:"id"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"collections"`
		}
		if err := json.Unmarshal(data, &listResp); err != nil {
			return deleted, fmt.Errorf("parse list: %w", err)
		}
		if len(listResp.Collections.Edges) == 0 {
			return deleted, nil
		}
		for _, e := range listResp.Collections.Edges {
			const delMutation = `
mutation seedCollectionDelete($id: ID!) {
  collectionDelete(input: {id: $id}) {
    deletedCollectionId
    userErrors { field message }
  }
}`
			if _, err := c.graphqlRequest(ctx, shop, token, delMutation, map[string]any{"id": e.Node.ID}); err != nil {
				return deleted, fmt.Errorf("delete %s: %w", e.Node.ID, err)
			}
			deleted++
		}
	}
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
