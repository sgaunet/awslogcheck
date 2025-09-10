package app

import "errors"

// Static errors for wrapping.
var (
	ErrNoRulesFolder     = errors.New("no rules folder found")
	ErrLogGroupNotFound  = errors.New("log group not found")
	ErrRulesDirNotFound  = errors.New("rules directory not found")
	ErrServiceNotConfig  = errors.New("service not configured")
	ErrSMTPConfigMissing = errors.New("smtp configuration missing")
	ErrSMTPServerFormat  = errors.New("smtp server format should be: host:port")
)