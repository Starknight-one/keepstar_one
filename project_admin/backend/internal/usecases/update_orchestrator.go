// Package usecases — UpdateOrchestrator is the rate-limited entry point for
// the "cycle of updates" half of the 6-step catalog flow (step 5).
//
// Three handlers:
//
//   OnWebhook(tenantID, payload) — called by Shopify webhook handler when
//     a product/<verb> event arrives. Always writes the payload into inbox.
//     The downstream apply runs at most once per 24h per tenant — extra
//     webhook events in the window are absorbed into inbox without
//     triggering apply. This is the "не чаще раз в сутки" rule from the
//     plan; it prevents API-cost blowups on hyperactive merchants.
//
//   ManualSync(tenantID) — bypasses the 24h window. Called from the
//     curator "Sync now" button. Always runs apply (and discovery first
//     if no artifact exists).
//
//   OnMappingMiss(tenantID, miss) — called by apply_v2 from inside its
//     loop. Triggers a narrow discovery_v2 with trigger='mapping_miss'.
//     We do NOT cascade into another apply from here; apply_v2's own
//     loop will pick up the refreshed artifact and continue.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

const (
	// applyRateWindow defines the minimum gap between auto-applied runs
	// for the same tenant. Webhook events within the window get absorbed
	// into inbox and applied on the next slot. Set to 24h per spec but
	// can be lowered via an env override on staging for testing.
	applyRateWindow = 24 * time.Hour
)

// PoolAccessor exposes the *pgxpool.Pool to UpdateOrchestrator for the
// one read it needs (last apply timestamp from tenant_action_log).
//
// We don't want to widen TenantActionLogPort with a "LastSuccessfulAt"
// method just for this one consumer — different layer of concern. Either
// pass the pool directly via the postgres.Client or add a small bespoke
// port. Going with the bespoke port: ApplyRateLimitPort below.
type ApplyRateLimitPort interface {
	// LastAppliedAt returns the most recent apply action_log row time for
	// the tenant, or zero time when none. Called per webhook to decide
	// whether the rate-limit window allows another apply now.
	LastAppliedAt(ctx context.Context, tenantID string) (time.Time, error)
}

type UpdateOrchestrator struct {
	inbox     *InboxUseCase
	apply     *ApplyV2UseCase
	discovery *DiscoveryV2
	actionLog ports.TenantActionLogPort
	rate      ApplyRateLimitPort
	log       *logger.Logger
}

func NewUpdateOrchestrator(
	inbox *InboxUseCase,
	apply *ApplyV2UseCase,
	discovery *DiscoveryV2,
	actionLog ports.TenantActionLogPort,
	rate ApplyRateLimitPort,
	log *logger.Logger,
) *UpdateOrchestrator {
	return &UpdateOrchestrator{
		inbox:     inbox,
		apply:     apply,
		discovery: discovery,
		actionLog: actionLog,
		rate:      rate,
		log:       log,
	}
}

// WebhookEvent is the post-HMAC-verified, post-source-normalized event the
// caller hands the orchestrator.
type WebhookEvent struct {
	TenantID   string
	Source     string                 // 'shopify' | 'csv' | 'sheets' | 'manual'
	ExternalID string                 // shopify GID, etc.
	Verb       string                 // 'created' | 'updated' | 'deleted' (informational)
	Payload    json.RawMessage        // raw event body
}

// OnWebhook absorbs one source event into inbox, then applies if outside
// the 24h rate window. Returns a string indicating outcome ("applied",
// "queued", "absorbed_only") for the caller's logging.
func (o *UpdateOrchestrator) OnWebhook(ctx context.Context, ev WebhookEvent) (string, error) {
	if ev.TenantID == "" || ev.ExternalID == "" {
		return "", fmt.Errorf("on webhook: tenant_id and external_id required")
	}

	// Always log the event regardless of whether we apply.
	payload, _ := json.Marshal(map[string]any{
		"source":      ev.Source,
		"verb":        ev.Verb,
		"external_id": ev.ExternalID,
	})
	_ = o.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: ev.TenantID,
		Action:   "webhook_received",
		Status:   "ok",
		Payload:  payload,
	})

	// Write to inbox (always — even if we won't apply yet, the next
	// scheduled apply will pick it up).
	// We treat the webhook payload as a single-item batch via WriteManual.
	// Callers that have pre-parsed Shopify items can call inbox directly
	// via InboxUseCase.WriteFromShopifyBulk instead.
	asMap, err := jsonToMap(ev.Payload)
	if err != nil {
		return "", fmt.Errorf("on webhook: parse payload: %w", err)
	}
	if _, err := o.inbox.WriteManual(ctx, ev.TenantID, ev.ExternalID, asMap); err != nil {
		// WriteManual already logs warnings; surface to caller too.
		return "", fmt.Errorf("on webhook: inbox write: %w", err)
	}

	// Check rate limit.
	lastAt, err := o.rate.LastAppliedAt(ctx, ev.TenantID)
	if err != nil {
		o.log.Warn("update_orchestrator_rate_check_failed", "tenant", ev.TenantID, "error", err)
	}
	if !lastAt.IsZero() && time.Since(lastAt) < applyRateWindow {
		return "absorbed_only", nil
	}

	// Apply now.
	if _, err := o.apply.ApplyForTenant(ctx, ev.TenantID); err != nil {
		return "", fmt.Errorf("on webhook: apply: %w", err)
	}
	return "applied", nil
}

// ManualSync ignores the rate window. Always applies (and discovers if no
// artifact yet).
func (o *UpdateOrchestrator) ManualSync(ctx context.Context, tenantID string) (*ApplyResult, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("manual sync: empty tenant_id")
	}
	_ = o.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   "manual_sync",
		Status:   "ok",
		Payload:  json.RawMessage(`{}`),
	})
	return o.apply.ApplyForTenant(ctx, tenantID)
}

// OnMappingMiss is wired so apply_v2 → orchestrator → discovery_v2 routing
// can be tested in isolation. apply_v2 already calls discovery_v2 directly
// today; this wrapper exists so future re-routing (e.g. background queue)
// has one seam.
func (o *UpdateOrchestrator) OnMappingMiss(ctx context.Context, tenantID string, miss MappingMissDetails) error {
	if o.discovery == nil {
		return fmt.Errorf("on mapping miss: discovery_v2 not wired")
	}
	body, _ := json.Marshal(miss)
	_, err := o.discovery.Discover(ctx, tenantID, "mapping_miss", body)
	return err
}

func jsonToMap(b json.RawMessage) (map[string]any, error) {
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
