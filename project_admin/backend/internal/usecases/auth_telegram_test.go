package usecases

// Pre-launch scenario verification for Telegram OIDC + legacy widget auth.
// Each TestScenario_NNN_* maps to a numbered scenario in
// docs/pre_launch_scenarios.md. Red tests document desired behavior that is
// NOT YET implemented — they fail until the underlying gap is closed.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/adapters/telegram"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
)

// telegramFakeAuth extends fakeAuth with real Telegram ID linkage state so
// we can exercise the fast-path (existing telegram user) and email-cascade
// branches without mocking the HTTP-level Exchange step.
type telegramFakeAuth struct {
	*fakeAuth
	byTelegram map[int64]string // telegram_id → userID
	linkLog    []string         // userIDs that got LinkTelegramID called
}

func newTelegramFakeAuth() *telegramFakeAuth {
	return &telegramFakeAuth{fakeAuth: newFakeAuth(), byTelegram: map[int64]string{}}
}

func (f *telegramFakeAuth) GetUserByTelegramID(_ context.Context, tgID int64) (*domain.AdminUser, error) {
	id, ok := f.byTelegram[tgID]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return f.byID[id], nil
}

func (f *telegramFakeAuth) LinkTelegramID(_ context.Context, userID string, tgID int64, _, _ string) error {
	f.byTelegram[tgID] = userID
	f.linkLog = append(f.linkLog, userID)
	return nil
}

func (f *telegramFakeAuth) CreateTelegramUser(_ context.Context, u *domain.AdminUser, tgID int64, _, _ string) (*domain.AdminUser, error) {
	u.ID = "user-" + u.Email
	f.users[strings.ToLower(u.Email)] = u
	f.byID[u.ID] = u
	f.byTelegram[tgID] = u.ID
	return u, nil
}

func mkTelegramUC(withOIDC bool) (*TelegramAuthUseCase, *telegramFakeAuth, *fakeAdminCatalog, *fakeMemberships, *fakeOAuthState) {
	auth := newTelegramFakeAuth()
	cat := newFakeAdminCatalog()
	mem := newFakeMemberships()
	state := newFakeOAuthState()
	sess := newRichSessions()
	sessUC := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx",
		15*time.Minute, 30*24*time.Hour, logger.New("error"))
	var oidc *telegram.OIDCClient
	if withOIDC {
		oidc = telegram.NewOIDCClient("12345:test-bot-token", "https://app.example.com/auth/telegram/callback")
	}
	uc := NewTelegramAuthUseCase("12345:test-bot-token", oidc, state, auth, cat, mem, sessUC, logger.New("error"))
	return uc, auth, cat, mem, state
}

// TestScenario_032_TelegramNewUser verifies:
// «Я как новый пользователь нажимаю "Continue with Telegram" → redirect к
// Telegram OIDC → callback → создаётся state + новый тенант + admin_user с
// привязанным telegram_id, выдаётся session pair.» (sec 5, scenario 32)
func TestScenario_032_TelegramNewUser_CreatesTenantAndUser(t *testing.T) {
	uc, auth, cat, mem, _ := mkTelegramUC(true)

	user, err := uc.findOrCreateOIDC(context.Background(), &telegram.OIDCUserInfo{
		Sub:       "777111",
		FirstName: "New",
		LastName:  "User",
		Username:  "newuser_tg",
	})
	if err != nil {
		t.Fatalf("findOrCreateOIDC: %v", err)
	}
	if user.ID == "" {
		t.Errorf("user.ID empty")
	}
	if !strings.HasSuffix(user.Email, "@telegram.keepstar.local") {
		t.Errorf("synthetic email malformed: %q", user.Email)
	}
	if len(cat.createdLog) != 1 {
		t.Errorf("tenant not created: %d", len(cat.createdLog))
	}
	if _, ok, _ := mem.HasMembership(context.Background(), user.ID, user.TenantID); !ok {
		t.Errorf("owner membership not granted")
	}
	if auth.byTelegram[777111] != user.ID {
		t.Errorf("telegram_id not registered against new user; got %v", auth.byTelegram)
	}
}

// TestScenario_033_TelegramExistingEmailUser_LinksTelegramID was the
// original "Telegram → existing email user merge by email" scenario.
//
// MARKED N/A: Telegram OIDC scope does NOT expose email — the OIDCUserInfo
// payload contains only sub / first_name / last_name / username / photo_url.
// Without an email from Telegram there's no way to cascade onto an existing
// account by email; the only stable identifier is telegram_id (scenario 34
// covers the fast-path).
//
// Kept as t.Skip rather than deleted so future readers see the scenario was
// considered and dropped on purpose, not forgotten.
func TestScenario_033_TelegramExistingEmailUser_LinksTelegramID(t *testing.T) {
	t.Skip("scenario 33 N/A — Telegram OIDC does not expose email scope; see docs/pre_launch_scenarios.md")
}

