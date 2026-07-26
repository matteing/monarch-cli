package textsafe

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"
)

func TestTerminalReplacesAllControlCharacters(t *testing.T) {
	input := "safe\x1b[2J\nnext\u009b31m\u202ereversed\u2066isolated\u2069\u200b\u2028line\u2029paragraph"
	if got, want := Terminal(input), "safe [2J next 31m reversed isolated   line paragraph"; got != want {
		t.Fatalf("Terminal(%q) = %q, want %q", input, got, want)
	}
}

func TestTerminalAnchorsIsolatedCombiningMarks(t *testing.T) {
	input := "\u0301leading a\u0301 \u0301after-space"
	want := "◌\u0301leading a\u0301 ◌\u0301after-space"
	if got := Terminal(input); got != want {
		t.Fatalf("Terminal(%q) = %q, want %q", input, got, want)
	}
}

func TestEscapeJSONPreservesValuesAndStructuralWhitespace(t *testing.T) {
	value := "\x7f\u0085\u202e\u0301a\u0301\u2028\u2029\U000e0001"
	raw, err := json.MarshalIndent(map[string]string{"value": value}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	safe := EscapeJSON(string(raw))
	for _, char := range safe {
		if char >= unicode.MaxASCII && (terminalControl(char) || unicode.IsMark(char)) {
			t.Fatalf("escaped JSON retained unsafe character %U: %q", char, safe)
		}
	}
	if safe[len(safe)-1] != '\n' {
		t.Fatalf("escaped JSON lost its structural newline: %q", safe)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(safe), &decoded); err != nil {
		t.Fatalf("escaped JSON is invalid: %v: %q", err, safe)
	}
	if decoded["value"] != value {
		t.Fatalf("decoded value = %q, want %q", decoded["value"], value)
	}
	for _, escape := range []string{`\u007f`, `\u0085`, `\u202e`, `\u0301`, `\u2028`, `\u2029`, `\udb40\udc01`} {
		if !strings.Contains(safe, escape) {
			t.Fatalf("escaped JSON does not contain %q: %q", escape, safe)
		}
	}
}

func FuzzTerminalNeverReturnsControlCharacters(f *testing.F) {
	f.Add("safe\x1b[2J\nnext\u009b31m\u202e\u2066\u2069\u200b\u2028\u2029")
	f.Add("plain text")
	f.Fuzz(func(t *testing.T, input string) {
		hasBase := false
		for _, char := range Terminal(input) {
			if unicode.IsControl(char) || unicode.Is(unicode.Cf, char) || unicode.Is(unicode.Zl, char) || unicode.Is(unicode.Zp, char) {
				t.Fatalf("Terminal(%q) retained unsafe character %U", input, char)
			}
			if unicode.IsMark(char) && !hasBase {
				t.Fatalf("Terminal(%q) returned an unanchored combining mark %U", input, char)
			}
			if !unicode.IsMark(char) {
				hasBase = !unicode.IsSpace(char)
			}
		}
	})
}

func FuzzEscapeJSONPreservesDecodedStrings(f *testing.F) {
	for _, seed := range []string{"plain", "\u202e\u0085", "\u0301a\u0301", "\U000e0001"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		safe := EscapeJSON(string(raw))
		var before, after string
		if err := json.Unmarshal(raw, &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(safe), &after); err != nil {
			t.Fatalf("EscapeJSON returned invalid JSON: %v: %q", err, safe)
		}
		if after != before {
			t.Fatalf("decoded value changed from %q to %q", before, after)
		}
		for _, char := range safe {
			if char >= unicode.MaxASCII && (terminalControl(char) || unicode.IsMark(char)) {
				t.Fatalf("EscapeJSON retained unsafe character %U", char)
			}
		}
	})
}
