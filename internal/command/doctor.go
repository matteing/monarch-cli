package command

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/session"
)

type doctorResult struct {
	Profile string          `json:"profile"`
	Healthy bool            `json:"healthy"`
	Checks  []doctorCheck   `json:"checks"`
	Error   *apperr.Details `json:"error,omitempty"`
}

type doctorCheck struct {
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Detail string          `json:"detail"`
	Error  *apperr.Details `json:"error,omitempty"`
}

func (a *application) doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use: "doctor", Short: "Verify keyring, session, and API access", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := doctorResult{Profile: a.config.Profile, Healthy: false, Checks: []doctorCheck{
				{Name: "keyring", Status: "pending"},
				{Name: "session", Status: "pending"},
				{Name: "api", Status: "pending"},
			}}
			value, err := a.store.Load(a.config.Profile)
			if err != nil {
				switch {
				case errors.Is(err, session.ErrNotFound):
					result.Checks[0] = doctorCheck{Name: "keyring", Status: "ok", Detail: "credential store is accessible"}
					result.Checks[1] = doctorErrorCheck("session", err)
					result.Checks[2] = doctorCheck{Name: "api", Status: "skipped", Detail: "no saved session to verify"}
				case errors.Is(err, session.ErrInvalidSession):
					result.Checks[0] = doctorCheck{Name: "keyring", Status: "ok", Detail: "credential store is accessible"}
					result.Checks[1] = doctorErrorCheck("session", err)
					result.Checks[2] = doctorCheck{Name: "api", Status: "skipped", Detail: "saved session is invalid"}
				default:
					result.Checks[0] = doctorErrorCheck("keyring", err)
					result.Checks[1] = doctorCheck{Name: "session", Status: "skipped", Detail: "keyring check failed"}
					result.Checks[2] = doctorCheck{Name: "api", Status: "skipped", Detail: "keyring check failed"}
				}
				return a.writeDoctorResult(result, err)
			}

			result.Checks[0] = doctorCheck{Name: "keyring", Status: "ok", Detail: "saved session loaded"}
			if verifyErr := a.verify(cmd.Context(), value); verifyErr != nil {
				if apperr.KindOf(verifyErr) == apperr.KindAuth {
					result.Checks[1] = doctorErrorCheck("session", verifyErr)
					result.Checks[2] = doctorCheck{Name: "api", Status: "skipped", Detail: "session authentication failed"}
				} else {
					result.Checks[1] = doctorCheck{Name: "session", Status: "ok", Detail: "saved session is structurally valid"}
					result.Checks[2] = doctorErrorCheck("api", verifyErr)
				}
				return a.writeDoctorResult(result, verifyErr)
			}
			result.Healthy = true
			result.Checks[1] = doctorCheck{Name: "session", Status: "ok", Detail: "session is valid"}
			result.Checks[2] = doctorCheck{Name: "api", Status: "ok", Detail: "Monarch API is reachable"}
			return a.writeDoctorResult(result, nil)
		},
	}
}

func doctorErrorCheck(name string, err error) doctorCheck {
	details := apperr.Describe(err)
	return doctorCheck{Name: name, Status: "error", Detail: details.Message, Error: &details}
}

func (a *application) writeDoctorResult(result doctorResult, cause error) error {
	if cause != nil {
		details := apperr.Describe(cause)
		result.Error = &details
	}
	var err error
	if a.config.Output == "json" {
		err = writeJSON(a.out, result)
	} else {
		rows := make([][]string, 0, len(result.Checks))
		for _, check := range result.Checks {
			rows = append(rows, []string{check.Name, check.Status, check.Detail})
		}
		err = a.writeTable([]string{"CHECK", "STATUS", "DETAIL"}, rows)
	}
	if err != nil {
		return err
	}
	if cause != nil {
		return reportedError{cause: cause}
	}
	return nil
}

type reportedError struct{ cause error }

func (e reportedError) Error() string { return e.cause.Error() }
func (e reportedError) Unwrap() error { return e.cause }
func (reportedError) Reported() bool  { return true }
