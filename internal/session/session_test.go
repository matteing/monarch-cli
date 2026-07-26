package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/matteing/monarch-cli/internal/apperr"
)

func TestTokenConstructorEnforcesCredentialInvariants(t *testing.T) {
	if _, err := NewToken(" token "); err == nil {
		t.Fatal("NewToken accepted surrounding whitespace")
	}
	if _, err := NewToken("token\nInjected: yes"); err == nil {
		t.Fatal("NewToken accepted a control character")
	}
}

func TestKeyringStoreLoadRejectsNonMinimalRecordsWithoutWriting(t *testing.T) {
	records := map[string]string{
		"retired cookie mode":   `{"version":1,"mode":"cookie","created_at":"2026-07-25T12:00:00Z"}`,
		"retired cookie fields": `{"version":1,"mode":"cookie","cookies":{"csrftoken":"csrf","session_id":"sid"},"created_at":"2026-07-25T12:00:00Z"}`,
		"unknown field":         `{"version":1,"mode":"token","token":"token","created_at":"2026-07-25T12:00:00Z","password":"must-not-be-ignored"}`,
		"duplicate field":       `{"version":1,"mode":"token","token":"old","token":"new","created_at":"2026-07-25T12:00:00Z"}`,
		"field alias":           `{"version":1,"mode":"token","Token":"token","created_at":"2026-07-25T12:00:00Z"}`,
		"redundant null":        `{"version":1,"mode":"token","token":"token","cookies":null,"created_at":"2026-07-25T12:00:00Z"}`,
	}
	for name, record := range records {
		t.Run(name, func(t *testing.T) {
			backend := &fakeKeyring{value: record}
			store := KeyringStore{backend: backend, readBlocked: func(_, _ string) bool { return false }}
			if _, err := store.Load("default"); apperr.KindOf(err) != apperr.KindAuth || !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("record error = %v, kind = %q", err, apperr.KindOf(err))
			}
			if backend.setCalls != 0 || backend.value != record {
				t.Fatalf("Load mutated record: writes=%d value=%s", backend.setCalls, backend.value)
			}
		})
	}
}

func TestSessionRequiresCreationTime(t *testing.T) {
	value := mustToken(t, "token")
	value.CreatedAt = time.Time{}
	if err := value.Validate(); err == nil {
		t.Fatal("session without creation time was accepted")
	}
}

func TestValidateProfile(t *testing.T) {
	for _, profile := range []string{"default", "work-2", "a.b_c"} {
		if err := ValidateProfile(profile); err != nil {
			t.Errorf("ValidateProfile(%q): %v", profile, err)
		}
	}
	for _, profile := range []string{"", "../escape", "space name", "-leading"} {
		if err := ValidateProfile(profile); err == nil {
			t.Errorf("ValidateProfile(%q) succeeded", profile)
		}
	}
}

func TestKeyringStoreRoundTripUsesInjectedBackend(t *testing.T) {
	backend := &fakeKeyring{}
	store := KeyringStore{backend: backend, readBlocked: func(_, _ string) bool { return false }}
	value := mustToken(t, "token")
	if err := store.Save("default", value); err != nil {
		t.Fatal(err)
	}
	if backend.account != "session:default" {
		t.Fatalf("unexpected persisted keyring record: account=%q value=%s", backend.account, backend.value)
	}
	loaded, err := store.Load("default")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != ModeToken || loaded.Token() != "token" {
		t.Fatalf("loaded session = mode %q token %q", loaded.Mode, loaded.Token())
	}
	if err := store.Delete("default"); err != nil {
		t.Fatal(err)
	}
	if !backend.deleted {
		t.Fatal("Delete did not reach the injected keyring")
	}
}

