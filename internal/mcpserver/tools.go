package mcpserver

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/matteing/monarch-cli/internal/monarch"
)

func registerTools(server *mcp.Server, service monarch.Service) {
	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_accounts_list",
		Title:       "List Monarch accounts",
		Description: "List financial accounts and exact balances. Hidden and deactivated accounts are omitted by default.",
		InputSchema: accountsSchema(),
	}, func(ctx context.Context, input monarch.ListAccountsParams) (monarch.AccountsResult, error) {
		return service.ListAccounts(ctx, input)
	})

	addTool(server, &mcp.Tool{
		Name:        "monarch_accounts_refresh",
		Title:       "Refresh Monarch accounts",
		Description: "Ask Monarch to asynchronously refresh one or more connected accounts. Acceptance does not mean syncing is complete.",
		InputSchema: refreshAccountsSchema(),
	}, refreshAnnotations(), func(ctx context.Context, input monarch.RefreshAccountsParams) (monarch.AccountRefreshResult, error) {
		return service.RefreshAccounts(ctx, input)
	})

	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_transactions_list",
		Title:       "List Monarch transactions",
		Description: fmt.Sprintf("List or search transactions with optional date and ID filters. Results are capped at %d and paginated with an opaque cursor.", monarch.MaxTransactionPageSize),
		InputSchema: transactionsSchema(),
	}, func(ctx context.Context, input monarch.ListTransactionsParams) (monarch.TransactionPage, error) {
		return service.ListTransactions(ctx, input)
	})

	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_transaction_get",
		Title:       "Get a Monarch transaction",
		Description: "Get one transaction and its category, merchant, account, tags, notes, and attachment metadata.",
		InputSchema: transactionSchema(),
	}, func(ctx context.Context, input TransactionInput) (monarch.TransactionResult, error) {
		return service.GetTransaction(ctx, input.ID)
	})

	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_categories_list",
		Title:       "List Monarch categories",
		Description: "List transaction and budget categories with their category groups.",
		InputSchema: emptySchema(),
	}, func(ctx context.Context, _ EmptyInput) (monarch.CategoriesResult, error) {
		return service.ListCategories(ctx)
	})

	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_budgets_get",
		Title:       "Get Monarch budgets",
		Description: "Get planned, actual, remaining, and rollover amounts by category and category group. Omit both months for the current month.",
		InputSchema: budgetsSchema(),
	}, func(ctx context.Context, input monarch.MonthRange) (monarch.BudgetReport, error) {
		return service.GetBudgets(ctx, input)
	})

	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_cashflow_summary",
		Title:       "Summarize Monarch cashflow",
		Description: "Return income, expenses, savings, and savings rate. Omit both dates for the current month.",
		InputSchema: dateRangeSchema(),
	}, func(ctx context.Context, input monarch.DateRange) (monarch.CashflowSummary, error) {
		return service.GetCashflow(ctx, input)
	})

	addReadOnlyTool(server, &mcp.Tool{
		Name:        "monarch_financial_overview",
		Title:       "Get a Monarch financial overview",
		Description: fmt.Sprintf("Return current net worth and accounts with cashflow, budget, and up to %d transactions for the requested range. Independent reads are not an atomic snapshot. Omit both dates for the current month.", monarch.FinancialOverviewTransactionLimit),
		InputSchema: dateRangeSchema(),
	}, func(ctx context.Context, input monarch.DateRange) (monarch.FinancialOverview, error) {
		return service.GetFinancialOverview(ctx, input)
	})
}

func addTool[Input, Output any](
	server *mcp.Server,
	tool *mcp.Tool,
	annotations *mcp.ToolAnnotations,
	invoke func(context.Context, Input) (Output, error),
) {
	tool.Annotations = annotations
	tool.OutputSchema = outputSchema[Output]()
	mcp.AddTool(server, tool, func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input Input,
	) (*mcp.CallToolResult, Output, error) {
		output, err := invoke(ctx, input)
		return nil, output, toolError(err)
	})
}

func addReadOnlyTool[Input, Output any](
	server *mcp.Server,
	tool *mcp.Tool,
	read func(context.Context, Input) (Output, error),
) {
	addTool(server, tool, readOnlyAnnotations(), read)
}

func readOnlyAnnotations() *mcp.ToolAnnotations {
	return toolAnnotations(true, true)
}

func refreshAnnotations() *mcp.ToolAnnotations {
	return toolAnnotations(false, false)
}

func toolAnnotations(readOnly, idempotent bool) *mcp.ToolAnnotations {
	openWorld, destructive := true, false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		IdempotentHint:  idempotent,
		OpenWorldHint:   &openWorld,
		DestructiveHint: &destructive,
	}
}

func outputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	requireArrayValues(schema)
	return schema
}

// Go slices can be nil, so reflection describes them as nullable. Public
// result collections are emitted as arrays (or omitted when tagged omitempty),
// never explicit nulls; narrow the generated schema to that contract.
func requireArrayValues(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if slices.Contains(schema.Types, "array") {
		schema.Type, schema.Types = "array", nil
	}
	for _, child := range schema.Properties {
		requireArrayValues(child)
	}
	for _, child := range schema.Defs {
		requireArrayValues(child)
	}
	for _, child := range schema.Definitions {
		requireArrayValues(child)
	}
	requireArrayValues(schema.Items)
	for _, children := range [][]*jsonschema.Schema{schema.PrefixItems, schema.ItemsArray, schema.AllOf, schema.AnyOf, schema.OneOf} {
		for _, child := range children {
			requireArrayValues(child)
		}
	}
}
