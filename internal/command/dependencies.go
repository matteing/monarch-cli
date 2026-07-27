package command

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/matteing/monarch-cli/internal/auth"
	"github.com/matteing/monarch-cli/internal/config"
	"github.com/matteing/monarch-cli/internal/mcpserver"
	"github.com/matteing/monarch-cli/internal/monarch"
	"github.com/matteing/monarch-cli/internal/session"
)

// ServiceFactory constructs a Monarch service without performing network I/O.
type ServiceFactory func(session.Session, time.Duration) (monarch.Service, error)

// AuthenticateFunc performs one password or MFA authentication attempt.
type AuthenticateFunc func(context.Context, time.Duration, string, string, string) (session.Session, error)

// VerifyFunc verifies that reader can access the authenticated Monarch account.
type VerifyFunc func(context.Context, monarch.Reader) error

// MCPRunner serves MCP over caller-provided streams.
type MCPRunner func(context.Context, monarch.Service, string, io.Reader, io.Writer, *slog.Logger) error

// Dependencies contains command side effects that tests and embedders may replace.
type Dependencies struct {
	Store        session.Store
	Input        io.Reader
	Output       io.Writer
	ErrorOutput  io.Writer
	NewService   ServiceFactory
	Authenticate AuthenticateFunc
	Verify       VerifyFunc
	RunMCP       MCPRunner
	ConfigIssues []config.Issue
}

type application struct {
	config       config.Config
	configIssues []config.Issue
	store        session.Store
	in           io.Reader
	out          io.Writer
	errOut       io.Writer
	newService   ServiceFactory
	authenticate AuthenticateFunc
	verifyReader VerifyFunc
	runMCP       MCPRunner
	logger       *slog.Logger
	version      string
}

func productionDependencies() Dependencies {
	return Dependencies{
		Store: session.KeyringStore{}, Input: os.Stdin, Output: os.Stdout, ErrorOutput: os.Stderr,
		NewService: func(value session.Session, timeout time.Duration) (monarch.Service, error) {
			return monarch.NewClient(value, timeout)
		},
		Authenticate: func(ctx context.Context, timeout time.Duration, email, password, code string) (session.Session, error) {
			return auth.New(timeout).Login(ctx, email, password, code)
		},
		Verify: func(ctx context.Context, reader monarch.Reader) error {
			_, err := reader.ListAccounts(ctx, monarch.ListAccountsParams{})
			return err
		},
		RunMCP: mcpserver.RunIO,
	}
}

func withDependencyDefaults(deps Dependencies) Dependencies {
	defaults := productionDependencies()
	if deps.Store == nil {
		deps.Store = defaults.Store
	}
	if deps.Input == nil {
		deps.Input = defaults.Input
	}
	if deps.Output == nil {
		deps.Output = defaults.Output
	}
	if deps.ErrorOutput == nil {
		deps.ErrorOutput = defaults.ErrorOutput
	}
	if deps.NewService == nil {
		deps.NewService = defaults.NewService
	}
	if deps.Authenticate == nil {
		deps.Authenticate = defaults.Authenticate
	}
	if deps.Verify == nil {
		deps.Verify = defaults.Verify
	}
	if deps.RunMCP == nil {
		deps.RunMCP = defaults.RunMCP
	}
	return deps
}

func (a *application) service() (monarch.Service, error) {
	value, err := a.store.Load(a.config.Profile)
	if err != nil {
		return nil, err
	}
	return a.newService(value, a.config.Timeout)
}
