package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteing/monarch-cli/internal/monarch"
)

func TestTransactionsGroupByMonthAndAdaptColumns(t *testing.T) {
	transactions := []monarch.Transaction{
		transactionForTest("july-1", "2026-07-25", "Coffee", "Dining", "Checking", "-4.50"),
		transactionForTest("july-2", "2026-07-02", "Rent", "Housing", "Checking", "-1200.00"),
		transactionForTest("june-1", "2026-06-29", "Payroll", "Income", "Checking", "2500.00"),
	}

	grouped := renderTransactions(transactions, true, 100)
	for _, value := range []string{"July 2026", "June 2026", "CATEGORY", "ACCOUNT"} {
		if !strings.Contains(grouped, value) {
			t.Fatalf("grouped transactions do not contain %q", value)
		}
	}
	if strings.Contains(grouped, "july-1") {
		t.Fatal("100-column transaction table unexpectedly includes IDs")
	}

	narrow := renderTransactions(transactions, false, 50)
	for _, value := range []string{"CATEGORY", "ACCOUNT", "july-1"} {
		if strings.Contains(narrow, value) {
			t.Fatalf("narrow transaction table unexpectedly contains %q", value)
		}
	}
	wide := renderTransactions(transactions, false, 140)
	if !strings.Contains(wide, "july-1") {
		t.Fatal("wide transaction table does not include IDs")
	}
}

func TestTransactionColumnsStayFixedAcrossPages(t *testing.T) {
	short := renderTransactions([]monarch.Transaction{
		transactionForTest("", "2026-07-25", "Cafe", "Dining", "Card", "-4.50"),
	}, false, 100)
	long := renderTransactions([]monarch.Transaction{
		transactionForTest("", "2026-07-24", "A merchant name that would normally widen its column", "Home improvement and maintenance", "Everyday checking account", "-12345.67"),
	}, false, 100)

	shortBorder := strings.SplitN(short, "\n", 2)[0]
	longBorder := strings.SplitN(long, "\n", 2)[0]
	if shortBorder != longBorder {
		t.Fatalf("transaction column borders changed between pages:\nshort: %q\n long: %q", shortBorder, longBorder)
	}
}

func TestTransactionPagerKeyboardAndResizeFlow(t *testing.T) {
	var cursors []string
	opts := TransactionOptions{
		Context: context.Background(), PageSize: 2, GroupByMonth: true,
		Fetch: func(_ context.Context, cursor string) (monarch.TransactionPage, error) {
			cursors = append(cursors, cursor)
			if cursor == "" {
				return monarch.TransactionPage{
					Transactions: []monarch.Transaction{
						transactionForTest("", "2026-07-25", "One", "", "", ""),
						transactionForTest("", "2026-07-24", "Two", "", "", ""),
					},
					TotalCount: 3, NextCursor: "page-2",
				}, nil
			}
			return monarch.TransactionPage{Transactions: []monarch.Transaction{
				transactionForTest("", "2026-06-30", "Three", "", "", ""),
			}, TotalCount: 3}, nil
		},
	}
	model := newTransactionModel(opts)
	update := func(message tea.Msg) tea.Cmd {
		t.Helper()
		next, command := model.Update(message)
		model = next.(transactionModel)
		return command
	}

	update(runTransactionCommand(t, model.Init()))
	if model.page != 0 || model.loading || len(model.pages[0].Transactions) != 2 {
		t.Fatalf("initial page was not loaded: page=%d loading=%t data=%+v", model.page, model.loading, model.pages[0])
	}
	update(tea.WindowSizeMsg{Width: 58, Height: 12})
	if model.viewport.Width() != 54 || model.viewport.Height() != 7 {
		t.Fatalf("viewport size = %dx%d, want 54x7", model.viewport.Width(), model.viewport.Height())
	}
	view := model.View()
	assertViewFits(t, view, 58, 12)
	for _, value := range []string{"Monarch CLI", "Transactions"} {
		if strings.Contains(view.Content, value) {
			t.Fatalf("transaction pager unexpectedly contains page heading %q", value)
		}
	}
	update(tea.WindowSizeMsg{Width: 58, Height: 8})
	update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.viewport.YOffset() == 0 {
		t.Fatal("down arrow did not scroll the current page")
	}

	command := update(tea.KeyPressMsg{Code: tea.KeyRight})
	if !model.loading {
		t.Fatal("right arrow did not start loading the next page")
	}
	update(runTransactionCommand(t, command))
	if model.page != 1 || model.pages[1].Transactions[0].Merchant.Name != "Three" {
		t.Fatalf("next page was not selected: page=%d data=%+v", model.page, model.pages[1])
	}
	update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if model.page != 0 {
		t.Fatalf("left arrow selected page %d, want 0", model.page)
	}
	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != "page-2" {
		t.Fatalf("fetch cursors = %#v", cursors)
	}
	if !model.View().AltScreen {
		t.Fatal("transaction pager does not use the alternate screen")
	}
}

func TestResponsiveTableDoesNotExceedRequestedWidth(t *testing.T) {
	rendered := RenderTable([]string{"NAME", "TYPE", "BALANCE", "ID"}, [][]string{{"A very long account name", "Depository", "$42.00", "long-account-id"}}, 36)
	for _, line := range strings.Split(rendered, "\n") {
		if width := lipgloss.Width(line); width > 36 {
			t.Fatalf("rendered line width = %d, want <= 36: %q", width, line)
		}
	}
}

