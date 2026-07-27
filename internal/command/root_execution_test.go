package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/buildinfo"
	"github.com/matteing/monarch-cli/internal/config"
	"github.com/matteing/monarch-cli/internal/monarch"
	"github.com/matteing/monarch-cli/internal/session"
)

type commandStore struct {
	value       session.Session
	loadErr     error
	loadCalls   int
	saveCalls   int
	deleteCalls int
}

func (s *commandStore) Load(string) (session.Session, error) {
	s.loadCalls++
	return s.value, s.loadErr
}

func (s *commandStore) Save(_ string, value session.Session) error {
	s.saveCalls++
	s.value = value
	return nil
}

func (s *commandStore) Delete(string) error {
	s.deleteCalls++
	return nil
}

type commandReader struct {
	accountParams   monarch.ListAccountsParams
	accountsCalls   int
	budgetReport    monarch.BudgetReport
	budgetsCalls    int
	transactionsErr error
}

func (r *commandReader) ListAccounts(_ context.Context, params monarch.ListAccountsParams) (monarch.AccountsResult, error) {
	r.accountParams = params
	r.accountsCalls++
	return monarch.AccountsResult{Accounts: []monarch.Account{}}, nil
}

func (*commandReader) RefreshAccounts(_ context.Context, params monarch.RefreshAccountsParams) (monarch.AccountRefreshResult, error) {
	return monarch.AccountRefreshResult{Accepted: true, AccountIDs: params.AccountIDs}, nil
}

func (r *commandReader) ListTransactions(context.Context, monarch.ListTransactionsParams) (monarch.TransactionPage, error) {
	return monarch.TransactionPage{Transactions: []monarch.Transaction{}}, r.transactionsErr
}

func (*commandReader) GetTransaction(context.Context, string) (monarch.TransactionResult, error) {
	return monarch.TransactionResult{}, nil
}

func (*commandReader) ListCategories(context.Context) (monarch.CategoriesResult, error) {
	return monarch.CategoriesResult{Categories: []monarch.Category{}}, nil
}

func (r *commandReader) GetBudgets(context.Context, monarch.MonthRange) (monarch.BudgetReport, error) {
	r.budgetsCalls++
	return r.budgetReport, nil
}

func (*commandReader) GetCashflow(context.Context, monarch.DateRange) (monarch.CashflowSummary, error) {
	return monarch.CashflowSummary{}, nil
}

func (*commandReader) GetFinancialOverview(context.Context, monarch.DateRange) (monarch.FinancialOverview, error) {
	return monarch.FinancialOverview{Accounts: []monarch.Account{}, Transactions: []monarch.Transaction{}}, nil
}

func testSession(t *testing.T) session.Session {
	t.Helper()
	value, err := session.NewToken("test-token")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func commandDependencies(store session.Store, service monarch.Service, input io.Reader, output, errOutput io.Writer) Dependencies {
	return Dependencies{
		Store: store, Input: input, Output: output, ErrorOutput: errOutput,
		NewService: func(session.Session, time.Duration) (monarch.Service, error) { return service, nil },
		Verify:     func(context.Context, monarch.Reader) error { return nil },
		Authenticate: func(context.Context, time.Duration, string, string, string) (session.Session, error) {
			return session.Session{}, errors.New("unexpected authentication")
		},
		RunMCP: func(context.Context, monarch.Service, string, io.Reader, io.Writer, *slog.Logger) error {
			return errors.New("unexpected MCP run")
		},
	}
}

func executeTestRoot(cfg config.Config, deps Dependencies, args ...string) (*cobra.Command, error) {
	root := NewRootWithDependencies(cfg, deps)
	root.SetArgs(args)
	return root, root.Execute()
}

func TestRestoreSignalDefaultsAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	calls := 0
	cleanup := restoreSignalDefaultsOnCancel(ctx, func() {
		calls++
		close(stopped)
	})

	// Canceling this injected context models NotifyContext receiving the first
	// signal without delivering a real signal to the test process.
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("signal notification was not unregistered after cancellation")
	}
	cleanup()
	if calls != 1 {
		t.Fatalf("notification stop calls = %d, want 1", calls)
	}
}

func TestSignalCleanupIsIdempotentAndCancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	cleanup := restoreSignalDefaultsOnCancel(ctx, func() {
		calls++
		cancel()
	})

	cleanup()
	cleanup()
	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("context error = %v, want canceled", err)
	}
	if calls != 1 {
		t.Fatalf("notification stop calls = %d, want 1", calls)
	}
}

func TestHelpAndVersionBypassDeferredConfigurationErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"--version"}, {"auth", "--help"}, {"completion", "bash"},
		{"__complete", "accounts", ""}, {"__completeNoDesc", "accounts", ""},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var output bytes.Buffer
			deps := commandDependencies(&commandStore{}, &commandReader{}, strings.NewReader(""), &output, &output)
			deps.ConfigIssues = []config.Issue{{Field: config.FieldTimeout, Kind: config.IssueInvalidInput, Err: errors.New("invalid timeout")}}
			_, err := executeTestRoot(config.Default(), deps, args...)
			if err != nil {
				t.Fatalf("execute %v: %v", args, err)
			}
			if output.Len() == 0 {
				t.Fatalf("execute %v produced no help/version output", args)
			}
		})
	}
}

func TestResolvedVersionPrefersReleaseInjection(t *testing.T) {
	original := buildinfo.Version
	buildinfo.Version = "v9.8.7"
	t.Cleanup(func() { buildinfo.Version = original })
	root := NewRootWithDependencies(config.Default(), commandDependencies(&commandStore{}, &commandReader{}, strings.NewReader(""), io.Discard, io.Discard))
	if got := root.Version; got != "v9.8.7" {
		t.Fatalf("version = %q", got)
	}
}

func TestValidFlagsOverrideBadLowerPrecedenceConfiguration(t *testing.T) {
	var output bytes.Buffer
	store := &commandStore{}
	cfg := config.Default()
	cfg.Output = "broken"
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), &output, &output)
	deps.ConfigIssues = []config.Issue{{Field: config.FieldTimeout, Kind: config.IssueInvalidInput, Err: errors.New("invalid timeout")}}
	_, err := executeTestRoot(cfg, deps, "--output", "json", "--timeout", "10s", "auth", "logout")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"profile\": \"default\",\n  \"deleted\": true\n}\n"
	if output.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", output.String(), want)
	}
}

func TestConfigurationSourceFailuresAreUnavailable(t *testing.T) {
	store := &commandStore{}
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), io.Discard, io.Discard)
	deps.ConfigIssues = []config.Issue{{Kind: config.IssueUnavailable, Err: errors.New("config path unavailable")}}
	_, err := executeTestRoot(config.Default(), deps, "accounts", "list")
	if apperr.KindOf(err) != apperr.KindUnavailable || ExitCode(err) != 5 {
		t.Fatalf("error = %v, kind = %q, exit = %d", err, apperr.KindOf(err), ExitCode(err))
	}
	if store.loadCalls != 0 {
		t.Fatalf("credential store read %d time(s)", store.loadCalls)
	}
}

func TestUnknownCommandsAreInvalidInput(t *testing.T) {
	for _, args := range [][]string{{"typo"}, {"auth", "typo"}, {"transactions", "typo"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			deps := commandDependencies(&commandStore{}, &commandReader{}, strings.NewReader(""), io.Discard, io.Discard)
			_, err := executeTestRoot(config.Default(), deps, args...)
			if apperr.KindOf(err) != apperr.KindInvalidInput || ExitCode(err) != 2 {
				t.Fatalf("error = %v, kind = %q, exit = %d", err, apperr.KindOf(err), ExitCode(err))
			}
		})
	}
}

func TestInvalidProfileFailsBeforeCredentialAccess(t *testing.T) {
	store := &commandStore{loadErr: errors.New("keyring must not be called")}
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), io.Discard, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "--profile", "invalid profile", "accounts", "list")
	if apperr.KindOf(err) != apperr.KindInvalidInput || store.loadCalls != 0 {
		t.Fatalf("error = %v, kind = %q, store loads = %d", err, apperr.KindOf(err), store.loadCalls)
	}
}

