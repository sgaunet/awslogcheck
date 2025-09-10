package configapp

import "errors"

// Static errors for wrapping.
var (
	ErrRulesDirNotFound = errors.New("rules directory not found")
)