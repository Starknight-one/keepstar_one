// Package telegram implements the Telegram Login Widget HMAC verification.
// See https://core.telegram.org/widgets/login#checking-authorization. The
// widget posts a payload with a `hash` field; the rest of the fields form a
// sorted key=value string whose HMAC-SHA256 (keyed by SHA256(bot_token))
// must match the provided hash. We also reject payloads older than 1 hour
// per Telegram's recommendation.
package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxAuthAge = 3600 * time.Second

// User is the Telegram Login Widget payload. All fields except ID and Hash
// are optional — users may hide username or photo.
type User struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
	Hash      string
}

// Verify checks the HMAC signature and freshness of a widget payload. The
// input map corresponds 1:1 to the query-string fields Telegram emits
// (lowercase keys, string values). Returns the parsed user on success.
func Verify(fields map[string]string, botToken string) (*User, error) {
	if botToken == "" {
		return nil, errors.New("telegram bot token not configured")
	}
	hash, ok := fields["hash"]
	if !ok || hash == "" {
		return nil, errors.New("missing hash")
	}

	// Build the data-check string: sort keys alphabetically, join "k=v" with \n,
	// excluding the hash field itself.
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fields[k])
	}

	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(sb.String()))
	computed := hex.EncodeToString(mac.Sum(nil))

	providedBytes, err := hex.DecodeString(hash)
	if err != nil {
		return nil, errors.New("hash not hex")
	}
	if !hmac.Equal(mac.Sum(nil), providedBytes) {
		return nil, errors.New("hmac mismatch")
	}
	_ = computed // kept for debugging; the constant-time compare above is the guard.

	authDate, err := strconv.ParseInt(fields["auth_date"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("auth_date invalid: %w", err)
	}
	if time.Since(time.Unix(authDate, 0)) > maxAuthAge {
		return nil, errors.New("auth_date too old")
	}

	id, err := strconv.ParseInt(fields["id"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("id invalid: %w", err)
	}
	return &User{
		ID:        id,
		FirstName: fields["first_name"],
		LastName:  fields["last_name"],
		Username:  fields["username"],
		PhotoURL:  fields["photo_url"],
		AuthDate:  authDate,
		Hash:      hash,
	}, nil
}
