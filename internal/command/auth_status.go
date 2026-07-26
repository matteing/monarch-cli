package command

import (
	"time"

	"github.com/spf13/cobra"
)

type authStatus struct {
	Profile   string    `json:"profile"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

func (a *application) statusCommand() *cobra.Command {
	return &cobra.Command{
		Use: "status", Short: "Check the saved session", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := a.store.Load(a.config.Profile)
			if err != nil {
				return err
			}
			if err := a.verify(cmd.Context(), value); err != nil {
				return err
			}
			result := authStatus{Profile: a.config.Profile, Mode: string(value.Mode), CreatedAt: value.CreatedAt, Status: "valid"}
			return a.writeResult(result, []string{"PROFILE", "MODE", "CREATED", "STATUS"}, [][]string{{result.Profile, result.Mode, result.CreatedAt.Format(time.RFC3339), result.Status}})
		},
	}
}
