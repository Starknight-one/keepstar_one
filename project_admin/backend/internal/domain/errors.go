package domain

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrUserNotFound     = errors.New("user not found")
	ErrTenantNotFound   = errors.New("tenant not found")
	ErrProductNotFound  = errors.New("product not found")
	ErrCategoryNotFound = errors.New("category not found")
	ErrImportNotFound   = errors.New("import job not found")
	ErrEmailExists      = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized     = errors.New("unauthorized")
)
