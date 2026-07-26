package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m transactionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancel()
			m.canceled = true
			return m, tea.Quit
		case "left":
			if !m.loading && m.page > 0 {
				m.page--
				m.err = nil
				m.renderPage(true)
			}
			return m, nil
		case "right":
			if m.loading {
				return m, nil
			}
			current, ok := m.pages[m.page]
			if !ok || current.NextCursor == "" {
				return m, nil
			}
			target := m.page + 1
			if _, ok := m.pages[target]; ok {
				m.page = target
				m.err = nil
				m.renderPage(true)
				return m, nil
			}
			m.loading = true
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, m.fetch(target, current.NextCursor))
		}
	case transactionPageMsg:
		if m.canceled {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.pages[msg.index] = msg.page
		m.page = msg.index
		m.renderPage(true)
		return m, nil
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(msg)
		return m, command
	}

	if _, ok := m.pages[m.page]; ok {
		var command tea.Cmd
		m.viewport, command = m.viewport.Update(msg)
		return m, command
	}
	return m, nil
}
