// Package resend is an HTTP-based MailerPort implementation against Resend's
// /emails endpoint. We use HTTP rather than SMTP because Railway's egress
// network silently drops outbound 587 — the stdlib smtp.SendMail call hangs
// forever with no error to log, swallowing the magic-link delivery in install
// flow. Switching to HTTPS to api.resend.com sidesteps that entire class of
// firewall/timeout issues and gives us proper error semantics.
//
// Templates are duplicated from the smtp adapter for now. Both adapters
// implement the same MailerPort, so swapping in main.go is a one-line DI
// change. Long term we can lift the templates into a shared package; that's
// a refactor, not a fix.
package resend

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"keepstar-admin/internal/ports"
)

//go:embed templates/*.html
var templatesFS embed.FS

var subjectFor = map[string]string{
	"password_reset": "Reset your Keepstar One password",
	"email_verify":   "Confirm your email",
	"invitation":     "You've been invited to Keepstar One",
	"email_2fa":      "Your Keepstar One verification code",
	"magic_link":     "Sign in to Keepstar One",
}

const apiURL = "https://api.resend.com/emails"

// Mailer posts rendered HTML to Resend over HTTPS. The HTTP client carries
// a 10s timeout — fast feedback on misconfiguration beats Stalin-grade
// silence from the SMTP path.
type Mailer struct {
	apiKey   string
	from     string // "name <addr@domain>" if FromName set, else bare addr
	tmpl     *template.Template
	http     *http.Client
}

type Config struct {
	APIKey   string
	From     string // bare email, e.g. "noreply@updates.keepstar.one"
	FromName string // optional vanity, e.g. "Keepstar One"
}

func New(cfg Config) (*Mailer, error) {
	if cfg.APIKey == "" || cfg.From == "" {
		return nil, fmt.Errorf("resend: api_key and from are required")
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("resend: parse templates: %w", err)
	}
	from := cfg.From
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.From)
	}
	return &Mailer{
		apiKey: cfg.APIKey,
		from:   from,
		tmpl:   tmpl,
		http:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// resendRequest mirrors the JSON body Resend's /emails endpoint expects.
// Subject newlines are stripped per RFC 5322; Resend would 422 otherwise.
type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func (m *Mailer) Send(ctx context.Context, msg ports.EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if msg.To == "" {
		return fmt.Errorf("resend: recipient is empty")
	}

	tmplName := msg.Kind + ".html"
	var body bytes.Buffer
	if err := m.tmpl.ExecuteTemplate(&body, tmplName, msg.Data); err != nil {
		return fmt.Errorf("resend: render %s: %w", tmplName, err)
	}

	subject := msg.Subject
	if subject == "" {
		subject = subjectFor[msg.Kind]
	}
	subject = sanitizeHeader(subject)

	payload, err := json.Marshal(resendRequest{
		From:    m.from,
		To:      []string{msg.To},
		Subject: subject,
		HTML:    body.String(),
	})
	if err != nil {
		return fmt.Errorf("resend: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("resend: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(respBody))
}

func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
