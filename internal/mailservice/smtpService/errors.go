package smtpservice

import "errors"

// Static errors for wrapping.
var (
	ErrSMTPConfigMissing = errors.New("smtp login,password and server are mandatory")
	ErrSMTPServerFormat  = errors.New("smtp server format should be: host:port")
)