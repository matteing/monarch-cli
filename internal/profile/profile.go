// Package profile defines the syntax of named Monarch credential profiles.
package profile

import (
	"errors"
	"regexp"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Syntax is the concise profile-name contract reused in CLI help.
const Syntax = "1-64 characters; start with a letter or digit; use only letters, digits, dots, underscores, or hyphens"

// Validate rejects names that are unsafe or ambiguous at configuration and
// credential-vault boundaries.
func Validate(name string) error {
	if !namePattern.MatchString(name) {
		return errors.New("profile must be " + Syntax)
	}
	return nil
}
