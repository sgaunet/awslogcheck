package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
)

func isSmtpConfigured(smptpLogin string, smtpPassword string, smtpServer string) bool {
	if smptpLogin == "" || smtpPassword == "" || smtpServer == "" {
		return false
	}
	s, p, err := net.SplitHostPort(smtpServer)
	if err != nil {
		return false
	}
	if s == "" || p == "" {
		return false
	}
	return true
}

func sendSmtpMail(fromEmail string, dest string, subj string, body string, smtpUsername string, smtpPassword string, smtpServer string, tlsCnx bool) error {
	var w io.WriteCloser
	from := mail.Address{"", fromEmail}
	to := mail.Address{"", dest}

	headers := make(map[string]string)
	headers["From"] = from.String()
	headers["To"] = to.String()
	headers["Subject"] = subj
	headers["MIME-version"] = "1.0"
	headers["Content-Type"] = "text/html"
	headers["charset"] = "UTF-8"

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	host, _, err := net.SplitHostPort(smtpServer)
	if err != nil {
		return err
	}
	auth := smtp.PlainAuth("", smtpUsername, smtpPassword, host)

	c, err := smtp.Dial(smtpServer)
	if err != nil {
		return err
	}
	defer c.Quit()

	if tlsCnx {
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
	if err = c.Mail(from.Address); err != nil {
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
