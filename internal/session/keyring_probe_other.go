//go:build !darwin

package session

func keyringReadBlocked(_, _ string) bool { return false }
