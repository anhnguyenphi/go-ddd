// Package email is a NotificationService that "sends" by logging. Replace with
// an SMTP / provider client; the port stays the same.
package email

import (
	"context"
	"log/slog"

	"github.com/example/myapp/internal/notification/domain"
)

// LogSender implements domain.NotificationService by writing to the logger.
type LogSender struct{ logger *slog.Logger }

// NewLogSender builds the sender.
func NewLogSender(logger *slog.Logger) *LogSender { return &LogSender{logger: logger} }

var _ domain.NotificationService = (*LogSender)(nil)

// SendWelcome pretends to deliver a welcome email.
func (s *LogSender) SendWelcome(ctx context.Context, toEmail, name string) error {
	s.logger.InfoContext(ctx, "welcome email sent", "to", toEmail, "name", name)
	return nil
}
