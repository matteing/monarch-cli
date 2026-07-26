// Package session stores Monarch sessions in the operating system credential vault.
package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	sessionVersion = 1
	maxTokenBytes  = 64 * 1024

	// The go-keyring Windows backend rejects credential blobs over 2,560
	// bytes. Its macOS backend base64-encodes the complete value and sends it
	// to `security -i`, whose command limit is 4 KiB. This conservative shared
	// bound fits both backends with room for encoding and command overhead.
	maxSerializedSessionBytes = 2400
)

// ErrInvalidSession indicates that the keyring was readable but its saved
// credential record was malformed or failed structural validation.
var ErrInvalidSession = errors.New("invalid saved session")

var errSessionTooLarge = errors.New("session exceeds the credential-vault size limit")

// Mode identifies the authentication material kept for a session.
type Mode string

const (
	ModeToken  Mode = "token"  // ModeToken authenticates with one opaque API token.
	ModeCookie Mode = "cookie" // ModeCookie authenticates with the two required browser cookies.
)

// Session is the versioned credential record stored in the system keyring.
// Passwords and MFA codes are intentionally never represented here.
type Session struct {
	Version   int       `json:"version"`
	Mode      Mode      `json:"mode"`
	CreatedAt time.Time `json:"created_at"`

	token   string
	cookies map[string]string
}

type sessionJSON struct {
	Version   int               `json:"version"`
	Mode      Mode              `json:"mode"`
	Token     string            `json:"token,omitempty"`
	Cookies   map[string]string `json:"cookies,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// NewToken constructs a validated token-backed session.
func NewToken(token string) (Session, error) {
	value := Session{
		Version:   sessionVersion,
		Mode:      ModeToken,
		token:     token,
		CreatedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

// NewCookie constructs a validated browser-cookie-backed session. Only the
// cookies required for Monarch authentication are retained.
func NewCookie(cookies map[string]string) (Session, error) {
	value := Session{
		Version:   sessionVersion,
		Mode:      ModeCookie,
		cookies:   selectRequiredCookies(cookies),
		CreatedAt: time.Now().UTC(),
	}
	if err := value.Validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

// Token returns the token credential, or an empty string for cookie sessions.
func (s Session) Token() string { return s.token }

// Cookies returns a defensive copy of the browser credentials.
func (s Session) Cookies() map[string]string { return cloneCookies(s.cookies) }

// Cookie returns one browser credential without exposing mutable session state.
func (s Session) Cookie(name string) string { return s.cookies[name] }

// MarshalJSON preserves the version-one keyring format while credential fields
// remain private and immutable to callers.
func (s Session) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(sessionJSON{
		Version: s.Version, Mode: s.Mode, Token: s.token,
		Cookies: cloneCookies(s.cookies), CreatedAt: s.CreatedAt,
	})
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSerializedSessionBytes {
		return nil, fmt.Errorf("%w of %d bytes", errSessionTooLarge, maxSerializedSessionBytes)
	}
	return raw, nil
}

// UnmarshalJSON reads the version-one keyring format. Stored records must be
// canonical and minimal; extra cookies and unknown fields are rejected.
func (s *Session) UnmarshalJSON(raw []byte) error {
	if len(raw) > maxSerializedSessionBytes {
		return fmt.Errorf("%w of %d bytes", errSessionTooLarge, maxSerializedSessionBytes)
	}
	var decoded sessionJSON
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("session record contains more than one JSON value")
		}
		return err
	}
	if hasUnneededCookies(decoded.Cookies) {
		return errors.New("session record contains browser cookies that are not required for authentication")
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return errors.New("session record is not in its canonical minimal form")
	}
	*s = Session{
		Version: decoded.Version, Mode: decoded.Mode, CreatedAt: decoded.CreatedAt,
		token: decoded.Token, cookies: selectRequiredCookies(decoded.Cookies),
	}
	return nil
}

func hasUnneededCookies(cookies map[string]string) bool {
	for name := range cookies {
		if !requiredCookieName(name) {
			return true
		}
	}
	return false
}

// Validate rejects malformed or incomplete credential records.
func (s Session) Validate() error {
	if s.Version != sessionVersion {
		return fmt.Errorf("unsupported session version %d", s.Version)
	}
	if s.CreatedAt.IsZero() {
		return errors.New("session is missing its creation time")
	}
	switch s.Mode {
	case ModeToken:
		if s.token == "" {
			return errors.New("token session is missing its token")
		}
		if s.token != strings.TrimSpace(s.token) || len(s.token) > maxTokenBytes || containsControl(s.token) {
			return errors.New("token session contains an invalid token")
		}
		if len(s.cookies) != 0 {
			return errors.New("token session cannot contain browser cookies")
		}
	case ModeCookie:
		if s.token != "" {
			return errors.New("cookie session cannot contain a token")
		}
		if len(s.cookies) != len(requiredCookieNames) {
			return errors.New("cookie session requires only session_id and csrftoken")
		}
		for _, name := range requiredCookieNames {
			value := s.cookies[name]
			if !validCookie(name, value) {
				return errors.New("cookie session contains an invalid cookie")
			}
		}
	default:
		return fmt.Errorf("unsupported session mode %q", s.Mode)
	}
	_, err := s.MarshalJSON()
	return err
}

func containsControl(value string) bool {
	if !utf8.ValidString(value) {
		return true
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}
