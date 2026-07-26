package command

import (
	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/monarch"
)

func (a *application) accountsCommand() *cobra.Command {
	var includeHidden, includeDeactivated bool
	list := &cobra.Command{
		Use: "list", Short: "List accounts", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, err := a.reader()
			if err != nil {
				return err
			}
			result, err := reader.ListAccounts(cmd.Context(), monarch.ListAccountsParams{
				IncludeHidden:      includeHidden,
				IncludeDeactivated: includeDeactivated,
			})
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(result.Accounts))
			for _, account := range result.Accounts {
				rows = append(rows, []string{
					account.DisplayName,
					account.Type.Display,
					string(account.DisplayBalance),
					account.Mask,
					account.ID,
				})
			}
			return a.writeResult(result, []string{"NAME", "TYPE", "BALANCE", "MASK", "ID"}, rows)
		},
	}
	list.Flags().BoolVar(&includeHidden, "include-hidden", false, "include hidden accounts")
	list.Flags().BoolVar(&includeDeactivated, "include-deactivated", false, "include deactivated accounts")
	return commandGroup("accounts", "Read account balances", list)
}
