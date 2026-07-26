//go:build darwin

package session

import (
	"context"
	"os/exec"
)

// keyringReadBlocked distinguishes a missing item from a process that macOS
// has denied access to the login keychain. The probe requests item metadata,
// never the stored secret.
func keyringReadBlocked(service, account string) bool {
	return runKeyringProbe(runSecurityProbe, service, account, keyringProbeTimeout)
}

func runSecurityProbe(ctx context.Context, service, account string) ([]byte, error) {
	return exec.CommandContext(
		ctx,
		"/usr/bin/security",
		"find-generic-password",
		"-s", service,
		"-a", account,
	).CombinedOutput()
}
