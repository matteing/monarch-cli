// Package auth performs Monarch's interactive password and MFA exchange.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/buildinfo"
	"github.com/matteing/monarch-cli/internal/httpx"
	"github.com/matteing/monarch-cli/internal/session"
)

const (
	loginURL             = "https://api.monarch.com/auth/login/"
	maxLoginResponseSize = 1 << 20
	defaultLoginTimeout  = 30 * time.Second

	// These bounds are shared by every login surface and are measured in
	// Unicode characters, matching the interactive text widgets.
	MaxEmailCharacters    = 320
	MaxPasswordCharacters = 1024
	MaxTOTPCharacters     = 64
)

// Authenticator exchanges short-lived interactive credentials for a reusable token.
type Authenticator struct {
	httpClient *http.Client
	loginURL   string
}

// New returns an authenticator with a bounded HTTP client and Monarch's fixed endpoint.
func New(timeout time.Duration) *Authenticator {
	if timeout <= 0 {
		timeout = defaultLoginTimeout
	}
	return &Authenticator{
		httpClient: &http.Client{Timeout: timeout, CheckRedirect: httpx.RejectRedirects},
		loginURL:   loginURL,
	}
}

type loginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	SupportsMFA   bool   `json:"supports_mfa"`
	TrustedDevice bool   `json:"trusted_device"`
	TOTP          string `json:"totp,omitempty"`
}

type loginResponse struct {
	Token           string          `json:"token"`
	TokenExpiration json.RawMessage `json:"tokenExpiration"`
	Detail          string          `json:"detail"`
	ErrorCode       string          `json:"error_code"`
}

// Login attempts password authentication, optionally including a TOTP code.
func (a *Authenticator) Login(ctx context.Context, email, password, totp string) (session.Session, error) {
	const op = "Monarch login"
	if a == nil || a.httpClient == nil || strings.TrimSpace(a.loginURL) == "" {
		return session.Session{}, apperr.New(apperr.KindInternal, op, "login client is not configured", nil)
	}
	if ctx == nil {
		return session.Session{}, apperr.New(apperr.KindInternal, op, "login context is required", nil)
	}
	if err := ValidateLoginInput(email, password, totp); err != nil {
		return session.Session{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	payload, err := json.Marshal(loginRequest{
		Username: strings.TrimSpace(email), Password: password, SupportsMFA: true,
		TrustedDevice: true, TOTP: strings.TrimSpace(totp),
	})
	if err != nil {
		return session.Session{}, apperr.New(apperr.KindInternal, op, "could not encode login request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.loginURL, bytes.NewReader(payload))
	if err != nil {
		return session.Session{}, apperr.New(apperr.KindInternal, op, "could not create login request", err)
	}
	if !strings.EqualFold(req.URL.Scheme, "https") {
		return session.Session{}, apperr.New(apperr.KindInternal, op, "login endpoint must use HTTPS", nil)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return session.Session{}, apperr.New(apperr.KindUnavailable, op, "Monarch login is unavailable", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLoginResponseSize+1))
	if err != nil {
		return session.Session{}, apperr.New(apperr.KindUnavailable, op, "could not read Monarch's login response", err)
	}
	if len(body) > maxLoginResponseSize {
		return session.Session{}, apperr.New(apperr.KindUnavailable, op, "Monarch's login response exceeded the safety limit", nil)
	}
	var decoded loginResponse
	decodeErr := json.Unmarshal(body, &decoded)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && decoded.Token != "" {
		if decodeErr != nil {
			return session.Session{}, apperr.New(apperr.KindUnavailable, op, "Monarch returned malformed login JSON", decodeErr)
		}
		if strings.Count(decoded.Token, ".") == 2 {
			return session.Session{}, apperr.New(apperr.KindAuth, op, "Monarch returned a short-lived feature token; use browser-session login", nil)
		}
		if expiration := strings.TrimSpace(string(decoded.TokenExpiration)); expiration != "" && expiration != "null" && expiration != `"null"` {
			return session.Session{}, apperr.New(apperr.KindAuth, op, "Monarch returned a short-lived token; use browser-session login", nil)
		}
		value, err := session.NewToken(decoded.Token)
		if err != nil {
			return session.Session{}, apperr.New(apperr.KindUnavailable, op, "Monarch returned an invalid session token", err)
		}
		return value, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if decodeErr != nil {
			return session.Session{}, apperr.New(apperr.KindUnavailable, op, "Monarch returned malformed login JSON", decodeErr)
		}
		return session.Session{}, apperr.New(apperr.KindUnavailable, op, "Monarch login succeeded without returning a session token", nil)
	}
	if resp.StatusCode == http.StatusForbidden {
		if decodeErr == nil && strings.EqualFold(decoded.ErrorCode, "CAPTCHA_REQUIRED") {
			return session.Session{}, &apperr.Error{Kind: apperr.KindAuth, Op: op, Message: "programmatic login is blocked by CAPTCHA; retry with `monarch auth login --method browser-session`", StatusCode: resp.StatusCode}
		}
		if decodeErr == nil && mfaRequired(decoded.ErrorCode, decoded.Detail) {
			message := "MFA code required"
			if strings.TrimSpace(totp) != "" {
				message = "MFA code was rejected; try again"
			}
			return session.Session{}, &apperr.Error{Kind: apperr.KindMFARequired, Op: op, Message: message, StatusCode: resp.StatusCode}
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return session.Session{}, &apperr.Error{
			Kind: apperr.KindRateLimited, Op: op, Message: "Monarch rate-limited the login attempt",
			StatusCode: resp.StatusCode, Retryable: true,
			RetryAfter: httpx.RetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return session.Session{}, &apperr.Error{Kind: apperr.KindAuth, Op: op, Message: "Monarch rejected the login; check credentials or use browser-session login", StatusCode: resp.StatusCode}
	}
	return session.Session{}, &apperr.Error{Kind: apperr.KindUnavailable, Op: op, Message: fmt.Sprintf("Monarch login failed with HTTP %d", resp.StatusCode), StatusCode: resp.StatusCode, Retryable: resp.StatusCode >= 500}
}

// ValidateLoginInput applies the same bounded credential policy to direct and
// interactive login paths. Password text is never normalized.
func ValidateLoginInput(email, password, totp string) error {
	if strings.TrimSpace(email) == "" || password == "" {
		return errors.New("email and password are required")
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "email", value: email, limit: MaxEmailCharacters},
		{name: "password", value: password, limit: MaxPasswordCharacters},
		{name: "MFA code", value: totp, limit: MaxTOTPCharacters},
	} {
		if !utf8.ValidString(field.value) {
			return fmt.Errorf("%s contains invalid text", field.name)
		}
		if utf8.RuneCountInString(field.value) > field.limit {
			return fmt.Errorf("%s is limited to %d characters", field.name, field.limit)
		}
	}
	return nil
}

func mfaRequired(code, detail string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "MFA_REQUIRED", "MFA_CODE_REQUIRED", "TOTP_REQUIRED":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(detail)) {
	case "mfa required", "mfa code required", "totp required":
		return true
	default:
		return false
	}
}