// TestScenario_034_TelegramExistingUser_FastPath verifies:
// «Я как существующий Telegram-пользователь (был раньше) — fast path step 1,
// без баннера.» (sec 5, scenario 34)
func TestScenario_034_TelegramExistingUser_FastPath(t *testing.T) {
	uc, auth, cat, _, _ := mkTelegramUC(true)
	existing, _ := auth.CreateTelegramUser(context.Background(),
		&domain.AdminUser{Email: "bob@telegram.keepstar.local", TenantID: "t-bob", Role: domain.AdminRoleOwner},
		777333, "bob_tg", "")

	user, err := uc.findOrCreateOIDC(context.Background(), &telegram.OIDCUserInfo{
		Sub:       "777333",
		Username:  "bob_tg",
		FirstName: "Bob",
	})
	if err != nil {
		t.Fatalf("findOrCreateOIDC: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("fast-path returned wrong user: got %s, want %s", user.ID, existing.ID)
	}
	if len(cat.createdLog) != 0 {
		t.Errorf("tenant created on fast-path (should reuse existing): %d", len(cat.createdLog))
	}
}

// TestScenario_035_TelegramExpiredState_FriendlyRejection verifies:
// «Я как пользователь возвращаюсь с истёкшим/неправильным state — отклоняется
// (см. 26 — нужен дружелюбный экран, аналогично Google).» (sec 5, scenario 35)
//
// At the usecase boundary this test checks that an unknown / expired state
// returns an explicit error message. The "friendly screen" is rendered by the
// frontend AuthErrorPage from this message — but we at least verify the
// backend surface produces the right error so the FE has something to react
// to. EXPECTED TO FAIL ONLY IF the message text is fuzzed-up later.
func TestScenario_035_TelegramExpiredState_FriendlyRejection(t *testing.T) {
	uc, _, _, _, _ := mkTelegramUC(true)

	_, err := uc.CompleteOIDC(context.Background(), "any-code", "ghost-state", "", "")
	if err == nil {
		t.Fatal("expected err on unknown state")
	}
	if !strings.Contains(err.Error(), "invalid or expired state") {
		t.Errorf("err msg = %q, want it to contain 'invalid or expired state' (frontend keys on this)", err.Error())
	}
}

// TestScenario_036_TelegramTgAuthResultHashFormat verifies:
// «Я как пользователь возвращаюсь с handler URL содержащим `#tgAuthResult`
// (а не настоящий OIDC code) — backend имеет специальную обработку этого
// формата (commit 109378a).» (sec 5, scenario 36)
//
// Telegram's "OIDC" actually returns the widget JSON payload as a URL hash:
// `#tgAuthResult=<base64url(JSON)>`. The frontend strips it and POSTs to the
// legacy widget endpoint, which exercises the HMAC Verify() path (NOT
// Exchange()). This test seeds a valid widget HMAC and exercises Complete().
func TestScenario_036_TelegramTgAuthResultHashFormat_HandledViaWidget(t *testing.T) {
	uc, _, _, _, _ := mkTelegramUC(true)

	fields := makeTelegramWidgetPayload(t, "12345:test-bot-token", map[string]string{
		"id":         "777444",
		"first_name": "Hash",
		"username":   "hashfmt",
	})
	resp, err := uc.Complete(context.Background(), fields, "", "")
	if err != nil {
		t.Fatalf("Complete(widget): %v — backend should accept HMAC-signed payload from #tgAuthResult flow", err)
	}
	if resp == nil || resp.User == nil {
		t.Fatalf("nil user after telegram widget complete")
	}
}

// TestScenario_037_TelegramLegacyWidget_FallbackWorks verifies:
// «Я как пользователь имею аккаунт через Telegram legacy widget (старая
// интеграция) — fallback handler работает, новых OIDC redirect'ов система
// не делает.» (sec 5, scenario 37)
func TestScenario_037_TelegramLegacyWidget_FallbackWorks(t *testing.T) {
	uc, _, _, _, _ := mkTelegramUC(false) // no OIDC client

	fields := makeTelegramWidgetPayload(t, "12345:test-bot-token", map[string]string{
		"id":         "777555",
		"first_name": "Legacy",
		"username":   "legacy_tg",
	})
	resp, err := uc.Complete(context.Background(), fields, "", "")
	if err != nil {
		t.Fatalf("legacy widget Complete: %v", err)
	}
	if resp.User == nil {
		t.Errorf("user nil from legacy widget flow")
	}
	if uc.HasOIDC() {
		t.Error("HasOIDC()=true with nil OIDC client — must be false")
	}
}

// TestScenario_032b_TelegramStart_RequiresOIDC backs scenario 32: Start()
// surface error must be loud when OIDC isn't configured so the frontend
// doesn't render a dead-button OIDC entry point.
func TestScenario_032b_TelegramStart_RequiresOIDC(t *testing.T) {
	uc, _, _, _, _ := mkTelegramUC(false)
	if _, err := uc.Start(context.Background()); err == nil {
		t.Error("Start() should err when OIDC client is nil")
	}
}

// --- helpers --------------------------------------------------------------

// makeTelegramWidgetPayload builds a HMAC-signed Telegram Login Widget
// payload using the canonical algorithm (sorted key=value, joined by \n,
// HMAC-SHA256 with key=SHA256(bot_token), hex hash). Matches the
// production Verify() expectation byte-for-byte.
func makeTelegramWidgetPayload(t *testing.T, botToken string, fields map[string]string) map[string]string {
	t.Helper()
	if _, ok := fields["auth_date"]; !ok {
		fields["auth_date"] = strconv.FormatInt(time.Now().Unix(), 10)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fields[k])
	}

	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(sb.String()))
	fields["hash"] = hex.EncodeToString(mac.Sum(nil))
	return fields
}
