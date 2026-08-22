package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrInvalid       = errors.New("invalid input")
	ErrForbidden     = errors.New("forbidden")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrExpired       = errors.New("expired")
	ErrRevoked       = errors.New("revoked")
	ErrUnavailable   = errors.New("dependency unavailable")
	ErrVersion       = errors.New("version conflict")
	ErrLeaseLost     = errors.New("lease lost")
	ErrIllegalState  = errors.New("illegal state transition")
	ErrCapacity      = errors.New("capacity unavailable")
	ErrPrecondition  = errors.New("precondition failed")
	ErrAlreadyExists = errors.New("already exists")
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", ErrInvalid, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", ErrInvalid, e.Field, e.Message)
}

func (e FieldError) Unwrap() error { return ErrInvalid }

type StateError struct {
	Entity string
	From   string
	To     string
	Reason string
}

func (e StateError) Error() string {
	return fmt.Sprintf("%s: %s cannot move from %s to %s: %s", ErrIllegalState, e.Entity, e.From, e.To, e.Reason)
}

func (e StateError) Unwrap() error { return ErrIllegalState }

type DependencyError struct {
	Operation string
	Err       error
}

func (e DependencyError) Error() string {
	return fmt.Sprintf("%s: %s: %v", ErrUnavailable, e.Operation, e.Err)
}

func (e DependencyError) Unwrap() error {
	if e.Err == nil {
		return ErrUnavailable
	}
	return errors.Join(ErrUnavailable, e.Err)
}

func Require(condition bool, field, message string) error {
	if condition {
		return nil
	}
	return FieldError{Field: field, Message: message}
}
