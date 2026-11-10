package iam

import (
	"errors"
	"fmt"
)

const (
	CodeInvalidClient    = "invalid_client"
	CodeInvalidGrant     = "invalid_grant"
	CodeNoSigningKey     = "no_signing_key"
	CodeValidation       = "validation"
	CodeInvalidReference = "invalid_reference"
	CodeConflict         = "conflict"
	CodeInUse            = "in_use"
	CodeNotFound         = "not_found"
	CodeInternal         = "internal"
)

var (
	ErrNotConfigured    = errors.New("iam: client is not configured")
	ErrInvalidClient    = errors.New("iam: invalid client")
	ErrInvalidGrant     = errors.New("iam: invalid grant")
	ErrNoSigningKey     = errors.New("iam: no active signing key")
	ErrValidation       = errors.New("iam: validation error")
	ErrInvalidReference = errors.New("iam: invalid reference")
	ErrConflict         = errors.New("iam: conflict")
	ErrInUse            = errors.New("iam: record is in use")
	ErrNotFound         = errors.New("iam: not found")
	ErrInternal         = errors.New("iam: internal error")

	ErrInvalidToken     = errors.New("iam: invalid token")
	ErrTokenExpired     = errors.New("iam: token expired")
	ErrUnexpectedMethod = errors.New("iam: unexpected signing method")
	ErrKeyNotFound      = errors.New("iam: signing key not found in jwks")
	ErrForbidden        = errors.New("iam: permission denied")
)

// Error represents an HTTP error returned by the IAM API.
type Error struct {
	StatusCode int    `json:"-"`
	Status     string `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"error"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	code := e.Code
	if code == "" {
		code = "error"
	}
	msg := e.Message
	if msg == "" {
		msg = e.Status
	}
	return fmt.Sprintf("iam %s (%d): %s", code, e.StatusCode, msg)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Code {
	case CodeInvalidClient:
		return ErrInvalidClient
	case CodeInvalidGrant:
		return ErrInvalidGrant
	case CodeNoSigningKey:
		return ErrNoSigningKey
	case CodeValidation:
		return ErrValidation
	case CodeInvalidReference:
		return ErrInvalidReference
	case CodeConflict:
		return ErrConflict
	case CodeInUse:
		return ErrInUse
	case CodeNotFound:
		return ErrNotFound
	case CodeInternal:
		return ErrInternal
	default:
		switch e.StatusCode {
		case 401:
			return ErrInvalidGrant
		case 404:
			return ErrNotFound
		case 409:
			return ErrConflict
		case 400:
			return ErrValidation
		default:
			return ErrInternal
		}
	}
}

// IsCode checks if err is an IAM *Error with the given application code.
func IsCode(err error, code string) bool {
	var iamErr *Error
	if errors.As(err, &iamErr) {
		return iamErr.Code == code
	}
	return false
}
