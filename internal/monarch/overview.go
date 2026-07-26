package monarch

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/matteing/monarch-cli/internal/apperr"
)

// GetFinancialOverview executes independent reads concurrently and combines
// them. Accounts and net worth are current values regardless of dates. Budget
// values cover the complete months intersected by dates; cashflow and
// transactions use the exact dates. The returned reads are not an atomic
// upstream snapshot. Transaction order for equal dates is unspecified.
func (c *Client) GetFinancialOverview(ctx context.Context, dates DateRange) (FinancialOverview, error) {
	const op = "get financial overview"
	now := c.now()
	start, end := dates.StartDate, dates.EndDate
	if start == "" && end == "" {
		start, end = currentMonthDates(now)
	}
	if err := validateDateRange(start, end, false); err != nil {
		return FinancialOverview{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}

	var allAccounts AccountsResult
	var cashflow CashflowSummary
	var budget BudgetReport
	var transactions TransactionPage
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		allAccounts, err = c.ListAccounts(groupCtx, ListAccountsParams{IncludeHidden: true, IncludeDeactivated: true})
		return err
	})
	group.Go(func() error {
		var err error
		cashflow, err = c.getCashflowAt(groupCtx, DateRange{StartDate: start, EndDate: end}, now)
		return err
	})
	group.Go(func() error {
		var err error
		budget, err = c.getBudgetsAt(groupCtx, MonthRange{StartMonth: start[:7], EndMonth: end[:7]}, now)
		return err
	})
	group.Go(func() error {
		var err error
		transactions, err = c.ListTransactions(groupCtx, ListTransactionsParams{
			StartDate: start, EndDate: end, Limit: FinancialOverviewTransactionLimit,
		})
		return err
	})
	if err := group.Wait(); err != nil {
		return FinancialOverview{}, err
	}
	if err := ctx.Err(); err != nil {
		return FinancialOverview{}, err
	}
	currentNetWorth, err := netWorth(allAccounts.Accounts)
	if err != nil {
		return FinancialOverview{}, apperr.New(apperr.KindUnavailable, op, "Monarch returned an invalid account balance", err)
	}
	visibleAccounts := filterAccounts(allAccounts.Accounts, ListAccountsParams{})
	return FinancialOverview{
		AsOf: now.UTC().Format(time.RFC3339), NetWorth: currentNetWorth,
		Accounts: visibleAccounts, Cashflow: cashflow, Budget: budget,
		Transactions: transactions.Transactions,
	}, nil
}

func netWorth(accounts []Account) (Amount, error) {
	total := new(big.Rat)
	precision := 2
	for _, account := range accounts {
		// Monarch has exposed both the user-facing and effective inclusion flags
		// over time. Either true value means the returned balance participates.
		if !account.IncludeInNetWorth && !account.IncludeBalanceInNetWorth {
			continue
		}
		value := string(account.DisplayBalance)
		if value == "" {
			return "", fmt.Errorf("account %q has no display balance", account.ID)
		}
		if !validDecimal(value) {
			return "", fmt.Errorf("account %q has invalid balance %q", account.ID, value)
		}
		amount, ok := new(big.Rat).SetString(value)
		if !ok || amount == nil {
			return "", fmt.Errorf("account %q has invalid balance %q", account.ID, value)
		}
		scale, err := decimalScale(value)
		if err != nil {
			return "", fmt.Errorf("account %q has invalid balance %q: %w", account.ID, value, err)
		}
		precision = max(precision, scale)
		if account.IsAsset {
			total.Add(total, amount)
		} else {
			total.Sub(total, amount)
		}
	}
	return Amount(total.FloatString(precision)), nil
}

func decimalScale(value string) (int, error) {
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(value), "e")
	exponent := 0
	if hasExponent {
		var err error
		exponent, err = strconv.Atoi(exponentText)
		if err != nil {
			return 0, err
		}
	}
	fractionalDigits := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		fractionalDigits = len(mantissa) - dot - 1
	}
	return max(fractionalDigits-exponent, 0), nil
}
