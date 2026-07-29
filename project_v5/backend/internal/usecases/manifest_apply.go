package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ManifestApplier (RUNTIME_SPEC.md §4.3, R22) — the deterministic executor
// of the onboarding manifest. Meta-operations STAGE steps; nothing mutates
// the world until apply_manifest runs this. Steps run in stage order with
// create_tenant forced first and issue_surface_urls forced last; a failed
// step halts the run; a retry re-runs from it — every step executor is
// idempotent. There is NO DependsOn/topo-sort (R22).
//
// Step targets:
//   - create_tenant        → AdminGateway + brand-default theme via ThemePort (R22)
//   - define_entity        → EntityPort in-process
//   - define_value_set     → EntityPort in-process
//   - define_automation    → EntityPort in-process (operation fixed = notify)
//   - enable_operations    → OperationStorePort.EnableOperation (upsert)
//   - adopt_presets        → AdminGateway, then binding verification (§6.2)
//   - issue_ingest_door    → mints a v5_ingest_tokens row, rests at accepted;
//     the upload endpoints complete it (R25)
//   - register_user        → rests at accepted; completed by ExecuteStep via
//     the step-submit endpoint (R6 — credentials NEVER enter this struct's
//     persisted state)
//   - issue_surface_urls   → refuses (waits) until every other step applied,
//     then mints the CRM surface token and resolves both URLs
//
// Every step transition is persisted through OnboardingStatePort →
// onboarding-zone delta (the curator-visible assembly log). Delta params
// carry ONLY {stepId, op, status} — never step params (R6 discipline).

// Manifest op wire names — must match the seeded meta templates
// (internal/operations/seed/templates.go).
const (
	opCreateTenant      = "create_tenant"
	opDefineEntity      = "define_entity"
	opDefineValueSet    = "define_value_set"
	opDefineAutomation  = "define_automation"
	opEnableOperations  = "enable_operations"
	opAdoptPresets      = "adopt_presets"
	opIssueIngestDoor   = "issue_ingest_door"
	opRegisterUser      = "register_user"
	opIssueSurfaceURLs  = "issue_surface_urls"
	applierActor        = "manifest_applier"
	defaultRegisterRole = "owner"
)

// Sentinel errors — the onboarding handlers map these onto HTTP statuses.
var (
	ErrNoManifest           = errors.New("no manifest staged for this session")
	ErrStepNotFound         = errors.New("manifest step not found")
	ErrStepNotSubmittable   = errors.New("step does not accept a direct submit")
	ErrStepNotReady         = errors.New("step not ready: apply the manifest first")
	ErrTenantNotProvisioned = errors.New("tenant not provisioned: create_tenant has not applied")
	ErrInvalidSubmit        = errors.New("invalid submit payload")
)

// ManifestApplierConfig wires the applier. Registry is optional (spec-cache
// invalidation after tenant mutations); VerifyBindings is the §6.2 seam —
// nil logs a loud warning instead of verifying (the extended preview with
// bindingReport is a surfaces-workstream deliverable).
type ManifestApplierConfig struct {
	State    ports.OnboardingStatePort
	Tokens   ports.OnboardingTokenPort
	Gateway  ports.AdminGatewayPort
	Entities ports.EntityPort
	Ops      ports.OperationStorePort
	Themes   ports.ThemePort
	Registry ports.OperationRegistry
	// SurfaceBaseURL is the public v5 origin the issued URLs are built on,
	// e.g. https://v5-engine-dev.up.railway.app (env PUBLIC_BASE_URL).
	SurfaceBaseURL string
	// VerifyBindings, when set, fails the adopt_presets step on unresolved
	// required bindings (§6.2 R-validator).
	VerifyBindings func(ctx context.Context, tenantSlug string, presets []string) error
	Log            *slog.Logger
}

// ManifestApplier applies staged onboarding manifests. Construct with
// NewManifestApplier.
//
// Concurrency (m3 security review, 2026-07-28): every public entry point is
// serialized PER SESSION via sessionLocks. All of them run a load→mutate→
// persist cycle over the manifest zone, and the owner-decided auto-apply
// means a double-clicked registration submit or a concurrent upload+submit
// would BOTH see the manifest proposed and BOTH run Apply — two
// CreateTenant calls, two tenants (admin suffixes slugs), a split-brain
// manifest. An in-process lock is the correct scope: dev services are
// pinned to replicas=1 (§6.1). The map grows one mutex per onboarding
// session — bounded by session count, trivial at any realistic scale.
type ManifestApplier struct {
	cfg ManifestApplierConfig
	log *slog.Logger

	sessionLocks sync.Map // sessionID → *sync.Mutex
}

