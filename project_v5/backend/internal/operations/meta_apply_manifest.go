package operations

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ApplyManifestExecutor — the CONTROL meta-operation (RUNTIME_SPEC.md §4.3):
// flips the staged steps proposed→accepted and drives the deterministic
// ManifestApplier (usecases/manifest_apply.go). Returns a per-step digest as
// the LLM-facing summary; a failed step surfaces as an error outcome so the
// agent relays it and can retry (the applier re-runs from the failed step —
// every step executor is idempotent, R22).
//
// After every run the synthetic `manifestStep` EntitySet (R23) is refreshed
// in the Entities zone — the data path for the manifest_summary preset —
// and, once issue_surface_urls applies, a `surfaceLink` set carrying both
// URLs for the surface_links preset.

// Synthetic EntitySet slugs the apply digest publishes (compose_turn
// replicate sources).
const (
	manifestStepSetSlug = "manifestStep"
	surfaceLinkSetSlug  = "surfaceLink"
)

// ApplyManifestExecutor implements ports.Executor for apply_manifest.
type ApplyManifestExecutor struct {
	deps MetaExecutorDeps
	tmpl domain.OperationTemplate
}

var _ ports.Executor = (*ApplyManifestExecutor)(nil)

// NewApplyManifestExecutor builds the executor over the meta deps.
func NewApplyManifestExecutor(deps MetaExecutorDeps) *ApplyManifestExecutor {
	return &ApplyManifestExecutor{deps: deps, tmpl: seedRow("apply_manifest")}
}

func (e *ApplyManifestExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *ApplyManifestExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

func (e *ApplyManifestExecutor) SpecForTenant(context.Context, domain.Tenant, map[string]any) (domain.OperationSpec, error) {
	return e.tmpl.Spec(), nil
}

// Execute runs the applier and digests the outcome.
func (e *ApplyManifestExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	if octx.SessionID == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: applying requires a session"), nil
	}
	if e.deps.Applier == nil {
		// Boot wiring gap — fail loud, never pretend the plan ran.
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: manifest applier not wired"), nil
	}

	// Pre-check so an eager apply on an empty session is a self-correctable
	// invalid, not a failed turn.
	staged, err := e.deps.Onboarding.GetOnboarding(ctx, octx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get onboarding manifest: %w", err)
	}
	if staged == nil || len(staged.Steps) == 0 {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid,
			"invalid: nothing staged — stage manifest steps before applying"), nil
	}

	m, err := e.deps.Applier.Apply(ctx, octx.SessionID, stringInput(input, "upTo"))
	if err != nil {
		return nil, fmt.Errorf("apply manifest: %w", err)
	}

	digest := digestManifest(m)

	// Refresh the synthetic sets. Best-effort: the apply already committed;
	// a zone-write failure costs the next render, not the assembled tenant.
	sets := []domain.EntitySet{manifestStepSet(m)}
	if links := surfaceLinkSet(m); links != nil {
		sets = append(sets, *links)
	}
	if werr := writeSyntheticSets(ctx, e.deps, octx, e.tmpl.Name, sets...); werr != nil {
		e.deps.logger().Warn("apply_manifest: synthetic set write failed", "session", octx.SessionID, "err", werr)
	}

	result := &domain.OperationResult{
		Kind:     e.tmpl.Kind,
		Count:    digest.applied,
		Output:   map[string]any{"applied": digest.applied, "total": digest.total},
		Metadata: map[string]any{"steps": digest.stepMeta},
	}
	if digest.storefrontURL != "" {
		result.Output["storefrontUrl"] = digest.storefrontURL
		result.Output["crmUrl"] = digest.crmURL
	}
	switch {
	case digest.failedOp != "":
		result.Outcome = domain.OutcomeError
		result.Summary = fmt.Sprintf("error: apply halted at %s — %s (applied %d/%d; %s)",
			digest.failedOp, digest.failedErr, digest.applied, digest.total, digest.line)
	case len(digest.waiting) > 0:
		result.Outcome = domain.OutcomeOK
		result.Summary = fmt.Sprintf("ok: applied %d/%d, waiting on %s (%s)",
			digest.applied, digest.total, strings.Join(digest.waiting, ", "), digest.line)
	default:
		result.Outcome = domain.OutcomeOK
		result.Summary = fmt.Sprintf("ok: manifest fully applied %d/%d (%s)", digest.applied, digest.total, digest.line)
	}
	return result, nil
}

