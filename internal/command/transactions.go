package command

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/monarch"
	"github.com/matteing/monarch-cli/internal/tui"
)

func (a *application) transactionsCommand() *cobra.Command {
	var params monarch.ListTransactionsParams
	var group string
	list := &cobra.Command{
		Use:   "list",
		Short: "List transactions",
		Long: `List transactions.

In a terminal, use left/right to change API pages and up/down to scroll.
JSON and piped output return one page; pass --cursor to continue.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateTransactionGroup(group); err != nil {
				return err
			}
			reader, err := a.service()
			if err != nil {
				return err
			}
			if a.config.Output != "json" && tui.IsTerminal(a.in) && tui.IsTerminal(a.out) {
				return tui.RunTransactions(tui.TransactionOptions{
					Context: cmd.Context(), Input: a.in, Output: a.out,
					InitialCursor: params.Cursor, PageSize: params.Limit, GroupByMonth: group == "month",
					Fetch: func(ctx context.Context, cursor string) (monarch.TransactionPage, error) {
						request := params
						request.Cursor = cursor
						return reader.ListTransactions(ctx, request)
					},
				})
			}
			result, err := reader.ListTransactions(cmd.Context(), params)
			if err != nil {
				return err
			}
			if a.config.Output == "json" {
				return writeJSON(a.out, result)
			}
			rows := make([][]string, 0, len(result.Transactions))
			for _, transaction := range result.Transactions {
				rows = append(rows, []string{
					transaction.Date,
					merchantName(transaction),
					categoryName(transaction),
					transaction.Account.DisplayName,
					string(transaction.Amount),
					transaction.ID,
				})
			}
			if err := a.writeTable([]string{"DATE", "MERCHANT", "CATEGORY", "ACCOUNT", "AMOUNT", "ID"}, rows); err != nil {
				return err
			}
			if result.NextCursor != "" {
				if _, err := fmt.Fprintf(a.out, "\nNext cursor: %s\n", result.NextCursor); err != nil {
					return err
				}
			}
			return nil
		},
	}
	addTransactionDateRangeFlags(list, &params.StartDate, &params.EndDate)
	list.Flags().StringVar(&params.Search, "search", "", "merchant or description search")
	list.Flags().StringSliceVar(&params.AccountIDs, "account-id", nil, "account ID filter; repeat or comma-separate")
	list.Flags().StringSliceVar(&params.CategoryIDs, "category-id", nil, "category ID filter; repeat or comma-separate")
	list.Flags().StringSliceVar(&params.TagIDs, "tag-id", nil, "tag ID filter; repeat or comma-separate")
	list.Flags().IntVar(&params.Limit, "limit", monarch.DefaultTransactionPageSize, fmt.Sprintf("page size (1-%d)", monarch.MaxTransactionPageSize))
	list.Flags().StringVar(&params.Cursor, "cursor", "", "opaque continuation cursor")
	list.Flags().StringVar(&group, "group", "month", "group interactive results: month or none")

	get := &cobra.Command{
		Use: "get TRANSACTION_ID", Short: "Get one transaction", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reader, err := a.service()
			if err != nil {
				return err
			}
			result, err := reader.GetTransaction(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			transaction := result.Transaction
			return a.writeResult(result,
				[]string{"DATE", "MERCHANT", "CATEGORY", "ACCOUNT", "AMOUNT", "PENDING", "ID"},
				[][]string{{
					transaction.Date,
					merchantName(transaction),
					categoryName(transaction),
					transaction.Account.DisplayName,
					string(transaction.Amount),
					strconv.FormatBool(transaction.Pending),
					transaction.ID,
				}},
			)
		},
	}
	return commandGroup("transactions", "Read and search transactions", list, get)
}

func merchantName(transaction monarch.Transaction) string {
	if transaction.Merchant == nil {
		return ""
	}
	return transaction.Merchant.Name
}

func categoryName(transaction monarch.Transaction) string {
	if transaction.Category == nil {
		return ""
	}
	return transaction.Category.Name
}
