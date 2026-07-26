package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewValidatesFormatAndEmitsSelectedFormat(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "info", "yaml"); err == nil {
		t.Fatal("unknown log format was accepted")
	}
	var output bytes.Buffer
	logger, err := New(&output, "debug", "json")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("ready", "kind", "test")
	if text := output.String(); !strings.Contains(text, `"msg":"ready"`) || !strings.Contains(text, `"kind":"test"`) {
		t.Fatalf("unexpected JSON log: %s", text)
	}
}
