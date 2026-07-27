package monarch

import "context"

// Reader is the complete read-only Monarch surface.
type Reader interface {
	ListAccounts(context.Context, ListAccountsParams) (AccountsResult, error)
	ListTransactions(context.Context, ListTransactionsParams) (TransactionPage, error)
	GetTransaction(context.Context, string) (TransactionResult, error)
	ListCategories(context.Context) (CategoriesResult, error)
	GetBudgets(context.Context, MonthRange) (BudgetReport, error)
	GetCashflow(context.Context, DateRange) (CashflowSummary, error)
	GetFinancialOverview(context.Context, DateRange) (FinancialOverview, error)
}

// AccountRefresher requests asynchronous synchronization of connected accounts.
type AccountRefresher interface {
	RefreshAccounts(context.Context, RefreshAccountsParams) (AccountRefreshResult, error)
}

// Service is the complete Monarch surface exposed by the MCP server.
type Service interface {
	Reader
	AccountRefresher
}
