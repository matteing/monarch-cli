package session

import (
	"context"
	"strings"
	"time"
)

const keyringProbeTimeout = 2 * time.Second

type keyringProbeRunner func(context.Context, string, string) ([]byte, error)

// looksLikeKeyringAccessFailure recognizes macOS security errors that
// go-keyring currently collapses into ErrNotFound. Keep this list limited to
// access failures; a genuinely absent item must remain an authentication error.
func looksLikeKeyringAccessFailure(output string) bool {
	for _, message := range []string{
		"Unable to obtain authorization",
		"User interaction is not allowed",
		"SecKeychainSearchCreateFromAttributes",
	} {
		if strings.Contains(output, message) {
			return true
		}
	}
	return false
}

func runKeyringProbe(runner keyringProbeRunner, service, account string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	output, err := runner(ctx, service, account)
	if ctx.Err() != nil {
		return true
	}
	return err != nil && looksLikeKeyringAccessFailure(string(output))
}
