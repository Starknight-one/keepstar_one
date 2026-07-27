package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// NotifyExecutor delivers a staff notification (R12): channel "runtime"
// writes a v5_notifications row through ports.NotificationStore (the CRM
// audience — DDL default); channel "webhook" POSTs the payload to the
// configured URL. Typically run by automations (OperationRunner, Role
// system — min_role gates humans out); {field} placeholders in automation
// params are substituted by the runner BEFORE input validation, so this
// executor only ever sees final strings. No email/SMS pretence in v1.
//
// Config: {"channel":"runtime"} | {"channel":"webhook","url":...}
type NotifyExecutor struct {
	store ports.NotificationStore
	httpc *http.Client
	tmpl  domain.OperationTemplate
}

var _ ports.Executor = (*NotifyExecutor)(nil)

// NewNotifyExecutor wires the executor. httpc may be nil — a 10s-timeout
// client is used for the webhook channel.
func NewNotifyExecutor(store ports.NotificationStore, httpc *http.Client) *NotifyExecutor {
	if httpc == nil {
		httpc = &http.Client{Timeout: 10 * time.Second}
	}
	return &NotifyExecutor{store: store, httpc: httpc, tmpl: seedRow("notify")}
}

func (e *NotifyExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *NotifyExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant is the template — notify's contract is tenant-independent.
func (e *NotifyExecutor) SpecForTenant(_ context.Context, _ domain.Tenant, _ map[string]any) (domain.OperationSpec, error) {
	return e.tmpl.Spec(), nil
}

// Execute delivers on the configured channel. Input arrives validated
// ({title required, body, ref} — §4.2).
func (e *NotifyExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	title, _ := input["title"].(string)
	body, _ := input["body"].(string)
	ref, _ := input["ref"].(string)
	if title == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid, "invalid: title is required"), nil
	}
	if octx.TenantID == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: notify requires a resolved tenant"), nil
	}

	switch channel := cfgString(octx.Config, "channel"); channel {
	case "", "runtime":
		n := &domain.Notification{
			TenantID: octx.TenantID,
			Title:    title,
			Body:     body,
			RecordID: ref,
		}
		if err := e.store.CreateNotification(ctx, n); err != nil {
			return nil, fmt.Errorf("notify: %w", err)
		}
		return &domain.OperationResult{
			Kind:     e.tmpl.Kind,
			Outcome:  domain.OutcomeOK,
			Count:    1,
			RecordID: ref,
			Summary:  "ok: notification stored",
			Output:   map[string]any{"notificationId": n.ID},
			Metadata: map[string]any{"channel": "runtime", "audience": n.Audience},
		}, nil

	case "webhook":
		url := cfgString(octx.Config, "url")
		if url == "" {
			return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: webhook channel configured without url"), nil
		}
		if err := e.postWebhook(ctx, url, map[string]any{
			"tenantId": octx.TenantID, "title": title, "body": body, "ref": ref,
		}); err != nil {
			return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: webhook delivery failed: "+err.Error()), nil
		}
		return &domain.OperationResult{
			Kind:     e.tmpl.Kind,
			Outcome:  domain.OutcomeOK,
			Count:    1,
			RecordID: ref,
			Summary:  "ok: notification posted",
			Metadata: map[string]any{"channel": "webhook"},
		}, nil

	default:
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError,
			fmt.Sprintf("error: unknown notify channel %q", channel)), nil
	}
}

func (e *NotifyExecutor) postWebhook(ctx context.Context, url string, payload map[string]any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
