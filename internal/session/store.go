package session

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/profile"
)

const keyringService = "monarch-cli"

// ErrNotFound indicates that a credential profile has no saved session.
var ErrNotFound = errors.New("session not found")

// ValidateProfile checks a profile before it becomes part of a keyring account name.
func ValidateProfile(name string) error {
	return profile.Validate(name)
}

// Store persists credential records without a plaintext fallback. Native
// credential-vault operations may block and cannot be canceled once started.
type Store interface {
	Load(profile string) (Session, error)
	Save(profile string, value Session) error
	Delete(profile string) error
}

// KeyringStore stores session JSON in the native operating system keyring.
type KeyringStore struct {
	backend     keyringBackend
	readBlocked func(string, string) bool
}

type keyringBackend interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
	Delete(service, account string) error
}

type nativeKeyring struct{}

func (nativeKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (nativeKeyring) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}

func (nativeKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

func (s KeyringStore) dependencies() (keyringBackend, func(string, string) bool) {
	backend := s.backend
	if backend == nil {
		backend = nativeKeyring{}
	}
	readBlocked := s.readBlocked
	if readBlocked == nil {
		readBlocked = keyringReadBlocked
	}
	return backend, readBlocked
}

// Load reads and validates a profile's stored session.
func (s KeyringStore) Load(profile string) (Session, error) {
	const op = "load session"
	if err := ValidateProfile(profile); err != nil {
		return Session{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	backend, readBlocked := s.dependencies()
	account := accountName(profile)
	raw, err := backend.Get(keyringService, account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			if readBlocked(keyringService, account) {
				return Session{}, apperr.New(apperr.KindKeyring, op, "could not access the operating system keyring", err)
			}
			return Session{}, apperr.New(apperr.KindAuth, op, "no saved Monarch session; run `monarch auth login`", ErrNotFound)
		}
		return Session{}, apperr.New(apperr.KindKeyring, op, "could not read the operating system keyring", err)
	}
	if len(raw) > maxSerializedSessionBytes {
		cause := fmt.Errorf("%w of %d bytes", errSessionTooLarge, maxSerializedSessionBytes)
		return Session{}, apperr.New(apperr.KindAuth, op, "the saved session is too large; log in again", errors.Join(ErrInvalidSession, cause))
	}
	var value Session
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return Session{}, apperr.New(apperr.KindAuth, op, "the saved session is malformed; log in again", errors.Join(ErrInvalidSession, err))
	}
	if err := value.Validate(); err != nil {
		return Session{}, apperr.New(apperr.KindAuth, op, "the saved session is invalid; log in again", errors.Join(ErrInvalidSession, err))
	}
	return value, nil
}

// Save validates and writes a profile's session to the native keyring.
func (s KeyringStore) Save(profile string, value Session) error {
	const op = "save session"
	if err := ValidateProfile(profile); err != nil {
		return apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	if err := value.Validate(); err != nil {
		return apperr.New(apperr.KindInvalidInput, op, "refusing to save an invalid session", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return apperr.New(apperr.KindInvalidInput, op, "refusing to save an invalid session", err)
	}
	backend, _ := s.dependencies()
	if err := backend.Set(keyringService, accountName(profile), string(raw)); err != nil {
		return apperr.New(apperr.KindKeyring, op, "could not write the operating system keyring", err)
	}
	return nil
}

// Delete removes a profile's saved session. Missing sessions are treated as success.
func (s KeyringStore) Delete(profile string) error {
	const op = "delete session"
	if err := ValidateProfile(profile); err != nil {
		return apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	backend, readBlocked := s.dependencies()
	account := accountName(profile)
	err := backend.Delete(keyringService, account)
	if errors.Is(err, keyring.ErrNotFound) {
		if readBlocked(keyringService, account) {
			return apperr.New(apperr.KindKeyring, op, "could not access the operating system keyring", err)
		}
		return nil
	}
	if err != nil {
		return apperr.New(apperr.KindKeyring, op, "could not update the operating system keyring", err)
	}
	return nil
}

func accountName(profile string) string { return "session:" + profile }
