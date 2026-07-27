package monarch

import (
	"context"
	"errors"

	"github.com/matteing/monarch-cli/internal/apperr"
)

type refreshAccountsVariables struct {
	Input struct {
		AccountIDs []string `json:"accountIds"`
	} `json:"input"`
}

type refreshAccountsData struct {
	Refresh responseField[refreshAccountsPayload] `json:"forceRefreshAccounts"`
}

type refreshAccountsPayload struct {
	Success responseField[bool]   `json:"success"`
	Errors  []refreshPayloadError `json:"errors"`
}

type refreshPayloadError struct {
	Code string `json:"code"`
}

// RefreshAccounts asks Monarch to asynchronously sync the selected accounts.
func (c *Client) RefreshAccounts(ctx context.Context, params RefreshAccountsParams) (AccountRefreshResult, error) {
	const op = "refresh accounts"
	if err := ValidateAccountRefreshIDs(params.AccountIDs); err != nil {
		return AccountRefreshResult{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), nil)
	}

	var variables refreshAccountsVariables
	variables.Input.AccountIDs = append([]string(nil), params.AccountIDs...)
	data, err := executeMutation[refreshAccountsData](ctx, c, op, refreshAccountsMutation, variables)
	if err != nil {
		return AccountRefreshResult{}, err
	}
	if !data.Refresh.Present || data.Refresh.Null || !data.Refresh.Value.Success.Present || data.Refresh.Value.Success.Null {
		return AccountRefreshResult{}, unexpectedResponse(op, "GraphQL data.forceRefreshAccounts.success is missing or null")
	}
	if !data.Refresh.Value.Success.Value {
		errorCodes := make([]string, 0, len(data.Refresh.Value.Errors))
		for _, payloadErr := range data.Refresh.Value.Errors {
			errorCodes = append(errorCodes, payloadErr.Code)
		}
		if len(errorCodes) > 0 {
			return AccountRefreshResult{}, classifyErrorCodes(op, errorCodes)
		}
		return AccountRefreshResult{}, apperr.New(apperr.KindUnavailable, op, "Monarch did not accept the account refresh request", errors.New("refresh payload reported failure"))
	}
	return AccountRefreshResult{Accepted: true, AccountIDs: append([]string(nil), params.AccountIDs...)}, nil
}
