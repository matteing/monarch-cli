package monarch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/session"
)

func TestGraphQLRootsMustBePresentAndNonNull(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"data":null}`,
		`{"data":{}}`,
		`{"data":{"categories":null}}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return monarchResponse(http.StatusOK, body), nil
			}))
			_, err := client.ListCategories(context.Background())
			if apperr.KindOf(err) != apperr.KindUnavailable {
				t.Fatalf("error = %v (%s), want unavailable", err, apperr.KindOf(err))
			}
		})
	}
}

func TestGraphQLResponseRejectsTrailingJSON(t *testing.T) {
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"categories":[]}} {}`), nil
	}))
	_, err := client.ListCategories(context.Background())
	if apperr.KindOf(err) != apperr.KindUnavailable {
		t.Fatalf("error = %v (%s), want unavailable", err, apperr.KindOf(err))
	}
}

func TestGraphQLErrorClassificationScansEveryError(t *testing.T) {
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"errors":[{"message":"generic"},{"extensions":{"code":"UNAUTHENTICATED"}}]}`), nil
	}))
	_, err := client.ListCategories(context.Background())
	if apperr.KindOf(err) != apperr.KindAuth {
		t.Fatalf("error = %v (%s), want authentication", err, apperr.KindOf(err))
	}
}

func TestGraphQLErrorCodeClassification(t *testing.T) {
	for _, test := range []struct {
		code string
		want apperr.Kind
	}{
		{"RATE_LIMITED", apperr.KindRateLimited},
		{"NOT_FOUND", apperr.KindNotFound},
		{"BAD_USER_INPUT", apperr.KindInvalidInput},
		{"INTERNAL_SERVER_ERROR", apperr.KindUnavailable},
	} {
		t.Run(test.code, func(t *testing.T) {
			graphErrors := []graphQLError{{}}
			graphErrors[0].Extensions.Code = test.code
			err := classifyGraphQLErrors("test", graphErrors)
			if err.Kind != test.want {
				t.Fatalf("kind = %s, want %s", err.Kind, test.want)
			}
		})
	}
}

func TestRetryableHTTPStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
				if attempts.Add(1) == 1 {
					return monarchResponse(status, ""), nil
				}
				return monarchResponse(http.StatusOK, `{"data":{"categories":[]}}`), nil
			}))
			if _, err := client.ListCategories(context.Background()); err != nil {
				t.Fatal(err)
			}
			if attempts.Load() != 2 {
				t.Fatalf("attempts = %d, want 2", attempts.Load())
			}
		})
	}
}

func TestHTTPAuthenticationErrorsAreNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts.Add(1)
				return monarchResponse(status, ""), nil
			}))
			_, err := client.ListCategories(context.Background())
			if apperr.KindOf(err) != apperr.KindAuth || attempts.Load() != 1 {
				t.Fatalf("error = %v (%s), attempts = %d", err, apperr.KindOf(err), attempts.Load())
			}
		})
	}
}

func TestTransportErrorsAreRetried(t *testing.T) {
	var attempts atomic.Int32
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("temporary transport failure")
		}
		return monarchResponse(http.StatusOK, `{"data":{"categories":[]}}`), nil
	}))
	if _, err := client.ListCategories(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

func TestOversizedResponseIsRejectedWithoutRetry(t *testing.T) {
	var attempts atomic.Int32
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return monarchResponse(http.StatusOK, strings.Repeat("x", maxResponseSize+1)), nil
	}))
	_, err := client.ListCategories(context.Background())
	if apperr.KindOf(err) != apperr.KindUnavailable || attempts.Load() != 1 {
		t.Fatalf("error = %v (%s), attempts = %d", err, apperr.KindOf(err), attempts.Load())
	}
}

func TestLongRetryAfterIsNotShortened(t *testing.T) {
	var attempts atomic.Int32
	var waits atomic.Int32
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		response := monarchResponse(http.StatusTooManyRequests, "")
		response.Header.Set("Retry-After", "11")
		return response, nil
	}))
	client.retryWait = func(context.Context, int, time.Duration) error {
		waits.Add(1)
		return nil
	}
	_, err := client.ListCategories(context.Background())
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindRateLimited || appErr.RetryAfter != 11*time.Second {
		t.Fatalf("error = %#v, want rate limit with 11s retry-after", err)
	}
	if attempts.Load() != 1 || waits.Load() != 0 {
		t.Fatalf("attempts = %d, waits = %d; long retry-after must be surfaced", attempts.Load(), waits.Load())
	}
}

