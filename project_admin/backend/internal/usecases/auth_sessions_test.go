package usecases

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

// richSessions is a fuller SessionPort fake than the magic-link suite needs.
// It supports Revoke + RevokeAllForUser + FindActive returning revoked rows
// (so the breach-detection branch in Refresh can be exercised).
type richSessions struct {
	byID      map[string]*ports.Session
	byHash    map[string]string // tokenHash → sessionID
	nextID    int
	revokeAll int
}

func newRichSessions() *richSessions {
	return &richSessions{byID: map[string]*ports.Session{}, byHash: map[string]string{}}
}

func (r *richSessions) Create(_ context.Context, s *ports.Session) error {
	r.nextID++
	s.ID = "sess-" + itoa(r.nextID)
	r.byID[s.ID] = s
	r.byHash[s.TokenHash] = s.ID
	return nil
}
func (r *richSessions) FindActive(_ context.Context, tokenHash string) (*ports.Session, error) {
	id, ok := r.byHash[tokenHash]
	if !ok {
		return nil, nil
	}
	return r.byID[id], nil
}
func (r *richSessions) FindByID(_ context.Context, id string) (*ports.Session, error) {
	s, ok := r.byID[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}
func (r *richSessions) Revoke(_ context.Context, id string) error {
	s, ok := r.byID[id]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now()
	s.RevokedAt = &now
	return nil
}
func (r *richSessions) RevokeAllForUser(_ context.Context, userID string) error {
	r.revokeAll++
	now := time.Now()
	for _, s := range r.byID {
		if s.UserID == userID && s.RevokedAt == nil {
			t := now
			s.RevokedAt = &t
		}
	}
	return nil
}
func (r *richSessions) ListActiveForUser(_ context.Context, userID string) ([]*ports.Session, error) {
	var out []*ports.Session
	for _, s := range r.byID {
		if s.UserID == userID && s.RevokedAt == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b strings.Builder
	for n > 0 {
		b.WriteByte(byte('0' + n%10))
		n /= 10
	}
	s := []byte(b.String())
	// reverse
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return string(s)
}

func mkSessionsUC() (*SessionsUseCase, *richSessions, *fakeAuth) {
	sess := newRichSessions()
	auth := newFakeAuth()
	uc := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx",
		15*time.Minute, 30*24*time.Hour, logger.New("error"))
	return uc, sess, auth
}

func TestSessions_IssueCreatesRow(t *testing.T) {
	uc, sess, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})

	pair, err := uc.Issue(context.Background(), user, "Mozilla/5.0", "1.2.3.4")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Errorf("pair = %+v, want both tokens set", pair)
	}
	if len(sess.byID) != 1 {
		t.Errorf("session rows = %d, want 1", len(sess.byID))
	}
	for _, s := range sess.byID {
		if s.UserAgent != "Mozilla/5.0" || s.IP != "1.2.3.4" {
			t.Errorf("ua/ip not threaded: %+v", s)
		}
	}
}

func TestSessions_RefreshRotatesAndIssuesNew(t *testing.T) {
	uc, sess, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	pair, _ := uc.Issue(context.Background(), user, "", "")

	newPair, err := uc.Refresh(context.Background(), pair.RefreshToken, "", "")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Errorf("refresh token should rotate")
	}
	// Old session row should be revoked, new one active.
	var active, revoked int
	for _, s := range sess.byID {
		if s.RevokedAt == nil {
			active++
		} else {
			revoked++
		}
	}
	if active != 1 || revoked != 1 {
		t.Errorf("active=%d revoked=%d, want active=1 revoked=1", active, revoked)
	}
}

