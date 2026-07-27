package command

import "github.com/spf13/cobra"

func (a *application) mcpCommand() *cobra.Command {
	return &cobra.Command{
		Use: "mcp", Short: "Serve the Monarch MCP protocol over stdio", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := a.service()
			if err != nil {
				return err
			}
			a.logger.Debug("starting MCP server", "profile", a.config.Profile)
			return a.runMCP(cmd.Context(), service, a.version, a.in, a.out, a.logger)
		},
	}
}