func TestRetryAfterOverflowIsSaturatedAndNeverRetried(t *testing.T) {
	var attempts atomic.Int32
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		response := monarchResponse(http.StatusTooManyRequests, "")
		response.Header.Set("Retry-After", strings.Repeat("9", 64))
		return response, nil
	}))
	client.retryWait = func(context.Context, int, time.Duration) error {
		t.Fatal("overflowed Retry-After must not be shortened into a local retry")
		return nil
	}
	_, err := client.ListCategories(context.Background())
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.RetryAfter != time.Duration(1<<63-1) {
		t.Fatalf("error = %#v, want saturated Retry-After", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestGraphQLErrorHonorsRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		response := monarchResponse(http.StatusOK, `{"errors":[{"extensions":{"code":"RATE_LIMITED"}}]}`)
		response.Header.Set("Retry-After", "11")
		return response, nil
	}))
	client.retryWait = func(context.Context, int, time.Duration) error {
		t.Fatal("long GraphQL Retry-After must be surfaced")
		return nil
	}
	_, err := client.ListCategories(context.Background())
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindRateLimited || appErr.RetryAfter != 11*time.Second {
		t.Fatalf("error = %#v, want rate limit with 11s retry-after", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestTransactionPageCannotExceedRequestedLimit(t *testing.T) {
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":2,"results":[{"id":"1","amount":"1"},{"id":"2","amount":"2"}]}}}`), nil
	}))
	_, err := client.ListTransactions(context.Background(), ListTransactionsParams{Limit: 1})
	if apperr.KindOf(err) != apperr.KindUnavailable {
		t.Fatalf("error = %v (%s), want unavailable", err, apperr.KindOf(err))
	}
}

func TestTransactionPageMetadataMustDescribeACompletePage(t *testing.T) {
	for name, values := range map[string][4]int{
		"results beyond total": {2, 2, 2, 1},
		"empty before total":   {1, 2, 3, 0},
		"short before total":   {0, 2, 3, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateTransactionPage(values[0], values[1], values[2], values[3]); err == nil {
				t.Fatal("inconsistent page was accepted")
			}
		})
	}
	if err := validateTransactionPage(5, 2, 3, 0); err != nil {
		t.Fatalf("empty page after concurrent deletion was rejected: %v", err)
	}
}

func TestTransactionPaginationNeverEmitsAnUndecodableOffset(t *testing.T) {
	fingerprint, err := transactionCursorFingerprint(transactionFilters{})
	if err != nil {
		t.Fatal(err)
	}
	cursor := encodeCursor(maxTransactionOffset-1, fingerprint)
	client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":10000002,"results":[{"id":"1","amount":"1","date":"2026-07-25","account":{"id":"account"}},{"id":"2","amount":"1","date":"2026-07-24","account":{"id":"account"}}]}}}`), nil
	}))
	_, err = client.ListTransactions(context.Background(), ListTransactionsParams{Limit: 2, Cursor: cursor})
	if apperr.KindOf(err) != apperr.KindUnavailable {
		t.Fatalf("pagination boundary error = %v (%s)", err, apperr.KindOf(err))
	}
}

func TestTransactionCursorIsBoundToNormalizedFilters(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, monarchRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload graphQLRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		variables := payload.Variables.(map[string]any)
		call := calls.Add(1)
		if call == 2 && variables["offset"] != float64(1) {
			t.Errorf("second offset = %#v, want 1", variables["offset"])
		}
		return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":2,"results":[{"id":"txn","amount":"1.00","date":"2026-07-25","account":{"id":"account"}}]}}}`), nil
	}))

	first, err := client.ListTransactions(context.Background(), ListTransactionsParams{
		Search: "coffee", AccountIDs: []string{"b", "a"}, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor == "" {
		t.Fatal("first page did not return a cursor")
	}
	if _, err := client.ListTransactions(context.Background(), ListTransactionsParams{
		Search: "coffee", AccountIDs: []string{"a", "b"}, Limit: 1, Cursor: first.NextCursor,
	}); err != nil {
		t.Fatalf("normalized equivalent filters rejected: %v", err)
	}
	_, err = client.ListTransactions(context.Background(), ListTransactionsParams{
		Search: "tea", AccountIDs: []string{"a", "b"}, Limit: 1, Cursor: first.NextCursor,
	})
	if apperr.KindOf(err) != apperr.KindInvalidInput {
		t.Fatalf("mismatched cursor error = %v (%s), want invalid input", err, apperr.KindOf(err))
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, mismatch should fail before HTTP", calls.Load())
	}
}

func TestRequiredAmountsCannotDecodeAsEmptyStrings(t *testing.T) {
	tests := []struct {
		name string
		body string
		call func(*Client) error
	}{
		{
			name: "account",
			body: `{"data":{"accounts":[{"display_balance":"1"}]}}`,
			call: func(client *Client) error {
				_, err := client.ListAccounts(context.Background(), ListAccountsParams{})
				return err
			},
		},
		{
			name: "transaction list",
			body: `{"data":{"allTransactions":{"totalCount":1,"results":[{"id":"txn","amount":null}]}}}`,
			call: func(client *Client) error {
				_, err := client.ListTransactions(context.Background(), ListTransactionsParams{})
				return err
			},
		},
		{
			name: "transaction detail",
			body: `{"data":{"getTransaction":{"id":"txn"}}}`,
			call: func(client *Client) error {
				_, err := client.GetTransaction(context.Background(), "txn")
				return err
			},
		},
		{
			name: "cashflow",
			body: `{"data":{"aggregates":[{"summary":{"sum_income":"1","sum_expense":"1","savings":"0"}}]}}`,
			call: func(client *Client) error {
				_, err := client.GetCashflow(context.Background(), DateRange{StartDate: "2026-07-01", EndDate: "2026-07-31"})
				return err
			},
		},
		{
			name: "budget",
			body: `{"data":{"budgetData":{"monthly_amounts_by_category":[{"monthly_amounts":[{"planned_cash_flow_amount":"1","remaining_amount":"1"}]}],"monthly_amounts_by_category_group":[]}}}`,
			call: func(client *Client) error {
				_, err := client.GetBudgets(context.Background(), MonthRange{StartMonth: "2026-07", EndMonth: "2026-07"})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return monarchResponse(http.StatusOK, test.body), nil
			}))
			err := test.call(client)
			if apperr.KindOf(err) != apperr.KindUnavailable {
				t.Fatalf("error = %v (%s), want unavailable", err, apperr.KindOf(err))
			}
		})
	}
}

