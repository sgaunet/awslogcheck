package mailgunservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mailgun/mailgun-go/v4"
	"github.com/sgaunet/awslogcheck/internal/mailservice"
)

type mailgunService struct {
	domain        string
	privateAPIKey string
}

func NewMailgunService(domain string, privateAPIKey string) (mailservice.MailSender, error) {
	if !isMailGunConfigured(domain, privateAPIKey) {
		return nil, errors.New("service not configured (domain and privateKey mandatory)")
	}
	m := mailgunService{
		domain:        domain,
		privateAPIKey: privateAPIKey,
	}
	return &m, nil
}

func (m *mailgunService) Send(from string, sender string, subject string, body string, recipient string) error {
	// Create an instance of the Mailgun Client
	mg := mailgun.NewMailgun(m.domain, m.privateAPIKey)
	// The message object allows you to add attachments and Bcc recipients
	// message := mg.NewMessage(sender, subject, body, recipient)
	message := mg.NewMessage(sender, subject, "", recipient)
	message.SetHtml(body)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	// Send the message with a 10 second timeout
	resp, id, err := mg.Send(ctx, message)
	if err != nil {
		return err
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
