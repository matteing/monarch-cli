package command

import "github.com/spf13/cobra"

func (a *application) categoriesCommand() *cobra.Command {
	list := &cobra.Command{
		Use: "list", Short: "List categories", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, err := a.reader()
			if err != nil {
				return err
			}
			result, err := reader.ListCategories(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(result.Categories))
			for _, category := range result.Categories {
				rows = append(rows, []string{
					category.Name,
					category.Group.Name,
					category.Group.Type,
					category.ID,
				})
			}
			return a.writeResult(result, []string{"CATEGORY", "GROUP", "TYPE", "ID"}, rows)
		},
	}
	return commandGroup("categories", "Read transaction categories", list)
}
