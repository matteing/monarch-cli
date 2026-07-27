package monarch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matteing/monarch-cli/internal/apperr"
)

func TestRefreshAccountsSendsMutationAndReturnsAcceptance(t *testing.T) {
	wantIDs := []string{"account:1/path", "account.2"}
	transport := monarchRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request struct {
			Query     string                   `json:"query"`
			Variables refreshAccountsVariables `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(request.Query, "mutation Common_ForceRefreshAccountsMutation") ||
			!strings.Contains(request.Query, "forceRefreshAccounts(input: $input)") {
			t.Fatalf("unexpected mutation: %s", request.Query)
		}
		if !reflect.DeepEqual(request.Variables.Input.AccountIDs, wantIDs) {
			t.Fatalf("accountIds = %#v", request.Variables.Input.AccountIDs)
		}
		return monarchResponse(http.StatusOK, `{"data":{"forceRefreshAccounts":{"success":true,"errors":[]}}}`), nil
	})
	client := newTestClient(t, transport)
	result, err := client.RefreshAccounts(context.Background(), RefreshAccountsParams{AccountIDs: wantIDs})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || !reflect.DeepEqual(result.AccountIDs, wantIDs) {
		t.Fatalf("result = %+v", result)
	}
}

func TestRefreshAccountsValidatesIDsBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	transport := monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected request")
	})
	client := newTestClient(t, transport)
	for _, ids := range [][]string{nil, {"account", "account"}, {" account"}} {
		_, err := client.RefreshAccounts(context.Background(), RefreshAccountsParams{AccountIDs: ids})
		var appErr *apperr.Error
		if !errors.As(err, &appErr) || appErr.Kind != apperr.KindInvalidInput {
			t.Fatalf("IDs %#v returned %v", ids, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d", calls.Load())
	}
}

func TestRefreshAccountsClassifiesPayloadError(t *testing.T) {
	transport := monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return monarchResponse(http.StatusOK, `{"data":{"forceRefreshAccounts":{"success":false,"errors":[{"code":"NOT_FOUND"}]}}}`), nil
	})
	client := newTestClient(t, transport)
	_, err := client.RefreshAccounts(context.Background(), RefreshAccountsParams{AccountIDs: []string{"missing"}})
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindNotFound {
		t.Fatalf("error = %v", err)
	}
}

func TestRefreshAccountsDoesNotRetryAmbiguousTransportFailure(t *testing.T) {
	var calls atomic.Int32
	transport := monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("connection reset after write")
	})
	client := newTestClient(t, transport)
	_, err := client.RefreshAccounts(context.Background(), RefreshAccountsParams{AccountIDs: []string{"account"}})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("error = %v, HTTP calls = %d", err, calls.Load())
	}
}

func TestRefreshAccountsRejectsIncompleteAndFailedPayloads(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"missing root", `{"data":{}}`, "Monarch returned an unexpected response shape"},
		{"null root", `{"data":{"forceRefreshAccounts":null}}`, "Monarch returned an unexpected response shape"},
		{"missing success", `{"data":{"forceRefreshAccounts":{}}}`, "Monarch returned an unexpected response shape"},
		{"null success", `{"data":{"forceRefreshAccounts":{"success":null}}}`, "Monarch returned an unexpected response shape"},
		{"failed without details", `{"data":{"forceRefreshAccounts":{"success":false,"errors":[]}}}`, "Monarch did not accept the account refresh request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, monarchRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return monarchResponse(http.StatusOK, test.body), nil
			}))
			_, err := client.RefreshAccounts(context.Background(), RefreshAccountsParams{AccountIDs: []string{"account"}})
			details := apperr.Describe(err)
			if details.Kind != apperr.KindUnavailable || details.Message != test.wantMessage {
				t.Fatalf("error details = %+v", details)
			}
		})
	}
}
