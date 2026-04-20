// Package google is a stdlib-only OAuth2 client for Google Sign-In. We avoid
// pulling in golang.org/x/oauth2 since we only need a tiny surface: build an
// authorize URL, exchange a code for tokens, and extract the user's claims
// from the returned id_token. The id_token is an RS256 JWT issued by Google;
// because we exchange it over TLS with our client secret we do not re-verify
// the signature — the transport identity is the integrity guarantee.
package google

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
	authURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenURL = "https://oauth2.googleapis.com/token"
	scope    = "openid email profile"
)

// Client captures the OAuth registration that Google hands us once we register
// an application in console.cloud.google.com. RedirectURL must match one of the
// "Authorized redirect URIs" configured there, byte-for-byte.
type Client struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	http         *http.Client
}

// UserInfo is the subset of id_token claims we persist or surface to the
// frontend. Google marks `sub` as stable for the lifetime of the Google
// account, so we key off of it.
type UserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func NewClient(clientID, clientSecret, redirectURL string) *Client {
	return &Client{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

// BuildAuthURL returns the Google consent URL with the given opaque state.
// Caller is responsible for storing state server-side and verifying it in
// the callback.
func (c *Client) BuildAuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", scope)
	q.Set("state", state)
	q.Set("access_type", "online")
	q.Set("prompt", "select_account")
	return authURL + "?" + q.Encode()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Exchange trades an authorization code for tokens and returns the parsed
// id_token claims. Errors returned here collapse the several failure modes
// (network, malformed response, Google-side rejection, invalid JWT shape)
// into descriptive messages — callers just return 502 / generic error to
// the frontend.
func (c *Client) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("redirect_uri", c.RedirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
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

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode >= 400 || tr.Error != "" {
		return nil, fmt.Errorf("google token error: %s %s", tr.Error, tr.ErrorDesc)
	}
	if tr.IDToken == "" {
		return nil, fmt.Errorf("google response missing id_token")
	}
	return decodeIDToken(tr.IDToken)
}

// decodeIDToken splits the JWT and base64url-decodes the payload. We do NOT
// validate the RS256 signature: the token just came back to us over a TLS
// connection authenticated by our client_secret, which is Google's own
// recommendation for server-side auth-code flow.
func decodeIDToken(idToken string) (*UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("id_token malformed")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some libraries emit padded base64url; fall back to std URL decoding.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode id_token payload: %w", err)
		}
	}
	var info UserInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, fmt.Errorf("parse id_token claims: %w", err)
	}
	if info.Sub == "" || info.Email == "" {
		return nil, fmt.Errorf("id_token missing required claims")
	}
	return &info, nil
}
