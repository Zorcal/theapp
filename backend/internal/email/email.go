// Package email defines the Sender interface and shared types for outbound email.
package email

import "context"

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
