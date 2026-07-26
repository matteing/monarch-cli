package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/monarch"
)

func addDateRangeFlags(command *cobra.Command, dates *monarch.DateRange) {
	addPairedRangeFlags(command, &dates.StartDate, &dates.EndDate, "date", "YYYY-MM-DD", "for the current month")
}

func addTransactionDateRangeFlags(command *cobra.Command, start, end *string) {
	addPairedRangeFlags(command, start, end, "date", "YYYY-MM-DD", "to leave dates unfiltered")
}

func addMonthRangeFlags(command *cobra.Command, months *monarch.MonthRange) {
	addPairedRangeFlags(command, &months.StartMonth, &months.EndMonth, "month", "YYYY-MM", "for the current month")
}

func addPairedRangeFlags(command *cobra.Command, start, end *string, unit, layout, omittedMeaning string) {
	command.Flags().StringVar(start, "start-"+unit, "", fmt.Sprintf(
		"inclusive start %s (%s); provide with --end-%s, or omit both %s; range cannot exceed ten years",
		unit, layout, unit, omittedMeaning,
	))
	command.Flags().StringVar(end, "end-"+unit, "", fmt.Sprintf(
		"inclusive end %s (%s); provide with --start-%s, or omit both %s; range cannot exceed ten years",
		unit, layout, unit, omittedMeaning,
	))
}

func validateTransactionGroup(group string) error {
	if group != "month" && group != "none" {
		return apperr.New(apperr.KindInvalidInput, "list transactions", "group must be month or none", nil)
	}
	return nil
}
