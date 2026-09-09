// Package domain is the notification context's (small) domain layer.
package domain

import "context"

// NotificationService sends notifications to people. The concrete channel
// (email, SMS, push) is an infrastructure detail.
type NotificationService interface {
	SendWelcome(ctx context.Context, toEmail, name string) error
}
