package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/session"
)

func TestLoginCancellationUsesStableApplicationKind(t *testing.T) {
	if !errors.Is(ErrCanceled, context.Canceled) || apperr.KindOf(ErrCanceled) != apperr.KindCanceled {
		t.Fatalf("ErrCanceled = %v, kind = %q", ErrCanceled, apperr.KindOf(ErrCanceled))
	}
}

func TestRunLoginPreservesEscapeParentCancelAndDeadline(t *testing.T) {
	start := func(t *testing.T, ctx context.Context) (*io.PipeWriter, <-chan error) {
		t.Helper()
		value := testToken(t, "token")
		input, keyboard := io.Pipe()
		t.Cleanup(func() {
			input.Close()
			keyboard.Close()
		})
		done := make(chan error, 1)
		go func() {
			_, err := RunLogin(LoginOptions{
				Context: ctx, Input: input, Output: io.Discard,
				Profile: "default",
				Authenticate: func(context.Context, string, string, string) (session.Session, error) {
					return value, nil
				},
				Verify: func(context.Context, session.Session) error { return nil },
				Save:   func(string, session.Session) error { return nil },
			})
			done <- err
		}()
		return keyboard, done
	}
	wait := func(t *testing.T, done <-chan error) error {
		t.Helper()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("RunLogin did not stop")
			return nil
		}
	}

	t.Run("escape", func(t *testing.T) {
		keyboard, done := start(t, context.Background())
		if _, err := io.WriteString(keyboard, "\x1b"); err != nil {
			t.Fatal(err)
		}
		if err := wait(t, done); !errors.Is(err, ErrCanceled) {
			t.Fatalf("escape error = %v", err)
		}
	})

	t.Run("parent cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		_, done := start(t, ctx)
		cancel()
		if err := wait(t, done); !errors.Is(err, context.Canceled) {
			t.Fatalf("parent-cancel error = %v", err)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		_, done := start(t, ctx)
		if err := wait(t, done); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline error = %v", err)
		}
	})
}

func TestPasswordLoginExplainsSecretStorage(t *testing.T) {
	for _, phrase := range []string{"aren't stored anywhere", "discarded after login", "only keep the session token"} {
		if !strings.Contains(PasswordPrivacyNotice, phrase) {
			t.Fatalf("privacy notice does not contain %q: %s", phrase, PasswordPrivacyNotice)
		}
	}

	model := newLoginModel(LoginOptions{Profile: "default"})
	if model.inputs[1].EchoMode != textinput.EchoPassword {
		t.Fatal("password input is not masked")
	}
	rendered := strings.Join(strings.Fields(model.View().Content), " ")
	for _, phrase := range []string{"aren't stored anywhere", "session token"} {
		if !strings.Contains(rendered, phrase) {
			t.Fatalf("password privacy phrase %q is not visible in the login form", phrase)
		}
	}
	if strings.Contains(rendered, "Monarch CLI") {
		t.Fatal("login form unexpectedly contains a branded page heading")
	}
}

func TestLoginResizesInputsControlsAndTinyView(t *testing.T) {
	model := newLoginModel(LoginOptions{Profile: "default"})
	next, _ := model.Update(tea.WindowSizeMsg{Width: 34, Height: 12})
	model = next.(loginModel)
	if model.inputs[0].Width() != 20 || model.inputs[1].Width() != 20 {
		t.Fatalf("input widths = %d and %d, want 20", model.inputs[0].Width(), model.inputs[1].Width())
	}
	if !strings.Contains(model.View().Content, "enter  ·  tab  ·  esc") {
		t.Fatal("narrow login view does not use compact controls")
	}
	assertViewFits(t, model.View(), 34, 12)

	next, _ = model.Update(tea.WindowSizeMsg{Width: 12, Height: 5})
	model = next.(loginModel)
	assertViewFits(t, model.View(), 12, 5)
}

