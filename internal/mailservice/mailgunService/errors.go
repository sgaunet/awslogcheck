// Package mailgunservice provides Mailgun email service implementation.
package mailgunservice

import "errors"

// Static errors for wrapping.
var (
	ErrServiceNotConfigured = errors.New("service not configured (domain and privateKey mandatory)")
)