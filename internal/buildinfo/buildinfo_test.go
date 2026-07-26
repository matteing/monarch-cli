package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestInjectedVersionDrivesIdentity(t *testing.T) {
	original := Version
	Version = "v9.8.7"
	t.Cleanup(func() { Version = original })

	if got := Current(); got != "v9.8.7" {
		t.Fatalf("Current() = %q, want v9.8.7", got)
	}
	if got := UserAgent(); got != "monarch-cli/9.8.7" {
		t.Fatalf("UserAgent() = %q, want monarch-cli/9.8.7", got)
	}
}

func TestResolveVersionUsesSourceRevision(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ABCDEF0123456789"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	if got, want := resolveVersion("dev", info), "dev-abcdef012345-dirty"; got != want {
		t.Fatalf("resolveVersion() = %q, want %q", got, want)
	}
	info.Settings[0].Value = "unsafe/revision"
	if got := resolveVersion("dev", info); got != "dev-dirty" {
		t.Fatalf("unsafe revision produced %q", got)
	}
}
