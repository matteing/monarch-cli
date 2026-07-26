// Command monarch provides read-only Monarch Money access through a CLI and
// a local Model Context Protocol server.
package main

import (
	"os"

	"github.com/matteing/monarch-cli/internal/command"
)

func main() {
	if err := command.Execute(); err != nil {
		os.Exit(command.ExitCode(err))
	}
}
