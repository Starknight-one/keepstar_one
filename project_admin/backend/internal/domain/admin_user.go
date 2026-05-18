package domain

import "time"

type AdminRole string

const (
	AdminRoleOwner  AdminRole = "owner"
	AdminRoleEditor AdminRole = "editor"
)

type AdminUser struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	TenantID     string    `json:"tenantId"`
	Role         AdminRole `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}

// HasPassword reports whether the user has a password hash set. Used by
// the frontend after first sign-in (magic-link / OAuth) to decide whether
// to show the "set a password" promo (scenario 39) — passwordless users
// land on the promo, password-having users skip straight to the workspace.
func (u *AdminUser) HasPassword() bool {
	return u != nil && u.PasswordHash != ""
}

// MarshalJSON adds a `has_password` field to the wire representation so
// the frontend can branch without an extra round-trip. PasswordHash itself
// stays unexported via json:"-".
func (u AdminUser) MarshalJSON() ([]byte, error) {
	type alias AdminUser
	return marshalAdminUser(alias(u), u.PasswordHash != "")
}
