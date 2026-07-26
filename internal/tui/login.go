package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/matteing/monarch-cli/internal/auth"
	"github.com/matteing/monarch-cli/internal/session"
)

// ErrCanceled indicates that the user closed the interactive login form. It
// wraps context.Canceled so every transport maps it to the stable canceled
// error kind and exit code.
var ErrCanceled = fmt.Errorf("login canceled: %w", context.Canceled)

// PasswordPrivacyNotice is shown before password login. Keep this wording
// direct: the login secret is used for the request but never persisted.
const PasswordPrivacyNotice = "Your password and MFA code aren't stored anywhere. They're discarded after login; we only keep the session token."

// BrowserPrivacyNotice explains the distinct storage behavior of cookie import.
const BrowserPrivacyNotice = "No email or password is used. Only the required session_id and csrftoken cookies are stored."

// LoginOptions supplies the required dependencies for the interactive login
// form. Network callbacks must honor Context. Save begins only after successful
// verification; once that credential-vault commit starts, UI cancellation is
// deliberately ignored until it finishes.
type LoginOptions struct {
	Context      context.Context
	Input        io.Reader
	Output       io.Writer
	Method       auth.Method
	Profile      string
	Authenticate func(context.Context, string, string, string) (session.Session, error)
	ParseCookies func(string) (session.Session, error)
	Verify       func(context.Context, session.Session) error
	Save         func(string, session.Session) error
}

// RunLogin runs a small inline Bubble Tea form and returns the verified,
// persisted session. It does not use an alternate screen.
func RunLogin(opts LoginOptions) (session.Session, error) {
	if err := validateLoginOptions(opts); err != nil {
		return session.Session{}, err
	}
	// Credential-vault writes cannot be canceled once started. Keep Bubble Tea
	// alive long enough to observe that commit's result even if the caller's
	// context is canceled; the model watches operationContext and quits promptly
	// at every earlier stage.
	programContext := context.WithoutCancel(opts.Context)
	operationContext, cancel := context.WithCancel(opts.Context)
	defer cancel()
	opts.Context = operationContext
	model := newLoginModel(opts)
	model.cancel = cancel
	program := tea.NewProgram(
		model,
		tea.WithContext(programContext),
		tea.WithInput(opts.Input),
		tea.WithOutput(opts.Output),
		tea.WithoutSignalHandler(),
	)
	final, err := program.Run()
	if err != nil {
		return session.Session{}, err
	}
	result, ok := final.(loginModel)
	if !ok {
		return session.Session{}, errors.New("login UI returned an unexpected model")
	}
	if result.canceled {
		if result.cancelErr != nil {
			return session.Session{}, result.cancelErr
		}
		return session.Session{}, ErrCanceled
	}
	if result.err != nil {
		return session.Session{}, result.err
	}
	return result.result, nil
}

func validateLoginOptions(opts LoginOptions) error {
	if opts.Context == nil {
		return errors.New("login context is required")
	}
	if opts.Input == nil || opts.Output == nil {
		return errors.New("login input and output are required")
	}
	if err := session.ValidateProfile(opts.Profile); err != nil {
		return fmt.Errorf("invalid login profile: %w", err)
	}
	if opts.Verify == nil || opts.Save == nil {
		return errors.New("login verify and save callbacks are required")
	}
	switch opts.Method {
	case auth.MethodPassword:
		if opts.Authenticate == nil {
			return errors.New("password login requires an authenticate callback")
		}
	case auth.MethodBrowserSession:
		if opts.ParseCookies == nil {
			return errors.New("browser-session login requires a cookie parser")
		}
	default:
		return fmt.Errorf("unsupported login method %q", opts.Method)
	}
	return nil
}