func TestNullableTransactionRelationshipsStayNull(t *testing.T) {
	client := newTestClient(t, monarchRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload graphQLRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if !strings.Contains(payload.Query, "has_split_transactions: hasSplitTransactions") {
			t.Error("list projection omitted hasSplitTransactions")
		}
		if !strings.Contains(payload.Query, "notes") {
			t.Error("list projection omitted transaction notes")
		}
		if !strings.Contains(payload.Query, "split_transactions: splitTransactions") {
			t.Error("list projection omitted split transactions")
		}
		if !strings.Contains(payload.Query, "transactions_count: transactionsCount") {
			t.Error("list projection used the detail-only merchant count field")
		}
		return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":1,"results":[{"id":"txn","amount":"-1.20","date":"2026-07-25","notes":"Parent note","has_split_transactions":true,"category":null,"merchant":null,"account":{"id":"acct","display_name":"Checking"},"goal":null,"split_transactions":[{"id":"split","amount":"-1.20","notes":"Split note","category":null,"merchant":null}]}]}}}`), nil
	}))
	page, err := client.ListTransactions(context.Background(), ListTransactionsParams{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	transaction := page.Transactions[0]
	if transaction.Category != nil || transaction.Merchant != nil || transaction.Goal != nil {
		t.Fatalf("nullable relationships were fabricated: %+v", transaction)
	}
	if transaction.Notes != "Parent note" || len(transaction.SplitTransactions) != 1 ||
		transaction.SplitTransactions[0].Notes != "Split note" {
		t.Fatalf("list projection lost notes or splits: %+v", transaction)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"transactions":[{"id":"txn","amount":"-1.20","pending":false,"date":"2026-07-25","hide_from_reports":false,"notes":"Parent note","is_recurring":false,"needs_review":false,"has_split_transactions":true,"is_split_transaction":false,"split_transactions":[{"id":"split","amount":"-1.20","notes":"Split note","category":null,"merchant":null}],"category":null,"merchant":null,"account":{"id":"acct","display_name":"Checking"}}],"total_count":1}`
	if string(encoded) != want {
		t.Fatalf("projection mismatch\n got: %s\nwant: %s", encoded, want)
	}
}

func TestTransactionDetailProjectionMatchesModel(t *testing.T) {
	client := newTestClient(t, monarchRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload graphQLRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if !strings.Contains(payload.Query, "data_provider_description: dataProviderDescription") {
			t.Error("detail projection omitted dataProviderDescription")
		}
		if !strings.Contains(payload.Query, "notes") {
			t.Error("detail projection omitted transaction notes")
		}
		if !strings.Contains(payload.Query, "transactions_count: transactionCount") {
			t.Error("detail projection used the list-only merchant count field")
		}
		return monarchResponse(http.StatusOK, `{"data":{"getTransaction":{"id":"txn","amount":"-3.00","date":"2026-07-25","data_provider_description":"CARD PURCHASE","notes":"Parent note","has_split_transactions":true,"category":{"id":"cat","name":"Food","order":2,"icon":"fork","system_category":"FOOD","is_system_category":true,"is_disabled":false,"updated_at":"u","created_at":"c","group":{"id":"group","name":"Living","type":"expense"}},"merchant":{"id":"merchant","name":"Cafe","transactions_count":7},"account":{"id":"account","display_name":"Checking"},"split_transactions":[{"id":"split","amount":"-3.00","notes":"Split note","category":null,"merchant":null}],"goal":null}}}`), nil
	}))
	result, err := client.GetTransaction(context.Background(), "txn")
	if err != nil {
		t.Fatal(err)
	}
	transaction := result.Transaction
	if transaction.DataProviderDescription != "CARD PURCHASE" || transaction.Notes != "Parent note" || !transaction.HasSplitTransactions {
		t.Fatalf("detail projection lost scalar fields: %+v", transaction)
	}
	if transaction.Category == nil || transaction.Category.Group.Type != "expense" || transaction.Merchant == nil || transaction.Merchant.TransactionsCount != 7 {
		t.Fatalf("detail projection lost relationship fields: %+v", transaction)
	}
	if len(transaction.SplitTransactions) != 1 || transaction.SplitTransactions[0].Notes != "Split note" ||
		transaction.SplitTransactions[0].Category != nil || transaction.SplitTransactions[0].Merchant != nil {
		t.Fatalf("nullable split relationships were fabricated: %+v", transaction.SplitTransactions)
	}
	if transaction.Goal != nil {
		t.Fatalf("null goal became %+v", transaction.Goal)
	}
}

func TestNewClientRejectsRedirectsAndInvalidTimeout(t *testing.T) {
	if _, err := NewClient(mustTokenSession(t, "token"), 0); apperr.KindOf(err) != apperr.KindInvalidInput {
		t.Fatalf("zero timeout error = %v (%s)", err, apperr.KindOf(err))
	}
	client, err := NewClient(mustTokenSession(t, "token"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, "https://api.monarch.com/graphql", nil)
	redirect, _ := http.NewRequest(http.MethodPost, "https://example.test/elsewhere", nil)
	if err := client.httpClient.CheckRedirect(redirect, []*http.Request{request}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect error = %v, want http.ErrUseLastResponse", err)
	}
}

func TestFinancialOverviewFallsBackToAllAccountBalances(t *testing.T) {
	fixed := time.Date(2026, time.July, 25, 15, 4, 5, 0, time.FixedZone("local", -7*60*60))
	client := newTestClient(t, monarchRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload graphQLRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		switch {
		case strings.Contains(payload.Query, "GetAccounts"):
			return monarchResponse(http.StatusOK, `{"data":{"accounts":[{"id":"visible","type":{"name":"depository"},"subtype":{"name":"checking"},"display_balance":"10","current_balance":"10","deactivated_at":null,"is_hidden":false,"is_asset":true,"include_in_net_worth":true,"include_balance_in_net_worth":false},{"id":"hidden","type":{"name":"depository"},"subtype":{"name":"checking"},"display_balance":"4","current_balance":"4","deactivated_at":null,"is_hidden":true,"is_asset":false,"include_in_net_worth":false,"include_balance_in_net_worth":true},{"id":"old","type":{"name":"depository"},"subtype":{"name":"checking"},"display_balance":"2","current_balance":"2","deactivated_at":"2020-01-01","is_hidden":false,"is_asset":true,"include_in_net_worth":true,"include_balance_in_net_worth":false}]}}`), nil
		case strings.Contains(payload.Query, "GetCashflowSummary"):
			return monarchResponse(http.StatusOK, `{"data":{"aggregates":[{"summary":{"sum_income":"1","sum_expense":"1","savings":"0","savings_rate":"0"}}]}}`), nil
		case strings.Contains(payload.Query, "GetJointPlanningData"):
			return monarchResponse(http.StatusOK, `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[]}}}`), nil
		case strings.Contains(payload.Query, "GetTransactionsList"):
			return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":0,"results":[]}}}`), nil
		default:
			t.Errorf("unexpected query: %s", payload.Query)
			return monarchResponse(http.StatusBadRequest, ""), nil
		}
	}))
	client.now = func() time.Time { return fixed }
	overview, err := client.GetFinancialOverview(context.Background(), DateRange{})
	if err != nil {
		t.Fatal(err)
	}
	if overview.NetWorth != "8.00" {
		t.Fatalf("net worth = %q, want 8.00", overview.NetWorth)
	}
	if len(overview.Accounts) != 1 || overview.Accounts[0].ID != "visible" {
		t.Fatalf("visible accounts = %+v", overview.Accounts)
	}
	if overview.AsOf != "2026-07-25T22:04:05Z" {
		t.Fatalf("as_of = %q", overview.AsOf)
	}
}

func newTestClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()
	return newTestClientWithSession(t, mustTokenSession(t, "token"), transport)
}

func newTestClientWithSession(t *testing.T, value session.Session, transport http.RoundTripper) *Client {
	t.Helper()
	client, err := newClient(value, &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	client.retryWait = noRetryWait
	return client
}
