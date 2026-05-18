package usecases

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"keepstar-admin/internal/adapters/totp"
	"keepstar-admin/internal/crypto/secretbox"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// TwoFactorUseCase handles TOTP setup/confirm/verify and email 2FA code
// send/verify. TOTP secrets are stored encrypted via the shared secretbox
// (same envelope as Shopify tokens). Email 2FA codes live in auth_challenges
// as sha256 hashes with a short TTL.
type TwoFactorUseCase struct {
	users        ports.AuthPort
	challenges   ports.ChallengePort
	mailer       ports.MailerPort
	sessions     *SessionsUseCase
	box          *secretbox.Box
	issuer       string
	emailCodeTTL time.Duration
	log          *logger.Logger
}

func NewTwoFactorUseCase(
	users ports.AuthPort,
	challenges ports.ChallengePort,
	mailer ports.MailerPort,
	sessions *SessionsUseCase,
	box *secretbox.Box,
	issuer string,
	emailCodeTTL time.Duration,
	log *logger.Logger,
) *TwoFactorUseCase {
	return &TwoFactorUseCase{
		users: users, challenges: challenges, mailer: mailer,
		sessions: sessions, box: box, issuer: issuer,
		emailCodeTTL: emailCodeTTL, log: log,
	}
}

// Enabled reports whether the user has 2FA turned on (TOTP in this release).
// Called from Login to decide between issuing a pre-2FA token vs full pair.
func (uc *TwoFactorUseCase) Enabled(ctx context.Context, userID string) (bool, error) {
	_, enabled, err := uc.users.GetTOTPSecret(ctx, userID)
	return enabled, err
}

type TOTPSetupResult struct {
	Secret         string `json:"secret"`
	OTPAuthURL     string `json:"otpauth_url"`
}

// SetupTOTP mints a fresh secret, encrypts + stores it (disabled until
// confirmed), and returns the otpauth URL for the user to scan.
func (uc *TwoFactorUseCase) SetupTOTP(ctx context.Context, userID, userEmail string) (*TOTPSetupResult, error) {
	if uc.box == nil {
		return nil, errors.New("encryption not configured — set ADMIN_ENCRYPTION_KEY")
	}
	secret, err := totp.NewSecret()
	if err != nil {
		return nil, fmt.Errorf("mint secret: %w", err)
	}
	enc, err := uc.box.Seal([]byte(secret))
	if err != nil {
		return nil, fmt.Errorf("seal secret: %w", err)
	}
	if err := uc.users.SetTOTPSecret(ctx, userID, enc); err != nil {
		return nil, err
	}
	return &TOTPSetupResult{
		Secret:     secret,
		OTPAuthURL: totp.ProvisioningURL(secret, userEmail, uc.issuer),
	}, nil
}

// ConfirmTOTP verifies the first code and flips totp_enabled_at. A failed
// confirm leaves the secret in place so the user can retry without a round
// trip — we rely on the enable flag as the real activation signal.
func (uc *TwoFactorUseCase) ConfirmTOTP(ctx context.Context, userID, code string) error {
	enc, _, err := uc.users.GetTOTPSecret(ctx, userID)
	if err != nil {
		return err
	}
	if enc == "" {
		return errors.New("no totp secret set — call setup first")
	}
	plain, err := uc.box.Open(enc)
	if err != nil {
		return fmt.Errorf("decrypt secret: %w", err)
	}
	if !totp.Verify(string(plain), code) {
		return domain.ErrInvalidCredentials
	}
	return uc.users.EnableTOTP(ctx, userID)
}

// VerifyTOTP is the login-time second step. Returns a full token pair via
// SessionsUseCase when the code matches. Caller must have already validated
// the pre-2FA token and extracted userID from it.
func (uc *TwoFactorUseCase) VerifyTOTP(ctx context.Context, userID, code, ua, ip string) (*AuthResponse, error) {
	enc, enabled, err := uc.users.GetTOTPSecret(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !enabled || enc == "" {
		return nil, errors.New("totp not enabled")
	}
	plain, err := uc.box.Open(enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	if !totp.Verify(string(plain), code) {
		return nil, domain.ErrInvalidCredentials
	}
	return uc.issuePair(ctx, userID, ua, ip)
}

// DisableTOTP clears the secret + enabled flag. Requires the current TOTP
// code as re-auth proof of possession (sc 89) — session hijack alone shouldn't
// be enough to strip the second factor.
func (uc *TwoFactorUseCase) DisableTOTP(ctx context.Context, userID, code string) error {
	if code == "" {
		return domain.ErrInvalidCredentials
	}
	enc, enabled, err := uc.users.GetTOTPSecret(ctx, userID)
	if err != nil {
		return err
	}
	if !enabled || enc == "" {
		return errors.New("totp not enabled")
	}
	plain, err := uc.box.Open(enc)
	if err != nil {
		return fmt.Errorf("decrypt secret: %w", err)
	}
	if !totp.Verify(string(plain), code) {
		return domain.ErrInvalidCredentials
	}
	return uc.users.DisableTOTP(ctx, userID)
}

// SendEmailCode is the email 2FA "send" step called between password-check
// and code-verify. Writes a 6-digit numeric code to auth_challenges and
// emails it. Codes are single-use and expire in emailCodeTTL (default 15m).
func (uc *TwoFactorUseCase) SendEmailCode(ctx context.Context, userID, email string) error {
	if uc.mailer == nil {
		return errors.New("smtp not configured")
	}
	code, err := randomDigits(6)
	if err != nil {
		return err
	}
	ch := &ports.Challenge{
		UserID:    userID,
		Email:     email,
		Kind:      ports.ChallengeEmail2FA,
		CodeHash:  hashCode(code),
		ExpiresAt: time.Now().Add(uc.emailCodeTTL),
	}
	if err := uc.challenges.Create(ctx, ch); err != nil {
		return err
	}
	if err := uc.mailer.Send(ctx, ports.EmailMessage{
		To:   email,
		Kind: "email_2fa",
		Data: map[string]any{"Code": code, "TTLMinutes": int(uc.emailCodeTTL.Minutes())},
	}); err != nil {
		uc.log.Error("email_2fa_send_failed", "error", err)
		return err
	}
	return nil
}

// VerifyEmailCode consumes the code and returns a full token pair. Like
// VerifyTOTP, the caller has validated the pre-2FA token already.
func (uc *TwoFactorUseCase) VerifyEmailCode(ctx context.Context, userID, code, ua, ip string) (*AuthResponse, error) {
	ch, err := uc.challenges.FindActive(ctx, ports.ChallengeEmail2FA, hashCode(code))
	if err != nil {
		return nil, err
	}
	if ch == nil || ch.UserID != userID {
		return nil, domain.ErrInvalidCredentials
	}
	if err := uc.challenges.Consume(ctx, ch.ID); err != nil {
		return nil, err
	}
	return uc.issuePair(ctx, userID, ua, ip)
}

func (uc *TwoFactorUseCase) issuePair(ctx context.Context, userID, ua, ip string) (*AuthResponse, error) {
	user, err := uc.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	_ = uc.users.TouchLastLogin(ctx, userID)
	pair, err := uc.sessions.Issue(ctx, user, ua, ip)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{
		Token:        pair.AccessToken,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		User:         user,
	}, nil
}

// randomDigits returns a decimal code of the given length via crypto/rand.
func randomDigits(n int) (string, error) {
	out := make([]byte, n)
	bigTen := big.NewInt(10)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, bigTen)
		if err != nil {
			return "", err
		}
		out[i] = byte('0' + v.Int64())
	}
	return string(out), nil
}
