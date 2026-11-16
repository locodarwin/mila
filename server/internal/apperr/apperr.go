// Package apperr is the error a person can act on: a kind, which decides
// the HTTP status, and a message that is safe to show.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an Error.
type Kind int

const (
	Invalid Kind = iota + 1
	NotFound
	Forbidden
	Conflict
	Unauthenticated
	Gone
	// Unavailable is a service Mila depends on failing, such as email.
	Unavailable
	// UpdateRequired means the app on the device is another version than
	// the server's and must reload before it can sync.
	UpdateRequired
	// PreconditionFailed means the row changed since the version the
	// request named (an If-Match that no longer matches).
	PreconditionFailed
)

// Error is a failure the person can act on.
type Error struct {
	Kind    Kind
	Message string
	// Field names the field of the request at fault, when there is one
	// (only the REST entity endpoints say).
	Field string
	// Code, when set, is the error's code in place of its kind's, for a
	// case the app tells apart (a limit reached).
	Code string
}

func (e *Error) Error() string { return e.Message }

// New makes an Error.
func New(k Kind, format string, args ...any) error {
	return &Error{Kind: k, Message: fmt.Sprintf(format, args...)}
}

// KindOf returns the Kind of err, or 0 for unexpected errors.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}
