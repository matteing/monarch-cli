package command

import "github.com/spf13/cobra"

func (a *application) mcpCommand() *cobra.Command {
	return &cobra.Command{
		Use: "mcp", Short: "Serve the read-only MCP protocol over stdio", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, err := a.reader()
			if err != nil {
				return err
			}
			a.logger.Debug("starting MCP server", "profile", a.config.Profile)
			return a.runMCP(cmd.Context(), reader, a.version, a.in, a.out, a.logger)
		},
	}
}
