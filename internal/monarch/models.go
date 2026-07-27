// Package monarch provides a bounded, read-only client for Monarch Money.
package monarch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

const (
	maxAmountTextLength        = 256
	maxAmountExponentMagnitude = 1000
)

// Amount preserves the exact lexical representation of a decimal API value.
// It deliberately avoids float64 so JSON decoding cannot silently round money
// or rates. JSON null is represented by the empty string for nullable fields;
// every non-null value must be a finite decimal number.
type Amount string

// UnmarshalJSON accepts Monarch amount fields encoded as either JSON numbers or strings.
func (a *Amount) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*a = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("decode amount: %w", err)
		}
		if !validDecimal(value) {
			return fmt.Errorf("decode amount: %q is not a decimal number", value)
		}
		*a = Amount(value)
		return nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("decode amount: %w", err)
	}
	if !validDecimal(number.String()) {
		return fmt.Errorf("decode amount: %q is not a decimal number", number.String())
	}
	*a = Amount(number.String())
	return nil
}

func validDecimal(value string) bool {
	if len(value) > maxAmountTextLength || !decimalPattern.MatchString(value) {
		return false
	}
	if _, exponentText, ok := strings.Cut(strings.ToLower(value), "e"); ok {
		exponent, err := strconv.ParseInt(exponentText, 10, 32)
		if err != nil || exponent < -maxAmountExponentMagnitude || exponent > maxAmountExponentMagnitude {
			return false
		}
	}
	_, ok := new(big.Rat).SetString(value)
	return ok
}

// Named identifies a Monarch object by ID and display name.
type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AccountType describes an account's type or subtype.
type AccountType struct {
	Name    string `json:"name"`
	Display string `json:"display"`
}

// Account is the read-only account representation returned by this package.
type Account struct {
	ID                       string      `json:"id"`
	DisplayName              string      `json:"display_name"`
	Type                     AccountType `json:"type"`
	Subtype                  AccountType `json:"subtype"`
	DisplayBalance           Amount      `json:"display_balance"`
	CurrentBalance           Amount      `json:"current_balance"`
	Limit                    Amount      `json:"limit,omitempty"`
	UpdatedAt                string      `json:"updated_at,omitempty"`
	DisplayLastUpdatedAt     string      `json:"display_last_updated_at,omitempty"`
	DeactivatedAt            string      `json:"deactivated_at,omitempty"`
	IsHidden                 bool        `json:"is_hidden"`
	IsAsset                  bool        `json:"is_asset"`
	Mask                     string      `json:"mask,omitempty"`
	IncludeInNetWorth        bool        `json:"include_in_net_worth"`
	IncludeBalanceInNetWorth bool        `json:"include_balance_in_net_worth"`
	DataProvider             string      `json:"data_provider,omitempty"`
	IsManual                 bool        `json:"is_manual"`
	TransactionsCount        int         `json:"transactions_count"`
	HoldingsCount            int         `json:"holdings_count"`
	LogoURL                  string      `json:"logo_url,omitempty"`
}

// AccountsResult wraps account output so MCP structured results always have an object root.
type AccountsResult struct {
	Accounts []Account `json:"accounts"`
}

// RefreshAccountsParams identifies the connected accounts to refresh.
type RefreshAccountsParams struct {
	AccountIDs []string `json:"account_ids"`
}

// AccountRefreshResult confirms that Monarch accepted a refresh request.
// Refresh runs asynchronously; acceptance does not mean syncing is complete.
type AccountRefreshResult struct {
	Accepted   bool     `json:"accepted"`
	AccountIDs []string `json:"account_ids"`
}

// CategoryGroup describes a group such as income or expense.
type CategoryGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Category is a transaction or budget category.
type Category struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Order            int           `json:"order"`
	Icon             string        `json:"icon,omitempty"`
	SystemCategory   string        `json:"system_category,omitempty"`
	IsSystemCategory bool          `json:"is_system_category"`
	IsDisabled       bool          `json:"is_disabled"`
	UpdatedAt        string        `json:"updated_at,omitempty"`
	CreatedAt        string        `json:"created_at,omitempty"`
	Group            CategoryGroup `json:"group"`
}

// CategoriesResult wraps category output.
type CategoriesResult struct {
	Categories []Category `json:"categories"`
}

// Tag is a user-defined transaction tag.
type Tag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Order int    `json:"order"`
}

// Attachment is metadata for a transaction attachment. No attachment content is fetched.
type Attachment struct {
	ID               string `json:"id"`
	Extension        string `json:"extension,omitempty"`
	Filename         string `json:"filename,omitempty"`
	OriginalAssetURL string `json:"original_asset_url,omitempty"`
	SizeBytes        int64  `json:"size_bytes"`
}

// Merchant identifies a transaction merchant.
type Merchant struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	TransactionsCount int    `json:"transactions_count"`
}

// AccountReference is the compact account form nested in transactions.
type AccountReference struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// TransactionSplit is one child of a split transaction. Category and Merchant
// are pointers because Monarch may legitimately return either relationship as
// null; a non-null relationship is validated before it leaves the client.
type TransactionSplit struct {
	ID       string    `json:"id"`
	Amount   Amount    `json:"amount"`
	Notes    string    `json:"notes,omitempty"`
	Category *Category `json:"category"`
	Merchant *Merchant `json:"merchant"`
}

