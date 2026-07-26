// Package command defines the monarch command-line interface.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/buildinfo"
	"github.com/matteing/monarch-cli/internal/config"
	"github.com/matteing/monarch-cli/internal/logging"
	"github.com/matteing/monarch-cli/internal/profile"
	"github.com/matteing/monarch-cli/internal/textsafe"
)

// Execute loads configuration and runs the root command with signal cancellation.
func Execute() error {
	loaded := config.LoadMerged()
	deps := productionDependencies()
	deps.ConfigIssues = loaded.Issues
	root, app := buildRoot(loaded.Config, deps)
	ctx, notifyStop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	stop := restoreSignalDefaultsOnCancel(ctx, notifyStop)
	defer stop()
	return executeRoot(ctx, root, app)
}

// restoreSignalDefaultsOnCancel unregisters NotifyContext as soon as its context
// is canceled. A second interrupt then regains the operating system's default
// hard-stop behavior instead of being swallowed by a command that is still
// finishing an uncancelable local operation.
func restoreSignalDefaultsOnCancel(ctx context.Context, notifyStop context.CancelFunc) context.CancelFunc {
	var once sync.Once
	stop := func() { once.Do(notifyStop) }
	go func() {
		<-ctx.Done()
		stop()
	}()
	return stop
}

func executeRoot(ctx context.Context, root *cobra.Command, app *application) error {
	err := root.ExecuteContext(ctx)
	if err == nil {
		return nil
	}
	if app.logger != nil && app.config.Output != "json" {
		details := apperr.Describe(err)
		app.logger.Debug("command failed", "kind", details.Kind, "operation", details.Operation, "status_code", details.StatusCode, "retryable", details.Retryable, "retry_after_ms", details.RetryAfterMS)
	}
	var reported interface{ Reported() bool }
	if !errors.As(err, &reported) || !reported.Reported() {
		if writeErr := writeCommandError(app.errOut, app.config.Output, err); writeErr != nil {
			return errors.Join(err, fmt.Errorf("write command error: %w", writeErr))
		}
	}
	return err
}

// NewRootWithDependencies constructs the command tree with every external side
// effect injectable for offline command execution tests.
func NewRootWithDependencies(cfg config.Config, deps Dependencies) *cobra.Command {
	root, _ := buildRoot(cfg, withDependencyDefaults(deps))
	return root
}

func buildRoot(cfg config.Config, deps Dependencies) (*cobra.Command, *application) {
	deps = withDependencyDefaults(deps)
	app := &application{
		config: cfg, configIssues: deps.ConfigIssues,
		store: deps.Store, in: deps.Input, out: deps.Output, errOut: deps.ErrorOutput,
		newReader: deps.NewReader, authenticate: deps.Authenticate,
		verifyReader: deps.Verify, runMCP: deps.RunMCP, version: buildinfo.Current(),
	}
	root := &cobra.Command{
		Use: "monarch", Short: "Read-only Monarch Money CLI and MCP server",
		Long:    "monarch provides interactive login, read-only queries, JSON output, and a local stdio MCP server for Monarch Money.",
		Version: app.version, SilenceErrors: true, SilenceUsage: true,
		TraverseChildren: true, Args: subcommandsOnly, RunE: showHelp,
		Annotations: map[string]string{"help-only": "true"},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if configurationFree(cmd) {
				return nil
			}
			if err := app.validateConfiguration(cmd); err != nil {
				return err
			}
			logger, err := logging.New(app.errOut, app.config.LogLevel, app.config.LogFormat)
			if err != nil {
				return apperr.New(apperr.KindInvalidInput, "configure logging", err.Error(), err)
			}
			app.logger = logger
			return nil
		},
	}
	root.SetIn(app.in)
	root.SetOut(app.out)
	root.SetErr(app.errOut)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return apperr.New(apperr.KindInvalidInput, "parse flags", err.Error(), err)
	})
	root.PersistentFlags().StringVar(&app.config.Profile, "profile", cfg.Profile, "saved-session profile ("+profile.Syntax+")")
	root.PersistentFlags().StringVarP(&app.config.Output, "output", "o", cfg.Output, "output format: table or json")
	root.PersistentFlags().DurationVar(&app.config.Timeout, "timeout", cfg.Timeout, "request timeout")
	root.PersistentFlags().StringVar(&app.config.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, or error")
	root.PersistentFlags().StringVar(&app.config.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")

	root.AddCommand(app.authCommand(), app.doctorCommand(), app.accountsCommand(), app.transactionsCommand(), app.categoriesCommand(), app.budgetsCommand(), app.cashflowCommand(), app.overviewCommand(), app.mcpCommand())
	return root, app
}

func (a *application) validateConfiguration(cmd *cobra.Command) error {
	for _, issue := range a.configIssues {
		if issue.Field != "" && cmd.Flags().Changed(issue.Field) {
			continue
		}
		kind := apperr.KindInternal
		switch issue.Kind {
		case config.IssueInvalidInput:
			kind = apperr.KindInvalidInput
		case config.IssueUnavailable:
			kind = apperr.KindUnavailable
		}
		return apperr.New(kind, "load configuration", issue.Error(), issue.Err)
	}
	if err := a.config.Validate(); err != nil {
		return apperr.New(apperr.KindInvalidInput, "validate configuration", err.Error(), err)
	}
	return nil
}

func writeCommandError(output io.Writer, format string, err error) error {
	if format == "json" {
		raw, marshalErr := apperr.MarshalJSON(err)
		if marshalErr != nil {
			return marshalErr
		}
		_, writeErr := fmt.Fprintln(output, textsafe.EscapeJSON(string(raw)))
		return writeErr
	}
	_, writeErr := fmt.Fprintln(output, textsafe.Terminal(apperr.PublicMessage(err)))
	return writeErr
}

func configurationFree(cmd *cobra.Command) bool {
	if cmd.Annotations["help-only"] == "true" || cmd.Name() == "help" || strings.HasPrefix(cmd.Name(), "__complete") {
		return true
	}
	for current := cmd; current != nil; current = current.Parent() {
		if current.Name() == "completion" {
			return true
		}
	}
	return false
}

func showHelp(cmd *cobra.Command, _ []string) error { return cmd.Help() }

func commandGroup(use, short string, children ...*cobra.Command) *cobra.Command {
	command := &cobra.Command{
		Use: use, Short: short, Args: subcommandsOnly, RunE: showHelp,
		Annotations: map[string]string{"help-only": "true"},
	}
	command.AddCommand(children...)
	return command
}

func subcommandsOnly(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return apperr.New(apperr.KindInvalidInput, "find command", fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath()), nil)
}

func noArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return apperr.New(apperr.KindInvalidInput, "validate arguments", "this command does not accept positional arguments", nil)
	}
	return nil
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return apperr.New(apperr.KindInvalidInput, "validate arguments", fmt.Sprintf("expected %d positional argument(s), received %d", count, len(args)), nil)
		}
		return nil
	}
}

// ExitCode maps typed failures to stable process exit codes.
func ExitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	switch apperr.KindOf(err) {
	case apperr.KindInvalidInput, apperr.KindNotFound:
		return 2
	case apperr.KindAuth, apperr.KindMFARequired:
		return 3
	case apperr.KindRateLimited:
		return 4
	case apperr.KindUnavailable:
		return 5
	case apperr.KindKeyring:
		return 6
	case apperr.KindCanceled:
		return 130
	default:
		return 1
	}
}
