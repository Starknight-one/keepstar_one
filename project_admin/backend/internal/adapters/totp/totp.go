// Package totp is a stdlib-only implementation of RFC 6238 TOTP (SHA-1,
// 6 digits, 30-second step). We avoid importing github.com/pquerna/otp since
// our needs are minimal and a dependency audit is simpler without it.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	period = 30 // seconds
	digits = 6
)

// NewSecret returns a fresh 20-byte TOTP secret encoded in un-padded Base32,
// which is the format authenticator apps expect when users key in the secret
// manually. The raw bytes should be passed back to Verify unchanged.
func NewSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// ProvisioningURL builds the otpauth:// URL that encodes the secret + issuer.
// Authenticator apps read this (via QR or paste) and provision the entry.
func ProvisioningURL(secretBase32, accountEmail, issuer string) string {
	v := url.Values{}
	v.Set("secret", secretBase32)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(digits))
	v.Set("period", fmt.Sprint(period))
	label := url.PathEscape(issuer + ":" + accountEmail)
	return "otpauth://totp/" + label + "?" + v.Encode()
}

// Verify checks a 6-digit code against the secret. We accept skew of ±1 step
// (30 s) per RFC 6238 recommendation to tolerate minor clock drift.
func Verify(secretBase32, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return false
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secretBase32))
	if err != nil {
		return false
	}
	t := time.Now().Unix() / period
	for _, step := range []int64{t - 1, t, t + 1} {
		if hotp(secret, step) == code {
			return true
		}
	}
	return false
}

func hotp(secret []byte, counter int64) string {
	var ctr [8]byte
	binary.BigEndian.PutUint64(ctr[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(ctr[:])
	h := mac.Sum(nil)
	offset := h[len(h)-1] & 0x0f
	binCode := (uint32(h[offset])&0x7f)<<24 |
		uint32(h[offset+1])<<16 |
		uint32(h[offset+2])<<8 |
		uint32(h[offset+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, binCode%mod)
}
