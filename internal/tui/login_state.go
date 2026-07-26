package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/matteing/monarch-cli/internal/session"
)

func (m *loginModel) moveFocus(backward bool) {
	m.inputs[m.focused].Blur()
	if backward {
		m.focused = (m.focused - 1 + len(m.inputs)) % len(m.inputs)
	} else {
		m.focused = (m.focused + 1) % len(m.inputs)
	}
	m.inputs[m.focused].Focus()
}

func (m *loginModel) clearSecrets() {
	m.pendingEmail = ""
	m.pendingPassword = ""
	m.pendingSession = session.Session{}
	for index := range m.inputs {
		m.inputs[index].Reset()
	}
}

func (m loginModel) showMFAForm(err error) (tea.Model, tea.Cmd) {
	for index := range m.inputs {
		m.inputs[index].Reset()
	}
	m.inputs = []textinput.Model{newMFAInput()}
	m.focused = 0
	m.stage = stageMFA
	m.status = ""
	m.err = err
	m.resize(m.width, m.height)
	return m, m.inputs[0].Focus()
}

func (m loginModel) showCredentialForm(err error) (tea.Model, tea.Cmd) {
	email := m.pendingEmail
	m.clearSecrets()
	m.inputs = newCredentialInputs()
	m.focused = 0
	if email != "" {
		m.inputs[0].SetValue(email)
		m.inputs[0].Blur()
		m.focused = 1
	}
	m.stage = stageCredentials
	m.status = ""
	m.err = err
	m.resize(m.width, m.height)
	return m, m.inputs[m.focused].Focus()
}

func (m *loginModel) resize(width, height int) {
	m.width = max(width, 1)
	m.height = max(height, 1)
	contentWidth := responsiveFrame(m.width, 72).contentWidth
	inputWidth := min(max(contentWidth-12, 1), 48)
	for index := range m.inputs {
		m.inputs[index].SetWidth(inputWidth)
	}
}
