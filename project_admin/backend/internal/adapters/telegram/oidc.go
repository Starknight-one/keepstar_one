package telegram

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	oidcAuthURL  = "https://oauth.telegram.org/auth"
	oidcTokenURL = "https://oauth.telegram.org/token"
	oidcScope    = "openid profile"
)

// OIDCClient implements the standard OAuth2 / OIDC flow for Telegram.
// client_id   = numeric bot ID  (the part of the bot token before ":")
// client_secret = the full bot token
// No PKCE — we exchange over TLS authenticated by client_secret, which is
// Telegram's own recommended approach (same as Google's auth-code flow).
type OIDCClient struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	http         *http.Client
}

// OIDCUserInfo holds the id_token claims we care about.
// sub = Telegram user ID (numeric string, stable forever).
type OIDCUserInfo struct {
	Sub       string `json:"sub"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
}

// NewOIDCClient constructs the client from a raw bot token.
// The numeric bot ID is extracted automatically (before the first ":").
func NewOIDCClient(botToken, redirectURL string) *OIDCClient {
	clientID := botToken
	if idx := strings.Index(botToken, ":"); idx > 0 {
		clientID = botToken[:idx]
	}
	return &OIDCClient{
		ClientID:     clientID,
		ClientSecret: botToken,
		RedirectURL:  redirectURL,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

// BuildAuthURL returns the Telegram consent URL.
//
// Despite the OIDC-ish naming around this file, oauth.telegram.org/auth is
// NOT a standard OAuth2 / OIDC authorization endpoint. Telegram ignores
// `response_type`, `scope`, `redirect_uri`, and even `state`. The only
// parameters it honors are `bot_id`, `origin`, `return_to`, and
// `request_access`, and it returns the user payload as a URL hash on
// `return_to`: `#tgAuthResult=<base64url(JSON widget payload)>`.
//
// The opaque state is preserved by appending it to the return_to as a query
// param — Telegram passes the return_to through untouched, so the callback
// page can read it back from window.location.
func (c *OIDCClient) BuildAuthURL(state string) string {
	returnTo := c.RedirectURL
	if state != "" {
		sep := "?"
		if strings.Contains(returnTo, "?") {
			sep = "&"
		}
		returnTo = returnTo + sep + "state=" + url.QueryEscape(state)
	}
	q := url.Values{}
	q.Set("bot_id", c.ClientID)
	q.Set("return_to", returnTo)
	q.Set("request_access", "write")
	if origin := c.origin(); origin != "" {
		q.Set("origin", origin)
	}
	return oidcAuthURL + "?" + q.Encode()
}

// origin extracts scheme+host from the redirect URL (e.g. "https://admin.keepstar.one").
func (c *OIDCClient) origin() string {
	u, err := url.Parse(c.RedirectURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Exchange trades an authorization code for an id_token and returns parsed
// user claims. Errors collapse all failure modes into descriptive messages.
func (c *OIDCClient) Exchange(ctx context.Context, code string) (*OIDCUserInfo, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("redirect_uri", c.RedirectURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oidcTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token body: %w", err)
	}

	var tr oidcTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode >= 400 || tr.Error != "" {
		return nil, fmt.Errorf("telegram token error: %s %s", tr.Error, tr.ErrorDesc)
	}
	if tr.IDToken == "" {
		return nil, fmt.Errorf("telegram response missing id_token")
	}
	return decodeOIDCToken(tr.IDToken)
}

// decodeOIDCToken base64url-decodes the JWT payload without verifying the
// signature — the token came back over TLS authenticated by client_secret.
func decodeOIDCToken(idToken string) (*OIDCUserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("id_token malformed")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode id_token payload: %w", err)
		}
	}
	var info OIDCUserInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, fmt.Errorf("parse id_token claims: %w", err)
	}
	if info.Sub == "" {
		return nil, fmt.Errorf("id_token missing sub claim")
	}
	return &info, nil
}