// NewSyntheticSetPublisher builds the seam the ManifestApplier uses to write
// the manifest's synthetic EntitySets into the session DATA zone from apply
// paths that do NOT run through this executor — today the render-side
// auto-apply (usecases.ManifestApplier.EnsureZeroInputStep, owner's law #1).
//
// It exists because the two halves live in different packages on purpose:
// the applier owns the manifest zone and knows nothing about EntitySets, the
// set builders (manifestStepSet / surfaceLinkSet) live here next to the
// digest that shaped them, and operations must not import usecases. Without
// it, a step applied outside apply_manifest mints its URLs into a step
// Result the renderer cannot bind — the handover card ships empty.
//
// Same best-effort contract as Execute's inline write: the apply already
// committed, so a zone-write failure costs the next render, not the tenant.
func NewSyntheticSetPublisher(state ports.StatePort, log *slog.Logger) func(context.Context, string, *domain.OnboardingManifest) error {
	deps := MetaExecutorDeps{State: state, Log: log}
	return func(ctx context.Context, sessionID string, m *domain.OnboardingManifest) error {
		if m == nil || len(m.Steps) == 0 {
			return nil
		}
		sets := []domain.EntitySet{manifestStepSet(m)}
		if links := surfaceLinkSet(m); links != nil {
			sets = append(sets, *links)
		}
		octx := domain.OperationContext{SessionID: sessionID, ActorID: applierActorID}
		return writeSyntheticSets(ctx, deps, octx, "apply_manifest", sets...)
	}
}

// applierActorID labels data-zone deltas written by the deterministic
// applier rather than by an LLM tool call (mirrors usecases' applierActor).
const applierActorID = "manifest_applier"

// manifestDigest is the flattened per-run outcome.
type manifestDigest struct {
	applied, total int
	waiting        []string // accepted steps (trigger-completed or gated)
	failedOp       string
	failedErr      string
	line           string // per-step "op=status" digest
	storefrontURL  string
	crmURL         string
	stepMeta       []map[string]any
}

// digestManifest folds the applied manifest into counts, the per-step
// digest line and the issued URLs (issue_surface_urls result).
func digestManifest(m *domain.OnboardingManifest) manifestDigest {
	d := manifestDigest{total: len(m.Steps)}
	parts := make([]string, 0, len(m.Steps))
	for i := range m.Steps {
		st := &m.Steps[i]
		part := st.Op + "=" + st.Status
		switch st.Status {
		case domain.ManifestStepApplied, domain.ManifestStepSkipped:
			d.applied++
			if st.Op == "create_tenant" {
				if slug, _ := st.Result["slug"].(string); slug != "" {
					part += " slug=" + slug
				}
			}
			if st.Op == "issue_surface_urls" {
				d.storefrontURL, _ = st.Result["storefrontUrl"].(string)
				d.crmURL, _ = st.Result["crmUrl"].(string)
				if d.storefrontURL != "" {
					part += " storefront=" + d.storefrontURL + " crm=" + d.crmURL
				}
			}
		case domain.ManifestStepAccepted:
			d.waiting = append(d.waiting, st.Op)
		case domain.ManifestStepFailed:
			d.failedOp = st.Op
			d.failedErr = st.Error
		}
		parts = append(parts, part)
		d.stepMeta = append(d.stepMeta, map[string]any{"stepId": st.ID, "op": st.Op, "status": st.Status})
	}
	d.line = strings.Join(parts, "; ")
	return d
}

// manifestStepSet builds the synthetic manifestStep EntitySet — the
// manifest_summary preset's data ("вот всё твоё ПО"). Data keys (camelCase,
// §6.2): op, title, status, statusLabel, detail.
func manifestStepSet(m *domain.OnboardingManifest) domain.EntitySet {
	fields := []domain.FieldDef{
		{Key: "op", Label: "Operation", Type: domain.FieldText},
		{Key: "title", Label: "Step", Type: domain.FieldText},
		{Key: "status", Label: "Status", Type: domain.FieldText},
		{Key: "statusLabel", Label: "Status", Type: domain.FieldText},
		{Key: "detail", Label: "Detail", Type: domain.FieldText},
	}
	records := make([]domain.EntityRecord, 0, len(m.Steps))
	for i := range m.Steps {
		st := &m.Steps[i]
		records = append(records, domain.EntityRecord{
			ID:         st.ID,
			EntitySlug: manifestStepSetSlug,
			Data: map[string]any{
				"op":          st.Op,
				"title":       metaOpTitle(st.Op),
				"status":      st.Status,
				"statusLabel": manifestStatusLabel(st.Status),
				"detail":      manifestStepDetail(st),
			},
		})
	}
	return domain.EntitySet{
		Slug:      manifestStepSetSlug,
		Name:      "Manifest steps",
		Fields:    fields,
		Records:   records,
		Synthetic: true,
	}
}