func TestSessions_RefreshTokenReuseTriggersBreachRevoke(t *testing.T) {
	uc, sess, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})

	// Issue pair A and pair B (two devices).
	pairA, _ := uc.Issue(context.Background(), user, "", "")
	_, _ = uc.Issue(context.Background(), user, "", "")

	// Rotate A normally.
	rotated, err := uc.Refresh(context.Background(), pairA.RefreshToken, "", "")
	if err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	// Now use the ORIGINAL pairA.RefreshToken again — old row is revoked → breach.
	_, err = uc.Refresh(context.Background(), pairA.RefreshToken, "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("reuse should return ErrInvalidCredentials, got %v", err)
	}
	if sess.revokeAll != 1 {
		t.Errorf("RevokeAllForUser called %d times, want 1", sess.revokeAll)
	}
	// Even the legitimately-rotated session should now be revoked (family wipe).
	rotatedSess, _ := sess.FindActive(context.Background(), hashCode(rotated.RefreshToken))
	if rotatedSess == nil || rotatedSess.RevokedAt == nil {
		t.Errorf("rotated session not revoked after breach")
	}
}

func TestSessions_RefreshUnknownToken(t *testing.T) {
	uc, _, _ := mkSessionsUC()
	_, err := uc.Refresh(context.Background(), "ghost-token", "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestSessions_RefreshExpired(t *testing.T) {
	uc, sess, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	pair, _ := uc.Issue(context.Background(), user, "", "")

	// Backdate the session's ExpiresAt.
	for _, s := range sess.byID {
		s.ExpiresAt = time.Now().Add(-1 * time.Hour)
	}
	_, err := uc.Refresh(context.Background(), pair.RefreshToken, "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expired refresh: err = %v, want ErrInvalidCredentials", err)
	}
}

func TestSessions_RevokeByRefreshToken(t *testing.T) {
	uc, sess, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	pair, _ := uc.Issue(context.Background(), user, "", "")

	if err := uc.Revoke(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	for _, s := range sess.byID {
		if s.RevokedAt == nil {
			t.Errorf("session should be revoked")
		}
	}
}

func TestSessions_RevokeUnknownTokenIsNoop(t *testing.T) {
	uc, _, _ := mkSessionsUC()
	if err := uc.Revoke(context.Background(), "ghost"); err != nil {
		t.Errorf("Revoke unknown should be nil-error, got %v", err)
	}
}

func TestSessions_RevokeByIDChecksOwner(t *testing.T) {
	uc, _, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	pair, _ := uc.Issue(context.Background(), user, "", "")
	// Find the session ID via FindActive.
	sess, _ := uc.sessions.FindActive(context.Background(), hashCode(pair.RefreshToken))

	// Wrong requester rejected.
	if err := uc.RevokeByID(context.Background(), sess.ID, "different-user"); err == nil {
		t.Error("revoke by non-owner should be rejected")
	}
	// Correct requester succeeds.
	if err := uc.RevokeByID(context.Background(), sess.ID, user.ID); err != nil {
		t.Errorf("revoke by owner: %v", err)
	}
}

func TestSessions_ListActiveForUser(t *testing.T) {
	uc, _, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x", TenantID: "t1"})
	_, _ = uc.Issue(context.Background(), user, "device1", "ip1")
	_, _ = uc.Issue(context.Background(), user, "device2", "ip2")
	pair3, _ := uc.Issue(context.Background(), user, "device3", "ip3")
	_ = uc.Revoke(context.Background(), pair3.RefreshToken)

	got, err := uc.List(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("active sessions = %d, want 2 (3rd revoked)", len(got))
	}
}

func TestSessions_IssueForTenantUsesPassedTenant(t *testing.T) {
	uc, sess, auth := mkSessionsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "u@x", TenantID: "user-default-tenant",
	})
	pair, err := uc.IssueForTenant(context.Background(), user, "different-tenant", domain.AdminRoleOwner, "", "")
	if err != nil {
		t.Fatalf("IssueForTenant: %v", err)
	}
	if pair.RefreshToken == "" {
		t.Error("refresh empty")
	}
	for _, s := range sess.byID {
		if s.TenantID != "different-tenant" {
			t.Errorf("session.tenant = %q, want different-tenant", s.TenantID)
		}
	}
}
