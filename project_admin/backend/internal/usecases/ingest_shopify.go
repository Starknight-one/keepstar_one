// Package usecases — ShopifyIngester is the new entry point for Shopify
// bulk pulls in the 6-step flow. Replaces (eventually) harvester_lite.go's
// RunForTenant. Kept compact on purpose:
//   1. Caller (Shopify install handler or webhook) hands us parsed bulk
//      items {GID, raw payload bytes}.
//   2. We push them into inbox via InboxUseCase.
//   3. We call UpdateOrchestrator.ManualSync to apply — which cascades
//      to discovery_v2 if there's no artifact yet (first install path).
//
// Why not orchestrator.OnWebhook: bulk ingests are explicit "I want everything
// applied now" actions, not rate-limited events. Manual sync semantics fit.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/logger"
)

type ShopifyIngester struct {
	inbox        *InboxUseCase
	orchestrator *UpdateOrchestrator
	log          *logger.Logger
}

func NewShopifyIngester(inbox *InboxUseCase, orchestrator *UpdateOrchestrator, log *logger.Logger) *ShopifyIngester {
	return &ShopifyIngester{inbox: inbox, orchestrator: orchestrator, log: log}
}

// IngestBulkItems writes pre-parsed Shopify products into inbox, then
// runs apply. Returns the apply result (counts) plus the inbox write
// summary.
//
// Pre-parsed because Shopify bulk JSONL parsing already exists in the
// shopify client adapter — we don't duplicate it. Callers fetch the bulk
// URL, decode JSONL line-by-line, and pass us the slice.
func (i *ShopifyIngester) IngestBulkItems(ctx context.Context, tenantID string, items []ShopifyItem) (*IngestResult, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("ingest shopify: empty tenant_id")
	}
	inboxRes, err := i.inbox.WriteFromShopifyBulk(ctx, tenantID, items)
	if err != nil {
		return nil, fmt.Errorf("ingest shopify: inbox write: %w", err)
	}
	applyRes, err := i.orchestrator.ManualSync(ctx, tenantID)
	if err != nil {
		// Inbox state is preserved — re-running ManualSync later will catch up.
		return &IngestResult{Inbox: inboxRes}, fmt.Errorf("ingest shopify: apply: %w", err)
	}
	return &IngestResult{Inbox: inboxRes, Apply: applyRes}, nil
}

// IngestSingleWebhook is for the products/<verb> webhook event. Routes
// through UpdateOrchestrator.OnWebhook so the 24h rate-limit applies.
func (i *ShopifyIngester) IngestSingleWebhook(ctx context.Context, tenantID, gid, verb string, raw json.RawMessage) (string, error) {
	return i.orchestrator.OnWebhook(ctx, WebhookEvent{
		TenantID:   tenantID,
		Source:     "shopify",
		ExternalID: gid,
		Verb:       verb,
		Payload:    raw,
	})
}

// IngestResult bundles inbox-write counts and apply counts so callers can
// log a single summary line.
type IngestResult struct {
	Inbox *InboxWriteResult `json:"inbox"`
	Apply *ApplyResult      `json:"apply,omitempty"`
}
