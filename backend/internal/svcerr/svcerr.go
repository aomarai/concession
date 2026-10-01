// Package svcerr defines the errors services return to handlers so they can be
// mapped to HTTP statuses in one place.
package svcerr

import "errors"

var (
	// ErrNotFound means the resource does not exist, or the caller has no access
	// to it at all (existence is not revealed to non-members).
	ErrNotFound = errors.New("not found")
	// ErrForbidden means the caller can see the resource but their role does not
	// allow the operation.
	ErrForbidden = errors.New("forbidden")
	// ErrDuplicate means the resource already exists.
	ErrDuplicate = errors.New("already exists")
	// ErrInvalid matches validation failures; see Invalid and Message.
	ErrInvalid = errors.New("invalid input")
)

// messageError is a sentinel error with a message that is safe to show users.
type messageError struct {
	kind error
	msg  string
}

func (e messageError) Error() string        { return e.msg }
func (e messageError) Is(target error) bool { return target == e.kind }

// Invalid returns a validation error whose message is safe to show users.
func Invalid(msg string) error { return messageError{ErrInvalid, msg} }

// NotFound returns a not-found error with a user-facing message.
func NotFound(msg string) error { return messageError{ErrNotFound, msg} }

// Duplicate returns an already-exists error with a user-facing message.
func Duplicate(msg string) error { return messageError{ErrDuplicate, msg} }

// MessageOr returns the user-facing message carried by err (created with
// Invalid, NotFound or Duplicate), or fallback when it carries none.
func MessageOr(err error, fallback string) string {
	var me messageError
	if errors.As(err, &me) {
		return me.msg
	}
	return fallback
}

// Message returns the user-facing text of a validation error.
func Message(err error) string { return MessageOr(err, ErrInvalid.Error()) }
