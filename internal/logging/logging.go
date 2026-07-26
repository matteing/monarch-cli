// Package logging configures structured diagnostics on stderr.
package logging

import (
	"errors"
	"io"
	"log/slog"
	"strings"
)

// New constructs a standard-library logger without ever writing to protocol
// stdout. Supported levels are debug, info, warn, and error; supported formats
// are text and json.
func New(output io.Writer, level, format string) (*slog.Logger, error) {
	var parsed slog.Level
	switch strings.ToLower(level) {
	case "debug":
		parsed = slog.LevelDebug
	case "info":
		parsed = slog.LevelInfo
	case "warn":
		parsed = slog.LevelWarn
	case "error":
		parsed = slog.LevelError
	default:
		return nil, errors.New("log level must be debug, info, warn, or error")
	}

	options := &slog.HandlerOptions{Level: parsed}
	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(output, options)
	case "json":
		handler = slog.NewJSONHandler(output, options)
	default:
		return nil, errors.New("log format must be text or json")
	}
	return slog.New(handler), nil
}
