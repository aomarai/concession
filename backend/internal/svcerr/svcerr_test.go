package svcerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestInvalid(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", Invalid("title is required"))
	if !errors.Is(err, ErrInvalid) {
		t.Error("Invalid should match ErrInvalid")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("Invalid should not match other sentinels")
	}
	if got := Message(err); got != "title is required" {
		t.Errorf("Message = %q", got)
	}
	if got := Message(errors.New("other")); got != "invalid input" {
		t.Errorf("Message of foreign error = %q", got)
	}
}
