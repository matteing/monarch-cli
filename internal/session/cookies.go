package session

import (
	"errors"
	"regexp"
	"strings"

	"github.com/matteing/monarch-cli/internal/apperr"
)

const (
	// MaxCookieHeaderBytes is the shared input bound for browser-cookie import.
	MaxCookieHeaderBytes = 64 * 1024
	maxCookieValueBytes  = maxSerializedSessionBytes
)

var cookieNamePattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

var requiredCookieNames = [...]string{"session_id", "csrftoken"}

// ParseCookieHeader converts a copied browser Cookie header into a validated session.
func ParseCookieHeader(header string) (Session, error) {
	const op = "parse browser session"
	if len(header) > MaxCookieHeaderBytes {
		return Session{}, apperr.New(apperr.KindInvalidInput, op, "cookie header is unexpectedly large", nil)
	}
	if containsControl(header) {
		return Session{}, apperr.New(apperr.KindInvalidInput, op, "cookie header contains invalid text", nil)
	}
	cookies := make(map[string]string, len(requiredCookieNames))
	for _, pair := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || name == "" || value == "" {
			continue
		}
		if !requiredCookieName(name) {
			continue
		}
		if !validCookie(name, value) {
			return Session{}, apperr.New(apperr.KindInvalidInput, op, "cookie header contains an invalid cookie", nil)
		}
		cookies[name] = value
	}
	value, err := NewCookie(cookies)
	if err != nil {
		if errors.Is(err, errSessionTooLarge) {
			return Session{}, apperr.New(apperr.KindInvalidInput, op, "browser cookies exceed the credential-vault size limit", err)
		}
		return Session{}, apperr.New(apperr.KindInvalidInput, op, "browser cookies must include session_id and csrftoken", err)
	}
	return value, nil
}

func validCookie(name, value string) bool {
	return cookieNamePattern.MatchString(name) && value != "" && len(value) <= maxCookieValueBytes && !strings.Contains(value, ";") && !containsControl(value)
}

func requiredCookieName(name string) bool {
	for _, required := range requiredCookieNames {
		if name == required {
			return true
		}
	}
	return false
}

func selectRequiredCookies(cookies map[string]string) map[string]string {
	if len(cookies) == 0 {
		return nil
	}
	selected := make(map[string]string, len(requiredCookieNames))
	for _, name := range requiredCookieNames {
		if value, ok := cookies[name]; ok {
			selected[name] = value
		}
	}
	return selected
}

func cloneCookies(cookies map[string]string) map[string]string {
	if len(cookies) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(cookies))
	for name, value := range cookies {
		cloned[name] = value
	}
	return cloned
}
