package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/tui"
)

type logoutResult struct {
	Profile string `json:"profile"`
	Deleted bool   `json:"deleted"`
}

func (a *application) logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use: "logout", Short: "Delete the saved session", Args: noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := a.store.Delete(a.config.Profile); err != nil {
				return err
			}
			if a.config.Output == "json" {
				return writeJSON(a.out, logoutResult{Profile: a.config.Profile, Deleted: true})
			}
			message := fmt.Sprintf("Deleted session for profile %q.", a.config.Profile)
			if tui.IsTerminal(a.out) {
				return tui.WriteSuccess(a.out, message)
			}
			_, err := fmt.Fprintln(a.out, message)
			return err
		},
	}
}
