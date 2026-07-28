//go:build e2e

// Package e2e walks the owner's full 5-beat demo script against LIVE
// services (RUNTIME_SPEC.md §6.6 + owner flow ruling 2026-07-28):
//
//	gate → onboarding session → "I run a realtor agency…" → proposal blocks
//	(registration / uploader / design) → registration step-submit
//	(the USER's action auto-applies the staged manifest — never a 409
//	"apply the manifest first") → CSV + JSON upload → surface URLs from the
//	manifest → storefront search → booking via /operations/invoke
//	(schedule_slot) → CRM "any new leads?" → transition_status.
//
// Env (the test SKIPS when E2E_BASE_URL is unset; any other missing var is
// a hard failure so a half-configured run can never silently pass):
//
//	E2E_BASE_URL        v5 engine origin, e.g. https://v5-engine-dev.up.railway.app
//	E2E_ADMIN_URL       admin origin,     e.g. https://admin-dev-85d4.up.railway.app
//	ONBOARDING_PASSWORD the R5 gate password of the DEPLOYED v5 env
//	ADMIN_SERVICE_KEY   admin service-route key (X-Service-Key)
//	E2E_DATABASE_URL    optional — flat-moon Postgres; enables the R6
//	                    password-at-rest scan and record-row value asserts
//
// Run:
//
//	E2E_BASE_URL=… E2E_ADMIN_URL=… ONBOARDING_PASSWORD=… ADMIN_SERVICE_KEY=… \
//	  go test -tags=e2e -count=1 -timeout 30m -v ./e2e/
//
// The run spends real LLM money and writes the live dev DB. Tenants are
// disposable (no auto-GC v1, spec §8) — the created slug is logged.
//
// Universality note: nothing here hardcodes the realtor vertical beyond the
// FIXTURE content and the first user message. Operation instance names, the
// entity slug, its fields and the status vocabulary are all read back from
// the manifest the agent staged — the same walk works for any business.
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------- wire shapes (mirrors of the handlers' JSON, decode-only) ----------

type turnBlock struct {
	BlockID  string         `json:"blockId"`
	Kind     string         `json:"kind"`
	Text     string         `json:"text,omitempty"`
	Document map[string]any `json:"document,omitempty"`
	Display  string         `json:"display,omitempty"`
}

type manifestStatus struct {
	Staged       int    `json:"staged"`
	Applied      int    `json:"applied"`
	Failed       int    `json:"failed"`
	Total        int    `json:"total"`
	FailedStep   string `json:"failedStep,omitempty"`
	FailedReason string `json:"failedReason,omitempty"`
}

type turnResp struct {
	Document map[string]any  `json:"document"`
	Blocks   []turnBlock     `json:"blocks"`
	Manifest *manifestStatus `json:"manifest,omitempty"`

	raw []byte // full response body — substring assertions scan this
}

type manifestStep struct {
	ID     string         `json:"id"`
	Op     string         `json:"op"`
	Params map[string]any `json:"params"`
	Status string         `json:"status"`
	Result map[string]any `json:"result"`
	Error  string         `json:"error"`
}

type manifestDoc struct {
	Tenant struct {
		Name     string `json:"name"`
		Vertical string `json:"vertical"`
		ID       string `json:"id"`
		Slug     string `json:"slug"`
	} `json:"tenant"`
	Steps []manifestStep `json:"steps"`
}

type onboardSessionResp struct {
	SessionID      string          `json:"sessionId"`
	Manifest       *manifestDoc    `json:"manifest"`
	ManifestStatus *manifestStatus `json:"manifestStatus"`
}

type invokeResp struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  *struct {
		Operation  string `json:"operation"`
		Kind       string `json:"kind"`
		Outcome    string `json:"outcome"`
		EntityKind string `json:"entityKind"`
		RecordID   string `json:"recordId"`
		Summary    string `json:"summary"`
	} `json:"result"`
}

// fieldDef mirrors domain.FieldDef as it appears in define_entity params.
type fieldDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	ValueSetRef string `json:"valueSetRef"`
	RefTarget   string `json:"refTarget"`
	Required    bool   `json:"required"`
	Default     any    `json:"default"`
}

// opInstance is one enable_operations entry {template, instance, config}.
type opInstance struct {
	Template string         `json:"template"`
	Instance string         `json:"instance"`
	Config   map[string]any `json:"config"`
}

// ---------- flow harness ----------

const onboardingTenantSlug = "keepstar-onboarding"

type flow struct {
	t          *testing.T
	base       string // v5 origin, no trailing slash
	admin      string // admin origin, no trailing slash
	password   string // ONBOARDING_PASSWORD
	serviceKey string // ADMIN_SERVICE_KEY
	dbURL      string // optional
	http       *http.Client

	db *pgxpool.Pool // nil unless E2E_DATABASE_URL set

	// accumulated state
	onboardSession string
	regEmail       string
	regPassword    string
	tenantSlug     string
	tenantID       string
	storefrontURL  string
	crmURL         string
	crmToken       string
	storeSession   string
	crmSession     string
	productID      string
	productName    string
	visitorName    string
	visitorPhone   string
	leadRecordID   string
}