func TestKeyringStoreClassifiesMissingBlockedAndMalformedRecords(t *testing.T) {
	missing := &fakeKeyring{getErr: keyring.ErrNotFound}
	store := KeyringStore{backend: missing, readBlocked: func(_, _ string) bool { return false }}
	_, err := store.Load("default")
	if !errors.Is(err, ErrNotFound) || apperr.KindOf(err) != apperr.KindAuth {
		t.Fatalf("missing record error = %v, kind %q", err, apperr.KindOf(err))
	}

	store.readBlocked = func(_, _ string) bool { return true }
	if _, err := store.Load("default"); apperr.KindOf(err) != apperr.KindKeyring {
		t.Fatalf("blocked keyring kind = %q, want %q", apperr.KindOf(err), apperr.KindKeyring)
	}

	malformed := &fakeKeyring{value: `{not-json`}
	store = KeyringStore{backend: malformed, readBlocked: func(_, _ string) bool { return false }}
	if _, err := store.Load("default"); apperr.KindOf(err) != apperr.KindAuth || !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("malformed keyring error = %v, kind = %q", err, apperr.KindOf(err))
	}

	oversized := &fakeKeyring{value: strings.Repeat(" ", maxSerializedSessionBytes+1)}
	store = KeyringStore{backend: oversized, readBlocked: func(_, _ string) bool { return false }}
	if _, err := store.Load("default"); apperr.KindOf(err) != apperr.KindAuth || !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("oversized keyring error = %v, kind = %q", err, apperr.KindOf(err))
	}
}

func TestSessionSizeBoundIsEnforcedBeforeKeyringWrite(t *testing.T) {
	backend := &fakeKeyring{}
	store := KeyringStore{backend: backend, readBlocked: func(_, _ string) bool { return false }}
	value := Session{
		Version: sessionVersion, Mode: ModeToken,
		CreatedAt: time.Now().UTC(), token: strings.Repeat("x", maxSerializedSessionBytes),
	}
	if err := store.Save("default", value); apperr.KindOf(err) != apperr.KindInvalidInput {
		t.Fatalf("oversized save error = %v, kind = %q", err, apperr.KindOf(err))
	}
	if backend.setCalls != 0 {
		t.Fatalf("oversized record reached keyring %d time(s)", backend.setCalls)
	}
}

func TestKeyringStoreDoesNotHideBlockedDeleteAsMissing(t *testing.T) {
	backend := &fakeKeyring{delErr: keyring.ErrNotFound}
	blocked := KeyringStore{backend: backend, readBlocked: func(_, _ string) bool { return true }}
	if err := blocked.Delete("default"); apperr.KindOf(err) != apperr.KindKeyring {
		t.Fatalf("blocked delete error = %v, kind = %q", err, apperr.KindOf(err))
	}

	missing := KeyringStore{backend: backend, readBlocked: func(_, _ string) bool { return false }}
	if err := missing.Delete("default"); err != nil {
		t.Fatalf("missing delete = %v, want success", err)
	}
}

func TestKeyringProbeIsBounded(t *testing.T) {
	started := time.Now()
	blocked := runKeyringProbe(func(ctx context.Context, _, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, "service", "account", 20*time.Millisecond)
	if !blocked {
		t.Fatal("timed-out keyring probe was classified as a missing item")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("keyring probe took %s, want a bounded result", elapsed)
	}
}

func TestKeyringAccessFailureClassification(t *testing.T) {
	for _, output := range []string{
		"security: SecKeychainSearchCreateFromAttributes: One or more parameters passed to a function were not valid.",
		"security: SecKeychainItemCreateFromContent: Unable to obtain authorization for this operation.",
		"security: User interaction is not allowed.",
	} {
		if !looksLikeKeyringAccessFailure(output) {
			t.Fatalf("access failure was not recognized: %q", output)
		}
	}
	if looksLikeKeyringAccessFailure("The specified item could not be found in the keychain.") {
		t.Fatal("a genuinely missing item was classified as a keyring access failure")
	}
}

type fakeKeyring struct {
	value    string
	account  string
	getErr   error
	setErr   error
	delErr   error
	deleted  bool
	setCalls int
}

func (f *fakeKeyring) Get(_, _ string) (string, error) { return f.value, f.getErr }

func (f *fakeKeyring) Set(_, account, value string) error {
	f.setCalls++
	f.account = account
	f.value = value
	return f.setErr
}

func (f *fakeKeyring) Delete(_, _ string) error {
	f.deleted = true
	return f.delErr
}

func mustToken(t *testing.T, token string) Session {
	t.Helper()
	value, err := NewToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
