package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/matteing/monarch-cli/internal/apperr"
)

func TestBoundedNDJSONReaderResetsPerMessageAndCapsSession(t *testing.T) {
	input := "one\ntwo\n"
	reader := newBoundedNDJSONReader(strings.NewReader(input), 4, len(input))
	got, err := io.ReadAll(reader)
	if err != nil || string(got) != input {
		t.Fatalf("read = %q, error = %v", got, err)
	}

	reader = newBoundedNDJSONReader(strings.NewReader("12345\n"), 5, 100)
	if _, err := io.ReadAll(reader); !errors.Is(err, errMCPMessageTooLarge) {
		t.Fatalf("message-limit error = %v", err)
	}

	reader = newBoundedNDJSONReader(strings.NewReader("one\ntwo\n"), 8, 7)
	if _, err := io.ReadAll(reader); !errors.Is(err, errMCPSessionTooLarge) {
		t.Fatalf("session-limit error = %v", err)
	}
}

func TestRunIORejectsOversizedMessageBeforeToolDelegation(t *testing.T) {
	reader := &recordingReader{}
	input := strings.NewReader(strings.Repeat(" ", maxMCPMessageBytes) + "x\n")
	err := RunIO(context.Background(), reader, "test", input, &bytes.Buffer{}, nil)
	if !errors.Is(err, errMCPMessageTooLarge) || apperr.KindOf(err) != apperr.KindInvalidInput {
		t.Fatalf("RunIO error = %v", err)
	}
	if len(reader.calls) != 0 {
		t.Fatalf("oversized input delegated calls: %v", reader.calls)
	}
}

func TestRunIOCancellationReachesActiveReader(t *testing.T) {
	started := make(chan struct{})
	readerCanceled := make(chan error, 1)
	reader := &recordingReader{accountsFn: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		readerCanceled <- ctx.Err()
		return ctx.Err()
	}}

	ctx, cancel := context.WithCancel(context.Background())
	input, client := io.Pipe()
	t.Cleanup(func() {
		client.Close()
		input.Close()
	})

	done := make(chan error, 1)
	go func() {
		done <- RunIO(ctx, reader, "test", input, io.Discard, nil)
	}()
	go func() {
		messages := []string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"test"}}}`,
			`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"monarch_accounts_list","arguments":{}}}`,
		}
		for _, message := range messages {
			if _, err := fmt.Fprintln(client, message); err != nil {
				return
			}
		}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("tool call did not reach reader")
	}
	cancel()

	select {
	case err := <-readerCanceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reader context error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server cancellation did not reach reader")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunIO error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunIO did not return after cancellation")
	}
}
