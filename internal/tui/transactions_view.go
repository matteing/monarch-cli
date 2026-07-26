package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteing/monarch-cli/internal/textsafe"
)

func (m transactionModel) View() tea.View {
	contentWidth := m.contentWidth()
	var body strings.Builder

	if _, ok := m.pages[m.page]; ok {
		body.WriteString(m.viewport.View())
		body.WriteString("\n")
		body.WriteString(m.pageStatus())
		body.WriteString("\n")
		if m.loading {
			body.WriteString(m.spinner.View() + " Loading page…")
		} else if m.err != nil {
			body.WriteString(errorStyle.Render(textsafe.Terminal(m.err.Error())))
		} else {
			controls := "↑/↓ scroll  ·  ←/→ page  ·  q quit"
			if contentWidth < 52 {
				controls = "↑↓ scroll  ·  ←→ page  ·  q"
			}
			body.WriteString(mutedStyle.Render(controls))
		}
	} else if m.err != nil {
		body.WriteString(errorStyle.Render(textsafe.Terminal(m.err.Error())))
		body.WriteString("\n\n")
		body.WriteString(mutedStyle.Render("q quit"))
	} else {
		body.WriteString(m.spinner.View() + " Loading transactions…")
	}

	layout := responsiveFrame(m.width, 140)
	view := tea.NewView(lipgloss.NewStyle().
		Padding(1, layout.padding).
		Width(layout.frameWidth).
		MaxWidth(max(m.width, 1)).
		MaxHeight(max(m.height, 1)).
		Render(body.String()))
	view.AltScreen = true
	return view
}

func (m transactionModel) pageStatus() string {
	page, ok := m.pages[m.page]
	if !ok {
		return ""
	}
	status := fmt.Sprintf("Page %d", m.page+1)
	if m.opts.InitialCursor == "" && page.TotalCount > 0 && len(page.Transactions) > 0 {
		start := 1
		for index := 0; index < m.page; index++ {
			previous, present := m.pages[index]
			if !present {
				return mutedStyle.Render(status)
			}
			start += len(previous.Transactions)
		}
		end := start + len(page.Transactions) - 1
		status += fmt.Sprintf(" · %d–%d of %d", start, end, page.TotalCount)
	}
	if !m.viewport.AtTop() || !m.viewport.AtBottom() {
		status += fmt.Sprintf(" · %d%%", int(m.viewport.ScrollPercent()*100))
	}
	return mutedStyle.Render(status)
}