func TestPasswordLoginMFAKeyboardFlow(t *testing.T) {
	var receivedEmail, receivedPassword, receivedCode string
	var verified, saved bool
	opts := LoginOptions{
		Context: context.Background(), Profile: "default",
		Authenticate: func(_ context.Context, email, password, code string) (session.Session, error) {
			receivedEmail, receivedPassword, receivedCode = email, password, code
			if code == "" {
				return session.Session{}, apperr.New(apperr.KindMFARequired, "login", "MFA required", nil)
			}
			return testToken(t, "token"), nil
		},
		Verify: func(_ context.Context, value session.Session) error {
			verified = value.Token() == "token"
			return nil
		},
		Save: func(profile string, value session.Session) error {
			saved = profile == "default" && value.Token() == "token"
			return nil
		},
	}
	model := newLoginModel(opts)
	update := func(msg tea.Msg) tea.Cmd {
		t.Helper()
		next, command := model.Update(msg)
		model = next.(loginModel)
		return command
	}
	typeText := func(value string) {
		t.Helper()
		for _, char := range value {
			update(tea.KeyPressMsg{Code: char, Text: string(char)})
		}
	}

	typeText("person@example.com")
	update(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeText("secret")
	command := update(tea.KeyPressMsg{Code: tea.KeyEnter})
	result := loginCommandResult(t, command)
	update(result)
	if model.stage != stageMFA || len(model.inputs) != 1 {
		t.Fatalf("MFA form state = stage %d, inputs %d", model.stage, len(model.inputs))
	}

	typeText("123456")
	result = loginCommandResult(t, update(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if result.err != nil {
		t.Fatalf("MFA authentication failed: %v", result.err)
	}
	if receivedEmail != "person@example.com" || receivedPassword != "secret" || receivedCode != "123456" {
		t.Fatalf("unexpected MFA request values: %q %q %q", receivedEmail, receivedPassword, receivedCode)
	}

	verify := update(result)().(verifyResultMsg)
	if model.pendingEmail != "" || model.pendingPassword != "" || model.inputs[0].Value() != "" {
		t.Fatal("login secrets remained in the model after authentication")
	}
	saveCommand := update(verify)
	if model.stage != stageSaving {
		t.Fatalf("stage = %d, want saving", model.stage)
	}
	save := saveCommand().(saveResultMsg)
	update(save)
	if save.err != nil || !verified || !saved || model.result.Token() != "token" {
		t.Fatalf("login was not verified and saved: save=%v verified=%t saved=%t token=%q", save.err, verified, saved, model.result.Token())
	}
}

func TestRepeatedMFARequiredStaysOnMFAForm(t *testing.T) {
	model := newLoginModel(LoginOptions{Profile: "default"})
	model.pendingEmail = "person@example.com"
	model.pendingPassword = "secret"
	mfaErr := apperr.New(apperr.KindMFARequired, "login", "MFA code was rejected", nil)

	next, _ := model.Update(loginResultMsg{err: mfaErr})
	model = next.(loginModel)
	next, _ = model.Update(loginResultMsg{err: mfaErr})
	model = next.(loginModel)
	if model.stage != stageMFA || len(model.inputs) != 1 || model.pendingPassword != "secret" {
		t.Fatalf("repeated MFA state = stage %d inputs %d pending=%q", model.stage, len(model.inputs), model.pendingPassword)
	}
	if !strings.Contains(model.View().Content, "rejected") {
		t.Fatal("repeated MFA error is not visible inline")
	}
}

func TestCancelDuringVerifyCancelsWorkAndNeverSaves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var saves atomic.Int32
	model := newLoginModel(LoginOptions{
		Context: ctx, Profile: "default",
		Verify: func(ctx context.Context, _ session.Session) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
		Save: func(string, session.Session) error {
			saves.Add(1)
			return nil
		},
	})
	model.cancel = cancel
	model.stage = stageWorking
	value := testToken(t, "token")
	result := make(chan tea.Msg, 1)
	go func() { result <- model.verify(value)() }()
	<-started

	next, quit := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = next.(loginModel)
	if quit == nil || !model.canceled {
		t.Fatal("escape did not cancel the in-flight login")
	}
	message := (<-result).(verifyResultMsg)
	if !errors.Is(message.err, context.Canceled) || saves.Load() != 0 {
		t.Fatalf("verify result = %v, saves = %d", message.err, saves.Load())
	}
}

func TestCanceledContextBeforeSaveCommandNeverCallsSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var saves atomic.Int32
	model := newLoginModel(LoginOptions{
		Context: ctx, Profile: "default",
		Save: func(string, session.Session) error {
			saves.Add(1)
			return nil
		},
	})
	value := testToken(t, "token")
	_, command := model.Update(verifyResultMsg{value: value})
	cancel()
	result := command().(saveResultMsg)
	if !errors.Is(result.err, context.Canceled) || saves.Load() != 0 {
		t.Fatalf("save result = %v, saves = %d", result.err, saves.Load())
	}
}

func TestRunLoginWaitsForStartedCredentialVaultCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input, keyboard := io.Pipe()
	defer input.Close()
	defer keyboard.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	type outcome struct {
		value session.Session
		err   error
	}
	done := make(chan outcome, 1)
	value := testToken(t, "token")
	go func() {
		result, err := RunLogin(LoginOptions{
			Context: ctx, Input: input, Output: io.Discard,
			Profile: "default",
			Authenticate: func(context.Context, string, string, string) (session.Session, error) {
				return value, nil
			},
			Verify: func(context.Context, session.Session) error { return nil },
			Save: func(string, session.Session) error {
				close(started)
				<-release
				return nil
			},
		})
		done <- outcome{value: result, err: err}
	}()

	if _, err := io.WriteString(keyboard, "person@example.com\rsecret\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("credential-vault commit did not start")
	}
	cancel()
	select {
	case result := <-done:
		t.Fatalf("RunLogin returned before the commit finished: value=%v err=%v", result.value.Mode, result.err)
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	select {
	case result := <-done:
		if result.err != nil || result.value.Mode != session.ModeToken {
			t.Fatalf("RunLogin result = mode %q, error %v", result.value.Mode, result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLogin did not return after the commit finished")
	}
}

func TestRunLoginReturnsCancellationAfterStartedCommitFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input, keyboard := io.Pipe()
	defer input.Close()
	defer keyboard.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	value := testToken(t, "token")
	go func() {
		_, err := RunLogin(LoginOptions{
			Context: ctx, Input: input, Output: io.Discard,
			Profile: "default",
			Authenticate: func(context.Context, string, string, string) (session.Session, error) {
				return value, nil
			},
			Verify: func(context.Context, session.Session) error { return nil },
			Save: func(string, session.Session) error {
				close(started)
				<-release
				return errors.New("keyring failed")
			},
		})
		done <- err
	}()
	if _, err := io.WriteString(keyboard, "person@example.com\rsecret\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("credential-vault commit did not start")
	}
	cancel()
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunLogin error = %v, want cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLogin hung after a canceled credential-vault failure")
	}
}

func TestLoginShowsRecoverableAndSanitizedInlineErrors(t *testing.T) {
	model := newLoginModel(LoginOptions{Profile: "default"})
	model.focused = 1
	next, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(loginModel)
	if command == nil || model.stage != stageCredentials || !strings.Contains(model.View().Content, "email address") {
		t.Fatal("blank credentials did not produce recoverable inline feedback")
	}

	model.err = errors.New("\x1b[2Junsafe")
	if strings.Contains(model.View().Content, "\x1b[2J") {
		t.Fatal("terminal control sequence survived inline error rendering")
	}
}

func TestLoginProjectsVisibleInputWithoutChangingCredentials(t *testing.T) {
	model := newLoginModel(LoginOptions{Profile: "default"})
	raw := "\u0301person\u202e@example.com"
	model.inputs[0].SetValue(raw)
	model.inputs[0].Blur()

	rendered := model.View().Content
	if model.inputs[0].Value() != raw {
		t.Fatalf("email = %q, want original %q", model.inputs[0].Value(), raw)
	}
	if strings.Contains(rendered, "\u202e") {
		t.Fatalf("visible input retained unsafe terminal text: %q", rendered)
	}
	if !strings.Contains(rendered, "◌") {
		t.Fatalf("visible input did not anchor its leading combining mark: %q", rendered)
	}

	limited := newInput("Email", "", false)
	limited.SetWidth(0)
	limited.CharLimit = 2
	limited.SetValue("\u0301a")
	if rendered := terminalSafeInputView(limited); !strings.Contains(rendered, "a") {
		t.Fatalf("display projection was truncated by the credential limit: %q", rendered)
	}

	narrow := textinput.New()
	narrow.Prompt = ""
	narrow.SetWidth(3)
	narrow.SetValue("a\u202ebc")
	narrow.SetCursor(2)
	if rendered := terminalSafeInputView(narrow); strings.Contains(rendered, "a") || lipgloss.Width(rendered) > narrow.Width()+1 {
		t.Fatalf("projected viewport retained stale offsets: width = %d, field = %d: %q", lipgloss.Width(rendered), narrow.Width(), rendered)
	}
}

func TestValidateLoginOptions(t *testing.T) {
	valid := LoginOptions{
		Context: context.Background(), Input: strings.NewReader(""), Output: &strings.Builder{},
		Profile: "default",
		Authenticate: func(context.Context, string, string, string) (session.Session, error) {
			return testToken(t, "token"), nil
		},
		Verify: func(context.Context, session.Session) error { return nil },
		Save:   func(string, session.Session) error { return nil },
	}
	if err := validateLoginOptions(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*LoginOptions){
		"context": func(opts *LoginOptions) { opts.Context = nil },
		"io":      func(opts *LoginOptions) { opts.Output = nil },
		"profile": func(opts *LoginOptions) { opts.Profile = "../bad" },
		"auth":    func(opts *LoginOptions) { opts.Authenticate = nil },
		"verify":  func(opts *LoginOptions) { opts.Verify = nil },
		"save":    func(opts *LoginOptions) { opts.Save = nil },
	} {
		t.Run(name, func(t *testing.T) {
			opts := valid
			mutate(&opts)
			if err := validateLoginOptions(opts); err == nil {
				t.Fatal("invalid options were accepted")
			}
		})
	}
}

func loginCommandResult(t *testing.T, command tea.Cmd) loginResultMsg {
	t.Helper()
	if command == nil {
		t.Fatal("login did not return a command")
	}
	message := command()
	if result, ok := message.(loginResultMsg); ok {
		return result
	}
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("login command returned %T, want login result batch", message)
	}
	for index := len(batch) - 1; index >= 0; index-- {
		if result, ok := batch[index]().(loginResultMsg); ok {
			return result
		}
	}
	t.Fatal("login command batch did not contain a login result")
	return loginResultMsg{}
}

func testToken(t *testing.T, token string) session.Session {
	t.Helper()
	value, err := session.NewToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
