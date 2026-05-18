package usecases

// Pre-launch scenario verification for the email-verify flow.
// See docs/pre_launch_scenarios.md sec 15 (scenarios 100-102).

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

// emailVerifyMailer is a local recording mailer (mirrors fakeMailer but
// scoped per-test).
type emailVerifyMailer struct {
	sent []ports.EmailMessage
}

func (m *emailVerifyMailer) Send(_ context.Context, msg ports.EmailMessage) error {
	m.sent = append(m.sent, msg)
	return nil
}

// emailVerifyAuth extends fakeAuth with mutable email_verified_at semantics.
type emailVerifyAuth struct {
	*fakeAuth
	verifiedAt map[string]bool // userID → verified flag
}

func newEmailVerifyAuth() *emailVerifyAuth {
	return &emailVerifyAuth{fakeAuth: newFakeAuth(), verifiedAt: map[string]bool{}}
}

func (e *emailVerifyAuth) MarkEmailVerified(_ context.Context, userID string) error {
	e.verifiedAt[userID] = true
	return nil
}

func mkEmailVerifyUC() (*EmailVerifyUseCase, *emailVerifyAuth, *fakeChallenges, *emailVerifyMailer) {
	auth := newEmailVerifyAuth()
	ch := newFakeChallenges()
	mail := &emailVerifyMailer{}
	uc := NewEmailVerifyUseCase(auth, ch, mail, "https://app.example.com", 24*time.Hour, logger.New("error"))
	return uc, auth, ch, mail
}

// TestScenario_100_IssueEmailVerifyLink verifies:
// «Я как пользователь могу запросить email verify (Settings → Verify email)
// — backend issue'ит email_verify challenge.» (sec 15, scenario 100)
func TestScenario_100_IssueEmailVerifyLink(t *testing.T) {
	uc, _, ch, mail := mkEmailVerifyUC()

	if err := uc.Issue(context.Background(), "user-1", "u@x.com"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if ch.calls != 1 {
		t.Errorf("expected 1 challenge, got %d", ch.calls)
	}
	if len(mail.sent) != 1 {
		t.Errorf("expected 1 email, got %d", len(mail.sent))
	}
	if mail.sent[0].Kind != "email_verify" {
		t.Errorf("email kind = %q, want email_verify", mail.sent[0].Kind)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	if !strings.HasPrefix(link, "https://app.example.com/auth/verify-email?token=") {
		t.Errorf("link malformed: %q", link)
	}
}

// TestScenario_101_VerifyEmailLink_FlipsVerifiedAt verifies:
// «Я как пользователь кликаю по verify-ссылке → backend Consume →
// admin_users.email_verified_at=NOW().» (sec 15, scenario 101)
func TestScenario_101_VerifyEmailLink_FlipsVerifiedAt(t *testing.T) {
	uc, auth, _, mail := mkEmailVerifyUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "verify@x.com", TenantID: "t1",
	})
	if err := uc.Issue(context.Background(), user.ID, user.Email); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/verify-email?token=")

	if err := uc.Verify(context.Background(), token); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !auth.verifiedAt[user.ID] {
		t.Errorf("MarkEmailVerified was not called for user %s", user.ID)
	}

	// Reuse must fail (single-use).
	if err := uc.Verify(context.Background(), token); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("reused verify token: err=%v, want ErrInvalidCredentials", err)
	}
}

// TestScenario_102_ResendEmailVerify_NoOpForUnknownEmail verifies:
// «Я как пользователь могу resend verify-ссылку с rate-limit.»
// (sec 15, scenario 102)
//
// The current implementation issues a new challenge on each Resend call.
// Rate-limit behavior is a separate gap — for now we verify the basic
// happy path: known email → Issue runs; unknown email → silent no-op.
func TestScenario_102_ResendEmailVerify_HandlesKnownAndUnknown(t *testing.T) {
	uc, auth, ch, mail := mkEmailVerifyUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "resend@x.com", TenantID: "t1",
	})

	if err := uc.Resend(context.Background(), "resend@x.com"); err != nil {
		t.Fatalf("Resend(known): %v", err)
	}
	if ch.calls != 1 || len(mail.sent) != 1 {
		t.Errorf("Resend(known) should issue: ch=%d, sent=%d", ch.calls, len(mail.sent))
	}

	// Unknown email: silent no-op (no challenge, no email, no error).
	if err := uc.Resend(context.Background(), "ghost@x.com"); err != nil {
		t.Errorf("Resend(unknown): err=%v, want nil", err)
	}
	if ch.calls != 1 || len(mail.sent) != 1 {
		t.Errorf("Resend(unknown) leaked: ch=%d, sent=%d", ch.calls, len(mail.sent))
	}
}
