package tui

import (
	"strings"
	"testing"
)

func TestRenderTableOmitsPageTitleAndIncludesColumnsAndRows(t *testing.T) {
	rendered := RenderTable([]string{"NAME", "BALANCE"}, [][]string{{"Checking", "$42.00"}})
	for _, value := range []string{"NAME", "BALANCE", "Checking", "$42.00"} {
		if !strings.Contains(rendered, value) {
			t.Fatalf("rendered table does not contain %q: %q", value, rendered)
		}
	}
	for _, value := range []string{"Monarch CLI", "Accounts"} {
		if strings.Contains(rendered, value) {
			t.Fatalf("rendered table unexpectedly contains page title %q: %q", value, rendered)
		}
	}
}

func TestTerminalRenderersSanitizeUntrustedText(t *testing.T) {
	rendered := RenderTable([]string{"NA\x1b[2JME"}, [][]string{{"value\x1b[2J"}})
	if strings.Contains(rendered, "\x1b[2J") {
		t.Fatalf("table retained a terminal command: %q", rendered)
	}

	var output strings.Builder
	if err := WriteSuccess(&output, "saved\x1b[2J"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[2J") {
		t.Fatalf("success output retained a terminal command: %q", output.String())
	}
}