func TestLoginExplainsHowToReplaceAnInvalidSavedSession(t *testing.T) {
	loadErr := apperr.New(apperr.KindAuth, "load session", "the saved session is malformed; log in again", session.ErrInvalidSession)
	store := &commandStore{loadErr: loadErr}
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), io.Discard, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "--output", "json", "auth", "login")
	if apperr.KindOf(err) != apperr.KindAuth || ExitCode(err) != 3 || !strings.Contains(apperr.PublicMessage(err), "--force") {
		t.Fatalf("error = %v, kind = %q, exit = %d", err, apperr.KindOf(err), ExitCode(err))
	}
	if store.saveCalls != 0 {
		t.Fatalf("save called %d time(s)", store.saveCalls)
	}
}

func TestDoctorWritesPartialJSONAndReturnsHealthExit(t *testing.T) {
	var output bytes.Buffer
	missing := apperr.New(apperr.KindAuth, "load session", "no saved session", session.ErrNotFound)
	store := &commandStore{loadErr: missing}
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), &output, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "--output", "json", "doctor")
	if ExitCode(err) != 3 {
		t.Fatalf("error = %v, exit = %d", err, ExitCode(err))
	}
	for _, phrase := range []string{`"healthy": false`, `"name": "keyring"`, `"status": "ok"`, `"name": "session"`, `"name": "api"`, `"status": "skipped"`} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("doctor output missing %q:\n%s", phrase, output.String())
		}
	}
	var result doctorResult
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if result.Error == nil || result.Error.Kind != apperr.KindAuth || result.Error.Message == "" {
		t.Fatalf("doctor top-level error = %+v", result.Error)
	}
}

func TestDoctorDistinguishesValidSessionFromUnavailableAPI(t *testing.T) {
	var output bytes.Buffer
	store := &commandStore{value: testSession(t)}
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), &output, io.Discard)
	deps.Verify = func(context.Context, monarch.Reader) error {
		return apperr.New(apperr.KindUnavailable, "list accounts", "Monarch is unavailable", errors.New("network detail"))
	}
	_, err := executeTestRoot(config.Default(), deps, "--output", "json", "doctor")
	if ExitCode(err) != 5 {
		t.Fatalf("error = %v, exit = %d", err, ExitCode(err))
	}
	for _, phrase := range []string{`"name": "keyring"`, `"name": "session"`, `"detail": "saved session is structurally valid"`, `"name": "api"`, `"kind": "unavailable"`} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("doctor output missing %q:\n%s", phrase, output.String())
		}
	}
}

func TestMCPCommandUsesInjectedRunnerAndStreams(t *testing.T) {
	var output bytes.Buffer
	input := strings.NewReader("protocol input")
	store := &commandStore{value: testSession(t)}
	reader := &commandReader{}
	deps := commandDependencies(store, reader, input, &output, io.Discard)
	called := false
	deps.RunMCP = func(_ context.Context, got monarch.Service, version string, gotInput io.Reader, gotOutput io.Writer, logger *slog.Logger) error {
		called = true
		if got != reader || gotInput != input || gotOutput != &output || version == "" || logger == nil {
			t.Fatalf("unexpected MCP dependencies: reader=%T version=%q input=%T output=%T logger=%v", got, version, gotInput, gotOutput, logger)
		}
		return nil
	}
	_, err := executeTestRoot(config.Default(), deps, "mcp")
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("MCP runner was not called")
	}
}

