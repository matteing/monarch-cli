// Package textsafe projects untrusted API and user text for terminal display.
// ANSI stripping alone is insufficient: Unicode controls, bidirectional format
// characters, separators, and isolated combining marks are dangerous without
// containing an ANSI escape sequence. JSON projection preserves decoded data.
package textsafe

import (
	"strings"
	"unicode"
	"unicode/utf16"
)

// Terminal replaces control and format characters with spaces so remote data
// cannot be interpreted as terminal commands or visually reorder trusted text.
// An isolated combining mark is anchored to a dotted circle instead of the
// trusted character that precedes the rendered value.
func Terminal(value string) string {
	var safe strings.Builder
	safe.Grow(len(value))
	hasBase := false
	for _, char := range value {
		if terminalControl(char) {
			safe.WriteByte(' ')
			hasBase = false
			continue
		}
		if unicode.IsMark(char) {
			if !hasBase {
				safe.WriteRune('◌')
				hasBase = true
			}
			safe.WriteRune(char)
			continue
		}
		safe.WriteRune(char)
		hasBase = !unicode.IsSpace(char)
	}
	return safe.String()
}

// EscapeJSON makes already-encoded JSON safe to inspect in a terminal without
// changing its decoded values. encoding/json already escapes C0 controls
// inside strings, so structural ASCII whitespace must remain intact.
func EscapeJSON(value string) string {
	var safe strings.Builder
	safe.Grow(len(value))
	for _, char := range value {
		if char >= unicode.MaxASCII && (terminalControl(char) || unicode.IsMark(char)) {
			writeJSONEscape(&safe, char)
			continue
		}
		safe.WriteRune(char)
	}
	return safe.String()
}

func terminalControl(char rune) bool {
	return unicode.IsControl(char) || unicode.Is(unicode.Cf, char) || unicode.Is(unicode.Zl, char) || unicode.Is(unicode.Zp, char)
}

func writeJSONEscape(output *strings.Builder, char rune) {
	if char <= 0xffff {
		writeJSONCodeUnit(output, char)
		return
	}
	high, low := utf16.EncodeRune(char)
	writeJSONCodeUnit(output, high)
	writeJSONCodeUnit(output, low)
}

func writeJSONCodeUnit(output *strings.Builder, char rune) {
	const hexadecimal = "0123456789abcdef"
	output.WriteString(`\u`)
	for shift := 12; shift >= 0; shift -= 4 {
		output.WriteByte(hexadecimal[char>>shift&0x0f])
	}
}
