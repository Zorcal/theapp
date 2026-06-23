package testingx

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/zorcal/theapp/backend/internal/email"
)

var _ email.Sender = (*CaptureEmailSender)(nil)

// CaptureEmailSender is an email.Sender that captures sent messages instead of delivering them.
// MagicLinkToken extracts the sign-in token from the most recently sent message.
type CaptureEmailSender struct {
	mu       sync.Mutex
	messages []email.Message
}

// SendEmail captures the message.
func (s *CaptureEmailSender) SendEmail(_ context.Context, m email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
	return nil
}

// MagicLinkToken extracts the magic-link token from the most recently captured message.
// The token is the value of the "token" query parameter in the sign-in URL embedded in the text body.
// Fails the test if no message has been captured or no token is found.
func (s *CaptureEmailSender) MagicLinkToken(t *testing.T) string {
	t.Helper()
	body := s.lastMessage(t).TextBody
	for word := range strings.FieldsSeq(body) {
		u, err := url.Parse(word)
		if err != nil {
			continue
		}
		if tok := u.Query().Get("token"); tok != "" {
			return tok
		}
	}
	t.Fatalf("CaptureEmailSender.MagicLinkToken: no token found in text body: %q", body)
	return ""
}

func (s *CaptureEmailSender) lastMessage(t *testing.T) email.Message {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		t.Fatal("CaptureEmailSender: no messages captured")
	}
	return s.messages[len(s.messages)-1]
}
