package ports

import "context"

// EmailMessage is the uniform shape every mailer adapter consumes. Plain
// fields only — rendering happens in the adapter from //go:embed templates.
type EmailMessage struct {
	To      string
	Subject string
	Kind    string         // "password_reset" | "email_verify" | "invitation" | "email_2fa"
	Data    map[string]any // placeholders for the template
}

type MailerPort interface {
	Send(ctx context.Context, msg EmailMessage) error
}
