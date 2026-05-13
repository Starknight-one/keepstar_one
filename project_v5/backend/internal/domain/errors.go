package domain

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

var (
	ErrSessionNotFound = &Error{Code: "SESSION_NOT_FOUND", Message: "Session not found"}
	ErrProductNotFound = &Error{Code: "PRODUCT_NOT_FOUND", Message: "Product not found"}
	ErrTenantNotFound  = &Error{Code: "TENANT_NOT_FOUND", Message: "tenant not found"}
	ErrPresetNotFound    = &Error{Code: "PRESET_NOT_FOUND", Message: "preset not found"}
	ErrComponentNotFound = &Error{Code: "COMPONENT_NOT_FOUND", Message: "component not found"}
	ErrInvalidQuery      = &Error{Code: "INVALID_QUERY", Message: "Invalid query"}
)
