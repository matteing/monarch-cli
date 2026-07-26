package tui

import (
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteing/monarch-cli/internal/auth"
	"github.com/matteing/monarch-cli/internal/textsafe"
)

func (m loginModel) View() tea.View {
	var body strings.Builder
	gap := "\n\n"
	if m.height < 16 {
		gap = "\n"
	}

	if m.opts.Method == auth.MethodBrowserSession {
		body.WriteString(mutedStyle.Render("Copy the Cookie request header from a signed-in app.monarch.com tab."))
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render(BrowserPrivacyNotice))
	} else {
		body.WriteString(mutedStyle.Render(PasswordPrivacyNotice))
	}
	body.WriteString(gap)

	if m.stage == stageWorking || m.stage == stageSaving {
		body.WriteString(m.spinner.View())
		body.WriteString(" ")
		body.WriteString(textsafe.Terminal(m.status))
	} else if m.stage == stageSaveFailed {
		body.WriteString(errorStyle.Render(textsafe.Terminal(m.err.Error())))
		body.WriteString(gap)
		body.WriteString(mutedStyle.Render("enter retry  ·  q return error  ·  esc cancel"))
	} else {
		for index := range m.inputs {
			body.WriteString(terminalSafeInputView(m.inputs[index]))
			body.WriteString("\n")
		}
		if m.err != nil {
			body.WriteString(errorStyle.Render(textsafe.Terminal(m.err.Error())))
			body.WriteString("\n")
		}
		if m.height >= 16 {
			body.WriteString("\n")
		}
		controls := "enter continue  ·  esc cancel"
		if len(m.inputs) > 1 {
			controls = "enter continue  ·  tab switch  ·  esc cancel"
		}
		if m.width < 52 {
			controls = "enter  ·  esc"
			if len(m.inputs) > 1 {
				controls = "enter  ·  tab  ·  esc"
			}
		}
		body.WriteString(mutedStyle.Render(controls))
	}

	layout := responsiveFrame(m.width, 72)
	verticalPadding := 1
	if m.height < 10 {
		verticalPadding = 0
	}
	style := lipgloss.NewStyle().
		Padding(verticalPadding, layout.padding).
		Width(layout.frameWidth).
		MaxWidth(max(m.width, 1)).
		MaxHeight(max(m.height, 1))
	return tea.NewView(style.Render(body.String()))
}

// terminalSafeInputView projects visible input without changing the value that
// editing and authentication operate on. Secret fields are already masked by
// textinput, so only normally echoed values need projection.
func terminalSafeInputView(input textinput.Model) string {
	if input.EchoMode != textinput.EchoNormal {
		return input.View()
	}
	raw := []rune(input.Value())
	position := min(input.Position(), len(raw))
	display := input
	// Projection can add a dotted-circle anchor, so the credential's input
	// limit must not truncate the display-only clone.
	display.CharLimit = 0
	display.SetValue(textsafe.Terminal(string(raw)))
	// SetValue preserves the clone's private viewport when its old cursor still
	// falls inside it. Move to an edge once so textinput recomputes offsets for
	// the projected widths before restoring the mapped cursor.
	display.CursorEnd()
	display.SetCursor(utf8.RuneCountInString(textsafe.Terminal(string(raw[:position]))))
	return display.View()
}
