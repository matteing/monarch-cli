package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/monarch"
)

type recordingReader struct {
	accountsParams     monarch.ListAccountsParams
	refreshParams      monarch.RefreshAccountsParams
	transactionsParams monarch.ListTransactionsParams
	transactionID      string
	budgetRange        monarch.MonthRange
	cashflowRange      monarch.DateRange
	overviewRange      monarch.DateRange
	transactionResult  monarch.TransactionResult
	calls              []string
	err                error
	accountsFn         func(context.Context) error
}

func (r *recordingReader) ListAccounts(ctx context.Context, params monarch.ListAccountsParams) (monarch.AccountsResult, error) {
	r.calls = append(r.calls, "accounts")
	r.accountsParams = params
	if r.accountsFn != nil {
		return monarch.AccountsResult{}, r.accountsFn(ctx)
	}
	return monarch.AccountsResult{Accounts: []monarch.Account{}}, r.err
}

func (r *recordingReader) RefreshAccounts(_ context.Context, params monarch.RefreshAccountsParams) (monarch.AccountRefreshResult, error) {
	r.calls = append(r.calls, "refresh_accounts")
	r.refreshParams = params
	return monarch.AccountRefreshResult{Accepted: true, AccountIDs: append([]string(nil), params.AccountIDs...)}, r.err
}

func (r *recordingReader) ListTransactions(_ context.Context, params monarch.ListTransactionsParams) (monarch.TransactionPage, error) {
	r.calls = append(r.calls, "transactions")
	r.transactionsParams = params
	return monarch.TransactionPage{Transactions: []monarch.Transaction{}}, r.err
}

func (r *recordingReader) GetTransaction(_ context.Context, id string) (monarch.TransactionResult, error) {
	r.calls = append(r.calls, "transaction")
	r.transactionID = id
	return r.transactionResult, r.err
}

func (r *recordingReader) ListCategories(context.Context) (monarch.CategoriesResult, error) {
	r.calls = append(r.calls, "categories")
	return monarch.CategoriesResult{Categories: []monarch.Category{}}, r.err
}

func (r *recordingReader) GetBudgets(_ context.Context, months monarch.MonthRange) (monarch.BudgetReport, error) {
	r.calls = append(r.calls, "budgets")
	r.budgetRange = months
	return monarch.BudgetReport{Categories: []monarch.CategoryBudget{}, Groups: []monarch.GroupBudget{}}, r.err
}

func (r *recordingReader) GetCashflow(_ context.Context, dates monarch.DateRange) (monarch.CashflowSummary, error) {
	r.calls = append(r.calls, "cashflow")
	r.cashflowRange = dates
	return monarch.CashflowSummary{}, r.err
}

func (r *recordingReader) GetFinancialOverview(_ context.Context, dates monarch.DateRange) (monarch.FinancialOverview, error) {
	r.calls = append(r.calls, "overview")
	r.overviewRange = dates
	return monarch.FinancialOverview{
		Accounts: []monarch.Account{},
		Budget: monarch.BudgetReport{
			Categories: []monarch.CategoryBudget{},
			Groups:     []monarch.GroupBudget{},
		},
		Transactions: []monarch.Transaction{},
	}, r.err
}

func connectTestServer(t *testing.T, service monarch.Service) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()
	server := NewWithLogger(service, "test", nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverSession.Close() })
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientSession.Close() })
	return ctx, clientSession
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func TestServerPublishesExactToolsAndSchemaSnapshot(t *testing.T) {
	reader := &recordingReader{}
	ctx, session := connectTestServer(t, reader)
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"monarch_accounts_list", "monarch_accounts_refresh", "monarch_budgets_get", "monarch_cashflow_summary",
		"monarch_categories_list", "monarch_financial_overview", "monarch_transaction_get",
		"monarch_transactions_list",
	}
	gotNames := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		gotNames = append(gotNames, tool.Name)
		assertToolMetadata(t, tool)
	}
	if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("tool names = %v, want %v", gotNames, wantNames)
	}

	got, err := marshalToolSchemaSnapshot(result.Tools)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/tool-schemas.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("tool schemas changed; inspect the contract and update testdata/tool-schemas.json intentionally\n\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func assertToolMetadata(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	if tool.Annotations == nil ||
		tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint ||
		tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Errorf("tool %s lacks complete safety annotations", tool.Name)
		return
	}
	if tool.Name == "monarch_accounts_refresh" {
		if tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint {
			t.Errorf("refresh tool incorrectly claims read-only or idempotent behavior")
		}
	} else if !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
		t.Errorf("tool %s lacks read-only annotations", tool.Name)
	}
	if tool.InputSchema == nil || tool.OutputSchema == nil {
		t.Errorf("tool %s lacks explicitly attached schemas", tool.Name)
	}
}

