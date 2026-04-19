package handlers

import (
	"io"
	"net/http"
	"strings"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

// ShopifyHandler serves the four Shopify-related HTTP routes:
//   - install redirect (authenticated — admin starts the flow)
//   - OAuth callback  (unauthenticated — signed state proves identity)
//   - webhook         (unauthenticated — HMAC proves identity)
//   - manual resync   (authenticated)
type ShopifyHandler struct {
	shopify *usecases.ShopifyUseCase
	log     *logger.Logger
}

func NewShopifyHandler(shopify *usecases.ShopifyUseCase, log *logger.Logger) *ShopifyHandler {
	return &ShopifyHandler{shopify: shopify, log: log}
}

// HandleInstall — GET /admin/api/integrations/shopify/install?shop=xxx.myshopify.com
// Wrapped in authMW — only admins can start an install.
func (h *ShopifyHandler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, http.StatusBadRequest, "missing shop")
		return
	}
	tenantID := TenantID(r.Context())
	redirectURL, err := h.shopify.StartOAuth(r.Context(), tenantID, shop)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_install_failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleCallback — GET /admin/api/integrations/shopify/callback
// No authMW: the `state` param is the unforgeable credential — it was stamped
// at install time with the tenant it belongs to.
func (h *ShopifyHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	q := r.URL.Query()
	shop := q.Get("shop")
	code := q.Get("code")
	state := q.Get("state")
	if shop == "" || code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing shop, code, or state")
		return
	}
	integration, err := h.shopify.CompleteOAuth(r.Context(), shop, code, state, q)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_callback_failed", "shop", shop, "error", err)
		writeError(w, http.StatusBadRequest, "oauth callback failed")
		return
	}
	// Land the merchant back on the integrations page.
	target := "/integrations?connected=shopify&id=" + integration.ID
	http.Redirect(w, r, target, http.StatusFound)
}

// HandleWebhook — POST /admin/api/webhooks/shopify
// Unauthenticated route: HMAC verification is the auth layer. The body is
// read raw (no re-marshaling) per Shopify docs.
func (h *ShopifyHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	topic := r.Header.Get("X-Shopify-Topic")
	shop := r.Header.Get("X-Shopify-Shop-Domain")
	sig := r.Header.Get("X-Shopify-Hmac-Sha256")
	if topic == "" || shop == "" || sig == "" {
		writeError(w, http.StatusBadRequest, "missing shopify headers")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body")
		return
	}
	if !h.shopify.Client().VerifyWebhookHMAC(body, sig) {
		h.log.Warn("shopify_webhook_bad_hmac", "shop", shop, "topic", topic)
		writeError(w, http.StatusUnauthorized, "bad hmac")
		return
	}

	// Ack fast (<5s per Shopify SLA); process in background.
	go func() {
		ctx := r.Context()
		if err := h.shopify.HandleWebhook(ctx, topic, shop, body); err != nil {
			h.log.FromContext(ctx).Error("shopify_webhook_handle_failed",
				"shop", shop, "topic", topic, "error", err)
		}
	}()
	w.WriteHeader(http.StatusOK)
}

// HandleResync — POST /admin/api/integrations/shopify/{id}/resync
// Authenticated.
func (h *ShopifyHandler) HandleResync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	tenantID := TenantID(r.Context())
	path := strings.TrimPrefix(r.URL.Path, "/admin/api/integrations/shopify/")
	id := strings.TrimSuffix(path, "/resync")
	if id == "" || id == path {
		writeError(w, http.StatusBadRequest, "missing integration id")
		return
	}
	if err := h.shopify.FullResync(r.Context(), tenantID, id); err != nil {
		h.log.FromContext(r.Context()).Error("shopify_resync_failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "resync started"})
}
