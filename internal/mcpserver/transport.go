package mcpserver

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/monarch"
)

const (
	maxMCPMessageBytes = 512 * 1024
	maxMCPSessionBytes = 16 * 1024 * 1024
)

var (
	errMCPMessageTooLarge = errors.New("MCP message exceeds the input limit")
	errMCPSessionTooLarge = errors.New("MCP session exceeds the input limit")
)

// RunIO serves MCP JSON-RPC over caller-provided streams. Closing a session does
// not close the caller's streams.
func RunIO(ctx context.Context, reader monarch.Reader, version string, input io.Reader, output io.Writer, logger *slog.Logger) error {
	boundedInput := newBoundedNDJSONReader(input, maxMCPMessageBytes, maxMCPSessionBytes)
	transport := &mcp.IOTransport{Reader: io.NopCloser(boundedInput), Writer: nopWriteCloser{output}}
	err := NewWithLogger(reader, version, logger).Run(ctx, transport)
	if errors.Is(err, errMCPMessageTooLarge) || errors.Is(err, errMCPSessionTooLarge) {
		return apperr.New(apperr.KindInvalidInput, "serve MCP", "MCP input exceeded the safety limit", err)
	}
	return err
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// boundedNDJSONReader buffers one bounded protocol line before exposing it to
// the SDK. The per-message ceiling prevents json.Decoder from allocating an
// arbitrary RawMessage, while the generous session budget caps the SDK's
// internal pending-request queue without constraining ordinary tool sessions.
type boundedNDJSONReader struct {
	source           *bufio.Reader
	messageLimit     int
	remainingSession int
	current          []byte
}

func newBoundedNDJSONReader(source io.Reader, messageLimit, sessionLimit int) *boundedNDJSONReader {
	return &boundedNDJSONReader{
		source: bufio.NewReader(source), messageLimit: messageLimit,
		remainingSession: sessionLimit,
	}
}

func (r *boundedNDJSONReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	for len(r.current) == 0 {
		message, err := r.readMessage()
		if err != nil {
			return 0, err
		}
		r.current = message
	}
	count := copy(destination, r.current)
	r.current = r.current[count:]
	return count, nil
}

func (r *boundedNDJSONReader) readMessage() ([]byte, error) {
	message := make([]byte, 0, min(r.messageLimit, 4096))
	for {
		fragment, err := r.source.ReadSlice('\n')
		if len(fragment) > r.remainingSession {
			return nil, errMCPSessionTooLarge
		}
		r.remainingSession -= len(fragment)
		if len(fragment) > r.messageLimit-len(message) {
			return nil, errMCPMessageTooLarge
		}
		message = append(message, fragment...)

		switch {
		case err == nil:
			return message, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(message) > 0:
			return message, nil
		default:
			return nil, err
		}
	}
}
