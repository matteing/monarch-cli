// Package config loads non-secret command settings.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/matteing/monarch-cli/internal/profile"
)

const defaultTimeout = 30 * time.Second

const maxConfigBytes = 64 * 1024

const (
	// FieldTimeout identifies the timeout setting when reporting a deferred
	// source error. A CLI flag for the same field may supersede that error.
	FieldTimeout = "timeout"
)

// Config contains non-secret settings shared by CLI commands and the MCP server.
type Config struct {
	Profile   string        `json:"profile"`
	Output    string        `json:"output"`
	Timeout   time.Duration `json:"-"`
	LogLevel  string        `json:"log_level"`
	LogFormat string        `json:"log_format"`
}

type rawConfig struct {
	Profile   string `json:"profile"`
	Output    string `json:"output"`
	Timeout   string `json:"timeout"`
	LogLevel  string `json:"log_level"`
	LogFormat string `json:"log_format"`
}

type diskConfig struct {
	Profile   *string `json:"profile"`
	Output    *string `json:"output"`
	Timeout   *string `json:"timeout"`
	LogLevel  *string `json:"log_level"`
	LogFormat *string `json:"log_format"`
}

// IssueKind classifies a deferred configuration-source failure without coupling
// this package to a presentation-specific error vocabulary.
type IssueKind uint8

const (
	IssueInternal IssueKind = iota
	IssueInvalidInput
	IssueUnavailable
)

// Issue records a configuration-source error that must be checked after CLI
// flags have had their final opportunity to override the affected field.
type Issue struct {
	Field string
	Err   error
	Kind  IssueKind
}

// Error returns the source error's user-facing text.
func (i Issue) Error() string {
	if i.Err == nil {
		return "invalid configuration"
	}
	return i.Err.Error()
}

// Loaded is a best-effort merged configuration plus deferred source issues.
// It lets command construction, help, and version output proceed even when a
// value that a CLI flag can replace is malformed.
type Loaded struct {
	Config Config
	Issues []Issue
}

// Default returns conservative defaults suitable for both humans and MCP clients.
func Default() Config {
	return Config{Profile: "default", Output: "table", Timeout: defaultTimeout, LogLevel: "info", LogFormat: "text"}
}

// Path returns the platform-appropriate path for the optional JSON config file.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "monarch-cli", "config.json"), nil
}

// LoadMerged reads and merges the optional config file and environment without
// performing final validation. Higher-precedence sources replace raw lower-
// precedence values before values such as durations are parsed.
func LoadMerged() Loaded {
	path, err := Path()
	if err != nil {
		return Loaded{Config: Default(), Issues: []Issue{{Err: err, Kind: IssueUnavailable}}}
	}
	return loadMerged(path, os.Getenv)
}

func loadMerged(path string, getenv func(string) string) Loaded {
	defaults := Default()
	raw := rawConfig{
		Profile: defaults.Profile, Output: defaults.Output,
		Timeout: defaults.Timeout.String(), LogLevel: defaults.LogLevel,
		LogFormat: defaults.LogFormat,
	}
	var issues []Issue

	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		rawFile, readErr := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
		var disk diskConfig
		switch {
		case readErr != nil:
			issues = append(issues, Issue{Err: fmt.Errorf("read %s: %w", path, readErr), Kind: IssueUnavailable})
		case len(rawFile) > maxConfigBytes:
			issues = append(issues, Issue{Err: fmt.Errorf("decode %s: config file exceeds %d bytes", path, maxConfigBytes), Kind: IssueInvalidInput})
		default:
			decoder := json.NewDecoder(bytes.NewReader(rawFile))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&disk); err != nil {
				issues = append(issues, Issue{Err: fmt.Errorf("decode %s: %w", path, err), Kind: IssueInvalidInput})
			} else if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				issues = append(issues, Issue{Err: fmt.Errorf("decode %s: trailing JSON data", path), Kind: IssueInvalidInput})
			} else {
				mergeDisk(&raw, disk)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		issues = append(issues, Issue{Err: fmt.Errorf("open %s: %w", path, err), Kind: IssueUnavailable})
	}

	mergeEnvironment(&raw, getenv)
	cfg := Config{
		Profile: raw.Profile, Output: raw.Output,
		Timeout: defaults.Timeout, LogLevel: raw.LogLevel, LogFormat: raw.LogFormat,
	}
	if duration, err := time.ParseDuration(raw.Timeout); err != nil {
		issues = append(issues, Issue{
			Field: FieldTimeout, Kind: IssueInvalidInput,
			Err: fmt.Errorf("invalid timeout %q: %w", raw.Timeout, err),
		})
	} else {
		cfg.Timeout = duration
	}
	return Loaded{Config: cfg, Issues: issues}
}

func mergeDisk(target *rawConfig, source diskConfig) {
	if source.Profile != nil {
		target.Profile = *source.Profile
	}
	if source.Output != nil {
		target.Output = *source.Output
	}
	if source.Timeout != nil {
		target.Timeout = *source.Timeout
	}
	if source.LogLevel != nil {
		target.LogLevel = *source.LogLevel
	}
	if source.LogFormat != nil {
		target.LogFormat = *source.LogFormat
	}
}

func mergeEnvironment(raw *rawConfig, getenv func(string) string) {
	settings := []struct {
		target *string
		name   string
	}{
		{&raw.Profile, "MONARCH_PROFILE"},
		{&raw.Output, "MONARCH_OUTPUT"},
		{&raw.Timeout, "MONARCH_TIMEOUT"},
		{&raw.LogLevel, "MONARCH_LOG_LEVEL"},
		{&raw.LogFormat, "MONARCH_LOG_FORMAT"},
	}
	for _, setting := range settings {
		if value := getenv(setting.name); value != "" {
			*setting.target = value
		}
	}
}

// Validate rejects unsafe or surprising configuration values.
func (c Config) Validate() error {
	if err := profile.Validate(c.Profile); err != nil {
		return err
	}
	if c.Output != "table" && c.Output != "json" {
		return errors.New("output must be table or json")
	}
	if c.Timeout < time.Second || c.Timeout > 5*time.Minute {
		return errors.New("timeout must be between 1s and 5m")
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("log level must be debug, info, warn, or error")
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return errors.New("log format must be text or json")
	}
	return nil
}
