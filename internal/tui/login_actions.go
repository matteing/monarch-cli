package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/matteing/monarch-cli/internal/auth"
	"github.com/matteing/monarch-cli/internal/session"
)

func (m loginModel) submitCredentials() (tea.Model, tea.Cmd) {
	email := strings.TrimSpace(m.inputs[0].Value())
	password := m.inputs[1].Value()
	if email == "" {
		m.err = errors.New("enter an email address to continue")
		m.focused = 0
		m.inputs[1].Blur()
		return m, m.inputs[0].Focus()
	}
	if password == "" {
		m.err = errors.New("enter a password to continue")
		m.focused = 1
		m.inputs[0].Blur()
		return m, m.inputs[1].Focus()
	}
	if err := auth.ValidateLoginInput(email, password, ""); err != nil {
		m.err = err
		return m, nil
	}
	if m.opts.Context.Err() != nil {
		return m.stop(m.opts.Context.Err())
	}
	m.pendingEmail = email
	m.pendingPassword = password
	m.stage = stageWorking
	m.status = "Signing in…"
	m.err = nil
	return m, tea.Batch(m.spinner.Tick, m.authenticate(email, password, ""))
}

func (m loginModel) submitMFA() (tea.Model, tea.Cmd) {
	code := strings.TrimSpace(m.inputs[0].Value())
	if code == "" {
		m.err = errors.New("enter an MFA code to continue")
		return m, nil
	}
	if err := auth.ValidateLoginInput(m.pendingEmail, m.pendingPassword, code); err != nil {
		m.err = err
		return m, nil
	}
	m.inputs[0].Reset()
	m.stage = stageWorking
	m.status = "Verifying MFA code…"
	m.err = nil
	return m, tea.Batch(m.spinner.Tick, m.authenticate(m.pendingEmail, m.pendingPassword, code))
}

func (m loginModel) authenticate(email, password, code string) tea.Cmd {
	return func() tea.Msg {
		value, err := m.opts.Authenticate(m.opts.Context, email, password, code)
		return loginResultMsg{value: value, err: err}
	}
}

func (m loginModel) verify(value session.Session) tea.Cmd {
	return func() tea.Msg {
		if err := m.opts.Verify(m.opts.Context, value); err != nil {
			return verifyResultMsg{err: fmt.Errorf("verify session before saving: %w", err)}
		}
		return verifyResultMsg{value: value}
	}
}

func (m loginModel) save(value session.Session) tea.Cmd {
	return func() tea.Msg {
		if err := m.opts.Context.Err(); err != nil {
			return saveResultMsg{err: err}
		}
		if err := m.opts.Save(m.opts.Profile, value); err != nil {
			return saveResultMsg{err: err}
		}
		return saveResultMsg{value: value}
	}
}
