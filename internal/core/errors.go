package core

import (
	"errors"
	"fmt"
)

// Sentinel errors used throughout the application.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrQuotaExceeded = errors.New("quota exceeded")
	ErrBuildFailed   = errors.New("build failed")
	ErrDeployFailed  = errors.New("deploy failed")
	ErrInvalidInput  = errors.New("invalid input")
	ErrExpired       = errors.New("expired")
	ErrInvalidToken  = errors.New("invalid token")
)

// AppError is a structured application error with an HTTP status code.
type AppError struct {
	Code    int    // HTTP status code
	Message string // User-facing message
	Err     error  // Underlying error
}

// NewAppError creates a new AppError.
func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}
