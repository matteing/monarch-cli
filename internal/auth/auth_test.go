package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/session"
)

func TestLoginReturnsValidatedTokenSession(t *testing.T) {
	transport := authRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var request loginRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Username != "person@example.com" || request.Password != "secret" || request.TOTP != "123456" || !request.SupportsMFA || !request.TrustedDevice {
			t.Errorf("unexpected request: %+v", request)
		}
		return authResponse(http.StatusOK, `{"token":"token-value"}`), nil
	})

	value, err := testAuthenticator(transport).Login(context.Background(), " person@example.com ", "secret", " 123456 ")
	if err != nil {
		t.Fatal(err)
	}
	if value.Mode != session.ModeToken || value.Token() != "token-value" {
		t.Fatalf("unexpected session: mode=%q token=%q", value.Mode, value.Token())
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("Login returned an invalid session: %v", err)
	}
}

func TestLoginClassifiesOnlyExplicitMFAResponses(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"MFA_REQUIRED"}`,
		`{"detail":"MFA required"}`,
	} {
		transport := authRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return authResponse(http.StatusForbidden, body), nil
		})
		_, err := testAuthenticator(transport).Login(context.Background(), "person@example.com", "secret", "")
		if apperr.KindOf(err) != apperr.KindMFARequired {
			t.Fatalf("body %s: kind = %q, want %q: %v", body, apperr.KindOf(err), apperr.KindMFARequired, err)
		}
	}

	transport := authRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return authResponse(http.StatusForbidden, `{"detail":"account disabled","error_code":"ACCOUNT_DISABLED"}`), nil
	})
	_, err := testAuthenticator(transport).Login(context.Background(), "person@example.com", "secret", "")
	if apperr.KindOf(err) != apperr.KindAuth {
		t.Fatalf("non-MFA 403 kind = %q, want %q: %v", apperr.KindOf(err), apperr.KindAuth, err)
	}
}

func TestLoginAllowsRepeatedExplicitMFAChallenge(t *testing.T) {
	transport := authRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return authResponse(http.StatusForbidden, `{"error_code":"TOTP_REQUIRED"}`), nil
	})
	_, err := testAuthenticator(transport).Login(context.Background(), "person@example.com", "secret", "000000")
	if apperr.KindOf(err) != apperr.KindMFARequired || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("repeated MFA response = %v, kind %q", err, apperr.KindOf(err))
	}
}

func TestLoginRejectsInvalidOrShortLivedTokens(t *testing.T) {
	for _, body := range []string{
		`{"token":"header.payload.signature","tokenExpiration":"2026-07-25T12:00:00Z"}`,
		`{"token":" ","tokenExpiration":null}`,
		`{"token":"token\nvalue","tokenExpiration":null}`,
	} {
		transport := authRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return authResponse(http.StatusOK, body), nil
		})
		if _, err := testAuthenticator(transport).Login(context.Background(), "person@example.com", "secret", ""); err == nil {
			t.Fatalf("Login accepted invalid response %s", body)
		}
	}
}

func TestLoginRejectsOversizedResponse(t *testing.T) {
	body := `{"token":"token"}` + strings.Repeat(" ", maxLoginResponseSize)
	transport := authRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return authResponse(http.StatusOK, body), nil
	})
	_, err := testAuthenticator(transport).Login(context.Background(), "person@example.com", "secret", "")
	if apperr.KindOf(err) != apperr.KindUnavailable || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestLoginRejectsOversizedCredentialsBeforeHTTP(t *testing.T) {
	var calls int
	authenticator := testAuthenticator(authRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return authResponse(http.StatusOK, `{"token":"token"}`), nil
	}))
	for name, credentials := range map[string][3]string{
		"email":    {strings.Repeat("e", MaxEmailCharacters+1), "password", ""},
		"password": {"person@example.com", strings.Repeat("p", MaxPasswordCharacters+1), ""},
		"MFA":      {"person@example.com", "password", strings.Repeat("1", MaxTOTPCharacters+1)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := authenticator.Login(context.Background(), credentials[0], credentials[1], credentials[2])
			if apperr.KindOf(err) != apperr.KindInvalidInput {
				t.Fatalf("error = %v, kind = %q", err, apperr.KindOf(err))
			}
		})
	}
	if calls != 0 {
		t.Fatalf("oversized credentials reached HTTP %d time(s)", calls)
	}
}

func TestNewRejectsEveryRedirectAndUsesBoundedTimeout(t *testing.T) {
	authenticator := New(0)
	if authenticator.httpClient.Timeout != defaultLoginTimeout {
		t.Fatalf("default timeout = %s, want %s", authenticator.httpClient.Timeout, defaultLoginTimeout)
	}
	previous, _ := http.NewRequest(http.MethodPost, "https://api.monarch.com/auth/login/", nil)
	target, _ := http.NewRequest(http.MethodPost, "http://api.monarch.com/auth/login/", nil)
	if err := authenticator.httpClient.CheckRedirect(target, []*http.Request{previous}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("HTTPS downgrade redirect error = %v, want http.ErrUseLastResponse", err)
	}
	target, _ = http.NewRequest(http.MethodPost, "https://example.com/steal", nil)
	if err := authenticator.httpClient.CheckRedirect(target, []*http.Request{previous}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("cross-origin redirect error = %v, want http.ErrUseLastResponse", err)
	}
}

func TestLoginHonorsContextCancellation(t *testing.T) {
	transport := authRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testAuthenticator(transport).Login(ctx, "person@example.com", "secret", "")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled login error = %v", err)
	}
}

func TestLoginPreservesRetryAfter(t *testing.T) {
	transport := authRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		response := authResponse(http.StatusTooManyRequests, `{}`)
		response.Header.Set("Retry-After", "12")
		return response, nil
	})
	_, err := testAuthenticator(transport).Login(context.Background(), "person@example.com", "secret", "")
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindRateLimited || appErr.RetryAfter != 12*time.Second {
		t.Fatalf("error = %#v, want rate limit with 12s retry-after", err)
	}
}

func testAuthenticator(transport http.RoundTripper) *Authenticator {
	return &Authenticator{
		httpClient: &http.Client{Timeout: time.Second, Transport: transport},
		loginURL:   "https://example.test/login",
	}
}

type authRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn authRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func authResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
