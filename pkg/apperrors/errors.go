package apperrors

import "errors"

var (
	ErrNotFound    = errors.New("not found")
	ErrInvalidRole = errors.New("invalid role")
	ErrLastAdmin   = errors.New("cannot remove last admin")
)
