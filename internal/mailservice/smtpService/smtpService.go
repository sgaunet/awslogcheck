// Package smtpservice provides SMTP email service implementation.
package smtpservice

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"

	"github.com/sgaunet/awslogcheck/internal/mailservice"
)

type smtpService struct {
	smtpLogin    string
	smtpPassword string
	smtpServer   string
	tls          bool
}

// NewSMTPService creates a new SMTP service instance.
//nolint:ireturn // Factory function intentionally returns interface for dependency injection
func NewSMTPService(smtplogin string, smtpPassword string, smtpServer string,
	tls bool) (mailservice.MailSender, error) {
	s := smtpService{
		smtpLogin:    smtplogin,
		smtpPassword: smtpPassword,
		smtpServer:   smtpServer,
		tls:          tls,
	}
	if err := s.isSMTPConfigured(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *smtpService) Send(from string, _ string, subject string, body string, recipient string) error {
	message := s.buildEmailMessage(from, recipient, subject, body)
	host, auth, err := s.prepareAuthentication()
	if err != nil {
		return err
	}
	c, err := s.establishConnection(host)
	if err != nil {
		return err
	}
	defer s.closeConnection(c)
	return s.sendEmailData(c, auth, from, recipient, message)
}

func (s *smtpService) buildEmailMessage(from, recipient, subject, body string) string {
	fromEmail := mail.Address{Name: "", Address: from}
	to := mail.Address{Name: "", Address: recipient}

	headers := map[string]string{
		"From":         fromEmail.String(),
		"To":           to.String(),
		"Subject":      subject,
		"MIME-version": "1.0",
		"Content-Type": "text/html",
		"charset":      "UTF-8",
	}

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	return message + "\r\n" + body
}

func (s *smtpService) prepareAuthentication() (string, smtp.Auth, error) {
	host, _, err := net.SplitHostPort(s.smtpServer)
	if err != nil {
		return "", nil, fmt.Errorf("failed to split host:port: %w", err)
	}
	auth := smtp.PlainAuth("", s.smtpLogin, s.smtpPassword, host)
	return host, auth, nil
}

func (s *smtpService) establishConnection(host string) (*smtp.Client, error) {
	c, err := smtp.Dial(s.smtpServer)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SMTP server: %w", err)
	}
	if s.tls {
		if err := s.startTLS(c, host); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (s *smtpService) startTLS(c *smtp.Client, host string) error {
	tlsconfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
	}
	if err := c.StartTLS(tlsconfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}
	return nil
}

func (s *smtpService) closeConnection(c *smtp.Client) {
	if err := c.Quit(); err != nil {
		// Ignore quit errors
		_ = err
	}
}

func (s *smtpService) sendEmailData(c *smtp.Client, auth smtp.Auth, from, recipient, message string) error {
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("failed to set mail from: %w", err)
	}
	if err := c.Rcpt(recipient); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}
	if _, err := w.Write([]byte(message)); err != nil {
		return fmt.Errorf("failed to write email data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}
	return nil
}

func (s *smtpService) isSMTPConfigured() error {
	if s.smtpLogin == "" || s.smtpPassword == "" || s.smtpServer == "" {
		return fmt.Errorf("%w", ErrSMTPConfigMissing)
	}
	host, port, err := net.SplitHostPort(s.smtpServer)
	if err != nil {
		return fmt.Errorf("%w", ErrSMTPServerFormat)
	}
	if host == "" || port == "" {
		return fmt.Errorf("%w", ErrSMTPServerFormat)
	}
	return nil
}