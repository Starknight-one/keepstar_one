package config

import "testing"

func TestValidate_JWTSecret(t *testing.T) {
	cases := []struct {
		name      string
		env       string
		jwt       string // value for JWT_SECRET; "" means unset (→ insecure default)
		wantError bool
	}{
		{"dev with default is fine", "development", "", false},
		{"prod with insecure default is rejected", "production", insecureJWTDefault, true},
		{"prod with unset (falls back to default) is rejected", "production", "", true},
		{"prod with strong secret is fine", "production", "a-long-random-production-secret-7f3a", false},
		{"prod alias 'prod' with default rejected", "prod", insecureJWTDefault, true},
		{"prod alias 'PRODUCTION' (case) with default rejected", "PRODUCTION", insecureJWTDefault, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", c.env)
			t.Setenv("JWT_SECRET", c.jwt) // "" → Load() substitutes the default

			cfg := Load()
			err := cfg.Validate()
			if c.wantError && err == nil {
				t.Fatalf("expected validation error, got nil (jwt=%q env=%q)", cfg.JWTSecret, c.env)
			}
			if !c.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
