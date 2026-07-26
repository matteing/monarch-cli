package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMergedAppliesPrecedenceBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"profile":"disk","output":"broken","timeout":"not-a-duration"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"MONARCH_PROFILE": "environment",
		"MONARCH_TIMEOUT": "45s",
	}
	loaded := loadMerged(path, func(name string) string { return environment[name] })
	if len(loaded.Issues) != 0 {
		t.Fatalf("issues = %v, want none", loaded.Issues)
	}
	if loaded.Config.Profile != "environment" || loaded.Config.Output != "broken" || loaded.Config.Timeout != 45*time.Second {
		t.Fatalf("unexpected merged config: %+v", loaded.Config)
	}
}

func TestLoadMergedDefersInvalidFinalTimeout(t *testing.T) {
	loaded := loadMerged(filepath.Join(t.TempDir(), "missing.json"), func(name string) string {
		if name == "MONARCH_TIMEOUT" {
			return "invalid"
		}
		return ""
	})
	if len(loaded.Issues) != 1 || loaded.Issues[0].Field != FieldTimeout || loaded.Issues[0].Kind != IssueInvalidInput {
		t.Fatalf("issues = %+v", loaded.Issues)
	}
	if loaded.Config.Timeout != defaultTimeout {
		t.Fatalf("fallback timeout = %s, want %s", loaded.Config.Timeout, defaultTimeout)
	}
}

func TestLoadMergedReportsMalformedFileButKeepsEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"profile":`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := loadMerged(path, func(name string) string {
		if name == "MONARCH_PROFILE" {
			return "environment"
		}
		return ""
	})
	if len(loaded.Issues) != 1 || loaded.Issues[0].Kind != IssueInvalidInput {
		t.Fatalf("issues = %+v", loaded.Issues)
	}
	if loaded.Config.Profile != "environment" {
		t.Fatalf("profile = %q", loaded.Config.Profile)
	}
}

func TestLoadMergedRejectsConfigLargerThanLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	contents := append([]byte(`{"profile":"disk"}`), bytes.Repeat([]byte(" "), maxConfigBytes)...)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded := loadMerged(path, func(string) string { return "" })
	if len(loaded.Issues) != 1 || loaded.Issues[0].Kind != IssueInvalidInput {
		t.Fatalf("issues = %+v", loaded.Issues)
	}
	if !bytes.Contains([]byte(loaded.Issues[0].Error()), []byte("exceeds 65536 bytes")) {
		t.Fatalf("issue = %q", loaded.Issues[0].Error())
	}
	if loaded.Config.Profile != Default().Profile {
		t.Fatalf("oversized file was merged: %+v", loaded.Config)
	}
}

func TestLoadMergedIgnoresNullAndPreservesExplicitEmpty(t *testing.T) {
	for name, contents := range map[string]string{
		"null":  `{"profile":null}`,
		"empty": `{"profile":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded := loadMerged(path, func(string) string { return "" })
			if name == "null" {
				if len(loaded.Issues) != 0 || loaded.Config.Profile != Default().Profile {
					t.Fatalf("loaded = %+v", loaded)
				}
				return
			}
			if loaded.Config.Profile != "" {
				t.Fatalf("explicit empty profile was replaced with %q", loaded.Config.Profile)
			}
			if err := loaded.Config.Validate(); err == nil {
				t.Fatal("explicit empty profile passed validation")
			}
		})
	}
}

func TestValidateBounds(t *testing.T) {
	cfg := Default()
	cfg.Profile = "../escape"
	if err := cfg.Validate(); err == nil {
		t.Fatal("unsafe profile was accepted")
	}

	cfg = Default()
	cfg.Timeout = 500 * time.Millisecond
	if err := cfg.Validate(); err == nil {
		t.Fatal("sub-second timeout was accepted")
	}
	cfg = Default()
	cfg.Output = "yaml"
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown output was accepted")
	}
	cfg = Default()
	cfg.LogLevel = "fatal"
	if err := cfg.Validate(); err == nil {
		t.Fatal("fatal log level was accepted")
	}
}

func TestLoadMergedClassifiesConfigReadFailureAsUnavailable(t *testing.T) {
	loaded := loadMerged(t.TempDir(), func(string) string { return "" })
	if len(loaded.Issues) != 1 || loaded.Issues[0].Kind != IssueUnavailable {
		t.Fatalf("issues = %+v", loaded.Issues)
	}
}
