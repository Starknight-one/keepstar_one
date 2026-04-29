// Package smtp is a minimal transactional mailer backing the auth flows —
// password reset, email verification, invitations, and the email-2FA code.
//
// Uses net/smtp with STARTTLS (auto-upgraded by the server on port 587) and
// //go:embed for the four HTML templates. Only one external string escape:
// the subject header is sanitized to strip newlines per RFC 5322 §2.2.
package smtp

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"

	"keepstar-admin/internal/ports"
)

//go:embed templates/*.html
var templatesFS embed.FS

// subjectFor maps a challenge kind to a human-readable subject line.
var subjectFor = map[string]string{
	"password_reset": "Reset your Keepstar One password",
	"email_verify":   "Confirm your email",
	"invitation":     "You've been invited to Keepstar One",
	"email_2fa":      "Your Keepstar One verification code",
	"magic_link":     "Sign in to Keepstar One",
}

type Mailer struct {
	host     string
	port     string
	user     string
	password string
	from     string
	fromName string
	tmpl     *template.Template
}

// Config holds the SMTP connection parameters. Only host + from are required
// to construct the mailer; user/password are optional (unauthenticated relays
// still exist inside private VPCs).
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
	FromName string
}

func New(cfg Config) (*Mailer, error) {
	if cfg.Host == "" || cfg.From == "" {
		return nil, fmt.Errorf("smtp: host and from are required")
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("smtp: parse templates: %w", err)
	}
	return &Mailer{
		host:     cfg.Host,
		port:     cfg.Port,
		user:     cfg.User,
		password: cfg.Password,
		from:     cfg.From,
		fromName: cfg.FromName,
		tmpl:     tmpl,
	}, nil
}

// Send renders the template keyed by msg.Kind and dispatches the email.
// Context is accepted for cancellation signalling but net/smtp doesn't
// expose a way to plumb it into the session — we honour it before dial.
func (m *Mailer) Send(ctx context.Context, msg ports.EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if msg.To == "" {
		return fmt.Errorf("smtp: recipient is empty")
	}

	tmplName := msg.Kind + ".html"
	var body bytes.Buffer
	if err := m.tmpl.ExecuteTemplate(&body, tmplName, msg.Data); err != nil {
		return fmt.Errorf("smtp: render %s: %w", tmplName, err)
	}

	subject := msg.Subject
	if subject == "" {
		subject = subjectFor[msg.Kind]
	}
	subject = sanitizeHeader(subject)

	fromHeader := m.from
	if m.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", m.fromName, m.from)
	}

	raw := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + msg.To,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body.String(),
	}, "\r\n")

	addr := m.host + ":" + m.port
	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.password, m.host)
	}

	if err := smtp.SendMail(addr, auth, m.from, []string{msg.To}, []byte(raw)); err != nil {
		return fmt.Errorf("smtp: send: %w", err)
	}
	return nil
}

func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