type toolSchemaSnapshot struct {
	Name         string          `json:"name"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

func marshalToolSchemaSnapshot(tools []*mcp.Tool) ([]byte, error) {
	snapshot := make([]toolSchemaSnapshot, 0, len(tools))
	for _, tool := range tools {
		input, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal %s input schema: %w", tool.Name, err)
		}
		output, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal %s output schema: %w", tool.Name, err)
		}
		snapshot = append(snapshot, toolSchemaSnapshot{
			Name:         tool.Name,
			InputSchema:  input,
			OutputSchema: output,
		})
	}
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].Name < snapshot[j].Name })

	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tool schema snapshot: %w", err)
	}
	return append(raw, '\n'), nil
}

func TestAllToolsDelegateEveryInput(t *testing.T) {
	reader := &recordingReader{}
	ctx, session := connectTestServer(t, reader)
	calls := []struct {
		name string
		args map[string]any
	}{
		{"monarch_accounts_list", map[string]any{"include_hidden": true, "include_deactivated": true}},
		{"monarch_accounts_refresh", map[string]any{"account_ids": []string{"account:1/path", "account.2"}}},
		{"monarch_transactions_list", map[string]any{
			"start_date": "2026-07-01", "end_date": "2026-07-31", "search": "coffee",
			"account_ids": []string{"account:1/path"}, "category_ids": []string{"category.1"},
			"tag_ids": []string{"tag_1"}, "limit": 7, "cursor": "cursor",
		}},
		{"monarch_transaction_get", map[string]any{"id": "transaction:1/path"}},
		{"monarch_categories_list", map[string]any{}},
		{"monarch_budgets_get", map[string]any{"start_month": "2026-06", "end_month": "2026-07"}},
		{"monarch_cashflow_summary", map[string]any{"start_date": "2026-07-01", "end_date": "2026-07-31"}},
		{"monarch_financial_overview", map[string]any{"start_date": "2026-06-01", "end_date": "2026-06-30"}},
	}
	for _, call := range calls {
		result := callTool(t, ctx, session, call.name, call.args)
		if result.IsError || result.StructuredContent == nil || len(result.Content) == 0 {
			t.Fatalf("call %s returned %+v", call.name, result)
		}
	}
	if !reader.accountsParams.IncludeHidden || !reader.accountsParams.IncludeDeactivated {
		t.Fatalf("account params = %+v", reader.accountsParams)
	}
	if strings.Join(reader.refreshParams.AccountIDs, ",") != "account:1/path,account.2" {
		t.Fatalf("refresh params = %+v", reader.refreshParams)
	}
	if got := reader.transactionsParams; got.StartDate != "2026-07-01" || got.EndDate != "2026-07-31" || got.Search != "coffee" || got.Limit != 7 || got.Cursor != "cursor" || strings.Join(got.AccountIDs, ",") != "account:1/path" || strings.Join(got.CategoryIDs, ",") != "category.1" || strings.Join(got.TagIDs, ",") != "tag_1" {
		t.Fatalf("transaction params = %+v", got)
	}
	if reader.transactionID != "transaction:1/path" || reader.budgetRange.StartMonth != "2026-06" || reader.cashflowRange.StartDate != "2026-07-01" || reader.overviewRange.StartDate != "2026-06-01" {
		t.Fatalf("delegation state: id=%q budget=%+v cashflow=%+v overview=%+v", reader.transactionID, reader.budgetRange, reader.cashflowRange, reader.overviewRange)
	}
}

func TestSuccessfulToolReturnsCompleteStructuredContract(t *testing.T) {
	category := &monarch.Category{
		ID:               "category-1",
		Name:             "Coffee",
		Order:            3,
		Icon:             "coffee",
		SystemCategory:   "FOOD_AND_DRINK",
		IsSystemCategory: true,
		UpdatedAt:        "2026-07-25T10:00:00Z",
		CreatedAt:        "2025-01-02T03:04:05Z",
		Group: monarch.CategoryGroup{
			ID: "group-1", Name: "Food", Type: "expense",
		},
	}
	merchant := &monarch.Merchant{ID: "merchant-1", Name: "Poetry Coffee", TransactionsCount: 8}
	want := monarch.TransactionResult{Transaction: monarch.Transaction{
		ID:                      "transaction-1",
		Amount:                  "-4.75",
		Pending:                 true,
		Date:                    "2026-07-25",
		HideFromReports:         true,
		DataProviderDescription: "POETRY COFFEE 042",
		PlaidName:               "Poetry Coffee",
		Notes:                   "Morning espresso",
		IsRecurring:             true,
		ReviewStatus:            "reviewed",
		NeedsReview:             true,
		HasSplitTransactions:    true,
		IsSplitTransaction:      true,
		SplitTransactions: []monarch.TransactionSplit{{
			ID: "split-1", Amount: "-4.75", Notes: "Coffee", Category: category, Merchant: merchant,
		}},
		CreatedAt: "2026-07-25T10:00:00Z",
		UpdatedAt: "2026-07-25T10:01:00Z",
		Attachments: []monarch.Attachment{{
			ID: "attachment-1", Extension: "pdf", Filename: "receipt.pdf",
			OriginalAssetURL: "https://example.invalid/receipt.pdf", SizeBytes: 1234,
		}},
		Category: category,
		Merchant: merchant,
		Account:  monarch.AccountReference{ID: "account-1", DisplayName: "Everyday Card"},
		Goal:     &monarch.Named{ID: "goal-1", Name: "Travel"},
		Tags:     []monarch.Tag{{ID: "tag-1", Name: "Work", Color: "#123456", Order: 2}},
	}}
	reader := &recordingReader{transactionResult: want}
	ctx, session := connectTestServer(t, reader)

	result := callTool(t, ctx, session, "monarch_transaction_get", map[string]any{"id": "transaction-1"})
	if result.IsError || result.StructuredContent == nil || len(result.Content) == 0 {
		t.Fatalf("result = %+v", result)
	}

	gotJSON, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("structured content:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

func TestSchemaRejectsInvalidInputBeforeReader(t *testing.T) {
	reader := &recordingReader{}
	ctx, session := connectTestServer(t, reader)
	for _, args := range []map[string]any{
		{"limit": 0},
		{"start_date": "2026-07-01"},
		{"account_ids": []string{""}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "monarch_transactions_list", Arguments: args})
		if err != nil {
			t.Fatalf("arguments %+v returned protocol error: %v", args, err)
		}
		if !result.IsError {
			t.Fatalf("arguments %+v were accepted", args)
		}
	}
	if len(reader.calls) != 0 {
		t.Fatalf("reader calls = %v", reader.calls)
	}
}

func TestRefreshSchemaRejectsMissingEmptyAndDuplicateIDs(t *testing.T) {
	reader := &recordingReader{}
	ctx, session := connectTestServer(t, reader)
	for _, args := range []map[string]any{
		{},
		{"account_ids": []string{}},
		{"account_ids": []string{"account", "account"}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "monarch_accounts_refresh", Arguments: args})
		if err != nil {
			t.Fatalf("arguments %+v returned protocol error: %v", args, err)
		}
		if !result.IsError {
			t.Fatalf("arguments %+v were accepted", args)
		}
	}
	if len(reader.calls) != 0 {
		t.Fatalf("reader calls = %v", reader.calls)
	}
}

func TestToolErrorsUseSafeStructuredEnvelope(t *testing.T) {
	reader := &recordingReader{err: &apperr.Error{
		Kind: apperr.KindRateLimited, Op: "list accounts", Message: "rate limited",
		StatusCode: 429, Retryable: true, RetryAfter: 2 * time.Second,
		Err: errors.New("Authorization: secret"),
	}}
	ctx, session := connectTestServer(t, reader)
	result := callTool(t, ctx, session, "monarch_accounts_list", map[string]any{})
	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %+v", result)
	}
	raw, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, phrase := range []string{`\"kind\":\"rate_limited\"`, `\"status_code\":429`, `\"retry_after_ms\":2000`} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("error content missing %q: %s", phrase, text)
		}
	}
	if strings.Contains(text, "Authorization") || strings.Contains(text, "secret") {
		t.Fatalf("error leaked cause: %s", text)
	}
}

func TestToolDeadlineUsesUnavailableEnvelope(t *testing.T) {
	reader := &recordingReader{err: context.DeadlineExceeded}
	ctx, session := connectTestServer(t, reader)
	result := callTool(t, ctx, session, "monarch_accounts_list", map[string]any{})
	if !result.IsError {
		t.Fatalf("result = %+v", result)
	}
	raw, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, phrase := range []string{`\"kind\":\"unavailable\"`, `\"message\":\"operation timed out\"`, `\"retryable\":true`} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("deadline error content missing %q: %s", phrase, text)
		}
	}
}

func TestToolCancellationReachesReader(t *testing.T) {
	started := make(chan struct{})
	reader := &recordingReader{accountsFn: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	ctx, session := connectTestServer(t, reader)
	callCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: "monarch_accounts_list", Arguments: map[string]any{}})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil && callCtx.Err() == nil {
			t.Fatal("canceled call returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled tool call did not return")
	}
}
