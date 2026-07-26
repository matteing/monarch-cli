package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/auth"
	"github.com/matteing/monarch-cli/internal/session"
	"github.com/matteing/monarch-cli/internal/tui"
)

type loginResult struct {
	Profile       string `json:"profile"`
	Mode          string `json:"mode"`
	Authenticated bool   `json:"authenticated"`
	PasswordSaved bool   `json:"password_saved"`
	MFACodeSaved  bool   `json:"mfa_code_saved"`
}

func (a *application) loginCommand() *cobra.Command {
	var method string
	var force bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Sign in and save a session credential",
		Long: "Sign in to Monarch and save the returned session in the OS keyring.\n\n" +
			"Your password and MFA code are not stored anywhere. They are discarded after login; password login keeps only the session token.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loginMethod := auth.Method(method)
			if !loginMethod.Valid() {
				return apperr.New(apperr.KindInvalidInput, "login", "pick password or browser-session for --method", nil)
			}
			if !force {
				existing, loadErr := a.store.Load(a.config.Profile)
				if loadErr == nil {
					verifyErr := a.verify(cmd.Context(), existing)
					if verifyErr == nil {
						return apperr.New(apperr.KindInvalidInput, "login", "this profile already has a valid session; pass --force to replace it", nil)
					}
					if apperr.KindOf(verifyErr) != apperr.KindAuth {
						return verifyErr
					}
				} else if errors.Is(loadErr, session.ErrInvalidSession) {
					return apperr.New(apperr.KindAuth, "login", "the saved session is invalid; pass --force to replace it", loadErr)
				} else if !errors.Is(loadErr, session.ErrNotFound) {
					return loadErr
				}
			}

			value, err := a.login(cmd.Context(), loginMethod)
			if err != nil {
				return err
			}
			if a.config.Output == "json" {
				return writeJSON(a.out, loginResult{
					Profile: a.config.Profile, Mode: string(value.Mode), Authenticated: true,
					PasswordSaved: false, MFACodeSaved: false,
				})
			}
			message := loginSuccessMessage(value.Mode)
			if tui.IsTerminal(a.out) {
				return tui.WriteSuccess(a.out, message)
			}
			_, err = fmt.Fprintln(a.out, message)
			return err
		},
	}
	command.Flags().StringVar(&method, "method", string(auth.MethodPassword), "login method: password or browser-session")
	command.Flags().BoolVar(&force, "force", false, "replace any existing saved session")
	return command
}

func loginSuccessMessage(mode session.Mode) string {
	if mode == session.ModeCookie {
		return "Signed in. Only the required authentication cookies were retained in the OS keyring."
	}
	return "Signed in. Only the session token was saved to the OS keyring."
}

func (a *application) login(ctx context.Context, method auth.Method) (session.Session, error) {
	if !tui.IsTerminal(a.in) || !tui.IsTerminal(a.errOut) {
		return session.Session{}, apperr.New(apperr.KindInvalidInput, "login", "interactive login requires a terminal", nil)
	}
	authenticate := func(ctx context.Context, email, password, code string) (session.Session, error) {
		return a.authenticate(ctx, a.config.Timeout, email, password, code)
	}
	return tui.RunLogin(tui.LoginOptions{
		Context: ctx, Input: a.in, Output: a.errOut, Method: method, Profile: a.config.Profile,
		Authenticate: authenticate,
		ParseCookies: session.ParseCookieHeader,
		Verify:       a.verify,
		Save:         a.store.Save,
	})
}