func TestJSONErrorContractIncludesSafeMetadata(t *testing.T) {
	err := &apperr.Error{
		Kind: apperr.KindRateLimited, Op: "list transactions", Message: "rate limited",
		StatusCode: 429, Retryable: true, RetryAfter: 1500 * time.Millisecond,
	}
	var output bytes.Buffer
	if writeErr := writeCommandError(&output, "json", err); writeErr != nil {
		t.Fatal(writeErr)
	}
	want := "{\"error\":{\"kind\":\"rate_limited\",\"message\":\"rate limited\",\"operation\":\"list transactions\",\"status_code\":429,\"retryable\":true,\"retry_after_ms\":1500}}\n"
	if output.String() != want {
		t.Fatalf("output = %q\nwant   = %q", output.String(), want)
	}

	output.Reset()
	if writeErr := writeCommandError(&output, "json", errors.New("secret upstream body")); writeErr != nil {
		t.Fatal(writeErr)
	}
	if strings.Contains(output.String(), "secret upstream body") || !strings.Contains(output.String(), "unexpected internal error") {
		t.Fatalf("unsafe internal JSON error: %s", output.String())
	}

	output.Reset()
	unsafe := apperr.New(apperr.KindInvalidInput, "validate", "unsafe\u0085\u202e\u0301text", nil)
	if writeErr := writeCommandError(&output, "json", unsafe); writeErr != nil {
		t.Fatal(writeErr)
	}
	for _, char := range []rune{'\u0085', '\u202e', '\u0301'} {
		if strings.ContainsRune(output.String(), char) {
			t.Fatalf("JSON error retained unsafe terminal character %U: %q", char, output.String())
		}
	}
	if !strings.Contains(output.String(), `unsafe\u0085\u202e\u0301text`) {
		t.Fatalf("JSON error did not preserve escaped message: %q", output.String())
	}
}

func TestDeadlineUsesUnavailableExitAndJSONContracts(t *testing.T) {
	if got := ExitCode(context.DeadlineExceeded); got != 5 {
		t.Fatalf("deadline exit = %d, want 5", got)
	}
	var output bytes.Buffer
	if err := writeCommandError(&output, "json", context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{`"kind":"unavailable"`, `"message":"operation timed out"`, `"retryable":true`} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("deadline JSON missing %q: %s", phrase, output.String())
		}
	}
}

func TestCancellationUsesCanceledExitAndJSONContracts(t *testing.T) {
	if got := ExitCode(context.Canceled); got != 130 {
		t.Fatalf("canceled exit = %d, want 130", got)
	}
	var output bytes.Buffer
	if err := writeCommandError(&output, "json", context.Canceled); err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{`"kind":"canceled"`, `"message":"operation canceled"`} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("canceled JSON missing %q: %s", phrase, output.String())
		}
	}
}

func TestPlainErrorContractSanitizesAndHidesInternalCauses(t *testing.T) {
	var output bytes.Buffer
	typed := apperr.New(apperr.KindInvalidInput, "validate", "unsafe\x1b[2J\u202etext", nil)
	if err := writeCommandError(&output, "table", typed); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "unsafe [2J text\n"; got != want {
		t.Fatalf("typed error output = %q, want %q", got, want)
	}

	output.Reset()
	if err := writeCommandError(&output, "table", errors.New("secret upstream body")); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "unexpected internal error\n"; got != want {
		t.Fatalf("internal error output = %q, want %q", got, want)
	}
}

func TestDoctorReportsMalformedSessionWithoutBlamingKeyring(t *testing.T) {
	var output bytes.Buffer
	cause := apperr.New(apperr.KindAuth, "load session", "the saved session is malformed; log in again", session.ErrInvalidSession)
	store := &commandStore{loadErr: cause}
	deps := commandDependencies(store, &commandReader{}, strings.NewReader(""), &output, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "--output", "json", "doctor")
	if ExitCode(err) != 3 {
		t.Fatalf("error = %v, exit = %d", err, ExitCode(err))
	}
	for _, phrase := range []string{
		`"name": "keyring"`, `"status": "ok"`, `"name": "session"`,
		`"status": "error"`, `"name": "api"`, `"detail": "saved session is invalid"`,
	} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("doctor output missing %q:\n%s", phrase, output.String())
		}
	}
}

