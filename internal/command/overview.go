package command

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/monarch"
)

func (a *application) overviewCommand() *cobra.Command {
	var dates monarch.DateRange
	command := &cobra.Command{
		Use: "overview", Short: "Get a compact financial overview", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, err := a.reader()
			if err != nil {
				return err
			}
			result, err := reader.GetFinancialOverview(cmd.Context(), dates)
			if err != nil {
				return err
			}
			return a.writeResult(result,
				[]string{"AS OF", "NET WORTH", "INCOME", "EXPENSE", "SAVINGS", "ACCOUNTS", "TRANSACTIONS"},
				[][]string{{
					result.AsOf,
					string(result.NetWorth),
					string(result.Cashflow.SumIncome),
					string(result.Cashflow.SumExpense),
					string(result.Cashflow.Savings),
					strconv.Itoa(len(result.Accounts)),
					strconv.Itoa(len(result.Transactions)),
				}},
			)
		},
	}
	addDateRangeFlags(command, &dates)
	return command
}
