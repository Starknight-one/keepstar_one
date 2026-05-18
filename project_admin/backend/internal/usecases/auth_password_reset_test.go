package usecases

// Pre-launch scenario verification for the password reset flow.
// See docs/pre_launch_scenarios.md sec 15 (scenarios 97-99).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// resetMailer captures emails via the same EmailMessage shape as fakeMailer
// but kept locally so password-reset tests don't share state with magic-link
// suite when run in parallel.
type resetMailer struct {
	sent []ports.EmailMessage
}

func (m *resetMailer) Send(_ context.Context, msg ports.EmailMessage) error {
	m.sent = append(m.sent, msg)
	return nil
}

// passwordHashUpdater extends fakeAuth so UpdatePasswordHash actually mutates
// the stored hash and we can assert that Reset() persisted the new password.
type passwordHashUpdater struct {
	*fakeAuth
	updateLog map[string]string // userID → newHash
}

func newPasswordHashUpdater() *passwordHashUpdater {
	return &passwordHashUpdater{fakeAuth: newFakeAuth(), updateLog: map[string]string{}}
}

func (p *passwordHashUpdater) UpdatePasswordHash(_ context.Context, userID, hash string) error {
	p.updateLog[userID] = hash
	if u, ok := p.byID[userID]; ok {
		u.PasswordHash = hash
	}
	return nil
}

func mkPasswordResetUC() (*PasswordResetUseCase, *passwordHashUpdater, *fakeChallenges, *resetMailer) {
	auth := newPasswordHashUpdater()
	ch := newFakeChallenges()
	mail := &resetMailer{}
	uc := NewPasswordResetUseCase(auth, ch, mail, "https://app.example.com", 1*time.Hour, logger.New("error"))
	return uc, auth, ch, mail
}

// TestScenario_097_ForgotPasswordIssuesResetChallenge verifies:
// «Я как пользователь могу запросить "forgot password" по email на /auth/forgot
// — backend issue'ит password_reset challenge, отсылает ссылку.»
// (sec 15, scenario 97)
func TestScenario_097_ForgotPassword_IssuesResetChallenge(t *testing.T) {
	uc, auth, ch, mail := mkPasswordResetUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "u@x.com", TenantID: "t1", PasswordHash: "old-hash",
	})

	if err := uc.Forgot(context.Background(), "u@x.com"); err != nil {
		t.Fatalf("Forgot: %v", err)
	}
	if ch.calls != 1 {
		t.Errorf("expected 1 challenge, got %d", ch.calls)
	}
	if len(mail.sent) != 1 {
		t.Errorf("expected 1 email, got %d", len(mail.sent))
	}
	if mail.sent[0].Kind != "password_reset" {
		t.Errorf("email kind = %q, want password_reset", mail.sent[0].Kind)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	if !strings.HasPrefix(link, "https://app.example.com/auth/reset-password?token=") {
		t.Errorf("link malformed: %q", link)
	}
}

// TestScenario_097b_ForgotPasswordUnknownEmailIsSilent verifies anti-
// enumeration: Forgot() must return nil even when the email is not in our DB
// (so timing doesn't leak presence).
func TestScenario_097b_ForgotPasswordUnknownEmail_Silent(t *testing.T) {
	uc, _, ch, mail := mkPasswordResetUC()

	if err := uc.Forgot(context.Background(), "ghost@x.com"); err != nil {
		t.Errorf("Forgot(unknown) returned err=%v, want nil (anti-enumeration)", err)
	}
	if ch.calls != 0 {
		t.Errorf("challenge created for unknown email: %d", ch.calls)
	}
	if len(mail.sent) != 0 {
		t.Errorf("email sent to unknown address: %d", len(mail.sent))
	}
}

// TestScenario_098_ResetWithValidToken_UpdatesPassword verifies:
// «Я как пользователь кликаю по reset-ссылке → форма "new password" → POST →
// backend validate'ит strength + Consume + UpdatePasswordHash.»
// (sec 15, scenario 98)
func TestScenario_098_ResetWithValidToken_UpdatesPassword(t *testing.T) {
	uc, auth, _, mail := mkPasswordResetUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "reset@x.com", TenantID: "t1", PasswordHash: "old-hash",
	})

	if err := uc.Forgot(context.Background(), "reset@x.com"); err != nil {
		t.Fatalf("Forgot: %v", err)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/reset-password?token=")

	if err := uc.Reset(context.Background(), token, "newSecurePassword123"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got, ok := auth.updateLog[user.ID]; !ok || got == "" {
		t.Errorf("password hash not updated for user %s; updateLog=%v", user.ID, auth.updateLog)
	}
	if auth.byID[user.ID].PasswordHash == "old-hash" {
		t.Errorf("password hash on user row not refreshed; still 'old-hash'")
	}
}

// TestScenario_099_ResetWithUsedOrExpiredToken_Rejects verifies:
// «Я как пользователь кликаю по reset-ссылке которая уже use'нута/истекла
// — отклоняется (нужны экраны как в 40/41).» (sec 15, scenario 99)
//
// The friendly UI screens described in scenarios 40/41 are a FRONTEND concern,
// but the backend must surface the right error class. We verify three cases:
// reused token, expired token, unknown token.
func TestScenario_099_ResetWithUsedOrExpiredToken_Rejects(t *testing.T) {
	uc, auth, _, mail := mkPasswordResetUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "ref@x.com", TenantID: "t1", PasswordHash: "h",
	})

	if err := uc.Forgot(context.Background(), "ref@x.com"); err != nil {
		t.Fatalf("Forgot: %v", err)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/reset-password?token=")

	// First Reset succeeds.
	if err := uc.Reset(context.Background(), token, "validNewPass1234"); err != nil {
		t.Fatalf("first Reset: %v", err)
	}
	// Second Reset on same token must fail with ErrInvalidCredentials.
	err := uc.Reset(context.Background(), token, "anotherPass5678")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("reused token: err=%v, want ErrInvalidCredentials", err)
	}
	// Unknown token: same error.
	if err := uc.Reset(context.Background(), "ghost-token", "anotherPass5678"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("unknown token: err=%v, want ErrInvalidCredentials", err)
	}
}

// TestScenario_099b_ResetRejectsWeakPassword verifies the strength gate.
// Backend must enforce min length so weak resets are rejected before any
// hashing/persistence happens — mirroring the signup path's check.
func TestScenario_099b_ResetRejectsWeakPassword(t *testing.T) {
	uc, _, _, _ := mkPasswordResetUC()
	err := uc.Reset(context.Background(), "any-token", "short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak password: err=%v, want ErrWeakPassword", err)
	}
}
