// handler_integrations_shopify.go — install / OAuth callback / webhook routes
// for the Shopify integration. After the curator-driven pivot (2026-04-27)
// these routes go through ShopifyV2UseCase; the legacy ShopifyUseCase was
// removed (it wrote directly to master_products which polluted the master
// catalog with mis-classified vertical='cosmetics' entries).
package handlers

import (
	"context"
	"io"
	"net/http"
	"time"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

// ShopifyHandler serves the install / callback / webhook routes against the
// V2 use case. The dump-to-staging + discover endpoints live on ShopifyV2Handler
// (handler_integrations_shopify_v2.go).
type ShopifyHandler struct {
	v2  *usecases.ShopifyV2UseCase
	log *logger.Logger
}

func NewShopifyHandler(v2 *usecases.ShopifyV2UseCase, log *logger.Logger) *ShopifyHandler {
	return &ShopifyHandler{v2: v2, log: log}
}

// HandleInstall — GET /admin/api/integrations/shopify/install?shop=xxx.myshopify.com
// Wrapped in authMW. Returns JSON {install_url} for the frontend to redirect to.
//
// Why JSON instead of a 302: the frontend authenticates via Bearer token in
// localStorage, so a full-page nav to this endpoint wouldn't include the
// Authorization header. Fetch + window.location.href is the workable pattern.
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
	installURL, err := h.v2.StartOAuth(r.Context(), tenantID, shop)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_install_failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"install_url": installURL})
}

// HandleAppEntry — GET /admin/api/integrations/shopify/app
// Unauthenticated: signed `hmac` query param is the credential. This is the
// "App URL" registered in the Shopify Partner Dashboard. Shopify redirects
// here in three cases — App Store install, Partner test-install, and merchant
// clicking "Open app" inside their Shopify admin — and the use case picks the
// right next step (start OAuth vs. bounce into our admin) based on whether
// the shop is already known.
//
// Why this handler exists separately from HandleInstall: HandleInstall sits
// behind authMW (the in-app "Connect Shopify" button) and assumes a known
// tenant_id from session. This entry has no session — Shopify is the caller —
// so we auto-provision a tenant when the shop is new.
func (h *ShopifyHandler) HandleAppEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	q := r.URL.Query()
	shop := q.Get("shop")
	if shop == "" || q.Get("hmac") == "" {
		writeError(w, http.StatusBadRequest, "missing shop or hmac")
		return
	}
	if !h.v2.Client().VerifyInstallHMAC(q) {
		h.log.Warn("shopify_app_entry_bad_hmac", "shop", shop)
		writeError(w, http.StatusUnauthorized, "bad hmac")
		return
	}
	res, err := h.v2.StartInstallEntry(r.Context(), shop)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_app_entry_failed", "shop", shop, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target := res.InstallURL
	if target == "" {
		target = res.RedirectURL
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// HandleCallback — GET /admin/api/integrations/shopify/callback
// Unauthenticated: signed `state` is the credential.
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
	integration, flowKind, err := h.v2.CompleteOAuth(r.Context(), shop, code, state, q)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_callback_failed", "shop", shop, "error", err)
		writeError(w, http.StatusBadRequest, "oauth callback failed")
		return
	}
	// Install flow: the merchant has no Keepstar account yet — landing them
	// on the generic /auth/sign-in page next to Google/Telegram buttons is
	// confusing (they don't know a magic-link is coming, and those buttons
	// don't apply). Send them to a dedicated "check your inbox" page that
	// names the magic-link flow explicitly. Connect flow keeps the in-app
	// redirect since the user is already logged in.
	target := "/integrations?connected=shopify&id=" + integration.ID
	if flowKind == "install" {
		target = "/auth/install-complete?shop=" + shop
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// HandleWebhook — POST /admin/api/webhooks/shopify
// Unauthenticated: HMAC verification is the auth layer. Raw body, no re-marshal.
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
	if !h.v2.Client().VerifyWebhookHMAC(body, sig) {
		h.log.Warn("shopify_webhook_bad_hmac", "shop", shop, "topic", topic)
		writeError(w, http.StatusUnauthorized, "bad hmac")
		return
	}
	// Ack fast (<5s per Shopify SLA); process in background. We must NOT
	// inherit r.Context() — it gets cancelled the moment the handler returns
	// (right after WriteHeader below), and any DB call in the goroutine then
	// fails with "context canceled". Detach onto a fresh background context
	// with a generous timeout instead.
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer cancel()
		if err := h.v2.HandleWebhook(bgCtx, topic, shop, body); err != nil {
			h.log.Error("shopify_webhook_handle_failed",
				"shop", shop, "topic", topic, "error", err)
		}
	}()
	w.WriteHeader(http.StatusOK)
}
