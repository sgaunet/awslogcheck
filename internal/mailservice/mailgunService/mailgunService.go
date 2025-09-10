package mailgunservice

import (
	"context"
	"fmt"
	"time"

	"github.com/mailgun/mailgun-go/v4"
	"github.com/sgaunet/awslogcheck/internal/mailservice"
)

type mailgunService struct {
	domain        string
	privateAPIKey string
}

// NewMailgunService creates a new Mailgun service instance.
//nolint:ireturn // Factory function intentionally returns interface for dependency injection
func NewMailgunService(domain string, privateAPIKey string) (mailservice.MailSender, error) {
	if !isMailGunConfigured(domain, privateAPIKey) {
		return nil, fmt.Errorf("%w", ErrServiceNotConfigured)
	}
	m := mailgunService{
		domain:        domain,
		privateAPIKey: privateAPIKey,
	}
	return &m, nil
}

func (m *mailgunService) Send(_ string, sender string, subject string, body string, recipient string) error {
	// Create an instance of the Mailgun Client
	mg := mailgun.NewMailgun(m.domain, m.privateAPIKey)
	// The message object allows you to add attachments and Bcc recipients
	// message := mg.NewMessage(sender, subject, body, recipient)
	message := mailgun.NewMessage(sender, subject, "", recipient)
	message.SetHTML(body)
	const emailTimeoutSeconds = 10
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*emailTimeoutSeconds)
	defer cancel()
	// Send the message with a 10 second timeout
	resp, id, err := mg.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send email via mailgun: %w", err)
	}
	fmt.Printf("ID: %s Resp: %s\n", id, resp)
	return nil
}

func isMailGunConfigured(domain string, apikey string) bool {
	if domain == "" || apikey == "" {
		return false
	}
	return true
}
