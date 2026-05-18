package domain

import "encoding/json"

// marshalAdminUser is split into its own file so the alias trick stays
// readable in admin_user.go. The inner alias type avoids recursive
// MarshalJSON calls.
func marshalAdminUser(alias any, hasPassword bool) ([]byte, error) {
	// json package can't flatten an embedded alias when given a struct
	// wrapper; do it manually via marshal-then-splice.
	innerBytes, err := json.Marshal(alias)
	if err != nil {
		return nil, err
	}
	if len(innerBytes) < 2 || innerBytes[0] != '{' {
		return innerBytes, nil
	}
	// Strip closing brace, append `,"has_password":<bool>}`.
	out := make([]byte, 0, len(innerBytes)+24)
	out = append(out, innerBytes[:len(innerBytes)-1]...)
	if len(innerBytes) > 2 {
		out = append(out, ',')
	}
	if hasPassword {
		out = append(out, []byte(`"has_password":true}`)...)
	} else {
		out = append(out, []byte(`"has_password":false}`)...)
	}
	return out, nil
}
