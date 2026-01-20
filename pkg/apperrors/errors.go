package apperrors

import "errors"

var (
	ErrNotFound               = errors.New("not found")
	ErrConflict               = errors.New("conflict")
	ErrDatasourceLimitReached = errors.New("datasource limit reached")
	ErrInvalidRole            = errors.New("invalid role")
	ErrLastAdmin              = errors.New("cannot remove last admin")
	ErrCredentialsKeyMismatch = errors.New("datasource credentials were encrypted with a different key")
)
