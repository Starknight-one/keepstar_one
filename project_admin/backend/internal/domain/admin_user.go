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
