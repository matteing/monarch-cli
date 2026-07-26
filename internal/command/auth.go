package command

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/session"
)

func (a *application) authCommand() *cobra.Command {
	return commandGroup("auth", "Manage the saved Monarch session", a.loginCommand(), a.statusCommand(), a.logoutCommand())
}

func (a *application) verify(ctx context.Context, value session.Session) error {
	client, err := a.newReader(value, a.config.Timeout)
	if err != nil {
		return err
	}
	return a.verifyReader(ctx, client)
}
