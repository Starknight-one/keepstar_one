//go:build e2e

package e2e

// End-to-end smoke verification against the admin-production dev stand.
// Build tag `e2e` keeps these out of the default `go test ./...` run.
//
// Run with:
//   BASE_URL=https://admin-production-4ae4.up.railway.app go test -tags=e2e -v ./e2e/...
//
// Each test uses a uniquely-prefixed email (`e2e-<run-uuid>-<test>@keepstar.test`)
// so cleanup is grep-friendly later. Test users are NOT actively cleaned up —
// they sit in the dev DB until a manual `DELETE WHERE email LIKE 'e2e-%'`.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------
// Test fixture: shared HTTP client + base URL + email prefix per test run.
// ---------------------------------------------------------------------

var (
	baseURL string
	runID   string
	client  = &http.Client{Timeout: 30 * time.Second}
)

func TestMain(m *testing.M) {
	baseURL = os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://admin-production-4ae4.up.railway.app"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	runID = hex.EncodeToString(b)

	fmt.Printf("e2e suite: BASE_URL=%s runID=%s\n", baseURL, runID)
	os.Exit(m.Run())
}

func testEmail(label string) string {
	return fmt.Sprintf("e2e-%s-%s@keepstar.test", runID, label)
}

// testCompany returns a per-test-run unique company name so tenant slug
// (catalog.tenants.slug UNIQUE constraint) doesn't collide between e2e
// runs on the same DB.
func testCompany(label string) string {
	return fmt.Sprintf("E2E-%s-%s", runID, label)
}

func doRequest(t *testing.T, method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, baseURL+path, rdr)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("http call %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	respBody := make([]byte, 0, 1024)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			respBody = append(respBody, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return resp, respBody
}

// ---------------------------------------------------------------------
// E2E scenarios from sec 1-15 of docs/pre_launch_scenarios.md.
// ---------------------------------------------------------------------

