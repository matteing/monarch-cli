package command

import (
	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/monarch"
)

func (a *application) budgetsCommand() *cobra.Command {
	var months monarch.MonthRange
	get := &cobra.Command{
		Use: "get", Short: "Get budgets for an inclusive month range", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, err := a.reader()
			if err != nil {
				return err
			}
			result, err := reader.GetBudgets(cmd.Context(), months)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(result.Categories)+len(result.Groups))
			for _, category := range result.Categories {
				for _, amount := range category.MonthlyAmounts {
					rows = append(rows, []string{
						amount.Month,
						"category",
						category.Category.Name,
						string(amount.PlannedCashFlowAmount),
						string(amount.ActualAmount),
						string(amount.RemainingAmount),
					})
				}
			}
			for _, group := range result.Groups {
				for _, amount := range group.MonthlyAmounts {
					rows = append(rows, []string{
						amount.Month,
						"group",
						group.CategoryGroup.Name,
						string(amount.PlannedCashFlowAmount),
						string(amount.ActualAmount),
						string(amount.RemainingAmount),
					})
				}
			}
			return a.writeResult(result, []string{"MONTH", "SCOPE", "NAME", "PLANNED", "ACTUAL", "REMAINING"}, rows)
		},
	}
	addMonthRangeFlags(get, &months)
	return commandGroup("budgets", "Read budget planning data", get)
}
