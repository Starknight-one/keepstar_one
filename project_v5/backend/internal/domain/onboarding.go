package domain

import "time"

// Onboarding manifest — shared contracts for the onboarding system form
// (RUNTIME_SPEC.md §4.3, rulings R4/R6/R22). The manifest is the declarative
// assembly plan an onboarding session stages: meta-operations append
// ManifestSteps (status proposed) into the v5_chat_session_state.onboarding
// zone; apply_manifest flips them proposed→accepted and the deterministic
// ManifestApplier (M3) runs them in stage order — create_tenant forced
// first, a failed step halts, retry re-runs from it (executors idempotent).
// There is NO DependsOn/topo-sort (R22).

// ManifestStep statuses (closed set).
const (
	ManifestStepProposed = "proposed"
	ManifestStepAccepted = "accepted"
	ManifestStepApplied  = "applied"
	ManifestStepFailed   = "failed"
	ManifestStepSkipped  = "skipped"
)

// ManifestTenant is the tenant the manifest assembles: name/vertical are
// proposed at stage time (create_tenant params); id/slug are filled by the
// applier once the admin service route creates the row.
type ManifestTenant struct {
	Name     string `json:"name"`
	Vertical string `json:"vertical"` // free label, not an enum (§4.3)
	ID       string `json:"id,omitempty"`
	Slug     string `json:"slug,omitempty"`
}

// LibraryHit is one search_library result kept in the manifest's
// libraryContext zone — template identity + the op-card content the beat-2
// operation_card preset binds (R23 synthetic opCard EntitySet).
type LibraryHit struct {
	Name        string         `json:"name"`
	Kind        OperationKind  `json:"kind"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Card        map[string]any `json:"card,omitempty"` // {input_summary, does, output_summary, why}
}

// ManifestStep is one staged assembly step. Params are PII-redacted for
// register_user (R6: credentials never enter state/deltas/LLM). Attempt
// counts applier runs; AppliedAt stamps the successful one.
type ManifestStep struct {
	ID        string         `json:"id"`
	Op        string         `json:"op"`
	Params    map[string]any `json:"params,omitempty"`
	Status    string         `json:"status"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Attempt   int            `json:"attempt,omitempty"`
	AppliedAt *time.Time     `json:"appliedAt,omitempty"`
}

// OnboardingManifest is the assembly plan stored in
// v5_chat_session_state.onboarding (§3.3). All truth lives in the DB row —
// GET /api/v1/onboard/session resumes from it (refresh-safe).
type OnboardingManifest struct {
	Version        int            `json:"version"`
	Tenant         ManifestTenant `json:"tenant"`
	Steps          []ManifestStep `json:"steps"`
	LibraryContext []LibraryHit   `json:"libraryContext,omitempty"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}
