package monarch

import (
	"context"
	"time"

	"github.com/matteing/monarch-cli/internal/apperr"
)

type budgetDataValue struct {
	Categories responseField[[]CategoryBudget] `json:"monthly_amounts_by_category"`
	Groups     responseField[[]GroupBudget]    `json:"monthly_amounts_by_category_group"`
}

type budgetsData struct {
	BudgetData responseField[budgetDataValue] `json:"budgetData"`
}

type budgetsVariables struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GetBudgets returns budget data for an inclusive month range.
func (c *Client) GetBudgets(ctx context.Context, months MonthRange) (BudgetReport, error) {
	return c.getBudgetsAt(ctx, months, c.now())
}

func (c *Client) getBudgetsAt(ctx context.Context, months MonthRange, now time.Time) (BudgetReport, error) {
	const op = "get budgets"
	start, end, err := normalizeMonthRange(months, now)
	if err != nil {
		return BudgetReport{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	endTime, err := parseMonth(end)
	if err != nil {
		return BudgetReport{}, apperr.New(apperr.KindInternal, op, "normalized budget month is invalid", err)
	}
	endDate := endTime.AddDate(0, 1, -1).Format("2006-01-02")
	data, err := execute[budgetsData](ctx, c, op, budgetsQuery, budgetsVariables{StartDate: start + "-01", EndDate: endDate})
	if err != nil {
		return BudgetReport{}, err
	}
	if !data.BudgetData.Present || data.BudgetData.Null {
		return BudgetReport{}, unexpectedResponse(op, "GraphQL data.budgetData is missing or null")
	}
	budget := data.BudgetData.Value
	if !budget.Categories.Present || budget.Categories.Null || !budget.Groups.Present || budget.Groups.Null {
		return BudgetReport{}, unexpectedResponse(op, "GraphQL budget category or group amounts are missing or null")
	}
	if err := validateBudgets(budget.Categories.Value, budget.Groups.Value); err != nil {
		return BudgetReport{}, unexpectedResponse(op, err.Error())
	}
	return BudgetReport{
		StartMonth: start,
		EndMonth:   end,
		Categories: budget.Categories.Value,
		Groups:     budget.Groups.Value,
	}, nil
}

type cashflowAggregate struct {
	Summary responseField[CashflowSummary] `json:"summary"`
}

type cashflowData struct {
	Aggregates responseField[[]cashflowAggregate] `json:"aggregates"`
}

type cashflowVariables struct {
	Filters transactionFilters `json:"filters"`
}

// GetCashflow returns aggregate cashflow for an inclusive date range. Empty
// dates default to the current month in the process's local timezone.
func (c *Client) GetCashflow(ctx context.Context, dates DateRange) (CashflowSummary, error) {
	return c.getCashflowAt(ctx, dates, c.now())
}

func (c *Client) getCashflowAt(ctx context.Context, dates DateRange, now time.Time) (CashflowSummary, error) {
	const op = "get cashflow"
	start, end := dates.StartDate, dates.EndDate
	if start == "" && end == "" {
		start, end = currentMonthDates(now)
	}
	if err := validateDateRange(start, end, false); err != nil {
		return CashflowSummary{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	variables := cashflowVariables{Filters: transactionFilters{StartDate: start, EndDate: end}}
	data, err := execute[cashflowData](ctx, c, op, cashflowQuery, variables)
	if err != nil {
		return CashflowSummary{}, err
	}
	if !data.Aggregates.Present || data.Aggregates.Null || len(data.Aggregates.Value) == 0 {
		return CashflowSummary{}, unexpectedResponse(op, "GraphQL data.aggregates is missing, null, or empty")
	}
	if err := validateCashflowAggregates(data.Aggregates.Value); err != nil {
		return CashflowSummary{}, unexpectedResponse(op, err.Error())
	}
	summary := data.Aggregates.Value[0].Summary
	result := summary.Value
	result.StartDate, result.EndDate = start, end
	return result, nil
}
