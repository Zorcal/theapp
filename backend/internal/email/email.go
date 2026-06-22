// Package email defines the Sender interface and shared types for outbound email.
package email

import (
	"context"
	"net/mail"
)

// Sender sends emails.
type Sender interface {
	SendEmail(ctx context.Context, m Message) error
}

// Message holds the fields for a single outbound email.
type Message struct {
	From     string   // Sender address, e.g. "App <noreply@example.com>"
	To       []string // Recipient addresses
	Subject  string
	HTMLBody string // HTML body; rendered by mail clients that support it
	TextBody string // Plain-text fallback; shown when HTML is unavailable
}

// Validate reports whether addr is a syntactically valid bare email address
// (e.g. "user@example.com"). Display-name forms ("Name <user@example.com>")
// are rejected. This is a best-effort format check — it does not verify that
// the address exists or is reachable. The only reliable validation is a
// successful magic-link verification, which proves the email reached the user.
func Validate(addr string) bool {
	parsed, err := mail.ParseAddress(addr)
	return err == nil && parsed.Address == addr
}
