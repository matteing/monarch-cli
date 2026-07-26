package apperr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestKindOfHandlesWrappingAndTypedNil(t *testing.T) {
	err := New(KindAuth, "authenticate", "authentication failed", errors.New("cause"))
	if got := KindOf(errors.Join(errors.New("outer"), err)); got != KindAuth {
		t.Fatalf("kind = %q, want %q", got, KindAuth)
	}
	var typedNil *Error
	var nilErr error = typedNil
	if got := KindOf(nilErr); got != KindInternal {
		t.Fatalf("typed-nil kind = %q, want %q", got, KindInternal)
	}
}

func TestDescribeDoesNotExposeUntypedErrors(t *testing.T) {
	details := Describe(errors.New("Authorization: secret"))
	if details.Kind != KindInternal || strings.Contains(details.Message, "secret") {
		t.Fatalf("unsafe details: %+v", details)
	}
}

func TestCancellationHasStablePublicContract(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", context.Canceled)
	if got := KindOf(err); got != KindCanceled {
		t.Fatalf("kind = %q, want %q", got, KindCanceled)
	}
	details := Describe(err)
	if details.Kind != KindCanceled || details.Message != "operation canceled" {
		t.Fatalf("details = %+v", details)
	}
	if got := PublicMessage(err); got != "operation canceled" {
		t.Fatalf("public message = %q", got)
	}
}

func TestDeadlineHasStablePublicContract(t *testing.T) {
	for name, err := range map[string]error{
		"raw":     context.DeadlineExceeded,
		"wrapped": New(KindAuth, "request", "stale typed message", context.DeadlineExceeded),
	} {
		t.Run(name, func(t *testing.T) {
			if got := KindOf(err); got != KindUnavailable {
				t.Fatalf("kind = %q, want %q", got, KindUnavailable)
			}
			details := Describe(err)
			if details.Kind != KindUnavailable || details.Message != "operation timed out" || !details.Retryable {
				t.Fatalf("details = %+v", details)
			}
			if got := PublicMessage(err); got != "operation timed out" {
				t.Fatalf("public message = %q", got)
			}
		})
	}
}
