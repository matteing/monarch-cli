package monarch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matteing/monarch-cli/internal/session"
)

func TestListAccountsUsesTokenAndFiltersHidden(t *testing.T) {
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Token token-value" {
			t.Errorf("Authorization = %q", got)
		}
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if !strings.Contains(request.Query, "query GetAccounts") {
			t.Errorf("unexpected query: %s", request.Query)
		}
		return monarchResponse(http.StatusOK, `{"data":{"accounts":[{"id":"visible","display_name":"Checking","type":{"name":"depository"},"subtype":{"name":"checking"},"display_balance":12.34,"current_balance":12.34,"deactivated_at":null,"is_hidden":false,"is_asset":true,"include_in_net_worth":true,"include_balance_in_net_worth":false},{"id":"hidden","display_name":"Secret","type":{"name":"depository"},"subtype":{"name":"checking"},"display_balance":1,"current_balance":1,"deactivated_at":null,"is_hidden":true,"is_asset":true,"include_in_net_worth":true,"include_balance_in_net_worth":false}]}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token-value"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ListAccounts(context.Background(), ListAccountsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].DisplayName != "Checking" || result.Accounts[0].DisplayBalance != "12.34" {
		t.Fatalf("unexpected accounts: %+v", result.Accounts)
	}
}

func TestCookieSessionHeaders(t *testing.T) {
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Csrftoken") != "csrf" {
			t.Errorf("missing CSRF header")
		}
		if r.Header.Get("Origin") != "https://app.monarch.com" {
			t.Errorf("missing Origin header")
		}
		cookie := r.Header.Get("Cookie")
		if !strings.Contains(cookie, "session_id=sid") || !strings.Contains(cookie, "csrftoken=csrf") {
			t.Errorf("Cookie = %q", cookie)
		}
		return monarchResponse(http.StatusOK, `{"data":{"categories":[]}}`), nil
	})
	client, err := newClient(mustCookieSession(t, map[string]string{"session_id": "sid", "csrftoken": "csrf"}), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	client.retryWait = noRetryWait
	if _, err := client.ListCategories(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func noRetryWait(context.Context, int, time.Duration) error { return nil }

func TestTransientResponsesAreRetried(t *testing.T) {
	var attempts atomic.Int32
	transport := monarchRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		if attempts.Add(1) < 3 {
			return monarchResponse(http.StatusServiceUnavailable, ""), nil
		}
		return monarchResponse(http.StatusOK, `{"data":{"categories":[]}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Timeout: 3 * time.Second, Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	client.retryWait = noRetryWait
	if _, err := client.ListCategories(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

func TestTransactionsReturnOpaqueNextCursor(t *testing.T) {
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		variables := request.Variables.(map[string]any)
		if variables["limit"] != float64(2) {
			t.Errorf("limit = %#v", variables["limit"])
		}
		return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":3,"results":[{"id":"1","amount":1.23,"date":"2026-07-25","account":{"id":"account"}},{"id":"2","amount":"4.56","date":"2026-07-24","account":{"id":"account"}}]}}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ListTransactions(context.Background(), ListTransactionsParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Transactions) != 2 || result.NextCursor == "" {
		t.Fatalf("unexpected page: %+v", result)
	}
	fingerprint, err := transactionCursorFingerprint(transactionFilters{})
	if err != nil {
		t.Fatal(err)
	}
	offset, err := decodeCursor(result.NextCursor, fingerprint)
	if err != nil || offset != 2 {
		t.Fatalf("next cursor offset = %d, err = %v", offset, err)
	}
}

func TestGetTransactionAcceptsNumericMonarchID(t *testing.T) {
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		variables := request.Variables.(map[string]any)
		if variables["id"] != "160820461792094418" {
			t.Errorf("id = %#v", variables["id"])
		}
		return monarchResponse(http.StatusOK, `{"data":{"getTransaction":{"id":"160820461792094418","amount":-12.34,"date":"2026-07-25","account":{"id":"account"}}}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetTransaction(context.Background(), "160820461792094418")
	if err != nil {
		t.Fatal(err)
	}
	if result.Transaction.Amount != "-12.34" {
		t.Fatalf("amount = %q", result.Transaction.Amount)
	}
}

func TestBudgetsUseInclusiveMonthBoundaries(t *testing.T) {
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		variables := request.Variables.(map[string]any)
		if variables["startDate"] != "2026-02-01" || variables["endDate"] != "2026-03-31" {
			t.Errorf("variables = %#v", variables)
		}
		return monarchResponse(http.StatusOK, `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[]}}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetBudgets(context.Background(), MonthRange{StartMonth: "2026-02", EndMonth: "2026-03"}); err != nil {
		t.Fatal(err)
	}
}

func TestCashflowReadsAggregateArray(t *testing.T) {
	transport := monarchRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"aggregates":[{"summary":{"sum_income":100,"sum_expense":40,"savings":60,"savings_rate":60}}]}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetCashflow(context.Background(), DateRange{StartDate: "2026-07-01", EndDate: "2026-07-31"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SumIncome != "100" || result.SumExpense != "40" || result.Savings != "60" || result.SavingsRate != "60" {
		t.Fatalf("unexpected cashflow summary: %+v", result)
	}
}

func TestCashflowRejectsMissingAggregate(t *testing.T) {
	transport := monarchRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"aggregates":[]}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetCashflow(context.Background(), DateRange{StartDate: "2026-07-01", EndDate: "2026-07-31"}); err == nil {
		t.Fatal("GetCashflow accepted a response without an aggregate summary")
	}
}

func TestCashflowReportsSchemaDriftAsUnexpectedShape(t *testing.T) {
	transport := monarchRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"aggregates":{"summary":{}}}}`), nil
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetCashflow(context.Background(), DateRange{StartDate: "2026-07-01", EndDate: "2026-07-31"})
	if err == nil || err.Error() != "Monarch returned an unexpected response shape" {
		t.Fatalf("schema drift error = %v", err)
	}
}

func TestFinancialOverviewCombinesConcurrentReads(t *testing.T) {
	var calls atomic.Int32
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var request graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		switch {
		case strings.Contains(request.Query, "GetAccounts"):
			return monarchResponse(http.StatusOK, `{"data":{"accounts":[]}}`), nil
		case strings.Contains(request.Query, "GetCashflowSummary"):
			return monarchResponse(http.StatusOK, `{"data":{"aggregates":[{"summary":{"sum_income":100,"sum_expense":40,"savings":60,"savings_rate":60}}]}}`), nil
		case strings.Contains(request.Query, "GetJointPlanningData"):
			return monarchResponse(http.StatusOK, `{"data":{"budgetData":{"monthly_amounts_by_category":[],"monthly_amounts_by_category_group":[]}}}`), nil
		case strings.Contains(request.Query, "GetTransactionsList"):
			return monarchResponse(http.StatusOK, `{"data":{"allTransactions":{"totalCount":0,"results":[]}}}`), nil
		default:
			t.Errorf("unexpected query: %s", request.Query)
			return monarchResponse(http.StatusBadRequest, ""), nil
		}
	})
	client, err := newClient(mustTokenSession(t, "token"), &http.Client{Transport: transport}, "https://example.test/graphql")
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time {
		return time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	}
	result, err := client.GetFinancialOverview(context.Background(), DateRange{StartDate: "2026-07-01", EndDate: "2026-07-31"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 || result.NetWorth != "0.00" || result.Cashflow.Savings != "60" {
		t.Fatalf("unexpected overview after %d calls: %+v", calls.Load(), result)
	}
}

type monarchRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn monarchRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func monarchResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func mustTokenSession(t *testing.T, token string) session.Session {
	t.Helper()
	value, err := session.NewToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustCookieSession(t *testing.T, cookies map[string]string) session.Session {
	t.Helper()
	value, err := session.NewCookie(cookies)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