func newFlow(t *testing.T) *flow {
	base := strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	if base == "" {
		t.Skip("E2E_BASE_URL not set; skipping live demo-flow e2e")
	}
	f := &flow{
		t:          t,
		base:       base,
		admin:      strings.TrimRight(os.Getenv("E2E_ADMIN_URL"), "/"),
		password:   os.Getenv("ONBOARDING_PASSWORD"),
		serviceKey: os.Getenv("ADMIN_SERVICE_KEY"),
		dbURL:      os.Getenv("E2E_DATABASE_URL"),
	}
	if f.admin == "" || f.password == "" || f.serviceKey == "" {
		t.Fatalf("E2E_BASE_URL is set but the run is half-configured: E2E_ADMIN_URL=%q ONBOARDING_PASSWORD set=%v ADMIN_SERVICE_KEY set=%v — set all three (they must match the DEPLOYED env values, not local ones)",
			f.admin, f.password != "", f.serviceKey != "")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	// Generous timeout: one onboarding turn = up to 8 sequential meta-ops +
	// two LLM calls; applies can add admin round-trips on top.
	f.http = &http.Client{Jar: jar, Timeout: 4 * time.Minute}

	if f.dbURL != "" {
		pool, err := pgxpool.New(context.Background(), f.dbURL)
		if err != nil {
			t.Fatalf("E2E_DATABASE_URL set but unreachable: %v", err)
		}
		t.Cleanup(pool.Close)
		f.db = pool
	}
	return f
}

// do performs one HTTP request and returns status + body. Body errors and
// transport errors are fatal — every step depends on the previous one.
func (f *flow) do(method, rawURL string, hdr map[string]string, body io.Reader, contentType string) (int, []byte) {
	f.t.Helper()
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		f.t.Fatalf("build %s %s: %v", method, rawURL, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := f.http.Do(req)
	if err != nil {
		f.t.Fatalf("%s %s failed on the wire: %v — is the service up and E2E_BASE_URL/E2E_ADMIN_URL correct?", method, rawURL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		f.t.Fatalf("%s %s: read body: %v", method, rawURL, err)
	}
	return resp.StatusCode, b
}

func (f *flow) postJSON(rawURL string, hdr map[string]string, payload any) (int, []byte) {
	f.t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		f.t.Fatalf("marshal request for %s: %v", rawURL, err)
	}
	return f.do(http.MethodPost, rawURL, hdr, bytes.NewReader(b), "application/json")
}

// pipelineTurn runs one POST /api/v1/pipeline turn. Retries ONCE on a 5xx
// (transient LLM upstream hiccups happen on live) before failing.
func (f *flow) pipelineTurn(tenantSlug, sessionID, query string) *turnResp {
	f.t.Helper()
	hdr := map[string]string{"X-Tenant-Slug": tenantSlug}
	payload := map[string]string{"sessionId": sessionID, "query": query}
	var status int
	var body []byte
	for attempt := 1; attempt <= 2; attempt++ {
		status, body = f.postJSON(f.base+"/api/v1/pipeline", hdr, payload)
		if status < 500 {
			break
		}
		f.t.Logf("  pipeline turn %q → %d (%s) — retrying once in 5s", trunc(query, 60), status, trunc(string(body), 120))
		time.Sleep(5 * time.Second)
	}
	if status != http.StatusOK {
		f.t.Fatalf("pipeline turn %q on tenant %s session %s → %d: %s — check v5 logs (v5_traces / v5_chat_session_traces for this session)",
			query, tenantSlug, sessionID, status, trunc(string(body), 400))
	}
	tr := &turnResp{raw: body}
	if err := json.Unmarshal(body, tr); err != nil {
		f.t.Fatalf("pipeline turn %q: response is not the documented JSON shape: %v — body: %s", query, err, trunc(string(body), 400))
	}
	return tr
}

// fetchManifest reads GET /api/v1/onboard/session — the refresh-safe truth.
func (f *flow) fetchManifest() *onboardSessionResp {
	f.t.Helper()
	status, body := f.do(http.MethodGet,
		f.base+"/api/v1/onboard/session?sessionId="+url.QueryEscape(f.onboardSession), nil, nil, "")
	if status != http.StatusOK {
		f.t.Fatalf("GET /api/v1/onboard/session → %d: %s — resume must be refresh-safe (§5.1); session %s",
			status, trunc(string(body), 300), f.onboardSession)
	}
	var out onboardSessionResp
	if err := json.Unmarshal(body, &out); err != nil {
		f.t.Fatalf("onboard session resume: bad JSON: %v — body: %s", err, trunc(string(body), 300))
	}
	return &out
}

func (f *flow) manifestOrEmpty() *manifestDoc {
	f.t.Helper()
	res := f.fetchManifest()
	if res.Manifest == nil {
		return &manifestDoc{}
	}
	return res.Manifest
}

// ---------- manifest inspection helpers ----------

func findStep(m *manifestDoc, op string) *manifestStep {
	for i := range m.Steps {
		if m.Steps[i].Op == op {
			return &m.Steps[i]
		}
	}
	return nil
}

func stagedOps(m *manifestDoc) []string {
	out := make([]string, 0, len(m.Steps))
	for _, s := range m.Steps {
		out = append(out, fmt.Sprintf("%s(%s)", s.Op, s.Status))
	}
	return out
}

// instances collects every enable_operations entry across all steps.
func instances(m *manifestDoc) []opInstance {
	var out []opInstance
	for _, s := range m.Steps {
		if s.Op != "enable_operations" {
			continue
		}
		var box struct {
			Operations []opInstance `json:"operations"`
		}
		if err := remarshal(s.Params, &box); err == nil {
			out = append(out, box.Operations...)
		}
	}
	return out
}

func instanceByTemplate(m *manifestDoc, template string) *opInstance {
	for _, inst := range instances(m) {
		if inst.Template == template {
			cp := inst
			return &cp
		}
	}
	return nil
}

// entityFields reads the define_entity step for slug and returns its fields.
func entityFields(m *manifestDoc, slug string) []fieldDef {
	for _, s := range m.Steps {
		if s.Op != "define_entity" {
			continue
		}
		var box struct {
			Entity struct {
				Slug   string     `json:"slug"`
				Fields []fieldDef `json:"fields"`
			} `json:"entity"`
		}
		if err := remarshal(s.Params, &box); err != nil {
			continue
		}
		if box.Entity.Slug == slug {
			return box.Entity.Fields
		}
	}
	return nil
}

// valueSetValues reads the define_value_set step for slug → ordered values.
func valueSetValues(m *manifestDoc, slug string) []string {
	for _, s := range m.Steps {
		if s.Op != "define_value_set" {
			continue
		}
		var box struct {
			Slug   string `json:"slug"`
			Values []struct {
				Value string `json:"value"`
			} `json:"values"`
		}
		if err := remarshal(s.Params, &box); err != nil || box.Slug != slug {
			continue
		}
		out := make([]string, 0, len(box.Values))
		for _, v := range box.Values {
			out = append(out, v.Value)
		}
		return out
	}
	return nil
}

func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func cfgStr(cfg map[string]any, key string) string {
	s, _ := cfg[key].(string)
	return s
}

func cfgInt(cfg map[string]any, key string, def int) int {
	if f64, ok := cfg[key].(float64); ok {
		return int(f64)
	}
	return def
}

func trunc(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// ---------- the walk ----------

func TestDemoFlow(t *testing.T) {
	f := newFlow(t)

	f.stepGate()
	f.stepOnboardSession()
	f.stepFirstTurn()
	f.stepProposal()
	f.stepRegistration()
	f.stepUpload("realtor_listings.csv", 8)
	f.stepUpload("realtor_listings.json", 4)
	f.stepSurfaceURLs()
	f.stepStorefrontSearch()
	f.stepBooking()
	f.stepCRMLeads()
	f.stepTransition()

	t.Logf("DEMO FLOW COMPLETE — tenant %s (id %s), storefront %s, crm %s, lead %s",
		f.tenantSlug, f.tenantID, f.storefrontURL, f.crmURL, f.leadRecordID)
}

// stepGate: wrong password → 403; right password → 200 + ks_onboard cookie.
func (f *flow) stepGate() {
	f.t.Log("STEP gate — POST /api/v1/onboard/gate")

	status, body := f.postJSON(f.base+"/api/v1/onboard/gate", nil,
		map[string]string{"password": "definitely-wrong-" + randHex(4)})
	if status != http.StatusForbidden {
		f.t.Fatalf("gate accepted a WRONG password (got %d, want 403): %s — R5 constant-time compare broken or ONBOARDING_PASSWORD unset on the service (that would be 503)",
			status, trunc(string(body), 200))
	}

	status, body = f.postJSON(f.base+"/api/v1/onboard/gate", nil,
		map[string]string{"password": f.password})
	if status != http.StatusOK {
		f.t.Fatalf("gate rejected the RIGHT password (got %d): %s — local ONBOARDING_PASSWORD must equal the DEPLOYED env value (rotation kicks cookies, R5)",
			status, trunc(string(body), 200))
	}
	u, _ := url.Parse(f.base)
	var got bool
	for _, c := range f.http.Jar.Cookies(u) {
		if c.Name == "ks_onboard" && c.Value != "" {
			got = true
		}
	}
	if !got {
		f.t.Fatalf("gate returned 200 but no ks_onboard cookie landed in the jar — check Set-Cookie attributes (Path=/, SameSite=Lax) on %s", f.base)
	}
}

// stepOnboardSession: cookie-gated onboarding session create.
func (f *flow) stepOnboardSession() {
	f.t.Log("STEP session — POST /api/v1/onboard/session")
	status, body := f.postJSON(f.base+"/api/v1/onboard/session", nil, map[string]string{})
	if status != http.StatusOK {
		f.t.Fatalf("onboard session create → %d: %s — 503 means the %q system tenant seed is missing (admin must deploy/migrate first, §3.4); 403 means the cookie did not stick",
			status, trunc(string(body), 300), onboardingTenantSlug)
	}
	var resp onboardSessionResp
	if err := json.Unmarshal(body, &resp); err != nil || resp.SessionID == "" {
		f.t.Fatalf("onboard session create: no sessionId in %s", trunc(string(body), 200))
	}
	f.onboardSession = resp.SessionID
	f.t.Logf("  onboarding session %s", f.onboardSession)
}

// stepFirstTurn: the owner's beat-1 message. Asserts ≥1 block; the
// deterministic fallback (blockId "fallback-1") is tolerated by design.
func (f *flow) stepFirstTurn() {
	f.t.Log("STEP turn 1 — 'I run a realtor agency…'")
	tr := f.pipelineTurn(onboardingTenantSlug, f.onboardSession,
		"I run a realtor agency. I need a public storefront where clients browse listings and book showings, and a CRM where my agents track incoming leads.")
	if len(tr.Blocks) < 1 {
		f.t.Fatalf("turn 1 produced ZERO blocks — onboarding turns must always answer in blocks (compose_turn or the deterministic fallback). Response: %s",
			trunc(string(tr.raw), 400))
	}
	if tr.Blocks[0].BlockID == "fallback-1" {
		f.t.Logf("  turn 1 answered with the deterministic fallback (tolerated) — agent staged nothing this turn")
	} else {
		f.t.Logf("  turn 1 → %d block(s)", len(tr.Blocks))
	}
}

// proposalMarkers: how each required beat-2 block is recognized in the raw
// response JSON — by preset name (documents carry __presetInUse) with a
// node-kind fallback.
var proposalMarkers = []struct {
	label    string
	patterns []string
}{
	{"registration form", []string{"registration_form", `"inputType":"password"`}},
	{"uploader", []string{"uploader_card", `"type":"upload"`}},
	{"design preview", []string{"design_system_preview"}},
}

// stepProposal: drive the agent until (a) the beat-2 blocks appeared and
// (b) the staged plan is complete enough to walk the rest of the demo.
func (f *flow) stepProposal() {
	f.t.Log("STEP proposal — 'show me what you propose'")

	queries := []string{
		"Show me what you propose — the plan with the registration form, the data uploader, and the design preview.",
		"Stage the complete plan for my realtor agency and show me the registration form, the data uploader, and the design system preview.",
		"Finish staging the plan — data upload, registration, the operations, the automation, the interface presets, and the URLs step. Then show the registration form, uploader and design preview again.",
	}

	found := map[string]bool{}
	var lastManifest *manifestDoc
	for i, q := range queries {
		tr := f.pipelineTurn(onboardingTenantSlug, f.onboardSession, q)
		raw := string(tr.raw)
		for _, m := range proposalMarkers {
			for _, p := range m.patterns {
				if strings.Contains(raw, p) {
					found[m.label] = true
				}
			}
		}
		lastManifest = f.manifestOrEmpty()
		missingBlocks := f.missingProposalBlocks(found)
		missingPlan := f.missingPlanParts(lastManifest)
		f.t.Logf("  attempt %d: blocks missing %v; plan missing %v; staged: %v",
			i+1, missingBlocks, missingPlan, stagedOps(lastManifest))
		if len(missingBlocks) == 0 && len(missingPlan) == 0 {
			return
		}
	}

	missingBlocks := f.missingProposalBlocks(found)
	missingPlan := f.missingPlanParts(lastManifest)
	f.t.Fatalf("proposal incomplete after %d turns.\n  blocks never rendered: %v (matched by preset name __presetInUse or node kind)\n  plan parts never staged: %v\n  staged steps: %v\n  Hints: empty blocks → compose_turn / Agent2 onboarding prompt; missing plan parts → search_library results or library content pass (M3); a fallback-only conversation → Agent1 emitted no tool calls (check v5_traces mode=onboarding for session %s)",
		len(queries), missingBlocks, missingPlan, stagedOps(lastManifest), f.onboardSession)
}

func (f *flow) missingProposalBlocks(found map[string]bool) []string {
	var out []string
	for _, m := range proposalMarkers {
		if !found[m.label] {
			out = append(out, m.label)
		}
	}
	return out
}

// missingPlanParts lists what the rest of the walk needs from the manifest.
func (f *flow) missingPlanParts(m *manifestDoc) []string {
	var missing []string
	for _, op := range []string{"create_tenant", "register_user", "issue_ingest_door", "issue_surface_urls"} {
		if findStep(m, op) == nil {
			missing = append(missing, op)
		}
	}
	if instanceByTemplate(m, "schedule_slot") == nil {
		missing = append(missing, "enable_operations→schedule_slot instance")
	}
	if instanceByTemplate(m, "transition_status") == nil {
		missing = append(missing, "enable_operations→transition_status instance")
	}
	return missing
}

// stepRegistration: submit the registration form with a FRESH email. The
// submit itself is the approval — the server auto-applies the staged
// manifest (owner ruling 2026-07-28); a 409 here is the exact live bug the
// ruling outlawed. Then: tenant provisioned (via admin service route) and
// the password is NOWHERE at rest in v5 tables (R6, when DB configured).
func (f *flow) stepRegistration() {
	f.t.Log("STEP registration — POST /api/v1/onboard/step/{id}/submit (auto-apply)")

	m := f.manifestOrEmpty()
	reg := findStep(m, "register_user")
	if reg == nil {
		f.t.Fatalf("no register_user step staged — stepProposal should have guaranteed it; staged: %v", stagedOps(m))
	}

	f.regEmail = fmt.Sprintf("e2e-owner-%d@keepstar-demo.test", time.Now().UnixNano())
	f.regPassword = "E2e-" + randHex(12)

	status, body := f.postJSON(
		f.base+"/api/v1/onboard/step/"+url.PathEscape(reg.ID)+"/submit", nil,
		map[string]string{
			"sessionId": f.onboardSession,
			"name":      "E2E Owner",
			"email":     f.regEmail,
			"password":  f.regPassword,
		})
	if status == http.StatusConflict {
		f.t.Fatalf("registration submit → 409 %q — THE outlawed bug (owner 2026-07-28): the user's form submit IS the approval; the server must auto-apply the staged manifest, never demand 'apply the manifest first'",
			trunc(string(body), 200))
	}
	if status != http.StatusOK {
		f.t.Fatalf("registration submit → %d: %s — step %s (status %q). ExecuteStep must auto-apply a proposed manifest and then provision through AdminGateway",
			status, trunc(string(body), 300), reg.ID, reg.Status)
	}
	var sub struct {
		Status string `json:"status"`
		Step   struct {
			Status string `json:"status"`
		} `json:"step"`
	}
	if err := json.Unmarshal(body, &sub); err != nil || sub.Status != "ok" {
		f.t.Fatalf("registration submit answered 200 but not ok: %s — a 200/{status:error} means admin-side provisioning failed (check ADMIN_SERVICE_KEY on BOTH services and admin logs)",
			trunc(string(body), 300))
	}
	if sub.Step.Status != "applied" {
		f.t.Fatalf("registration submit ok but step status is %q, want applied", sub.Step.Status)
	}

	// Auto-apply proof 1: the manifest now carries the provisioned tenant.
	m = f.manifestOrEmpty()
	if m.Tenant.Slug == "" || m.Tenant.ID == "" {
		f.t.Fatalf("after registration the manifest has no tenant slug/id — auto-apply did not run create_tenant; steps: %v", stagedOps(m))
	}
	f.tenantSlug, f.tenantID = m.Tenant.Slug, m.Tenant.ID
	f.t.Logf("  tenant provisioned: %s (%s)", f.tenantSlug, f.tenantID)

	// Auto-apply proof 2: the tenant + user EXIST on the admin service route.
	// ProvisionUser is idempotent (spec §5.5): 200 = existed (what we want),
	// 201 = admin just created it now (meaning registration had NOT
	// provisioned), 404 = tenant unknown.
	status, body = f.postJSON(
		f.admin+"/admin/api/service/v1/tenants/"+url.PathEscape(f.tenantSlug)+"/users",
		map[string]string{"X-Service-Key": f.serviceKey},
		map[string]string{"email": f.regEmail, "password": f.regPassword, "role": "owner"})
	switch status {
	case http.StatusOK:
		// existed — registration really provisioned it.
	case http.StatusCreated:
		f.t.Fatalf("admin service route CREATED the user on re-provision (201) — the registration submit did NOT provision %s on tenant %s", f.regEmail, f.tenantSlug)
	case http.StatusNotFound:
		f.t.Fatalf("admin service route does not know tenant %q — create_tenant did not reach admin (or admin/v5 point at different DBs)", f.tenantSlug)
	default:
		f.t.Fatalf("admin user re-provision check → %d: %s — verify ADMIN_SERVICE_KEY matches the deployed admin env", status, trunc(string(body), 300))
	}

	// R6: the password appears in NO v5 table touched by this session.
	f.assertNoPasswordAtRest()
}

// assertNoPasswordAtRest scans the session-scoped rows of every v5 table a
// credential could have leaked into (state, deltas, traces, operation runs)
// for the literal password substring. Skips (loudly) without a DB URL.
func (f *flow) assertNoPasswordAtRest() {
	if f.db == nil {
		f.t.Log("  E2E_DATABASE_URL not set — skipping the R6 password-at-rest scan (set it for full coverage)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	checks := []struct{ table, where string }{
		{"v5_chat_session_state", "r.session_id::text = $1"},
		{"v5_chat_session_deltas", "r.session_id::text = $1"},
		{"v5_chat_session_traces", "r.session_id::text = $1"},
		{"v5_operation_runs", "r.session_id = $1"},
	}
	for _, c := range checks {
		var n int
		q := "SELECT count(*) FROM " + c.table + " r WHERE " + c.where + " AND position($2 in r::text) > 0"
		if err := f.db.QueryRow(ctx, q, f.onboardSession, f.regPassword).Scan(&n); err != nil {
			f.t.Fatalf("R6 scan on %s failed: %v", c.table, err)
		}
		if n != 0 {
			f.t.Fatalf("R6 VIOLATION: the registration password is stored in %d row(s) of %s for session %s — credentials must flow handler→gateway→bcrypt ONLY",
				n, c.table, f.onboardSession)
		}
	}
	f.t.Log("  R6 password-at-rest scan clean (state, deltas, traces, operation runs)")
}

// stepUpload: multipart upload (sessionId field precedes the file — the
// server streams) then poll to a terminal status. The FIRST upload also
// exercises the owner ruling: the ingest token is resolved server-side, the
// upload is never bounced with 'apply the manifest first'.
func (f *flow) stepUpload(fixture string, minRows int) {
	f.t.Logf("STEP upload — %s", fixture)

	fpath := filepath.Join("testdata", fixture)
	data, err := os.ReadFile(fpath)
	if err != nil {
		f.t.Fatalf("fixture %s unreadable: %v (run from project_v5/backend/e2e)", fpath, err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// Field order is contractual: the door fields must precede the file part.
	if err := mw.WriteField("sessionId", f.onboardSession); err != nil {
		f.t.Fatalf("multipart sessionId: %v", err)
	}
	fw, err := mw.CreateFormFile("file", fixture)
	if err != nil {
		f.t.Fatalf("multipart file part: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		f.t.Fatalf("multipart write: %v", err)
	}
	mw.Close()

	status, body := f.do(http.MethodPost, f.base+"/api/v1/onboard/upload", nil, &buf, mw.FormDataContentType())
	if status == http.StatusConflict {
		f.t.Fatalf("upload %s → 409 %q — the upload IS the approval (owner 2026-07-28): ResolveIngestToken must auto-apply/mint, never bounce", fixture, trunc(string(body), 200))
	}
	if status != http.StatusAccepted {
		f.t.Fatalf("upload %s → %d (want 202): %s", fixture, status, trunc(string(body), 300))
	}
	var acc struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(body, &acc); err != nil || acc.JobID == "" {
		f.t.Fatalf("upload %s: no jobId in %s", fixture, trunc(string(body), 200))
	}
	f.t.Logf("  job %s accepted — polling", acc.JobID)

	deadline := time.Now().Add(3 * time.Minute)
	for {
		status, body = f.do(http.MethodGet,
			f.base+"/api/v1/onboard/upload/"+url.PathEscape(acc.JobID)+"?sessionId="+url.QueryEscape(f.onboardSession),
			nil, nil, "")
		if status != http.StatusOK {
			f.t.Fatalf("upload poll %s → %d: %s", acc.JobID, status, trunc(string(body), 300))
		}
		var st struct {
			Status         string   `json:"status"`
			Processed      int      `json:"processed"`
			ProjectionRows int      `json:"projectionRows"`
			Invalidated    bool     `json:"invalidated"`
			Errors         []string `json:"errors"`
		}
		if err := json.Unmarshal(body, &st); err != nil {
			f.t.Fatalf("upload poll %s: bad JSON: %v — %s", acc.JobID, err, trunc(string(body), 200))
		}
		switch st.Status {
		case "completed":
			if st.Processed <= 0 || st.ProjectionRows <= 0 {
				f.t.Fatalf("import %s completed but processed=%d projectionRows=%d (want both > 0) — the R26 mapping or the RebuildSearchProjection tail is broken; errors: %v",
					fixture, st.Processed, st.ProjectionRows, st.Errors)
			}
			if st.Processed < minRows {
				f.t.Fatalf("import %s processed %d rows, fixture has %d — rows were dropped (mapping/name inference); errors: %v",
					fixture, st.Processed, minRows, st.Errors)
			}
			f.t.Logf("  import completed: processed=%d projectionRows=%d invalidated=%v",
				st.Processed, st.ProjectionRows, st.Invalidated)
			return
		case "failed":
			f.t.Fatalf("import %s FAILED: %v — step stays re-uploadable per R25, but the demo flow requires a clean import", fixture, st.Errors)
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("import %s still %q after 3m — admin import job stuck (check admin logs)", fixture, st.Status)
		}
		time.Sleep(2 * time.Second)
	}
}

// stepSurfaceURLs: with registration + uploads done, ask for the links; the
// agent re-runs apply_manifest and issue_surface_urls mints both URLs into
// the manifest — the refresh-safe place we read them from.
func (f *flow) stepSurfaceURLs() {
	f.t.Log("STEP surface URLs — 'where are my links?'")

	queries := []string{
		"I have registered and uploaded all my data. Where are my links? Please finalize my workspace.",
		"Finalize everything and give me the storefront and CRM URLs. If a step failed, fix it and apply again.",
		"Apply the manifest again and issue my storefront and CRM URLs.",
	}
	var lastStatus *manifestStatus
	for i, q := range queries {
		f.pipelineTurn(onboardingTenantSlug, f.onboardSession, q)
		res := f.fetchManifest()
		lastStatus = res.ManifestStatus
		if res.Manifest != nil {
			if st := findStep(res.Manifest, "issue_surface_urls"); st != nil {
				sURL, _ := st.Result["storefrontUrl"].(string)
				cURL, _ := st.Result["crmUrl"].(string)
				if sURL != "" && cURL != "" {
					f.storefrontURL, f.crmURL = sURL, cURL
					break
				}
			}
		}
		f.t.Logf("  attempt %d: URLs not issued yet (manifest status %+v)", i+1, lastStatus)
	}
	if f.storefrontURL == "" || f.crmURL == "" {
		extra := ""
		if lastStatus != nil && lastStatus.Failed > 0 {
			extra = fmt.Sprintf(" — step %q failed: %s (fix that step; issue_surface_urls waits for every other step, §4.3)",
				lastStatus.FailedStep, lastStatus.FailedReason)
		}
		f.t.Fatalf("surface URLs never issued after %d turns%s; manifest status %+v — issue_surface_urls requires ALL other steps applied (registration + upload included)",
			len(queries), extra, lastStatus)
	}

	if !strings.Contains(f.storefrontURL, "/s/"+f.tenantSlug) {
		f.t.Fatalf("storefrontUrl %q does not address tenant %q (/s/{slug})", f.storefrontURL, f.tenantSlug)
	}
	if !strings.Contains(f.crmURL, "/crm/"+f.tenantSlug) {
		f.t.Fatalf("crmUrl %q does not address tenant %q (/crm/{slug})", f.crmURL, f.tenantSlug)
	}
	cu, err := url.Parse(f.crmURL)
	if err != nil || cu.Query().Get("k") == "" {
		f.t.Fatalf("crmUrl %q carries no surface token ?k= — CRM sessions are token-gated (R13)", f.crmURL)
	}
	f.crmToken = cu.Query().Get("k")
	f.t.Logf("  storefront %s\n  crm %s", f.storefrontURL, f.crmURL)
}

// stepStorefrontSearch: a PUBLIC storefront session on the new tenant finds
// the uploaded listings (projection rebuilt inside the import job).
func (f *flow) stepStorefrontSearch() {
	f.t.Log("STEP storefront — search the uploaded listings")

	hdr := map[string]string{"X-Tenant-Slug": f.tenantSlug}
	status, body := f.postJSON(f.base+"/api/v1/session/init", hdr, map[string]string{})
	if status != http.StatusOK {
		f.t.Fatalf("storefront session init on %s → %d: %s", f.tenantSlug, status, trunc(string(body), 200))
	}
	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(body, &sess); err != nil || sess.SessionID == "" {
		f.t.Fatalf("storefront session init: no sessionId in %s", trunc(string(body), 200))
	}
	f.storeSession = sess.SessionID

	for i, q := range []string{"Show me two bedroom apartments", "Show me all available listings"} {
		f.pipelineTurn(f.tenantSlug, f.storeSession, q)
		status, body = f.do(http.MethodGet, f.base+"/api/v1/session/"+f.storeSession, hdr, nil, "")
		if status != http.StatusOK {
			f.t.Fatalf("session state read → %d: %s", status, trunc(string(body), 200))
		}
		var st struct {
			Current struct {
				Data struct {
					Products []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"products"`
				} `json:"data"`
			} `json:"current"`
		}
		if err := json.Unmarshal(body, &st); err != nil {
			f.t.Fatalf("session state: bad JSON: %v", err)
		}
		if n := len(st.Current.Data.Products); n > 0 {
			f.productID = st.Current.Data.Products[0].ID
			f.productName = st.Current.Data.Products[0].Name
			f.t.Logf("  %d product(s) found; first: %q (%s)", n, f.productName, f.productID)
			return
		}
		f.t.Logf("  attempt %d (%q): products zone empty", i+1, q)
	}
	f.t.Fatalf("storefront search found NOTHING on tenant %s after 2 turns — either the projection was not rebuilt on import (the M1 MUST-fix) or cache invalidation missed (scope=all). Verify tenant_search_projection rows for tenant %s",
		f.tenantSlug, f.tenantID)
}

// stepBooking: invoke the staged schedule_slot instance exactly as the
// widget's booking form would (§4.8) and prove the lead record row.
func (f *flow) stepBooking() {
	f.t.Log("STEP booking — POST /api/v1/operations/invoke (schedule_slot)")

	m := f.manifestOrEmpty()
	inst := instanceByTemplate(m, "schedule_slot")
	if inst == nil {
		f.t.Fatalf("no schedule_slot instance in the manifest — stepProposal guaranteed it; staged: %v", stagedOps(m))
	}
	entity := cfgStr(inst.Config, "entity")
	dtField := cfgStr(inst.Config, "datetime_field")
	linkField := cfgStr(inst.Config, "link_field")
	if entity == "" || dtField == "" {
		f.t.Fatalf("schedule_slot instance %q config lacks entity/datetime_field: %v", inst.Instance, inst.Config)
	}

	f.visitorName = fmt.Sprintf("E2E Visitor %04d", time.Now().UnixNano()%10000)
	f.visitorPhone = fmt.Sprintf("+1555%07d", time.Now().UnixNano()%10000000)

	params := f.buildBookingParams(m, entity, dtField, linkField, inst.Config)

	status, body := f.postJSON(f.base+"/api/v1/operations/invoke",
		map[string]string{"X-Tenant-Slug": f.tenantSlug},
		map[string]any{
			"sessionId": f.storeSession,
			"operation": inst.Instance,
			"params":    params,
			"formId":    "e2e-booking",
		})
	if status != http.StatusOK {
		f.t.Fatalf("invoke %s → %d: %s", inst.Instance, status, trunc(string(body), 300))
	}
	var inv invokeResp
	if err := json.Unmarshal(body, &inv); err != nil {
		f.t.Fatalf("invoke %s: bad JSON: %v — %s", inst.Instance, err, trunc(string(body), 300))
	}
	if inv.Status != "ok" || inv.Result == nil || inv.Result.Outcome != "ok" {
		f.t.Fatalf("booking rejected: status=%q outcome=%v message=%q params=%v — invalid usually means a schema/guard mismatch (hours window, E.164 phone, RFC3339 datetime)",
			inv.Status, inv.Result, inv.Message, params)
	}
	if inv.Result.RecordID == "" {
		f.t.Fatalf("booking ok but no recordId — schedule_slot must return the created record id; body: %s", trunc(string(body), 300))
	}
	f.leadRecordID = inv.Result.RecordID
	f.t.Logf("  booked — %s record %s (%s)", inv.Result.EntityKind, f.leadRecordID, inv.Result.Summary)

	if f.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var slug, dbStatus string
		err := f.db.QueryRow(ctx,
			`SELECT entity_slug, COALESCE(status,'') FROM v5_entity_records WHERE id = $1::uuid AND tenant_id = $2::uuid`,
			f.leadRecordID, f.tenantID).Scan(&slug, &dbStatus)
		if err != nil {
			f.t.Fatalf("lead record row %s not found in v5_entity_records for tenant %s: %v", f.leadRecordID, f.tenantID, err)
		}
		if slug != entity {
			f.t.Fatalf("lead record %s has entity_slug %q, want %q", f.leadRecordID, slug, entity)
		}
		f.t.Logf("  DB: v5_entity_records row confirmed (entity=%s status=%q)", slug, dbStatus)
	}
}

// buildBookingParams assembles a valid input for the staged schedule_slot
// instance from the STAGED entity definition (universality: no fixed field
// list). Falls back to the canonical demo shape when the entity was staged
// outside define_entity (e.g. carried by a pack).
func (f *flow) buildBookingParams(m *manifestDoc, entity, dtField, linkField string, cfg map[string]any) map[string]any {
	// Business-hours-safe RFC3339 slot: tomorrow, middle of the window,
	// UTC — guards evaluate in the value's own location (R11).
	from, to := 9, 18
	if hours, ok := cfg["hours"].(map[string]any); ok {
		from, to = cfgInt(hours, "from", 9), cfgInt(hours, "to", 18)
	}
	hour := from + (to-from)/2
	slot := time.Now().UTC().AddDate(0, 0, 1)
	slotISO := fmt.Sprintf("%sT%02d:00:00Z", slot.Format("2006-01-02"), hour)

	params := map[string]any{
		dtField: slotISO,
	}
	if linkField != "" {
		params[linkField] = f.productID
	}

	fields := entityFields(m, entity)
	if fields == nil {
		f.t.Logf("  define_entity step for %q not found in the manifest — using canonical contact keys", entity)
		params["name"] = f.visitorName
		params["phone"] = f.visitorPhone
		return params
	}
	for _, fd := range fields {
		if _, done := params[fd.Key]; done {
			continue
		}
		lower := strings.ToLower(fd.Key)
		switch {
		case fd.Type == "phone":
			params[fd.Key] = f.visitorPhone
		case fd.Type == "email":
			params[fd.Key] = f.regEmail
		case fd.Type == "text" && strings.Contains(lower, "name"):
			params[fd.Key] = f.visitorName
		case fd.Type == "text" && fd.Required:
			params[fd.Key] = "Booked via the e2e demo walk"
		case fd.Type == "enum":
			// Status-like enums carry defaults server-side; only fill when
			// required AND we know the vocabulary.
			if fd.Required {
				if vals := valueSetValues(m, fd.ValueSetRef); len(vals) > 0 {
					params[fd.Key] = vals[0]
				}
			}
		case fd.Type == "number" && fd.Required:
			params[fd.Key] = 1
		case fd.Type == "money" && fd.Required:
			params[fd.Key] = 100
		case fd.Type == "bool" && fd.Required:
			params[fd.Key] = true
		case fd.Type == "date" && fd.Required:
			params[fd.Key] = slot.Format("2006-01-02")
		}
	}
	// A contact name is the demo's CRM assertion anchor — force one even if
	// the staged shape named it unexpectedly.
	if _, ok := params["name"]; !ok {
		for _, fd := range fields {
			if fd.Type == "text" && strings.Contains(strings.ToLower(fd.Key), "name") {
				params[fd.Key] = f.visitorName
			}
		}
	}
	return params
}

// stepCRMLeads: a staff (token-gated) CRM session surfaces the fresh lead.
func (f *flow) stepCRMLeads() {
	f.t.Log("STEP crm — 'any new leads?'")

	hdr := map[string]string{"X-Tenant-Slug": f.tenantSlug}
	status, body := f.postJSON(f.base+"/api/v1/session/init", hdr,
		map[string]string{"mode": "crm", "k": f.crmToken})
	if status != http.StatusOK {
		f.t.Fatalf("crm session init → %d: %s — the ?k= token from crmUrl must stamp role=staff (R13); token %q tenant %s",
			status, trunc(string(body), 200), f.crmToken, f.tenantSlug)
	}
	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(body, &sess); err != nil || sess.SessionID == "" {
		f.t.Fatalf("crm session init: no sessionId in %s", trunc(string(body), 200))
	}
	f.crmSession = sess.SessionID

	m := f.manifestOrEmpty()
	entity := "record"
	if inst := instanceByTemplate(m, "schedule_slot"); inst != nil {
		if e := cfgStr(inst.Config, "entity"); e != "" {
			entity = e
		}
	}
	queries := []string{
		"Any new leads? Show me the latest requests with contact details.",
		fmt.Sprintf("Show me the newest %s records with names and phone numbers.", entity),
	}
	for i, q := range queries {
		tr := f.pipelineTurn(f.tenantSlug, sess.SessionID, q)
		if strings.Contains(string(tr.raw), f.visitorName) {
			f.t.Logf("  lead surfaced in the CRM turn (visitor %q found in the rendered blocks)", f.visitorName)
			return
		}
		f.t.Logf("  attempt %d (%q): visitor %q not in the response yet", i+1, q, f.visitorName)
	}
	f.t.Fatalf("the CRM never surfaced the booked lead (visitor %q, record %s) after %d turns — check the entity query instance ('%s' zone → Agent2 binding) and that the booking wrote v5_entity_records on tenant %s",
		f.visitorName, f.leadRecordID, len(queries), entity, f.tenantSlug)
}

// stepTransition: advance the lead's status through the staged
// transition_status instance and prove the pipeline moved. The template is
// seeded min_role=staff (R14) — the invoke runs on the CRM (staff) session;
// the storefront visitor session is first asserted DENIED (spec §6.6).
func (f *flow) stepTransition() {
	f.t.Log("STEP transition — advance the lead status")

	m := f.manifestOrEmpty()
	inst := instanceByTemplate(m, "transition_status")
	if inst == nil {
		f.t.Fatalf("no transition_status instance in the manifest; staged: %v", stagedOps(m))
	}

	// R14 value assertion: a visitor session must NOT move the pipeline.
	status, body := f.postJSON(f.base+"/api/v1/operations/invoke",
		map[string]string{"X-Tenant-Slug": f.tenantSlug},
		map[string]any{
			"sessionId": f.storeSession,
			"operation": inst.Instance,
			"params":    map[string]any{"id": f.leadRecordID, "to_status": "anything"},
		})
	if status == http.StatusOK {
		var inv invokeResp
		if err := json.Unmarshal(body, &inv); err == nil && inv.Result != nil && inv.Result.Outcome == "ok" {
			f.t.Fatalf("R14 VIOLATION: the VISITOR storefront session executed staff-gated %q — min_role must be enforced at the registry choke point", inst.Instance)
		}
	}
	setSlug := cfgStr(inst.Config, "value_set")
	current := ""
	if sched := instanceByTemplate(m, "schedule_slot"); sched != nil {
		if defs, ok := sched.Config["defaults"].(map[string]any); ok {
			current, _ = defs[cfgStr(inst.Config, "field")].(string)
			if current == "" {
				current, _ = defs["status"].(string)
			}
		}
	}
	candidates := valueSetValues(m, setSlug)
	if len(candidates) == 0 {
		candidates = []string{"contacted", "in_progress", "done"}
		f.t.Logf("  value set %q not found in the manifest — falling back to generic candidates", setSlug)
	}

	var advancedTo string
	var lastMsg string
	for _, target := range candidates {
		if target == current {
			continue
		}
		status, body := f.postJSON(f.base+"/api/v1/operations/invoke",
			map[string]string{"X-Tenant-Slug": f.tenantSlug},
			map[string]any{
				"sessionId": f.crmSession, // staff session — R14
				"operation": inst.Instance,
				"params":    map[string]any{"id": f.leadRecordID, "to_status": target},
			})
		if status != http.StatusOK {
			f.t.Fatalf("invoke %s → %d: %s", inst.Instance, status, trunc(string(body), 300))
		}
		var inv invokeResp
		if err := json.Unmarshal(body, &inv); err != nil {
			f.t.Fatalf("invoke %s: bad JSON: %v", inst.Instance, err)
		}
		if inv.Status == "ok" && inv.Result != nil && inv.Result.Outcome == "ok" {
			advancedTo = target
			break
		}
		lastMsg = inv.Message
		f.t.Logf("  to_status %q rejected (%s) — trying the next pipeline value", target, trunc(lastMsg, 120))
	}
	if advancedTo == "" {
		f.t.Fatalf("transition_status %q accepted NO target from %v (record %s, assumed current %q); last rejection: %s — value-set or transitions-map mismatch",
			inst.Instance, candidates, f.leadRecordID, current, lastMsg)
	}
	f.t.Logf("  status advanced → %q", advancedTo)

	if f.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var dbStatus string
		if err := f.db.QueryRow(ctx,
			`SELECT COALESCE(status,'') FROM v5_entity_records WHERE id = $1::uuid`,
			f.leadRecordID).Scan(&dbStatus); err != nil {
			f.t.Fatalf("record %s vanished after transition: %v", f.leadRecordID, err)
		}
		if dbStatus != advancedTo {
			f.t.Fatalf("transition reported ok but v5_entity_records.status is %q, want %q — the status mirror column drifted (EntityPort is the only writer)",
				dbStatus, advancedTo)
		}
		f.t.Logf("  DB: status mirror column confirms %q", dbStatus)
	}
}
