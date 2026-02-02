package domain

// Error represents a domain error
type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// Domain errors
var (
	ErrSessionNotFound   = &Error{Code: "SESSION_NOT_FOUND", Message: "Session not found"}
	ErrProductNotFound   = &Error{Code: "PRODUCT_NOT_FOUND", Message: "Product not found"}
	ErrInvalidQuery      = &Error{Code: "INVALID_QUERY", Message: "Invalid query"}
	ErrLLMUnavailable    = &Error{Code: "LLM_UNAVAILABLE", Message: "AI service unavailable"}
	ErrRateLimitExceeded = &Error{Code: "RATE_LIMIT", Message: "Rate limit exceeded"}
	ErrTenantNotFound    = &Error{Code: "TENANT_NOT_FOUND", Message: "tenant not found"}
	ErrCategoryNotFound  = &Error{Code: "CATEGORY_NOT_FOUND", Message: "category not found"}
)
