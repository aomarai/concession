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

type invalidError struct{ msg string }

func (e invalidError) Error() string        { return e.msg }
func (e invalidError) Is(target error) bool { return target == ErrInvalid }

// Invalid returns a validation error whose message is safe to show users.
func Invalid(msg string) error { return invalidError{msg} }

// Message returns the user-facing text of a validation error.
func Message(err error) string {
	var ie invalidError
	if errors.As(err, &ie) {
		return ie.msg
	}
	return ErrInvalid.Error()
}
