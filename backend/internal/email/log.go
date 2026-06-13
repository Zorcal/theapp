package email

import (
	"context"
	"log/slog"
)

var _ Sender = (*LogSender)(nil)

// LogSender is a Sender that logs outbound emails instead of delivering them.
// Intended for local development and testing environments.
type LogSender struct {
	log *slog.Logger
}

// NewLogSender returns a LogSender that writes to the given logger.
func NewLogSender(log *slog.Logger) *LogSender {
	return &LogSender{log: log}
}

// SendEmail logs the email message at INFO level and returns nil.
func (s *LogSender) SendEmail(ctx context.Context, m Message) error {
	attrs := []slog.Attr{
		slog.String("from", m.From),
		slog.Any("to", m.To),
		slog.String("subject", m.Subject),
		slog.String("html_body", m.HTMLBody),
	}
	if m.TextBody != "" {
		attrs = append(attrs, slog.String("text_body", m.TextBody))
	}
	s.log.LogAttrs(ctx, slog.LevelInfo, "Send email", attrs...)
	return nil
}
