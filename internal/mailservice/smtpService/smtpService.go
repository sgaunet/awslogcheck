package smtpservice

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
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

func NewSmtpService(smtplogin string, smtpPassword string, smtpServer string, tls bool) (mailservice.MailSender, error) {
	s := smtpService{
		smtpLogin:    smtplogin,
		smtpPassword: smtpPassword,
		smtpServer:   smtpServer,
		tls:          tls,
	}
	if err := s.isSmtpConfigured(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *smtpService) isSmtpConfigured() error {
	if s.smtpLogin == "" || s.smtpPassword == "" || s.smtpServer == "" {
		return errors.New("smtp login,password and server are mandatory")
	}
	host, port, err := net.SplitHostPort(s.smtpServer)
	if err != nil {
		return errors.New("smtp server format should be: host:port")
	}
	if host == "" || port == "" {
		return errors.New("smtp server format should be: host:port")
	}
	return nil
}

func (s *smtpService) Send(from string, sender string, subject string, body string, recipient string) error {
	var w io.WriteCloser
	fromEmail := mail.Address{
		Name:    "",
		Address: from,
	}
	to := mail.Address{
		Name:    "",
		Address: sender,
	}

	headers := make(map[string]string)
	headers["From"] = fromEmail.String()
	headers["To"] = to.String()
	headers["Subject"] = subject
	headers["MIME-version"] = "1.0"
	headers["Content-Type"] = "text/html"
	headers["charset"] = "UTF-8"

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	host, _, err := net.SplitHostPort(s.smtpServer)
	if err != nil {
		return err
	}
	auth := smtp.PlainAuth("", s.smtpLogin, s.smtpPassword, host)

	c, err := smtp.Dial(s.smtpServer)
	if err != nil {
		return err
	}
	defer c.Quit()

	if s.tls {
		tlsconfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         host,
		}
		err = c.StartTLS(tlsconfig)
		if err != nil {
			return err
		}
	}
	// Auth
	if err = c.Auth(auth); err != nil {
		return err
	}
	if err = c.Mail(fromEmail.Address); err != nil {
		return err
	}
	if err = c.Rcpt(to.Address); err != nil {
		return err
	}
	if w, err = c.Data(); err != nil {
		return err
	}
	if _, err = w.Write([]byte(message)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return err
}
