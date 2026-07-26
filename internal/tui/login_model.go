package tui

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteing/monarch-cli/internal/auth"
	"github.com/matteing/monarch-cli/internal/session"
)

type loginStage int

const (
	stageCredentials loginStage = iota
	stageMFA
	stageWorking
	stageSaving
	stageSaveFailed
)

type loginResultMsg struct {
	value session.Session
	err   error
}

type verifyResultMsg struct {
	value session.Session
	err   error
}

type saveResultMsg struct {
	value session.Session
	err   error
}

type contextDoneMsg struct{ err error }

type loginModel struct {
	opts      LoginOptions
	inputs    []textinput.Model
	focused   int
	stage     loginStage
	spinner   spinner.Model
	status    string
	result    session.Session
	err       error
	canceled  bool
	cancelErr error
	width     int
	height    int
	cancel    context.CancelFunc

	// These values live only in the running model between the password and MFA
	// requests. clearSecrets removes them before the program returns.
	pendingEmail    string
	pendingPassword string
	pendingSession  session.Session
}

func newLoginModel(opts LoginOptions) loginModel {
	inputs := newCredentialInputs()
	inputs[0].Focus()

	indicator := spinner.New(spinner.WithSpinner(spinner.Dot))
	indicator.Style = lipgloss.NewStyle().Foreground(accentColor)
	return loginModel{
		opts: opts, inputs: inputs, spinner: indicator,
		width: 76, height: 24, cancel: func() {},
	}
}

func newCredentialInputs() []textinput.Model {
	email := newInput("Email", "you@example.com", false)
	email.CharLimit = auth.MaxEmailCharacters
	password := newInput("Password", "", true)
	password.CharLimit = auth.MaxPasswordCharacters
	return []textinput.Model{email, password}
}

func newMFAInput() textinput.Model {
	input := newInput("MFA code", "123456", true)
	input.CharLimit = auth.MaxTOTPCharacters
	return input
}

func newInput(prompt, placeholder string, secret bool) textinput.Model {
	input := textinput.New()
	input.Prompt = prompt + "  "
	input.Placeholder = placeholder
	input.SetWidth(48)
	if secret {
		input.EchoMode = textinput.EchoPassword
		input.EchoCharacter = '•'
	}
	return input
}

func (m loginModel) Init() tea.Cmd {
	return tea.Batch(m.inputs[0].Focus(), waitForContext(m.opts.Context))
}

func waitForContext(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return contextDoneMsg{err: ctx.Err()}
	}
}
