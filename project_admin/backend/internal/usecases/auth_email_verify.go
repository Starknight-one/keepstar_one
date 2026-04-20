package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type EmailVerifyUseCase struct {
	users      ports.AuthPort
	challenges ports.ChallengePort
	mailer     ports.MailerPort
	baseURL    string
	ttl        time.Duration
	log        *logger.Logger
}

func NewEmailVerifyUseCase(users ports.AuthPort, ch ports.ChallengePort, mailer ports.MailerPort, baseURL string, ttl time.Duration, log *logger.Logger) *EmailVerifyUseCase {
	return &EmailVerifyUseCase{users: users, challenges: ch, mailer: mailer, baseURL: baseURL, ttl: ttl, log: log}
}

// Issue mints a new verification link for the given user and emails it.
func (uc *EmailVerifyUseCase) Issue(ctx context.Context, userID, email string) error {
	token := randomHex(32)
	ch := &ports.Challenge{
		UserID:    userID,
		Email:     email,
		Kind:      ports.ChallengeEmailVerify,
		CodeHash:  hashCode(token),
		ExpiresAt: time.Now().Add(uc.ttl),
	}
	if err := uc.challenges.Create(ctx, ch); err != nil {
		return err
	}
	link := fmt.Sprintf("%s/auth/verify-email?token=%s", uc.baseURL, token)
	if uc.mailer != nil {
		if err := uc.mailer.Send(ctx, ports.EmailMessage{
			To:   email,
			Kind: "email_verify",
			Data: map[string]any{"Link": link},
		}); err != nil {
			uc.log.Error("email_verify_send_failed", "email", email, "error", err)
		}
	}
	return nil
}

// Verify consumes a token and flips email_verified_at on the user row.
func (uc *EmailVerifyUseCase) Verify(ctx context.Context, token string) error {
	ch, err := uc.challenges.FindActive(ctx, ports.ChallengeEmailVerify, hashCode(token))
	if err != nil {
		return err
	}
	if ch == nil {
		return domain.ErrInvalidCredentials
	}
	if err := uc.users.MarkEmailVerified(ctx, ch.UserID); err != nil {
		return err
	}
	return uc.challenges.Consume(ctx, ch.ID)
}

// Resend re-issues for an email — silent if the email is not registered.
func (uc *EmailVerifyUseCase) Resend(ctx context.Context, email string) error {
	user, err := uc.users.GetUserByEmail(ctx, email)
	if err != nil || user == nil {
		return nil
	}
	return uc.Issue(ctx, user.ID, user.Email)
}
