// Package apperr defines the small, stable error vocabulary shared by the CLI.
package apperr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Kind classifies an error without exposing credentials or upstream response bodies.
type Kind string

const (
	KindInvalidInput Kind = "invalid_input"
	KindAuth         Kind = "authentication"
	KindMFARequired  Kind = "mfa_required"
	KindNotFound     Kind = "not_found"
	KindRateLimited  Kind = "rate_limited"
	KindUnavailable  Kind = "unavailable"
	KindKeyring      Kind = "keyring"
	KindCanceled     Kind = "canceled"
	KindInternal     Kind = "internal"
)

// Error is the application's typed error. Message is safe to show to a user.
type Error struct {
	Kind       Kind
	Op         string
	Message    string
	StatusCode int
	Retryable  bool
	RetryAfter time.Duration
	Err        error
}

// Error returns a concise, user-facing description.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		if e.RetryAfter > 0 {
			return fmt.Sprintf("%s; retry after %s", e.Message, e.RetryAfter)
		}
		return e.Message
	}
	if e.Op != "" {
		return fmt.Sprintf("%s failed", e.Op)
	}
	return string(e.Kind)
}

// Unwrap exposes the underlying cause for errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// New constructs a typed error with a safe message.
func New(kind Kind, op, message string, cause error) *Error {
	return &Error{Kind: kind, Op: op, Message: message, Err: cause}
}

// KindOf returns the application's classification for err.
func KindOf(err error) Kind {
	if errors.Is(err, context.Canceled) {
		return KindCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindUnavailable
	}
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		return appErr.Kind
	}
	return KindInternal
}

// PublicMessage returns terminal-safe source text before display sanitation.
// Untyped errors are intentionally collapsed so internal causes cannot leak.
func PublicMessage(err error) string {
	if errors.Is(err, context.Canceled) {
		return "operation canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "operation timed out"
	}
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		return appErr.Error()
	}
	return "unexpected internal error"
}

// Details is the stable, safe error representation used by JSON CLI and MCP
// responses. RetryAfterMS is omitted when the upstream supplied no guidance.
type Details struct {
	Kind         Kind   `json:"kind"`
	Message      string `json:"message"`
	Operation    string `json:"operation,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	Retryable    bool   `json:"retryable"`
	RetryAfterMS int64  `json:"retry_after_ms,omitempty"`
}

// Envelope is the common machine-readable error contract.
type Envelope struct {
	Error Details `json:"error"`
}

// Describe extracts safe structured metadata from err.
func Describe(err error) Details {
	if errors.Is(err, context.Canceled) {
		return Details{Kind: KindCanceled, Message: "operation canceled"}
	}
	details := Details{Kind: KindOf(err), Message: "unexpected internal error"}
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		details.Message = appErr.Message
		if details.Message == "" {
			details.Message = appErr.Error()
		}
		details.Operation = appErr.Op
		details.StatusCode = appErr.StatusCode
		details.Retryable = appErr.Retryable
		details.RetryAfterMS = appErr.RetryAfter.Milliseconds()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		details.Kind = KindUnavailable
		details.Message = "operation timed out"
		details.Retryable = true
	}
	return details
}

// MarshalJSON returns the common machine-readable envelope for err.
func MarshalJSON(err error) ([]byte, error) {
	return json.Marshal(Envelope{Error: Describe(err)})
}
