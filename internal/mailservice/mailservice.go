// Package mailservice provides email service interfaces and implementations.
package mailservice

// MailSender defines the interface for sending emails.
type MailSender interface {
	Send(from string, sender string, subject string, body string, recipient string) error
}
