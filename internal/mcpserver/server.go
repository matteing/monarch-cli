// Package mcpserver exposes the Monarch reader through the Model Context Protocol.
package mcpserver

import (
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/matteing/monarch-cli/internal/monarch"
)

// NewWithLogger constructs an MCP server and forwards protocol
// lifecycle diagnostics to logger when it is non-nil.
func NewWithLogger(service monarch.Service, version string, logger *slog.Logger) *mcp.Server {
	var options *mcp.ServerOptions
	if logger != nil {
		options = &mcp.ServerOptions{Logger: logger}
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "monarch-cli", Version: version}, options)
	registerTools(server, service)
	return server
}
