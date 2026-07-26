package monarch

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/matteing/monarch-cli/internal/apperr"
)

func TestAccountElementsAndRequiredTypesAreValidated(t *testing.T) {
	for name, body := range map[string]string{
		"null element": `{"data":{"accounts":[null]}}`,
		"null type":    `{"data":{"accounts":[{"id":"account","type":null,"subtype":{"name":"checking"},"display_balance":"1","current_balance":"1"}]}}`,
		"null subtype": `{"data":{"accounts":[{"id":"account","type":{"name":"depository"},"subtype":null,"display_balance":"1","current_balance":"1"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			assertUnavailableResponse(t, body, func(client *Client) error {
				_, err := client.ListAccounts(context.Background(), ListAccountsParams{})
				return err
			})
		})
	}
}

func TestAccountPrivacyAndNetWorthFieldsRequirePresence(t *testing.T) {
	valid := `{"id":"account","type":{"name":"depository"},"subtype":{"name":"checking"},"display_balance":"1","current_balance":"1","deactivated_at":null,"is_hidden":false,"is_asset":true,"include_in_net_worth":true,"include_balance_in_net_worth":false}`
	for _, test := range []struct {
		name     string
		fragment string
	}{
		{"deactivated_at", `"deactivated_at":null,`},
		{"is_hidden", `"is_hidden":false,`},
		{"is_asset", `"is_asset":true,`},
		{"include_in_net_worth", `"include_in_net_worth":true,`},
		{"include_balance_in_net_worth", `,"include_balance_in_net_worth":false`},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := strings.Replace(valid, test.fragment, "", 1)
			assertUnavailableResponse(t, `{"data":{"accounts":[`+account+`]}}`, func(client *Client) error {
				_, err := client.ListAccounts(context.Background(), ListAccountsParams{})
				return err
			})
		})
	}

	nullHidden := strings.Replace(valid, `"is_hidden":false`, `"is_hidden":null`, 1)
	assertUnavailableResponse(t, `{"data":{"accounts":[`+nullHidden+`]}}`, func(client *Client) error {
		_, err := client.ListAccounts(context.Background(), ListAccountsParams{})
		return err
	})

	legacy := strings.Replace(valid, `"include_in_net_worth":true`, `"include_in_net_worth":null`, 1)
	legacy = strings.Replace(legacy, `"include_balance_in_net_worth":false`, `"include_balance_in_net_worth":true`, 1)
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"accounts":[`+legacy+`]}}`), nil
	}))
	result, err := client.ListAccounts(context.Background(), ListAccountsParams{})
	if err != nil || len(result.Accounts) != 1 || !result.Accounts[0].IncludeBalanceInNetWorth {
		t.Fatalf("legacy inclusion fields = %+v, error = %v", result.Accounts, err)
	}

	bothNull := strings.Replace(legacy, `"include_balance_in_net_worth":true`, `"include_balance_in_net_worth":null`, 1)
	assertUnavailableResponse(t, `{"data":{"accounts":[`+bothNull+`]}}`, func(client *Client) error {
		_, err := client.ListAccounts(context.Background(), ListAccountsParams{})
		return err
	})
}