// TestE2E_Health verifies the stand is reachable and /auth/config returns
// the documented JSON shape — pre-check used by other tests.
func TestE2E_Health_AuthConfig(t *testing.T) {
	resp, body := doRequest(t, http.MethodGet, "/admin/api/auth/config", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", resp.StatusCode, body)
	}
	var cfg struct {
		Google   bool `json:"google"`
		Email    bool `json:"email"`
		Telegram struct {
			Enabled     bool   `json:"enabled"`
			BotUsername string `json:"bot_username"`
		} `json:"telegram"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	// At least one auth path must be configured for prod-readiness.
	if !cfg.Google && !cfg.Email && !cfg.Telegram.Enabled {
		t.Errorf("no auth method configured on dev stand: %+v", cfg)
	}
}

// TestE2E_Scenario_001_SignupHappyPath verifies sec 1 scenario 1 end-to-end:
// POST /signup returns 201 with token pair + user.
func TestE2E_Scenario_001_SignupHappyPath(t *testing.T) {
	email := testEmail("signup-happy")
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/signup", map[string]string{
		"email":       email,
		"password":    "supersecret-e2e",
		"companyName": testCompany("happy"),
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", resp.StatusCode, body)
	}
	var auth struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &auth); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	if auth.AccessToken == "" || auth.RefreshToken == "" {
		t.Errorf("token pair empty: %+v", auth)
	}
	if auth.User.Email != email {
		t.Errorf("user.email=%q, want %q", auth.User.Email, email)
	}
}

// TestE2E_Scenario_002_DuplicateEmail verifies sec 1 scenario 2: second
// signup with same email returns 409.
func TestE2E_Scenario_002_DuplicateEmail_Returns409(t *testing.T) {
	email := testEmail("signup-dup")
	body := map[string]string{
		"email": email, "password": "supersecret-e2e", "companyName": testCompany("dup"),
	}
	// First signup.
	resp1, b1 := doRequest(t, http.MethodPost, "/admin/api/auth/signup", body, nil)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first signup status=%d; body=%s", resp1.StatusCode, b1)
	}
	// Second signup → 409.
	resp2, b2 := doRequest(t, http.MethodPost, "/admin/api/auth/signup", body, nil)
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("second signup status=%d, want 409; body=%s", resp2.StatusCode, b2)
	}
}

// TestE2E_Scenario_009_LoginHappyPath verifies sec 2 scenario 9: signup then
// login with same credentials returns 200 + token pair.
func TestE2E_Scenario_009_LoginHappyPath(t *testing.T) {
	email := testEmail("login-happy")
	_, _ = doRequest(t, http.MethodPost, "/admin/api/auth/signup", map[string]string{
		"email": email, "password": "supersecret-e2e", "companyName": testCompany("login"),
	}, nil)

	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/login", map[string]string{
		"email": email, "password": "supersecret-e2e",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", resp.StatusCode, body)
	}
	var auth struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(body, &auth)
	if auth.AccessToken == "" || auth.RefreshToken == "" {
		t.Errorf("token pair missing: %s", body)
	}
}

// TestE2E_Scenario_010_LoginUnknownEmail_Returns401 verifies sec 2 scenario 10.
func TestE2E_Scenario_010_LoginUnknownEmail_Returns401(t *testing.T) {
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/login", map[string]string{
		"email": testEmail("ghost-never-existed"), "password": "anything",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", resp.StatusCode, body)
	}
}

// TestE2E_Scenario_011_LoginWrongPassword_Returns401 verifies sec 2 scenario 11.
func TestE2E_Scenario_011_LoginWrongPassword_Returns401(t *testing.T) {
	email := testEmail("login-wrong")
	_, _ = doRequest(t, http.MethodPost, "/admin/api/auth/signup", map[string]string{
		"email": email, "password": "rightpw-e2e", "companyName": testCompany("wrong"),
	}, nil)
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/login", map[string]string{
		"email": email, "password": "wrongpassword",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", resp.StatusCode, body)
	}
}

// TestE2E_Scenario_016_RefreshRotates verifies sec 3 scenario 16.
func TestE2E_Scenario_016_RefreshRotates(t *testing.T) {
	email := testEmail("refresh-rotate")
	signupResp, sBody := doRequest(t, http.MethodPost, "/admin/api/auth/signup", map[string]string{
		"email": email, "password": "supersecret-e2e", "companyName": testCompany("rot"),
	}, nil)
	if signupResp.StatusCode != http.StatusCreated {
		t.Fatalf("signup: %s", sBody)
	}
	var auth struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(sBody, &auth)

	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/sessions/refresh", map[string]string{
		"refresh_token": auth.RefreshToken,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh status=%d; body=%s", resp.StatusCode, body)
	}
	var pair struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(body, &pair)
	if pair.RefreshToken == "" || pair.RefreshToken == auth.RefreshToken {
		t.Errorf("rotation failed: orig=%s rotated=%s", auth.RefreshToken, pair.RefreshToken)
	}
}

// TestE2E_Scenario_017_RefreshReuse_TriggersBreach verifies sec 3 scenario 17.
func TestE2E_Scenario_017_RefreshTokenReuse_Returns401(t *testing.T) {
	email := testEmail("refresh-breach")
	signupResp, sBody := doRequest(t, http.MethodPost, "/admin/api/auth/signup", map[string]string{
		"email": email, "password": "supersecret-e2e", "companyName": testCompany("breach"),
	}, nil)
	if signupResp.StatusCode != http.StatusCreated {
		t.Fatalf("signup: %s", sBody)
	}
	var auth struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(sBody, &auth)

	// First refresh rotates.
	_, _ = doRequest(t, http.MethodPost, "/admin/api/auth/sessions/refresh", map[string]string{
		"refresh_token": auth.RefreshToken,
	}, nil)
	// Second refresh with the original token → breach detection → 401.
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/sessions/refresh", map[string]string{
		"refresh_token": auth.RefreshToken,
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", resp.StatusCode, body)
	}
}

// TestE2E_Scenario_015_Logout verifies sec 2 scenario 15: logout returns 200
// regardless of token validity.
func TestE2E_Scenario_015_Logout(t *testing.T) {
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/logout", map[string]string{
		"refresh_token": "any",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200; body=%s", resp.StatusCode, body)
	}
}

// TestE2E_Scenario_023_GoogleStart verifies sec 4 scenario 23: Start endpoint
// returns a Google consent URL with state.
func TestE2E_Scenario_023_GoogleStart(t *testing.T) {
	resp, body := doRequest(t, http.MethodGet, "/admin/api/auth/google/start", nil, nil)
	if resp.StatusCode == http.StatusNotImplemented {
		t.Skip("google oauth not configured on this stand")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d; body=%s", resp.StatusCode, body)
	}
	var res struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	if !strings.HasPrefix(res.URL, "https://accounts.google.com/") {
		t.Errorf("URL not Google consent endpoint: %q", res.URL)
	}
	if res.State == "" {
		t.Errorf("state empty")
	}
}

// TestE2E_Scenario_032_TelegramStart verifies sec 5 scenario 32: Telegram
// OIDC Start returns a consent URL.
func TestE2E_Scenario_032_TelegramStart(t *testing.T) {
	resp, body := doRequest(t, http.MethodGet, "/admin/api/auth/telegram/start", nil, nil)
	if resp.StatusCode == http.StatusNotImplemented {
		t.Skip("telegram oidc not configured on this stand")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d; body=%s", resp.StatusCode, body)
	}
	var res struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(body, &res)
	if !strings.HasPrefix(res.URL, "https://oauth.telegram.org/") {
		t.Errorf("URL not Telegram consent endpoint: %q", res.URL)
	}
}

// TestE2E_Scenario_038_ForgotPassword_AnyEmail_Returns200 verifies sec 6
// scenario 38 + sec 15 scenario 97: /auth/password/forgot is always 200
// (anti-enumeration), regardless of whether the email exists.
//
// Note: backend route is `/admin/api/auth/password/forgot` (matches
// frontend ForgotPasswordPage). Earlier draft of this test hit the wrong
// `/auth/forgot` path which falls through to the SPA — fake 200 from HTML.
func TestE2E_Scenario_038_ForgotPassword_AnyEmail_Returns200(t *testing.T) {
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/password/forgot", map[string]string{
		"email": testEmail("forgot-unknown"),
	}, nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Skip("forgot-password endpoint not wired on this stand")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200 (anti-enum); body=%s", resp.StatusCode, body)
	}
	// Defend against SPA-fallback false-positive: real handler returns JSON.
	if strings.HasPrefix(string(body), "<!DOCTYPE html>") {
		t.Errorf("SPA fallback served instead of JSON — endpoint path wrong: body=%s", string(body)[:120])
	}
}

// TestE2E_Scenario_040_MagicConsume_BadCode_Returns401 verifies sec 6
// scenario 40 + 41: invalid magic-link code returns 401 with friendly
// "link expired or already used" message.
func TestE2E_Scenario_040_MagicConsume_BadCode_Returns401(t *testing.T) {
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/magic", map[string]string{
		"code": "definitely-not-a-real-magic-link-token",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "expired") && !strings.Contains(string(body), "used") {
		t.Errorf("scenario 40/41: error message not friendly. Got: %s", body)
	}
}

// TestE2E_Scenario_060_PendingShopLinkConsume_NoAuth_Returns401 verifies
// sec 9 scenario 60: consume endpoint requires auth.
//
// On dev stand THIS TEST FAILS: the endpoint is not in the routing table,
// so the SPA index (HTML) is returned with 200. The fall-through happens
// because the catch-all `/` route serves the SPA. This is a real gap —
// the unit-test scenario 60 verifies the usecase works, but the HTTP
// handler isn't wired into the router yet.
func TestE2E_Scenario_060_PendingShopLinkConsume_NoAuth(t *testing.T) {
	resp, body := doRequest(t, http.MethodPost, "/admin/api/auth/shop-pending-link/consume", map[string]string{
		"token": "any",
	}, nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Skip("shop-pending-link endpoint not wired on this stand")
	}
	// If the SPA fallback served us HTML instead of the API, the route is
	// not mounted — record as gap, not 401.
	if strings.HasPrefix(string(body), "<!DOCTYPE html>") {
		t.Errorf("scenario 60: /admin/api/auth/shop-pending-link/consume is NOT routed on dev stand — SPA fallback returned HTML")
		return
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", resp.StatusCode, body)
	}
}

// TestE2E_MethodNotAllowed verifies that key POST endpoints reject GET with
// 405. This is a small but useful regression net — wiring mistakes that
// silently accept the wrong method break the frontend.
func TestE2E_MethodNotAllowed_PostEndpoints(t *testing.T) {
	endpoints := []string{
		"/admin/api/auth/signup",
		"/admin/api/auth/login",
		"/admin/api/auth/sessions/refresh",
		"/admin/api/auth/logout",
		"/admin/api/auth/magic",
	}
	for _, ep := range endpoints {
		resp, body := doRequest(t, http.MethodGet, ep, nil, nil)
		if resp.StatusCode == http.StatusNotFound {
			// Endpoint not wired — skip silently.
			continue
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s GET: status=%d, want 405; body=%s", ep, resp.StatusCode, body)
		}
	}
}
