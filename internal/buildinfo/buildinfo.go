// Package buildinfo resolves the one executable version used by Cobra's
// --version output, MCP server metadata, and both HTTP User-Agent headers.
// Release builds inject a tag; source builds derive a useful VCS identity.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version is injected for release builds. Source installs fall back to build
// metadata recorded by the Go toolchain.
var Version = "dev"

// Current returns the injected release, module version, or source revision.
func Current() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	return resolveVersion(Version, info)
}

func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != "" && injected != "dev" {
		return injected
	}
	if info == nil {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if !isHexRevision(revision) {
		if modified {
			return "dev-dirty"
		}
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	version := "dev-" + strings.ToLower(revision)
	if modified {
		version += "-dirty"
	}
	return version
}

func isHexRevision(value string) bool {
	if len(value) < 7 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

// UserAgent returns the shared product identity for outbound requests.
func UserAgent() string {
	return "monarch-cli/" + strings.TrimPrefix(Current(), "v")
}
