package usecases

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/crypto/secretbox"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// statefulFakeAuth extends fakeAuth with real TOTP secret storage. The
// magic-link suite's fakeAuth no-ops these methods, which would break
// SetupTOTP → ConfirmTOTP → VerifyTOTP.
type statefulFakeAuth struct {
	*fakeAuth
	totpByUser map[string]string // userID → encrypted secret
	enabled    map[string]bool   // userID → totp enabled
}

func newStatefulFakeAuth() *statefulFakeAuth {
	return &statefulFakeAuth{
		fakeAuth:   newFakeAuth(),
		totpByUser: map[string]string{},
		enabled:    map[string]bool{},
	}
}
func (f *statefulFakeAuth) SetTOTPSecret(_ context.Context, userID, enc string) error {
	f.totpByUser[userID] = enc
	return nil
}
func (f *statefulFakeAuth) GetTOTPSecret(_ context.Context, userID string) (string, bool, error) {
	return f.totpByUser[userID], f.enabled[userID], nil
}
func (f *statefulFakeAuth) EnableTOTP(_ context.Context, userID string) error {
	f.enabled[userID] = true
	return nil
}
func (f *statefulFakeAuth) DisableTOTP(_ context.Context, userID string) error {
	delete(f.totpByUser, userID)
	delete(f.enabled, userID)
	return nil
}

// generateCode replicates the TOTP HOTP algorithm so tests can produce a
// valid code for `now` without depending on internal totp package helpers.
func generateCode(secretBase32 string) string {
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.ToUpper(secretBase32))
	if err != nil {
		return ""
	}
	counter := time.Now().Unix() / 30
	var ctr [8]byte
	binary.BigEndian.PutUint64(ctr[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(ctr[:])
	h := mac.Sum(nil)
	offset := h[len(h)-1] & 0x0f
	binCode := (uint32(h[offset])&0x7f)<<24 |
		uint32(h[offset+1])<<16 |
		uint32(h[offset+2])<<8 |
		uint32(h[offset+3])
	return fmt.Sprintf("%06d", binCode%1000000)
}

func mk2FAUC() (*TwoFactorUseCase, *statefulFakeAuth, *fakeChallenges, *fakeMailer, *secretbox.Box) {
	auth := newStatefulFakeAuth()
	ch := newFakeChallenges()
	mail := &fakeMailer{}
	sess := newRichSessions()
	sessUC := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx",
		15*time.Minute, 30*24*time.Hour, logger.New("error"))
	box, _ := secretbox.New(bytes32("test-test-test-test-test-test-12"))
	uc := NewTwoFactorUseCase(auth, ch, mail, sessUC, box, "keepstar-test", 15*time.Minute, logger.New("error"))
	return uc, auth, ch, mail, box
}

func bytes32(s string) []byte {
	b := []byte(s)
	for len(b) < 32 {
		b = append(b, 0)
	}
	return b[:32]
}

func TestTOTP_SetupGeneratesSecretAndStores(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	res, err := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if res.Secret == "" || res.OTPAuthURL == "" {
		t.Errorf("setup empty: %+v", res)
	}
	if _, ok := auth.totpByUser[user.ID]; !ok {
		t.Error("encrypted secret not stored")
	}
	if auth.enabled[user.ID] {
		t.Error("totp_enabled should still be false after Setup (waits for ConfirmTOTP)")
	}
}

func TestTOTP_ConfirmHappyPath(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	setup, err := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	code := generateCode(setup.Secret)
	if err := uc.ConfirmTOTP(context.Background(), user.ID, code); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}
	if !auth.enabled[user.ID] {
		t.Error("totp not enabled after successful confirm")
	}
}

