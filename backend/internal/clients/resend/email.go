package resend

import (
	"context"

	"github.com/resend/resend-go/v3"

	"github.com/zorcal/theapp/backend/internal/email"
)

var _ email.Sender = (*EmailClient)(nil)

// EmailClient sends transactional email through the Resend API.
type EmailClient struct {
	resend *resend.Client
}

// NewEmailClient returns a Client authenticated with the given Resend API key.
func NewEmailClient(apiKey string) *EmailClient {
	return &EmailClient{
		resend: resend.NewClient(apiKey),
	}
}

// SendEmail implements email.Sender.
func (c *EmailClient) SendEmail(ctx context.Context, p email.Message) error {
	req := resend.SendEmailRequest{
		From:    p.From,
		To:      p.To,
		Subject: p.Subject,
		Html:    p.HTMLBody,
		Text:    p.TextBody,
	}
	if _, err := c.resend.Emails.SendWithContext(ctx, &req); err != nil {
		return err
	}
	return nil
}
