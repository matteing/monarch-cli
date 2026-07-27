package command

import (
	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/monarch"
)

func (a *application) cashflowCommand() *cobra.Command {
	var dates monarch.DateRange
	summary := &cobra.Command{
		Use: "summary", Short: "Summarize cashflow for an inclusive date range", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, err := a.service()
			if err != nil {
				return err
			}
			result, err := reader.GetCashflow(cmd.Context(), dates)
			if err != nil {
				return err
			}
			return a.writeResult(result,
				[]string{"START", "END", "INCOME", "EXPENSE", "SAVINGS", "RATE"},
				[][]string{{
					result.StartDate,
					result.EndDate,
					string(result.SumIncome),
					string(result.SumExpense),
					string(result.Savings),
					string(result.SavingsRate),
				}},
			)
		},
	}
	addDateRangeFlags(summary, &dates)
	return commandGroup("cashflow", "Read cashflow aggregates", summary)
}
