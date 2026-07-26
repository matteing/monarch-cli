package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/matteing/monarch-cli/internal/monarch"
)

// TransactionOptions configures the interactive transaction pager. Context,
// terminal I/O, a bounded page size, and a context-aware Fetch
// callback are required.
type TransactionOptions struct {
	Context       context.Context
	Input         io.Reader
	Output        io.Writer
	InitialCursor string
	PageSize      int
	GroupByMonth  bool
	Fetch         func(context.Context, string) (monarch.TransactionPage, error)
}

// RunTransactions displays a responsive, read-only transaction pager.
func RunTransactions(opts TransactionOptions) error {
	if err := validateTransactionOptions(opts); err != nil {
		return err
	}
	programContext := opts.Context
	operationContext, cancel := context.WithCancel(opts.Context)
	defer cancel()
	opts.Context = operationContext
	model := newTransactionModel(opts)
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
		return err
	}
	result, ok := final.(transactionModel)
	if !ok {
		return errors.New("transaction UI returned an unexpected model")
	}
	if result.err != nil {
		return result.err
	}
	return nil
}

func validateTransactionOptions(opts TransactionOptions) error {
	if opts.Context == nil {
		return errors.New("transaction context is required")
	}
	if opts.Input == nil || opts.Output == nil {
		return errors.New("transaction input and output are required")
	}
	if opts.Fetch == nil {
		return errors.New("transaction fetch callback is required")
	}
	if err := monarch.ValidateTransactionPageSize(opts.PageSize); err != nil {
		return fmt.Errorf("invalid transaction page size: %w", err)
	}
	return nil
}