func TestCategoryElementsAndRequiredGroupsAreValidated(t *testing.T) {
	for name, body := range map[string]string{
		"null element": `{"data":{"categories":[null]}}`,
		"null group":   `{"data":{"categories":[{"id":"category","name":"Food","group":null}]}}`,
		"empty group":  `{"data":{"categories":[{"id":"category","name":"Food","group":{}}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			assertUnavailableResponse(t, body, func(client *Client) error {
				_, err := client.ListCategories(context.Background())
				return err
			})
		})
	}
}

func TestBudgetElementsAndRequiredOwnersAreValidated(t *testing.T) {
	tests := map[string]string{
		"null category budget":  `{"data":{"budgetData":{"monthly_amounts_by_category":[null],"monthly_amounts_by_category_group":[]}}}`,
		"null category":         `{"data":{"budgetData":{"monthly_amounts_by_category":[{"category":null,"monthly_amounts":[]}],"monthly_amounts_by_category_group":[]}}}`,
		"null category amounts": `{"data":{"budgetData":{"monthly_amounts_by_category":[{"category":{"id":"category"},"monthly_amounts":null}],"monthly_amounts_by_category_group":[]}}}`,
		"null category amount":  `{"data":{"budgetData":{"monthly_amounts_by_category":[{"category":{"id":"category"},"monthly_amounts":[null]}],"monthly_amounts_by_category_group":[]}}}`,
		"missing amount month":  `{"data":{"budgetData":{"monthly_amounts_by_category":[{"category":{"id":"category"},"monthly_amounts":[{"planned_cash_flow_amount":"1","actual_amount":"1","remaining_amount":"0"}]}],"monthly_amounts_by_category_group":[]}}}`,
		"null group budget":     `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[null]}}}`,
		"null category group":   `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[{"category_group":null,"monthly_amounts":[]}]}}}`,
		"null group amounts":    `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[{"category_group":{"id":"group"},"monthly_amounts":null}]}}}`,
		"null group amount":     `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[{"category_group":{"id":"group"},"monthly_amounts":[null]}]}}}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			assertUnavailableResponse(t, body, func(client *Client) error {
				_, err := client.GetBudgets(context.Background(), MonthRange{StartMonth: "2026-07", EndMonth: "2026-07"})
				return err
			})
		})
	}
}

func TestTransactionElementsAndRequiredRelationshipsAreValidated(t *testing.T) {
	listTests := map[string]string{
		"null transaction": `{"data":{"allTransactions":{"totalCount":1,"results":[null]}}}`,
		"null account":     `{"data":{"allTransactions":{"totalCount":1,"results":[{"id":"transaction","amount":"1","date":"2026-07-25","account":null}]}}}`,
		"null category group": `{"data":{"allTransactions":{"totalCount":1,"results":[{
			"id":"transaction","amount":"1","date":"2026-07-25","account":{"id":"account"},
			"category":{"id":"category","group":null}
		}]}}}`,
		"empty merchant": `{"data":{"allTransactions":{"totalCount":1,"results":[{
			"id":"transaction","amount":"1","date":"2026-07-25","account":{"id":"account"},"merchant":{}
		}]}}}`,
		"empty goal": `{"data":{"allTransactions":{"totalCount":1,"results":[{
			"id":"transaction","amount":"1","date":"2026-07-25","account":{"id":"account"},"goal":{}
		}]}}}`,
		"null tag": `{"data":{"allTransactions":{"totalCount":1,"results":[{
			"id":"transaction","amount":"1","date":"2026-07-25","account":{"id":"account"},"tags":[null]
		}]}}}`,
	}
	for name, body := range listTests {
		t.Run(name, func(t *testing.T) {
			assertUnavailableResponse(t, body, func(client *Client) error {
				_, err := client.ListTransactions(context.Background(), ListTransactionsParams{Limit: 1})
				return err
			})
		})
	}

	detailTests := map[string]string{
		"null attachment": `{"data":{"getTransaction":{"id":"transaction","amount":"1","date":"2026-07-25","account":{"id":"account"},"attachments":[null]}}}`,
		"null split":      `{"data":{"getTransaction":{"id":"transaction","amount":"1","date":"2026-07-25","account":{"id":"account"},"split_transactions":[null]}}}`,
	}
	for name, body := range detailTests {
		t.Run(name, func(t *testing.T) {
			assertUnavailableResponse(t, body, func(client *Client) error {
				_, err := client.GetTransaction(context.Background(), "transaction")
				return err
			})
		})
	}
}

func TestCashflowElementsAreValidated(t *testing.T) {
	assertUnavailableResponse(t, `{"data":{"aggregates":[null]}}`, func(client *Client) error {
		_, err := client.GetCashflow(context.Background(), DateRange{StartDate: "2026-07-01", EndDate: "2026-07-31"})
		return err
	})
}

func assertUnavailableResponse(t *testing.T, body string, call func(*Client) error) {
	t.Helper()
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, body), nil
	}))
	err := call(client)
	if apperr.KindOf(err) != apperr.KindUnavailable {
		t.Fatalf("error = %v (%s), want unavailable", err, apperr.KindOf(err))
	}
}
