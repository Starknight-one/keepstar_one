package usecases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// PasswordResetUseCase issues and redeems reset links.
//
// Enumeration protection: `Forgot` ALWAYS returns nil error regardless of
// whether the email exists. The handler returns 200 either way, and we go
// through the motions (hash a dummy code) so the response time doesn't leak.
type PasswordResetUseCase struct {
	users      ports.AuthPort
	challenges ports.ChallengePort
	mailer     ports.MailerPort
	baseURL    string
	resetTTL   time.Duration
	log        *logger.Logger
}

func NewPasswordResetUseCase(users ports.AuthPort, ch ports.ChallengePort, mailer ports.MailerPort, baseURL string, resetTTL time.Duration, log *logger.Logger) *PasswordResetUseCase {
	return &PasswordResetUseCase{
		users:      users,
		challenges: ch,
		mailer:     mailer,
		baseURL:    baseURL,
		resetTTL:   resetTTL,
		log:        log,
	}
}

// Forgot is best-effort — never surfaces "user not found" to the caller.
func (uc *PasswordResetUseCase) Forgot(ctx context.Context, email string) error {
	user, err := uc.users.GetUserByEmail(ctx, email)
	if err != nil || user == nil {
		// Spend cycles hashing a dummy code to equalize timing with the real path.
		_ = hashCode(randomHex(32))
		return nil
	}

	token := randomHex(32)
	hash := hashCode(token)
	ch := &ports.Challenge{
		UserID:    user.ID,
		Email:     user.Email,
		Kind:      ports.ChallengePasswordReset,
		CodeHash:  hash,
		ExpiresAt: time.Now().Add(uc.resetTTL),
	}
	if err := uc.challenges.Create(ctx, ch); err != nil {
		uc.log.Error("password_reset_challenge_failed", "email", email, "error", err)
		return nil
	}

	link := fmt.Sprintf("%s/auth/reset-password?token=%s", uc.baseURL, token)
	if uc.mailer != nil {
		if err := uc.mailer.Send(ctx, ports.EmailMessage{
			To:   user.Email,
			Kind: "password_reset",
			Data: map[string]any{"Email": user.Email, "Link": link},
		}); err != nil {
			uc.log.Error("password_reset_mail_failed", "email", email, "error", err)
		}
	}
	return nil
}

// Reset validates the token, updates the password hash, consumes the challenge.
func (uc *PasswordResetUseCase) Reset(ctx context.Context, token, newPassword string) error {
	if err := validatePasswordStrength(newPassword); err != nil {
		return err
	}

	ch, err := uc.challenges.FindActive(ctx, ports.ChallengePasswordReset, hashCode(token))
	if err != nil {
		return fmt.Errorf("find challenge: %w", err)
	}
	if ch == nil {
		return domain.ErrInvalidCredentials
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := uc.users.UpdatePasswordHash(ctx, ch.UserID, string(hashed)); err != nil {
		return err
	}
	if err := uc.challenges.Consume(ctx, ch.ID); err != nil {
		return err
	}
	return nil
}

// ---------- helpers ----------

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func randomHex(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		// Fall back to time-derived bytes; crypto/rand never fails on Linux but we
		// still need a deterministic string return rather than panicking.
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

var ErrWeakPassword = errors.New("password must be at least 10 characters")

func validatePasswordStrength(p string) error {
	if len(p) < 10 {
		return ErrWeakPassword
	}
	return nil
}
