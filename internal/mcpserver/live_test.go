package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestLiveStdioServer is opt-in because it reads real Monarch data through the
// named binary and the caller's saved OS-keyring session. It never logs tool
// results. Set MONARCH_LIVE_BINARY to enable it.
func TestLiveStdioServer(t *testing.T) {
	binary := os.Getenv("MONARCH_LIVE_BINARY")
	if binary == "" {
		t.Skip("set MONARCH_LIVE_BINARY to run the live MCP smoke test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "monarch-live-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 8 {
		t.Fatalf("live tool count = %d, want 8", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		assertToolMetadata(t, tool)
	}

	callLiveTool(t, ctx, session, "monarch_accounts_list", map[string]any{"include_hidden": true, "include_deactivated": true}, nil)
	callLiveTool(t, ctx, session, "monarch_categories_list", map[string]any{}, nil)
	callLiveTool(t, ctx, session, "monarch_budgets_get", map[string]any{}, nil)
	callLiveTool(t, ctx, session, "monarch_cashflow_summary", map[string]any{}, nil)
	callLiveTool(t, ctx, session, "monarch_financial_overview", map[string]any{}, nil)

	var page struct {
		Transactions []struct {
			ID string `json:"id"`
		} `json:"transactions"`
		NextCursor string `json:"next_cursor"`
	}
	callLiveTool(t, ctx, session, "monarch_transactions_list", map[string]any{"limit": 2}, &page)
	if len(page.Transactions) == 0 {
		t.Log("live account has no transactions; list shape was verified, skipping transaction detail")
		return
	}
	if page.Transactions[0].ID == "" {
		t.Fatal("live transaction list returned an empty transaction ID")
	}
	callLiveTool(t, ctx, session, "monarch_transaction_get", map[string]any{"id": page.Transactions[0].ID}, nil)
	if page.NextCursor != "" {
		callLiveTool(t, ctx, session, "monarch_transactions_list", map[string]any{"limit": 2, "cursor": page.NextCursor}, nil)
	}
}

func callLiveTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments any, target any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError || result.StructuredContent == nil || len(result.Content) == 0 {
		t.Fatalf("call %s returned an incomplete or error result", name)
	}
	if target == nil {
		return
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("encode %s result: %v", name, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode %s result shape: %v", name, err)
	}
}
