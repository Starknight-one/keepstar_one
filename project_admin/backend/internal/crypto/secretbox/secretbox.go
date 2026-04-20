// Package secretbox provides symmetric encryption for at-rest secrets
// (OAuth tokens, API keys) stored in admin.tenant_integrations.credentials_encrypted.
//
// Envelopes are versioned as "v1.{base64(nonce)}.{base64(ciphertext||tag)}" so
// that a future key rotation can ship v2. with dual-read without migrating
// existing rows. AES-256-GCM from stdlib — no new dependency.
//
// Key comes from ADMIN_ENCRYPTION_KEY env (base64 of 32 random bytes). Load
// is fail-closed: absent/malformed key → NewFromEnv returns an error and the
// server refuses to start.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	currentVersion = "v1"
	keyLen         = 32 // AES-256
)

var (
	ErrBadKey      = errors.New("secretbox: encryption key must be 32 bytes (base64-encoded)")
	ErrBadEnvelope = errors.New("secretbox: malformed envelope")
	ErrBadVersion  = errors.New("secretbox: unsupported envelope version")
)

type Box struct {
	aead cipher.AEAD
}

// New constructs a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != keyLen {
		return nil, ErrBadKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// NewFromBase64 decodes a base64-encoded 32-byte key.
func NewFromBase64(encoded string) (*Box, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, ErrBadKey
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: base64: %v", ErrBadKey, err)
	}
	return New(key)
}

// Seal encrypts plaintext into a "v1.{nonce}.{ct}" envelope string.
func (b *Box) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretbox: nonce: %w", err)
	}
	ct := b.aead.Seal(nil, nonce, plaintext, nil)
	return currentVersion + "." +
		base64.StdEncoding.EncodeToString(nonce) + "." +
		base64.StdEncoding.EncodeToString(ct), nil
}

// Open decrypts a "v1.{nonce}.{ct}" envelope.
func (b *Box) Open(envelope string) ([]byte, error) {
	parts := strings.SplitN(envelope, ".", 3)
	if len(parts) != 3 {
		return nil, ErrBadEnvelope
	}
	if parts[0] != currentVersion {
		return nil, ErrBadVersion
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: nonce: %v", ErrBadEnvelope, err)
	}
	ct, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: ciphertext: %v", ErrBadEnvelope, err)
	}
	if len(nonce) != b.aead.NonceSize() {
		return nil, ErrBadEnvelope
	}
	pt, err := b.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("secretbox: open: %w", err)
	}
	return pt, nil
}
