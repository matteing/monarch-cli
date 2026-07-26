package mcpserver

// EmptyInput is used by MCP tools that take no arguments.
type EmptyInput struct{}

// TransactionInput identifies a transaction.
type TransactionInput struct {
	ID string `json:"id"`
}
