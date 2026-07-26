package monarch

import "context"

type accountsData struct {
	Accounts responseField[[]accountResponse] `json:"accounts"`
}

// accountResponse tracks privacy- and aggregation-sensitive fields whose zero
// values are meaningful. GraphQL must send each selected field explicitly.
type accountResponse struct {
	Account
	DeactivatedAt            responseField[string] `json:"deactivated_at"`
	IsHidden                 responseField[bool]   `json:"is_hidden"`
	IsAsset                  responseField[bool]   `json:"is_asset"`
	IncludeInNetWorth        responseField[bool]   `json:"include_in_net_worth"`
	IncludeBalanceInNetWorth responseField[bool]   `json:"include_balance_in_net_worth"`
}

// ListAccounts returns accounts, applying privacy and lifecycle filters locally.
func (c *Client) ListAccounts(ctx context.Context, params ListAccountsParams) (AccountsResult, error) {
	const op = "list accounts"
	data, err := execute[accountsData](ctx, c, op, accountsQuery, nil)
	if err != nil {
		return AccountsResult{}, err
	}
	if !data.Accounts.Present || data.Accounts.Null {
		return AccountsResult{}, unexpectedResponse(op, "GraphQL data.accounts is missing or null")
	}
	accounts, err := validateAccountResponses(data.Accounts.Value)
	if err != nil {
		return AccountsResult{}, unexpectedResponse(op, err.Error())
	}
	return AccountsResult{Accounts: filterAccounts(accounts, params)}, nil
}

func filterAccounts(accounts []Account, params ListAccountsParams) []Account {
	result := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if !params.IncludeHidden && account.IsHidden {
			continue
		}
		if !params.IncludeDeactivated && account.DeactivatedAt != "" {
			continue
		}
		result = append(result, account)
	}
	return result
}