func TestTransactionRenderingSanitizesRowsGroupsAndErrors(t *testing.T) {
	rendered := renderTransactions([]monarch.Transaction{
		transactionForTest("", "\x1b[2Jbad", "merchant\x1b[2J", "", "", "1"),
	}, true, 80)
	if strings.Contains(rendered, "\x1b[2J") {
		t.Fatalf("terminal command survived transaction rendering: %q", rendered)
	}
	if !strings.Contains(rendered, "Unknown month") {
		t.Fatalf("malformed date did not use a static group: %q", rendered)
	}

	model := newTransactionModel(TransactionOptions{Context: context.Background(), PageSize: 25})
	model.loading = false
	model.err = errors.New("\x1b[2Junsafe")
	if strings.Contains(model.View().Content, "\x1b[2J") {
		t.Fatal("terminal command survived transaction error rendering")
	}
}

func TestTransactionQuitCancelsInFlightFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var calls atomic.Int32
	model := newTransactionModel(TransactionOptions{
		Context: ctx, PageSize: 25,
		Fetch: func(ctx context.Context, _ string) (monarch.TransactionPage, error) {
			calls.Add(1)
			close(started)
			<-ctx.Done()
			return monarch.TransactionPage{}, ctx.Err()
		},
	})
	model.cancel = cancel
	result := make(chan tea.Msg, 1)
	go func() { result <- model.fetch(0, "")() }()
	<-started
	next, quit := model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	model = next.(transactionModel)
	if quit == nil || !model.canceled {
		t.Fatal("q did not quit the transaction pager")
	}
	message := (<-result).(transactionPageMsg)
	if !errors.Is(message.err, context.Canceled) || calls.Load() != 1 {
		t.Fatalf("fetch result = %v, calls = %d", message.err, calls.Load())
	}
}

func TestRunTransactionsReturnsAnErrorAfterDisplayingIt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	fetchErr := errors.New("fetch failed")
	fetched := make(chan struct{})
	var output bytes.Buffer
	input := &signalReader{ctx: ctx, ready: fetched, delay: 50 * time.Millisecond, data: []byte("q")}

	err := RunTransactions(TransactionOptions{
		Context: ctx, Input: input, Output: &output, PageSize: 25,
		Fetch: func(context.Context, string) (monarch.TransactionPage, error) {
			close(fetched)
			return monarch.TransactionPage{}, fetchErr
		},
	})
	if !errors.Is(err, fetchErr) {
		t.Fatalf("RunTransactions() = %v, want %v; output %q", err, fetchErr, output.String())
	}
}

func TestValidateTransactionOptions(t *testing.T) {
	valid := TransactionOptions{
		Context: context.Background(), Input: strings.NewReader(""), Output: &strings.Builder{}, PageSize: 25,
		Fetch: func(context.Context, string) (monarch.TransactionPage, error) { return monarch.TransactionPage{}, nil },
	}
	if err := validateTransactionOptions(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*TransactionOptions){
		"context":   func(opts *TransactionOptions) { opts.Context = nil },
		"input":     func(opts *TransactionOptions) { opts.Input = nil },
		"output":    func(opts *TransactionOptions) { opts.Output = nil },
		"fetch":     func(opts *TransactionOptions) { opts.Fetch = nil },
		"page-zero": func(opts *TransactionOptions) { opts.PageSize = 0 },
		"page-high": func(opts *TransactionOptions) { opts.PageSize = 101 },
	} {
		t.Run(name, func(t *testing.T) {
			opts := valid
			mutate(&opts)
			if err := validateTransactionOptions(opts); err == nil {
				t.Fatal("invalid options were accepted")
			}
		})
	}
}

func TestTransactionTinyViewFits(t *testing.T) {
	model := newTransactionModel(TransactionOptions{Context: context.Background(), PageSize: 25})
	model.pages[0] = monarch.TransactionPage{Transactions: []monarch.Transaction{
		transactionForTest("", "2026-07-25", "Coffee", "", "", ""),
	}}
	model.loading = false
	model.resize(12, 4)
	model.renderPage(true)
	assertViewFits(t, model.View(), 12, 4)
}

func transactionForTest(id, date, merchant, category, account, amount string) monarch.Transaction {
	value := monarch.Transaction{
		ID: id, Date: date, Amount: monarch.Amount(amount),
		Account: monarch.AccountReference{DisplayName: account},
	}
	if merchant != "" {
		value.Merchant = &monarch.Merchant{Name: merchant}
	}
	if category != "" {
		value.Category = &monarch.Category{Name: category}
	}
	return value
}

func runTransactionCommand(t *testing.T, command tea.Cmd) transactionPageMsg {
	t.Helper()
	if command == nil {
		t.Fatal("transaction pager did not return a command")
	}
	message := command()
	if result, ok := message.(transactionPageMsg); ok {
		return result
	}
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("transaction command returned %T, want page result batch", message)
	}
	for index := len(batch) - 1; index >= 0; index-- {
		if result, ok := batch[index]().(transactionPageMsg); ok {
			return result
		}
	}
	t.Fatal("transaction command batch did not contain a page result")
	return transactionPageMsg{}
}

func assertViewFits(t *testing.T, view tea.View, width, height int) {
	t.Helper()
	for _, line := range strings.Split(view.Content, "\n") {
		if renderedWidth := lipgloss.Width(line); renderedWidth > width {
			t.Fatalf("view line width = %d, want <= %d: %q", renderedWidth, width, line)
		}
	}
	if renderedHeight := lipgloss.Height(view.Content); renderedHeight > height {
		t.Fatalf("view height = %d, want <= %d:\n%s", renderedHeight, height, view.Content)
	}
}

type signalReader struct {
	ctx    context.Context
	ready  <-chan struct{}
	delay  time.Duration
	data   []byte
	offset int
}

func (r *signalReader) Read(target []byte) (int, error) {
	select {
	case <-r.ready:
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
	if r.delay > 0 {
		timer := time.NewTimer(r.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
		r.delay = 0
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	written := copy(target, r.data[r.offset:])
	r.offset += written
	return written, nil
}
