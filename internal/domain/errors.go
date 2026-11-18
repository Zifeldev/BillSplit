package domain

import "errors"

// Domain errors are mapped to gRPC status codes in the transport layer.
var (
	ErrNotFound           = errors.New("not found")
	ErrAlreadyExists      = errors.New("already exists")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrForbidden          = errors.New("forbidden")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrUnavailable        = errors.New("unavailable") // upstream (e.g. market data) is down
)