// manifestStatusLabel renders a step status for humans.
func manifestStatusLabel(status string) string {
	switch status {
	case domain.ManifestStepProposed:
		return "Proposed"
	case domain.ManifestStepAccepted:
		return "Waiting"
	case domain.ManifestStepApplied:
		return "Done"
	case domain.ManifestStepFailed:
		return "Failed"
	case domain.ManifestStepSkipped:
		return "Skipped"
	}
	return status
}

// manifestStepDetail derives the one-line human detail a summary row shows.
func manifestStepDetail(st *domain.ManifestStep) string {
	if st.Error != "" {
		return st.Error
	}
	if w, _ := st.Result["waiting"].(string); w != "" {
		return w
	}
	if slug, _ := st.Result["slug"].(string); slug != "" && st.Op == "create_tenant" {
		return "workspace " + slug
	}
	if u, _ := st.Result["storefrontUrl"].(string); u != "" {
		return u
	}
	if st.Op == "seed_demo_data" {
		return seedStepDetail(st)
	}
	return ""
}

// seedStepDetail keeps the seed step honest in the summary card: a step that
// seeded NOTHING (no pack for this business class, or real data already in
// the workspace) otherwise renders as a bare "Done" with no detail, and the
// user is told their workspace is ready while both tabs are empty.
func seedStepDetail(st *domain.ManifestStep) string {
	if notes, ok := st.Result["notes"].([]string); ok && len(notes) > 0 {
		return strings.Join(notes, "; ")
	}
	// Step results round-trip through JSONB — []string comes back as []any.
	if raw, ok := st.Result["notes"].([]any); ok && len(raw) > 0 {
		parts := make([]string, 0, len(raw))
		for _, n := range raw {
			if s, ok := n.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	listings, records := numberDetail(st.Result["listings"]), numberDetail(st.Result["records"])
	if listings == "" && records == "" {
		return ""
	}
	return fmt.Sprintf("%s listings, %s demo records", listings, records)
}

// numberDetail renders a step-result count ("" when the key was absent).
func numberDetail(v any) string {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.Itoa(int(n))
	}
	return ""
}

// surfaceLinkSet builds the synthetic surfaceLink EntitySet once
// issue_surface_urls has applied (nil before) — the surface_links preset's
// data: two records {label, url, surface}.
func surfaceLinkSet(m *domain.OnboardingManifest) *domain.EntitySet {
	var storefront, crm string
	for i := range m.Steps {
		st := &m.Steps[i]
		if st.Op == "issue_surface_urls" && st.Status == domain.ManifestStepApplied {
			storefront, _ = st.Result["storefrontUrl"].(string)
			crm, _ = st.Result["crmUrl"].(string)
		}
	}
	if storefront == "" && crm == "" {
		return nil
	}
	fields := []domain.FieldDef{
		{Key: "label", Label: "Surface", Type: domain.FieldText},
		{Key: "url", Label: "URL", Type: domain.FieldText},
		{Key: "surface", Label: "Kind", Type: domain.FieldText},
	}
	set := &domain.EntitySet{
		Slug:      surfaceLinkSetSlug,
		Name:      "Surface links",
		Fields:    fields,
		Synthetic: true,
	}
	if storefront != "" {
		set.Records = append(set.Records, domain.EntityRecord{
			ID: "storefront", EntitySlug: surfaceLinkSetSlug,
			Data: map[string]any{"label": "Storefront", "url": storefront, "surface": "storefront"},
		})
	}
	if crm != "" {
		set.Records = append(set.Records, domain.EntityRecord{
			ID: "crm", EntitySlug: surfaceLinkSetSlug,
			Data: map[string]any{"label": "CRM", "url": crm, "surface": "crm"},
		})
	}
	return set
}
