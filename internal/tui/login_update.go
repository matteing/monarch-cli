package tui

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/session"
)

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.stage == stageSaving {
				return m, nil
			}
			m.cancel()
			m.clearSecrets()
			m.canceled = true
			return m, tea.Quit
		case "q":
			if m.stage == stageSaveFailed {
				m.clearSecrets()
				return m, tea.Quit
			}
		case "tab", "shift+tab":
			if m.stage == stageCredentials && len(m.inputs) > 1 {
				m.moveFocus(msg.String() == "shift+tab")
				return m, nil
			}
		case "enter":
			switch m.stage {
			case stageCredentials:
				if len(m.inputs) > 1 && m.focused == 0 {
					m.moveFocus(false)
					return m, nil
				}
				return m.submitCredentials()
			case stageMFA:
				return m.submitMFA()
			case stageSaveFailed:
				m.stage = stageSaving
				m.status = "Saving session…"
				m.err = nil
				return m, tea.Batch(m.spinner.Tick, m.save(m.pendingSession))
			}
		}
	case contextDoneMsg:
		if m.canceled {
			return m, nil
		}
		if m.stage == stageSaving {
			m.cancelErr = msg.err
			return m, nil
		}
		return m.stop(msg.err)
	case loginResultMsg:
		if contextTermination(msg.err) {
			return m.stop(msg.err)
		}
		if apperr.KindOf(msg.err) == apperr.KindMFARequired {
			return m.showMFAForm(msg.err)
		}
		if msg.err != nil {
			return m.showCredentialForm(msg.err)
		}
		m.clearSecrets()
		m.stage = stageWorking
		m.status = "Verifying session…"
		m.err = nil
		return m, m.verify(msg.value)
	case verifyResultMsg:
		if contextTermination(msg.err) {
			return m.stop(msg.err)
		}
		if msg.err != nil {
			return m.showCredentialForm(msg.err)
		}
		if err := m.opts.Context.Err(); err != nil {
			return m.stop(err)
		}
		m.pendingSession = msg.value
		m.stage = stageSaving
		m.status = "Saving session…"
		m.err = nil
		return m, m.save(msg.value)
	case saveResultMsg:
		if contextTermination(msg.err) {
			return m.stop(msg.err)
		}
		if msg.err != nil {
			if m.cancelErr != nil {
				return m.stop(m.cancelErr)
			}
			if err := m.opts.Context.Err(); err != nil {
				return m.stop(err)
			}
			m.stage = stageSaveFailed
			m.status = ""
			m.err = msg.err
			return m, nil
		}
		m.result = msg.value
		m.pendingSession = session.Session{}
		m.err = nil
		return m, tea.Quit
	case spinner.TickMsg:
		if m.stage != stageWorking && m.stage != stageSaving {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if m.stage == stageCredentials || m.stage == stageMFA {
		if len(m.inputs) == 0 {
			return m, nil
		}
		if m.focused < 0 || m.focused >= len(m.inputs) {
			m.focused = 0
		}
		var cmd tea.Cmd
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
		return m, cmd
	}
	return m, nil
}

func contextTermination(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (m loginModel) stop(err error) (tea.Model, tea.Cmd) {
	m.cancel()
	m.clearSecrets()
	m.canceled = true
	m.cancelErr = err
	return m, tea.Quit
}
