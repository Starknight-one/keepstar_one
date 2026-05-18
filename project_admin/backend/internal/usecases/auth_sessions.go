package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"keepstar-admin/internal/adapters/geoip"
	"keepstar-admin/internal/adapters/uadevice"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// SessionsUseCase mints/rotates refresh tokens and issues short-lived access
// JWTs. Refresh-token reuse triggers a family-wide revoke as the breach
// signal — callers who were legitimately mid-rotation see 401 and can sign
// in again.
type SessionsUseCase struct {
	sessions   ports.SessionPort
	users      ports.AuthPort
	geo        *geoip.Reader // optional; nil-safe Lookup keeps geo cols empty when unset
	secret     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	log        *logger.Logger
}

func NewSessionsUseCase(s ports.SessionPort, u ports.AuthPort, secret string, accessTTL, refreshTTL time.Duration, log *logger.Logger) *SessionsUseCase {
	return &SessionsUseCase{
		sessions: s, users: u, secret: secret,
		accessTTL: accessTTL, refreshTTL: refreshTTL, log: log,
	}
}

// SetGeoIP wires a MaxMind GeoLite2 reader. Pass nil to disable geo (default).
func (uc *SessionsUseCase) SetGeoIP(g *geoip.Reader) { uc.geo = g }

// enrich populates the parsed device + geo fields from the raw UserAgent/IP
// already set on sess. Idempotent — call before Create. Safe with nil geo.
func (uc *SessionsUseCase) enrich(sess *ports.Session) {
	info := uadevice.Parse(sess.UserAgent)
	sess.BrowserName = info.BrowserName
	sess.BrowserVersion = info.BrowserVersion
	sess.OSName = info.OSName
	sess.DeviceKind = info.DeviceKind
	if uc.geo != nil {
		if r, err := uc.geo.Lookup(sess.IP); err == nil {
			sess.GeoCountry = r.Country
			sess.GeoCity = r.City
		}
	}
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Issue creates a new refresh session and mints an access token.
// ua/ip are optional metadata for session list display.
func (uc *SessionsUseCase) Issue(ctx context.Context, user *domain.AdminUser, ua, ip string) (*TokenPair, error) {
	refresh := randomHex(32)
	sess := &ports.Session{
		UserID:    user.ID,
		TenantID:  user.TenantID,
		TokenHash: hashCode(refresh),
		UserAgent: ua,
		IP:        ip,
		ExpiresAt: time.Now().Add(uc.refreshTTL),
	}
	uc.enrich(sess)
	if err := uc.sessions.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	access, err := uc.signAccess(user, sess.ID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(uc.accessTTL.Seconds()),
	}, nil
}

// Refresh rotates the refresh token:
//  1. If not found → 401.
//  2. If found but already revoked → BREACH — revoke the entire family and 401.
//  3. Otherwise mark the used row revoked, create a new row, mint a new access.
func (uc *SessionsUseCase) Refresh(ctx context.Context, refreshToken, ua, ip string) (*TokenPair, error) {
	hash := hashCode(refreshToken)
	sess, err := uc.sessions.FindActive(ctx, hash)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, domain.ErrInvalidCredentials
	}
	if sess.RevokedAt != nil {
		// Revoked refresh token being reused — revoke everything.
		_ = uc.sessions.RevokeAllForUser(ctx, sess.UserID)
		uc.log.Warn("refresh_token_reuse_detected", "user_id", sess.UserID, "session_id", sess.ID)
		return nil, domain.ErrInvalidCredentials
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, domain.ErrInvalidCredentials
	}

	user, err := uc.users.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	// Rotate — revoke old, create new.
	if err := uc.sessions.Revoke(ctx, sess.ID); err != nil {
		return nil, err
	}

	next := randomHex(32)
	newSess := &ports.Session{
		UserID:    user.ID,
		TenantID:  user.TenantID,
		TokenHash: hashCode(next),
		UserAgent: ua,
		IP:        ip,
		ExpiresAt: time.Now().Add(uc.refreshTTL),
	}
	uc.enrich(newSess)
	if err := uc.sessions.Create(ctx, newSess); err != nil {
		return nil, err
	}

	access, err := uc.signAccess(user, newSess.ID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: next,
		ExpiresIn:    int64(uc.accessTTL.Seconds()),
	}, nil
}

func (uc *SessionsUseCase) Revoke(ctx context.Context, refreshToken string) error {
	sess, err := uc.sessions.FindActive(ctx, hashCode(refreshToken))
	if err != nil {
		return err
	}
	if sess == nil {
		return nil
	}
	return uc.sessions.Revoke(ctx, sess.ID)
}

func (uc *SessionsUseCase) RevokeByID(ctx context.Context, sessionID, requesterID string) error {
	sess, err := uc.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess == nil {
		return errors.New("session not found")
	}
	if sess.UserID != requesterID {
		return errors.New("forbidden")
	}
	return uc.sessions.Revoke(ctx, sessionID)
}

func (uc *SessionsUseCase) List(ctx context.Context, userID string) ([]*ports.Session, error) {
	return uc.sessions.ListActiveForUser(ctx, userID)
}

func (uc *SessionsUseCase) signAccess(user *domain.AdminUser, sessionID string) (string, error) {
	claims := jwt.MapClaims{
		"uid":   user.ID,
		"tid":   user.TenantID,
		"role":  string(user.Role),
		"sid":   sessionID,
		"exp":   time.Now().Add(uc.accessTTL).Unix(),
		"iat":   time.Now().Unix(),
		"scope": "access",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(uc.secret))
}

// IssueForTenant mints a session and access token scoped to a specific tenant
// + role. Used by the workspace picker: a user who belongs to multiple
// tenants picks one, and we re-issue so the `tid` claim matches their
// selection regardless of the user's default TenantID column.
func (uc *SessionsUseCase) IssueForTenant(ctx context.Context, user *domain.AdminUser, tenantID string, role domain.AdminRole, ua, ip string) (*TokenPair, error) {
	refresh := randomHex(32)
	sess := &ports.Session{
		UserID:    user.ID,
		TenantID:  tenantID,
		TokenHash: hashCode(refresh),
		UserAgent: ua,
		IP:        ip,
		ExpiresAt: time.Now().Add(uc.refreshTTL),
	}
	uc.enrich(sess)
	if err := uc.sessions.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	claims := jwt.MapClaims{
		"uid":   user.ID,
		"tid":   tenantID,
		"role":  string(role),
		"sid":   sess.ID,
		"exp":   time.Now().Add(uc.accessTTL).Unix(),
		"iat":   time.Now().Unix(),
		"scope": "access",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	access, err := tok.SignedString([]byte(uc.secret))
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(uc.accessTTL.Seconds()),
	}, nil
}