// lockSession serializes manifest work for one session; returns the unlock.
func (ap *ManifestApplier) lockSession(sessionID string) func() {
	v, _ := ap.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// NewManifestApplier constructs the applier.
func NewManifestApplier(cfg ManifestApplierConfig) *ManifestApplier {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &ManifestApplier{cfg: cfg, log: log}
}

// Apply runs the staged manifest for the session: flips proposed→accepted
// (up to and including upTo when given, in applier order), then executes
// each non-applied step in order. A step failure is RECORDED on the
// manifest (status failed + error) and halts the run — Apply still returns
// the manifest with a nil error so the caller can digest it; a non-nil
// error means infrastructure failure (manifest could not be loaded or
// persisted).
func (ap *ManifestApplier) Apply(ctx context.Context, sessionID, upTo string) (*domain.OnboardingManifest, error) {
	defer ap.lockSession(sessionID)()
	return ap.applyLocked(ctx, sessionID, upTo)
}

// applyLocked is Apply's worker — caller MUST hold the session lock.
func (ap *ManifestApplier) applyLocked(ctx context.Context, sessionID, upTo string) (*domain.OnboardingManifest, error) {
	m, err := ap.cfg.State.GetOnboarding(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if m == nil || len(m.Steps) == 0 {
		return nil, ErrNoManifest
	}

	order := orderedStepIndices(m)

	// Accept phase: everything targeted by this run flips proposed→accepted.
	targeted := make(map[string]bool, len(order))
	flipped := false
	for _, i := range order {
		st := &m.Steps[i]
		targeted[st.ID] = true
		if st.Status == domain.ManifestStepProposed {
			st.Status = domain.ManifestStepAccepted
			flipped = true
		}
		if upTo != "" && st.ID == upTo {
			break
		}
	}
	if flipped {
		if err := ap.persist(ctx, sessionID, m, "apply_manifest", ""); err != nil {
			return nil, err
		}
	}

	// Execution phase: stage order, create_tenant first, surface URLs last.
	mutated := false
	for _, i := range order {
		st := &m.Steps[i]
		if !targeted[st.ID] {
			continue
		}
		if st.Status == domain.ManifestStepApplied || st.Status == domain.ManifestStepSkipped {
			continue
		}
		st.Attempt++
		result, resting, stepErr := ap.runStep(ctx, sessionID, m, st)
		if stepErr != nil {
			st.Status = domain.ManifestStepFailed
			st.Error = stepErr.Error()
			ap.log.Warn("manifest step failed — halting run",
				"session", sessionID, "step", st.ID, "op", st.Op, "err", stepErr)
			if err := ap.persist(ctx, sessionID, m, st.Op, st.ID); err != nil {
				return nil, err
			}
			break // halt (R22); retry re-runs from this step
		}
		st.Error = ""
		if result != nil {
			st.Result = result
		}
		st.Status = resting
		if resting == domain.ManifestStepApplied {
			now := time.Now()
			st.AppliedAt = &now
		}
		if tenantMutatingOp(st.Op) {
			mutated = true
		}
		if err := ap.persist(ctx, sessionID, m, st.Op, st.ID); err != nil {
			return nil, err
		}
	}

	// Applied definitions/operations/presets change the tenant's agent-facing
	// spec surface → drop the registry spec cache (§6.1).
	if mutated && ap.cfg.Registry != nil && m.Tenant.Slug != "" {
		ap.cfg.Registry.InvalidateTenant(m.Tenant.Slug)
	}
	return m, nil
}

// ExecuteStep completes ONE step out of band — v1: the register_user step,
// invoked by the step-submit endpoint (R6). payload carries the submitted
// form values; the password inside it flows ONLY into
// AdminGatewayPort.ProvisionUser and is NEVER written to the manifest,
// deltas or logs. Returns a copy of the completed step.
//
// Owner decision 2026-07-28: the user submitting the registration form IS
// the approval — a still-proposed manifest is auto-applied here instead of
// bouncing with "apply the manifest first". The business user's action must
// never depend on the model having called apply_manifest.
func (ap *ManifestApplier) ExecuteStep(ctx context.Context, sessionID, stepID string, payload map[string]any) (*domain.ManifestStep, error) {
	defer ap.lockSession(sessionID)()
	m, err := ap.cfg.State.GetOnboarding(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if m == nil || len(m.Steps) == 0 {
		return nil, ErrNoManifest
	}
	idx := locateStep(m, stepID)
	if idx < 0 && (stepID == opRegisterUser || strings.HasPrefix(stepID, opRegisterUser+"-")) {
		// The registration form exists on screen, so the intent is proven —
		// but the model may not have staged a register_user step (dice).
		// Owner's law: a user action must never depend on the model having
		// called a tool. Stage the step deterministically and continue.
		staged := domain.ManifestStep{
			ID:     fmt.Sprintf("%s-%d", opRegisterUser, len(m.Steps)+1),
			Op:     opRegisterUser,
			Status: domain.ManifestStepProposed,
			Params: map[string]any{"role": defaultRegisterRole},
		}
		m.Steps = append(m.Steps, staged)
		idx = len(m.Steps) - 1
		// PERSIST before the auto-apply path below: Apply reloads the
		// manifest from state, and an in-memory-only step would vanish.
		if err := ap.persist(ctx, sessionID, m, opRegisterUser, staged.ID); err != nil {
			return nil, err
		}
	}
	if idx < 0 {
		return nil, ErrStepNotFound
	}
	st := &m.Steps[idx]
	if st.Op != opRegisterUser {
		return nil, ErrStepNotSubmittable
	}
	if st.Status == domain.ManifestStepApplied {
		cp := *st // already registered — idempotent replay
		return &cp, nil
	}
	if st.Status == domain.ManifestStepProposed {
		// Auto-apply (see doc comment): run the staged manifest now, then
		// carry on with the registration against the fresh manifest.
		applied, err := ap.applyLocked(ctx, sessionID, "")
		if err != nil {
			return nil, err
		}
		m = applied
		idx = locateStep(m, stepID)
		if idx < 0 {
			return nil, ErrStepNotFound
		}
		st = &m.Steps[idx]
		if st.Status == domain.ManifestStepProposed {
			// Defensive: Apply flips every targeted step off proposed; if the
			// step still is, surface the halting step's reason, not a shrug.
			return nil, stepNotReadyErr(m)
		}
	}
	if m.Tenant.Slug == "" {
		// The auto-apply halted before create_tenant could provision — the
		// 409 must say WHY (the failed step's reason), owner 2026-07-28.
		return nil, tenantNotProvisionedErr(m)
	}

	email := strings.TrimSpace(stringField(payload, "email"))
	password := stringField(payload, "password")
	if email == "" || password == "" {
		return nil, fmt.Errorf("%w: email and password are required", ErrInvalidSubmit)
	}
	role := stringField(st.Params, "role")
	if role == "" {
		role = defaultRegisterRole
	}

	userID, err := ap.cfg.Gateway.ProvisionUser(ctx, m.Tenant.Slug, email, password, role)
	if err != nil {
		// Stays accepted so the form can be re-submitted (mirror of R25 for
		// uploads). The recorded error is admin's RESPONSE message — the
		// request (and its password) is never echoed anywhere.
		st.Error = "registration failed: " + err.Error()
		if perr := ap.persist(ctx, sessionID, m, st.Op, st.ID); perr != nil {
			ap.log.Warn("register step error persist failed", "session", sessionID, "err", perr)
		}
		return nil, fmt.Errorf("provision user: %w", err)
	}

	scrubCredentialKeys(st.Params) // R6 belt-and-braces
	st.Error = ""
	st.Status = domain.ManifestStepApplied
	now := time.Now()
	st.AppliedAt = &now
	st.Result = map[string]any{"userId": userID, "email": email, "role": role}
	if err := ap.persist(ctx, sessionID, m, st.Op, st.ID); err != nil {
		return nil, err
	}
	cp := *st
	return &cp, nil
}

// RecordUploadJob stamps the accepted upload job onto the issue_ingest_door
// step so the poll endpoint (and the next agent turn) can find it.
func (ap *ManifestApplier) RecordUploadJob(ctx context.Context, sessionID, jobID, fileName string) error {
	return ap.mutateIngestStep(ctx, sessionID, func(st *domain.ManifestStep) {
		if st.Result == nil {
			st.Result = map[string]any{}
		}
		st.Result["jobId"] = jobID
		st.Result["fileName"] = fileName
		delete(st.Result, "lastError")
	}, false)
}

// CompleteIngestStep marks the issue_ingest_door step applied with the
// honest import outcome and consumes the upload token. Idempotent: an
// already-applied step is left untouched.
func (ap *ManifestApplier) CompleteIngestStep(ctx context.Context, sessionID string, status *ports.AdminImportStatus) error {
	return ap.mutateIngestStep(ctx, sessionID, func(st *domain.ManifestStep) {
		if st.Result == nil {
			st.Result = map[string]any{}
		}
		st.Result["processed"] = status.Processed
		st.Result["projectionRows"] = status.ProjectionRows
		st.Result["invalidated"] = status.Invalidated
		if len(status.Errors) > 0 {
			st.Result["errors"] = status.Errors
		}
		delete(st.Result, "lastError")
		st.Error = ""
		st.Status = domain.ManifestStepApplied
		now := time.Now()
		st.AppliedAt = &now
		if tok, _ := st.Result["token"].(string); tok != "" && ap.cfg.Tokens != nil {
			if err := ap.cfg.Tokens.MarkIngestTokenUsed(ctx, tok); err != nil {
				ap.log.Warn("mark ingest token used failed", "session", sessionID, "err", err)
			}
		}
	}, true)
}

// RecordIngestFailure records a failed import on the step WITHOUT changing
// its status — the step stays accepted so the business can re-upload (R25).
func (ap *ManifestApplier) RecordIngestFailure(ctx context.Context, sessionID string, errs []string) error {
	return ap.mutateIngestStep(ctx, sessionID, func(st *domain.ManifestStep) {
		if st.Result == nil {
			st.Result = map[string]any{}
		}
		st.Result["lastError"] = strings.Join(errs, "; ")
	}, false)
}

// ResolveIngestToken returns a usable ingest token for the session's
// issue_ingest_door step — the server-side door for uploads that arrive
// WITHOUT a (valid) token. Owner decision 2026-07-28: the user uploading a
// file IS the approval — a still-proposed manifest is auto-applied here
// (which mints the door); a stale/consumed recorded token is re-minted and
// stamped back onto the step so the next poll/turn sees it. Never bounces
// on "apply the manifest first".
func (ap *ManifestApplier) ResolveIngestToken(ctx context.Context, sessionID string) (*ports.IngestToken, error) {
	defer ap.lockSession(sessionID)()
	m, err := ap.cfg.State.GetOnboarding(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if m == nil || len(m.Steps) == 0 {
		return nil, ErrNoManifest
	}
	idx := findStepIndexByOp(m, opIssueIngestDoor)
	if idx < 0 {
		// The uploader is on screen and a file just arrived — the intent is
		// proven. Same law as register_user: never bounce a user action
		// because the model didn't stage the step. Stage it, PERSIST it
		// (the apply path reloads from state), and continue.
		m.Steps = append(m.Steps, domain.ManifestStep{
			ID:     fmt.Sprintf("%s-%d", opIssueIngestDoor, len(m.Steps)+1),
			Op:     opIssueIngestDoor,
			Status: domain.ManifestStepProposed,
			Params: map[string]any{"formats": []any{"csv", "json"}},
		})
		idx = len(m.Steps) - 1
		if err := ap.persist(ctx, sessionID, m, opIssueIngestDoor, m.Steps[idx].ID); err != nil {
			return nil, err
		}
	}
	if tok := ap.liveIngestToken(ctx, &m.Steps[idx], m.Tenant.Slug); tok != nil {
		return tok, nil
	}

	// Door never ran (manifest still proposed) → the upload is the approval:
	// apply now, which mints the door token.
	if m.Steps[idx].Status == domain.ManifestStepProposed {
		if m, err = ap.applyLocked(ctx, sessionID, ""); err != nil {
			return nil, err
		}
		idx = findStepIndexByOp(m, opIssueIngestDoor)
		if idx < 0 {
			return nil, fmt.Errorf("%w: no issue_ingest_door step staged", ErrStepNotFound)
		}
		if tok := ap.liveIngestToken(ctx, &m.Steps[idx], m.Tenant.Slug); tok != nil {
			return tok, nil
		}
	}

	// Minting needs the tenant; a halted apply surfaces the failed step's
	// reason (owner 2026-07-28: say WHY).
	if m.Tenant.ID == "" || m.Tenant.Slug == "" {
		return nil, tenantNotProvisionedErr(m)
	}

	// Recorded token is stale/consumed (or unreadable) — mint a fresh one and
	// stamp it on the step (persisted, so the armed uploader re-render and the
	// poll endpoint agree on the same token).
	st := &m.Steps[idx]
	formats := ingestFormats(st.Params)
	if len(formats) == 0 {
		formats = []string{"csv", "json"} // v5_ingest_tokens table default (§3.3)
	}
	t, err := ap.cfg.Tokens.MintIngestToken(ctx, sessionID, m.Tenant.ID, formats, 0)
	if err != nil {
		return nil, err
	}
	if t.TenantSlug == "" {
		t.TenantSlug = m.Tenant.Slug // mint returns the row; slug is manifest truth
	}
	if st.Result == nil {
		st.Result = map[string]any{}
	}
	st.Result["token"] = t.Token
	st.Result["formats"] = t.Formats
	st.Result["expiresAt"] = t.ExpiresAt.Format(time.RFC3339)
	if err := ap.persist(ctx, sessionID, m, st.Op, st.ID); err != nil {
		return nil, err
	}
	return t, nil
}

// liveIngestToken resolves the step's recorded token when it is still usable
// (known, unexpired, unconsumed); nil otherwise. tenantSlug backfills the
// admin-route address when the store did not resolve it.
func (ap *ManifestApplier) liveIngestToken(ctx context.Context, st *domain.ManifestStep, tenantSlug string) *ports.IngestToken {
	tokStr, _ := st.Result["token"].(string)
	if tokStr == "" || ap.cfg.Tokens == nil {
		return nil
	}
	tok, err := ap.cfg.Tokens.GetIngestToken(ctx, tokStr)
	if err != nil || !tok.Valid(time.Now()) {
		return nil
	}
	if tok.TenantSlug == "" {
		tok.TenantSlug = tenantSlug
	}
	return tok
}

// mutateIngestStep loads the manifest, applies fn to the issue_ingest_door
// step and persists. skipIfApplied short-circuits idempotent completions.
// Serialized per session (it is only ever called from the public
// RecordUploadJob / CompleteIngestStep / RecordIngestFailure entry points).
func (ap *ManifestApplier) mutateIngestStep(ctx context.Context, sessionID string, fn func(*domain.ManifestStep), skipIfApplied bool) error {
	defer ap.lockSession(sessionID)()
	m, err := ap.cfg.State.GetOnboarding(ctx, sessionID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrNoManifest
	}
	idx := -1
	for i := range m.Steps {
		if m.Steps[i].Op == opIssueIngestDoor {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrStepNotFound
	}
	st := &m.Steps[idx]
	if skipIfApplied && st.Status == domain.ManifestStepApplied {
		return nil
	}
	fn(st)
	return ap.persist(ctx, sessionID, m, st.Op, st.ID)
}

// --- step executors ---

// runStep dispatches one step. Returns (result, resting status, error);
// resting is applied for terminal steps, accepted for the two steps that
// complete via their trigger endpoints (§4.3).
func (ap *ManifestApplier) runStep(ctx context.Context, sessionID string, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	switch st.Op {
	case opCreateTenant:
		return ap.applyCreateTenant(ctx, m, st)
	case opDefineEntity:
		return ap.applyDefineEntity(ctx, m, st)
	case opDefineValueSet:
		return ap.applyDefineValueSet(ctx, m, st)
	case opDefineAutomation:
		return ap.applyDefineAutomation(ctx, m, st)
	case opEnableOperations:
		return ap.applyEnableOperations(ctx, m, st)
	case opAdoptPresets:
		return ap.applyAdoptPresets(ctx, m, st)
	case opIssueIngestDoor:
		return ap.applyIssueIngestDoor(ctx, sessionID, m, st)
	case opRegisterUser:
		// R6: rests at accepted; ExecuteStep (step-submit) completes it.
		scrubCredentialKeys(st.Params)
		role := stringField(st.Params, "role")
		if role == "" {
			role = defaultRegisterRole
		}
		return map[string]any{"role": role}, domain.ManifestStepAccepted, nil
	case opIssueSurfaceURLs:
		return ap.applyIssueSurfaceURLs(ctx, m, st)
	default:
		return nil, "", fmt.Errorf("unknown manifest op %q", st.Op)
	}
}

func (ap *ManifestApplier) applyCreateTenant(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	name := stringField(st.Params, "name")
	vertical := stringField(st.Params, "vertical")
	if name == "" || vertical == "" {
		return nil, "", fmt.Errorf("create_tenant: name and vertical are required")
	}
	// Idempotency guard: a re-run after a crash must NOT create a second
	// tenant (admin suffixes slugs on collision) — the manifest already
	// carries the provisioned identity.
	if m.Tenant.ID == "" || m.Tenant.Slug == "" {
		if ap.cfg.Gateway == nil {
			return nil, "", errors.New("create_tenant: admin gateway not wired")
		}
		t, err := ap.cfg.Gateway.CreateTenant(ctx, name, vertical)
		if err != nil {
			return nil, "", err
		}
		m.Tenant.ID, m.Tenant.Slug = t.TenantID, t.Slug
		m.Tenant.Name, m.Tenant.Vertical = name, vertical
	}
	// R22: the applier writes the brand-default theme in-process — the
	// design_system_preview widget renders exactly these tokens, no
	// staged-vs-live lie. Explicit write, so the tenant row exists even
	// though the values equal the defaults.
	if ap.cfg.Themes == nil {
		return nil, "", errors.New("create_tenant: theme port not wired")
	}
	if err := ap.cfg.Themes.UpsertTheme(ctx, m.Tenant.ID, domain.DefaultThemeTokens()); err != nil {
		return nil, "", fmt.Errorf("write brand-default theme: %w", err)
	}
	return map[string]any{"tenantId": m.Tenant.ID, "slug": m.Tenant.Slug}, domain.ManifestStepApplied, nil
}

func (ap *ManifestApplier) applyDefineEntity(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	var def domain.EntityDefinition
	if err := decodeInto(st.Params["entity"], &def); err != nil {
		return nil, "", fmt.Errorf("define_entity: %w", err)
	}
	if def.Slug == "" || def.Name == "" || len(def.Fields) == 0 {
		return nil, "", fmt.Errorf("define_entity: slug, name and fields are required")
	}
	def.TenantID = m.Tenant.ID
	// The define_entity schema has no statusField (the model cannot set it),
	// so every model-built tenant landed with status_field NULL and a dead
	// transition_status — the engine infers it from the fields instead.
	if def.StatusField == "" {
		if inferred := inferStatusField(def.Fields); inferred != "" {
			def.StatusField = inferred
			ap.log.Info("define_entity: status field inferred",
				"tenant", m.Tenant.Slug, "entity", def.Slug, "statusField", inferred)
		}
	}
	if err := ap.cfg.Entities.UpsertEntityDefinition(ctx, &def); err != nil {
		return nil, "", err
	}
	out := map[string]any{"entity": def.Slug, "fields": len(def.Fields)}
	if def.StatusField != "" {
		out["statusField"] = def.StatusField
	}
	return out, domain.ManifestStepApplied, nil
}

func (ap *ManifestApplier) applyDefineValueSet(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	var vs domain.ValueSet
	if err := decodeInto(st.Params, &vs); err != nil {
		return nil, "", fmt.Errorf("define_value_set: %w", err)
	}
	if vs.Slug == "" || vs.Name == "" || len(vs.Values) == 0 {
		return nil, "", fmt.Errorf("define_value_set: slug, name and values are required")
	}
	vs.TenantID = m.Tenant.ID
	if err := ap.cfg.Entities.UpsertValueSet(ctx, &vs); err != nil {
		return nil, "", err
	}
	return map[string]any{"valueSet": vs.Slug, "values": len(vs.Values)}, domain.ManifestStepApplied, nil
}

func (ap *ManifestApplier) applyDefineAutomation(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	a := domain.Automation{
		TenantID:      m.Tenant.ID,
		Name:          stringField(st.Params, "name"),
		EventType:     stringField(st.Params, "event_type"),
		EntitySlug:    stringField(st.Params, "entity"),
		OperationSlug: "notify", // fixed in v1 (§4.3)
		Enabled:       true,
	}
	if p, ok := st.Params["params"].(map[string]any); ok {
		a.OperationParams = p
	}
	if raw, ok := st.Params["predicate"]; ok && raw != nil {
		var pred domain.AutomationPredicate
		if err := decodeInto(raw, &pred); err != nil {
			return nil, "", fmt.Errorf("define_automation: predicate: %w", err)
		}
		if pred.Field != "" {
			a.Predicate = &pred
		}
	}
	if a.Name == "" || a.EventType == "" || len(a.OperationParams) == 0 {
		return nil, "", fmt.Errorf("define_automation: name, event_type and params are required")
	}
	if err := ap.cfg.Entities.UpsertAutomation(ctx, &a); err != nil {
		return nil, "", err
	}
	return map[string]any{"automation": a.Name, "eventType": a.EventType}, domain.ManifestStepApplied, nil
}

func (ap *ManifestApplier) applyEnableOperations(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	items, ok := st.Params["operations"].([]any)
	if !ok || len(items) == 0 {
		return nil, "", fmt.Errorf("enable_operations: operations[] is required")
	}
	enabled := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("enable_operations: malformed operations[] entry")
		}
		template := stringField(item, "template")
		instance := stringField(item, "instance")
		if template == "" || instance == "" {
			return nil, "", fmt.Errorf("enable_operations: template and instance are required")
		}
		cfg, _ := item["config"].(map[string]any)
		// The model writes config in the business's vocabulary and nothing
		// guarantees the named fields exist on the entity (live run:
		// datetime_field "viewingTime" on a lead that had no datetime field
		// at all → every booking invalid). Reconcile deterministically.
		cfg = ap.reconcileOperationEntity(ctx, m.Tenant.ID, instance, cfg)
		// EnableOperation upserts on (tenant_id, name) — a retry re-run is
		// safe and converges on the same config.
		if _, err := ap.cfg.Ops.EnableOperation(ctx, m.Tenant.ID, template, instance, cfg); err != nil {
			return nil, "", err
		}
		enabled = append(enabled, instance)
	}
	return map[string]any{"enabled": enabled}, domain.ManifestStepApplied, nil
}

func (ap *ManifestApplier) applyAdoptPresets(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	rawNames, ok := st.Params["presets"].([]any)
	if !ok || len(rawNames) == 0 {
		return nil, "", fmt.Errorf("adopt_presets: presets[] is required")
	}
	names := make([]string, 0, len(rawNames))
	for _, r := range rawNames {
		if s, ok := r.(string); ok && s != "" {
			names = append(names, s)
		}
	}
	if len(names) == 0 {
		return nil, "", fmt.Errorf("adopt_presets: presets[] is required")
	}
	// Adopt name-by-name and never halt the whole environment over a
	// cosmetic miss: the model sometimes invents preset names (live smoke:
	// "lead_cards"). Unknown/failed names are recorded as skipped — system
	// default presets still render — and the apply chain continues to the
	// steps that make the workspace live (surface URLs).
	adopted := make([]string, 0, len(names))
	skipped := make([]string, 0)
	invalidated := false
	for _, name := range names {
		res, err := ap.cfg.Gateway.AdoptPresets(ctx, m.Tenant.Slug, []string{name})
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		adopted = append(adopted, res.Adopted...)
		invalidated = invalidated || res.Invalidated
	}
	if len(skipped) > 0 {
		ap.log.Warn("adopt_presets: some presets skipped", "tenant", m.Tenant.Slug, "skipped", skipped)
	}
	// §6.2 R-validator: unresolved required bindings FAIL the step loudly
	// rather than rendering broken UI. The seam is injected because the
	// extended preview (bindingReport) is a surfaces-workstream deliverable.
	if len(adopted) > 0 && ap.cfg.VerifyBindings != nil {
		if err := ap.cfg.VerifyBindings(ctx, m.Tenant.Slug, adopted); err != nil {
			return nil, "", fmt.Errorf("binding verification: %w", err)
		}
	} else if ap.cfg.VerifyBindings == nil {
		ap.log.Warn("adopt_presets: binding verification unavailable — step applied WITHOUT §6.2 validation",
			"tenant", m.Tenant.Slug, "presets", adopted)
	}
	out := map[string]any{"adopted": adopted, "invalidated": invalidated}
	if len(skipped) > 0 {
		out["skipped"] = skipped
	}
	return out, domain.ManifestStepApplied, nil
}

func (ap *ManifestApplier) applyIssueIngestDoor(ctx context.Context, sessionID string, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	// Idempotent re-run: keep the already-minted token (the rendered armed
	// uploader may already carry it).
	if tok, _ := st.Result["token"].(string); tok != "" {
		return st.Result, domain.ManifestStepAccepted, nil
	}
	t, err := ap.cfg.Tokens.MintIngestToken(ctx, sessionID, m.Tenant.ID, ingestFormats(st.Params), 0)
	if err != nil {
		return nil, "", err
	}
	return map[string]any{
		"token":     t.Token,
		"formats":   t.Formats,
		"expiresAt": t.ExpiresAt.Format(time.RFC3339),
	}, domain.ManifestStepAccepted, nil
}

func (ap *ManifestApplier) applyIssueSurfaceURLs(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	// Refuses until every OTHER step has applied (§4.3) — including the two
	// trigger-completed steps. Waiting is a resting state, not a failure:
	// the run does not halt (this step is always last).
	var pending []string
	for i := range m.Steps {
		s := &m.Steps[i]
		if s.Op == opIssueSurfaceURLs {
			continue
		}
		if s.Status != domain.ManifestStepApplied && s.Status != domain.ManifestStepSkipped {
			pending = append(pending, s.Op)
		}
	}
	if len(pending) > 0 {
		res := map[string]any{"waiting": "pending steps: " + strings.Join(pending, ", ")}
		return res, domain.ManifestStepAccepted, nil
	}
	if ap.cfg.SurfaceBaseURL == "" {
		return nil, "", errors.New("issue_surface_urls: surface base URL not configured (PUBLIC_BASE_URL)")
	}
	// Idempotent re-run: keep already-issued URLs.
	if u, _ := st.Result["crmUrl"].(string); u != "" {
		return st.Result, domain.ManifestStepApplied, nil
	}
	token, err := ap.cfg.Tokens.MintSurfaceToken(ctx, m.Tenant.ID)
	if err != nil {
		return nil, "", err
	}
	base := strings.TrimRight(ap.cfg.SurfaceBaseURL, "/")
	return map[string]any{
		"storefrontUrl": base + "/s/" + m.Tenant.Slug,
		"crmUrl":        base + "/crm/" + m.Tenant.Slug + "?k=" + token,
	}, domain.ManifestStepApplied, nil
}

// --- helpers ---

// persist stamps UpdatedAt and writes the manifest zone + its delta. Delta
// params are identity-only ({stepId, op, status}) — step params NEVER enter
// the delta stream (R6).
func (ap *ManifestApplier) persist(ctx context.Context, sessionID string, m *domain.OnboardingManifest, tool, stepID string) error {
	m.UpdatedAt = time.Now()
	params := map[string]any{}
	if stepID == "" {
		params["phase"] = "accepted" // the proposed→accepted flip
	} else {
		params["stepId"] = stepID
		if idx := findStepIndex(m, stepID); idx >= 0 {
			params["op"] = m.Steps[idx].Op
			params["status"] = m.Steps[idx].Status
		}
	}
	info := domain.DeltaInfo{
		Trigger:   domain.TriggerSystem,
		Source:    domain.SourceSystem,
		ActorID:   applierActor,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "onboarding",
		Action:    domain.Action{Type: ports.OnboardingStepAction, Tool: tool, Params: params},
		Result:    domain.ResultMeta{Count: len(m.Steps)},
	}
	if _, err := ap.cfg.State.UpdateOnboarding(ctx, sessionID, m, info); err != nil {
		return fmt.Errorf("persist manifest: %w", err)
	}
	return nil
}

// orderedStepIndices: stage order with create_tenant forced first and
// issue_surface_urls forced last (R22 + §4.3 "runs last"). Stable within
// each group.
func orderedStepIndices(m *domain.OnboardingManifest) []int {
	var first, mid, last []int
	for i := range m.Steps {
		switch m.Steps[i].Op {
		case opCreateTenant:
			first = append(first, i)
		case opIssueSurfaceURLs:
			last = append(last, i)
		default:
			mid = append(mid, i)
		}
	}
	out := make([]int, 0, len(m.Steps))
	out = append(out, first...)
	out = append(out, mid...)
	return append(out, last...)
}

func findStepIndex(m *domain.OnboardingManifest, stepID string) int {
	for i := range m.Steps {
		if m.Steps[i].ID == stepID {
			return i
		}
	}
	return -1
}

// findStepIndexByOp resolves a step by its op wire name — the alias seeded
// form endpoints use (unique per manifest thanks to restage-replace). First
// match wins.
func findStepIndexByOp(m *domain.OnboardingManifest, op string) int {
	for i := range m.Steps {
		if m.Steps[i].Op == op {
			return i
		}
	}
	return -1
}

// locateStep resolves a step by ID first, then by its op wire name. Seeded
// documents address steps by OP (the registration_form seed posts to
// /onboard/step/register_user/submit) while staged step IDs carry an
// ordinal suffix (register_user-7); restage-replace keeps ops unique per
// manifest, so the op name is an unambiguous alias.
func locateStep(m *domain.OnboardingManifest, stepID string) int {
	if idx := findStepIndex(m, stepID); idx >= 0 {
		return idx
	}
	return findStepIndexByOp(m, stepID)
}

// firstFailedStep returns the first failed step in stage order, nil when the
// manifest has none.
func firstFailedStep(m *domain.OnboardingManifest) *domain.ManifestStep {
	for i := range m.Steps {
		if m.Steps[i].Status == domain.ManifestStepFailed {
			return &m.Steps[i]
		}
	}
	return nil
}

// stepNotReadyErr / tenantNotProvisionedErr wrap their sentinels with the
// halting step's reason (owner 2026-07-28: a 409 must answer WHY the flow is
// blocked, never just "apply the manifest first"). errors.Is still matches
// the sentinel, so handler status mapping is unchanged.
func stepNotReadyErr(m *domain.OnboardingManifest) error {
	if f := firstFailedStep(m); f != nil {
		return fmt.Errorf("%w (%s failed: %s)", ErrStepNotReady, f.Op, f.Error)
	}
	return ErrStepNotReady
}

func tenantNotProvisionedErr(m *domain.OnboardingManifest) error {
	if f := firstFailedStep(m); f != nil {
		return fmt.Errorf("%w (%s failed: %s)", ErrTenantNotProvisioned, f.Op, f.Error)
	}
	return ErrTenantNotProvisioned
}

// ingestFormats extracts the issue_ingest_door formats param ([]any of
// strings on the wire).
func ingestFormats(params map[string]any) []string {
	var formats []string
	if raw, ok := params["formats"].([]any); ok {
		for _, f := range raw {
			if s, ok := f.(string); ok {
				formats = append(formats, s)
			}
		}
	}
	return formats
}

// ManifestStatusSummary is the client-facing manifest digest (owner
// 2026-07-28): counts for contextual CTAs — the shell shows an Accept
// control only while staged > 0, a retry affordance + reason when failed >
// 0, nothing when the plan is fully applied. Rides additively on the
// pipeline response (PipelineExecuteResponse.Manifest) and the onboarding
// session resume payload.
type ManifestStatusSummary struct {
	Staged  int `json:"staged"`  // proposed steps awaiting apply
	Applied int `json:"applied"` // applied (or skipped) steps
	Failed  int `json:"failed"`  // failed steps (halted the run)
	Total   int `json:"total"`
	// FailedStep/FailedReason describe the FIRST failed step in stage order;
	// empty when Failed == 0.
	FailedStep   string `json:"failedStep,omitempty"`
	FailedReason string `json:"failedReason,omitempty"`
}

// ManifestStatusOf digests a manifest into its status summary; nil when
// there is no manifest (or no steps) — the wire omits the field.
func ManifestStatusOf(m *domain.OnboardingManifest) *ManifestStatusSummary {
	if m == nil || len(m.Steps) == 0 {
		return nil
	}
	s := &ManifestStatusSummary{Total: len(m.Steps)}
	for i := range m.Steps {
		switch m.Steps[i].Status {
		case domain.ManifestStepProposed:
			s.Staged++
		case domain.ManifestStepApplied, domain.ManifestStepSkipped:
			s.Applied++
		case domain.ManifestStepFailed:
			s.Failed++
			if s.FailedStep == "" {
				s.FailedStep = m.Steps[i].Op
				s.FailedReason = m.Steps[i].Error
			}
		}
	}
	return s
}

func requireTenant(m *domain.OnboardingManifest) error {
	if m.Tenant.ID == "" || m.Tenant.Slug == "" {
		return ErrTenantNotProvisioned
	}
	return nil
}

// tenantMutatingOp reports whether an applied step changed the tenant's
// agent-facing spec surface (registry spec cache scope, §6.1).
func tenantMutatingOp(op string) bool {
	switch op {
	case opDefineEntity, opDefineValueSet, opDefineAutomation, opEnableOperations, opAdoptPresets:
		return true
	}
	return false
}

// scrubCredentialKeys strips credential-class keys from a params map in
// place (R6 belt-and-braces — the register_user input schema has no such
// fields, but an LLM-staged step must never be able to smuggle them into
// persisted state).
func scrubCredentialKeys(params map[string]any) {
	for _, k := range []string{"email", "password", "name"} {
		delete(params, k)
	}
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// decodeInto JSON-round-trips an untyped params value into a typed struct.
func decodeInto(raw any, out any) error {
	if raw == nil {
		return errors.New("missing value")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