// Transaction is a read-only Monarch transaction. Category, Merchant, and Goal
// preserve Monarch's nullable relationship semantics; required value objects
// such as Account are validated before a Transaction is returned.
type Transaction struct {
	ID                      string             `json:"id"`
	Amount                  Amount             `json:"amount"`
	Pending                 bool               `json:"pending"`
	Date                    string             `json:"date"`
	HideFromReports         bool               `json:"hide_from_reports"`
	DataProviderDescription string             `json:"data_provider_description,omitempty"`
	PlaidName               string             `json:"plaid_name,omitempty"`
	Notes                   string             `json:"notes,omitempty"`
	IsRecurring             bool               `json:"is_recurring"`
	ReviewStatus            string             `json:"review_status,omitempty"`
	NeedsReview             bool               `json:"needs_review"`
	HasSplitTransactions    bool               `json:"has_split_transactions"`
	IsSplitTransaction      bool               `json:"is_split_transaction"`
	SplitTransactions       []TransactionSplit `json:"split_transactions,omitempty"`
	CreatedAt               string             `json:"created_at,omitempty"`
	UpdatedAt               string             `json:"updated_at,omitempty"`
	Attachments             []Attachment       `json:"attachments,omitempty"`
	Category                *Category          `json:"category"`
	Merchant                *Merchant          `json:"merchant"`
	Account                 AccountReference   `json:"account"`
	Goal                    *Named             `json:"goal,omitempty"`
	Tags                    []Tag              `json:"tags,omitempty"`
}

// TransactionPage is a bounded page with an opaque continuation cursor. A
// cursor is valid only with the same filters that produced it. Because Monarch
// exposes offset pagination, inserts or deletes can shift later pages.
type TransactionPage struct {
	Transactions []Transaction `json:"transactions"`
	TotalCount   int           `json:"total_count"`
	NextCursor   string        `json:"next_cursor,omitempty"`
}

// TransactionResult wraps a single transaction.
type TransactionResult struct {
	Transaction Transaction `json:"transaction"`
}

// BudgetAmount contains one month's planned and actual category amounts.
type BudgetAmount struct {
	Month                       string `json:"month"`
	PlannedCashFlowAmount       Amount `json:"planned_cash_flow_amount"`
	PlannedSetAsideAmount       Amount `json:"planned_set_aside_amount,omitempty"`
	ActualAmount                Amount `json:"actual_amount"`
	RemainingAmount             Amount `json:"remaining_amount"`
	PreviousMonthRolloverAmount Amount `json:"previous_month_rollover_amount,omitempty"`
	RolloverType                string `json:"rollover_type,omitempty"`
}

// CategoryBudget contains monthly amounts for one category.
type CategoryBudget struct {
	Category       Named          `json:"category"`
	MonthlyAmounts []BudgetAmount `json:"monthly_amounts"`
}

// GroupBudget contains monthly amounts for one category group.
type GroupBudget struct {
	CategoryGroup  CategoryGroup  `json:"category_group"`
	MonthlyAmounts []BudgetAmount `json:"monthly_amounts"`
}

// BudgetReport is the budget data for an inclusive month range.
type BudgetReport struct {
	StartMonth string           `json:"start_month"`
	EndMonth   string           `json:"end_month"`
	Categories []CategoryBudget `json:"categories"`
	Groups     []GroupBudget    `json:"groups"`
}

// CashflowSummary is Monarch's aggregate income, expense, and savings data.
type CashflowSummary struct {
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	SumIncome   Amount `json:"sum_income"`
	SumExpense  Amount `json:"sum_expense"`
	Savings     Amount `json:"savings"`
	SavingsRate Amount `json:"savings_rate"`
}

// FinancialOverview combines current accounts and net worth with bounded
// reports for the requested date range. Transactions use Monarch's date
// ordering. Monarch does not expose a stable secondary sort key, so
// transactions sharing a date have unspecified order.
type FinancialOverview struct {
	AsOf         string          `json:"as_of"`
	NetWorth     Amount          `json:"net_worth"`
	Accounts     []Account       `json:"accounts"`
	Cashflow     CashflowSummary `json:"cashflow"`
	Budget       BudgetReport    `json:"budget"`
	Transactions []Transaction   `json:"transactions"`
}

// ListAccountsParams controls local account filtering.
type ListAccountsParams struct {
	IncludeHidden      bool `json:"include_hidden,omitempty"`
	IncludeDeactivated bool `json:"include_deactivated,omitempty"`
}

// ListTransactionsParams controls server filters and bounded pagination.
// Limit zero means the default page size. Cursor must be reused with the same
// StartDate, EndDate, Search, and ID filters that produced it.
type ListTransactionsParams struct {
	StartDate   string   `json:"start_date,omitempty"`
	EndDate     string   `json:"end_date,omitempty"`
	Search      string   `json:"search,omitempty"`
	AccountIDs  []string `json:"account_ids,omitempty"`
	CategoryIDs []string `json:"category_ids,omitempty"`
	TagIDs      []string `json:"tag_ids,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Cursor      string   `json:"cursor,omitempty"`
}

// DateRange is an inclusive ISO date range.
type DateRange struct {
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

// MonthRange is an inclusive YYYY-MM budget month range.
type MonthRange struct {
	StartMonth string `json:"start_month,omitempty"`
	EndMonth   string `json:"end_month,omitempty"`
}