func TestTOTP_ConfirmBadCodeRejects(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")
	if _, err := uc.SetupTOTP(context.Background(), user.ID, "u@x"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err := uc.ConfirmTOTP(context.Background(), user.ID, "000000")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if auth.enabled[user.ID] {
		t.Error("enabled flag flipped despite bad code")
	}
}

func TestTOTP_ConfirmWithoutSetupErrors(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	if err := uc.ConfirmTOTP(context.Background(), user.ID, "123456"); err == nil {
		t.Error("expected err when secret not set")
	}
}

func TestTOTP_VerifyHappyPath(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	setup, _ := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	_ = uc.ConfirmTOTP(context.Background(), user.ID, generateCode(setup.Secret))

	resp, err := uc.VerifyTOTP(context.Background(), user.ID, generateCode(setup.Secret), "", "")
	if err != nil {
		t.Fatalf("VerifyTOTP: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("token pair empty: %+v", resp)
	}
	if resp.User == nil || resp.User.ID != user.ID {
		t.Errorf("user not returned: %+v", resp.User)
	}
}

func TestTOTP_VerifyDisabledErrors(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	setup, _ := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	// Skip ConfirmTOTP — enabled stays false.

	if _, err := uc.VerifyTOTP(context.Background(), user.ID, generateCode(setup.Secret), "", ""); err == nil {
		t.Error("expected err on Verify when totp disabled")
	}
}

func TestTOTP_DisableClearsState(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")
	setup, _ := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	_ = uc.ConfirmTOTP(context.Background(), user.ID, generateCode(setup.Secret))

	if err := uc.DisableTOTP(context.Background(), user.ID); err != nil {
		t.Fatalf("DisableTOTP: %v", err)
	}
	if auth.enabled[user.ID] {
		t.Error("enabled flag still true after Disable")
	}
	if _, ok := auth.totpByUser[user.ID]; ok {
		t.Error("secret still stored after Disable")
	}
}

func TestEnabled_Reflects2FAState(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	if v, _ := uc.Enabled(context.Background(), user.ID); v {
		t.Error("Enabled true before setup")
	}
	setup, _ := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	if v, _ := uc.Enabled(context.Background(), user.ID); v {
		t.Error("Enabled true after Setup but before Confirm")
	}
	_ = uc.ConfirmTOTP(context.Background(), user.ID, generateCode(setup.Secret))
	if v, _ := uc.Enabled(context.Background(), user.ID); !v {
		t.Error("Enabled false after Confirm")
	}
}

// --- Email 2FA path ---

func TestEmail2FA_SendCreatesChallengeAndMails(t *testing.T) {
	uc, _, ch, mail, _ := mk2FAUC()

	if err := uc.SendEmailCode(context.Background(), "user-1", "u@x.com"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	if ch.calls != 1 {
		t.Errorf("challenges created = %d, want 1", ch.calls)
	}
	if len(mail.sent) != 1 {
		t.Errorf("emails sent = %d, want 1", len(mail.sent))
	}
	if mail.sent[0].Kind != "email_2fa" {
		t.Errorf("email kind = %q, want email_2fa", mail.sent[0].Kind)
	}
	if mail.sent[0].To != "u@x.com" {
		t.Errorf("email to = %q, want u@x.com", mail.sent[0].To)
	}
	code, _ := mail.sent[0].Data["Code"].(string)
	if len(code) != 6 {
		t.Errorf("code length = %d, want 6", len(code))
	}
}

func TestEmail2FA_VerifyHappyPath(t *testing.T) {
	uc, auth, _, mail, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x.com")

	_ = uc.SendEmailCode(context.Background(), user.ID, user.Email)
	code, _ := mail.sent[0].Data["Code"].(string)

	resp, err := uc.VerifyEmailCode(context.Background(), user.ID, code, "", "")
	if err != nil {
		t.Fatalf("VerifyEmailCode: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("token pair empty after email 2FA verify")
	}
}

func TestEmail2FA_VerifyWrongCodeRejects(t *testing.T) {
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x.com")
	_ = uc.SendEmailCode(context.Background(), user.ID, user.Email)

	_, err := uc.VerifyEmailCode(context.Background(), user.ID, "000000", "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestEmail2FA_VerifyWrongUserRejects(t *testing.T) {
	uc, auth, _, mail, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x.com")
	_ = uc.SendEmailCode(context.Background(), user.ID, user.Email)
	code, _ := mail.sent[0].Data["Code"].(string)

	// Wrong userID — should be ErrInvalidCredentials.
	_, err := uc.VerifyEmailCode(context.Background(), "different-user", code, "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestEmail2FA_SendWithoutMailerErrors(t *testing.T) {
	auth := newStatefulFakeAuth()
	ch := newFakeChallenges()
	sess := newRichSessions()
	sessUC := NewSessionsUseCase(sess, auth, "test-secret", 15*time.Minute, 30*24*time.Hour, logger.New("error"))
	box, _ := secretbox.New(bytes32("test-test-test-test-test-test-12"))
	uc := NewTwoFactorUseCase(auth, ch, nil /* mailer */, sessUC, box, "keepstar-test", 15*time.Minute, logger.New("error"))

	if err := uc.SendEmailCode(context.Background(), "u1", "x@x"); err == nil {
		t.Error("expected err when mailer is nil")
	}
}

// --- regression: generateCode helper actually matches totp.Verify ---

func TestGenerateCodeMatchesAlgorithm(t *testing.T) {
	// Sanity check: round-trip through Setup → Confirm. If generateCode drifts
	// from the production algorithm, the TOTP tests above all break first;
	// this keeps the regression localized.
	uc, auth, _, _, _ := mk2FAUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	user, _ := auth.GetUserByEmail(context.Background(), "u@x")

	setup, _ := uc.SetupTOTP(context.Background(), user.ID, "u@x")
	if err := uc.ConfirmTOTP(context.Background(), user.ID, generateCode(setup.Secret)); err != nil {
		t.Errorf("generateCode disagreed with adapter: %v", err)
	}
}

// --- silence unused-import warning ---

var _ ports.ChallengePort = (*fakeChallenges)(nil)
