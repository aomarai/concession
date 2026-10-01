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

func TestMessageCarryingErrors(t *testing.T) {
	cases := []struct {
		err  error
		kind error
		msg  string
	}{
		{NotFound("no such user"), ErrNotFound, "no such user"},
		{Duplicate("already invited"), ErrDuplicate, "already invited"},
		{Invalid("bad"), ErrInvalid, "bad"},
	}
	for _, tc := range cases {
		wrapped := fmt.Errorf("ctx: %w", tc.err)
		if !errors.Is(wrapped, tc.kind) || MessageOr(wrapped, "fallback") != tc.msg {
			t.Errorf("%v: is=%v msg=%q", tc.err, errors.Is(wrapped, tc.kind), MessageOr(wrapped, "fallback"))
		}
	}
	if errors.Is(NotFound("x"), ErrDuplicate) {
		t.Error("kinds must not cross-match")
	}
	if got := MessageOr(ErrNotFound, "Not found"); got != "Not found" {
		t.Errorf("a bare sentinel carries no message, got %q", got)
	}
}