func TestExecuteRootWritesJSONErrorAndPreservesExitCode(t *testing.T) {
	var errOutput bytes.Buffer
	store := &commandStore{}
	reader := &commandReader{transactionsErr: apperr.New(apperr.KindInvalidInput, "list transactions", "limit must be between 1 and 100", nil)}
	deps := commandDependencies(store, reader, strings.NewReader(""), io.Discard, &errOutput)
	root, app := buildRoot(config.Default(), deps)
	root.SetArgs([]string{"--output", "json", "transactions", "list", "--limit", "0"})
	err := executeRoot(context.Background(), root, app)
	if ExitCode(err) != 2 || store.loadCalls != 1 {
		t.Fatalf("error = %v, exit = %d, store loads = %d", err, ExitCode(err), store.loadCalls)
	}
	want := "{\"error\":{\"kind\":\"invalid_input\",\"message\":\"limit must be between 1 and 100\",\"operation\":\"list transactions\",\"retryable\":false}}\n"
	if errOutput.String() != want {
		t.Fatalf("stderr = %q\nwant   = %q", errOutput.String(), want)
	}
}

func TestExecuteRootReportsSuccessWriterFailureAsSafeJSON(t *testing.T) {
	var errOutput bytes.Buffer
	deps := commandDependencies(&commandStore{}, &commandReader{}, strings.NewReader(""), failingWriter{errors.New("sensitive writer detail")}, &errOutput)
	root, app := buildRoot(config.Default(), deps)
	root.SetArgs([]string{"--output", "json", "auth", "logout"})
	err := executeRoot(context.Background(), root, app)
	if err == nil || ExitCode(err) != 1 {
		t.Fatalf("error = %v, exit = %d", err, ExitCode(err))
	}
	if strings.Contains(errOutput.String(), "sensitive") || !strings.Contains(errOutput.String(), "unexpected internal error") {
		t.Fatalf("unsafe stderr: %s", errOutput.String())
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestLogoutPropagatesSuccessWriterFailure(t *testing.T) {
	want := errors.New("write failed")
	deps := commandDependencies(&commandStore{}, &commandReader{}, strings.NewReader(""), failingWriter{want}, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "auth", "logout")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestAccountsCommandDelegatesToInjectedReader(t *testing.T) {
	reader := &commandReader{}
	store := &commandStore{value: testSession(t)}
	deps := commandDependencies(store, reader, strings.NewReader(""), io.Discard, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "accounts", "list", "--include-hidden", "--include-deactivated")
	if err != nil {
		t.Fatal(err)
	}
	if reader.accountsCalls != 1 || !reader.accountParams.IncludeHidden || !reader.accountParams.IncludeDeactivated {
		t.Fatalf("account delegation: calls=%d params=%+v", reader.accountsCalls, reader.accountParams)
	}
}

func TestBudgetsTableIncludesCategoryAndGroupRows(t *testing.T) {
	var output bytes.Buffer
	reader := &commandReader{budgetReport: monarch.BudgetReport{
		Categories: []monarch.CategoryBudget{{
			Category: monarch.Named{Name: "Groceries"},
			MonthlyAmounts: []monarch.BudgetAmount{{
				Month: "2026-07", PlannedCashFlowAmount: "500", ActualAmount: "425", RemainingAmount: "75",
			}},
		}},
		Groups: []monarch.GroupBudget{{
			CategoryGroup: monarch.CategoryGroup{Name: "Living"},
			MonthlyAmounts: []monarch.BudgetAmount{{
				Month: "2026-07", PlannedCashFlowAmount: "1000", ActualAmount: "850", RemainingAmount: "150",
			}},
		}},
	}}
	deps := commandDependencies(&commandStore{value: testSession(t)}, reader, strings.NewReader(""), &output, io.Discard)
	_, err := executeTestRoot(config.Default(), deps, "budgets", "get")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"SCOPE", "category", "Groceries", "group", "Living"} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("budget output missing %q:\n%s", phrase, output.String())
		}
	}
	if reader.budgetsCalls != 1 {
		t.Fatalf("budget calls = %d", reader.budgetsCalls)
	}
}
