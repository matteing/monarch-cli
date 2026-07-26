package mcpserver

import "github.com/matteing/monarch-cli/internal/apperr"

type structuredToolError struct{ cause error }

func (e structuredToolError) Error() string {
	raw, err := apperr.MarshalJSON(e.cause)
	if err != nil {
		return "unexpected internal error"
	}
	return string(raw)
}

func (e structuredToolError) Unwrap() error { return e.cause }

func toolError(err error) error {
	if err == nil {
		return nil
	}
	return structuredToolError{cause: err}
}
