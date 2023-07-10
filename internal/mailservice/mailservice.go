package mailservice

type MailSender interface {
	Send(from string, sender string, subject string, body string, recipient string) error
}
