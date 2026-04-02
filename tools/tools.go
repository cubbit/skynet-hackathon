package tools

import (
	"github.com/cubbit/ercubbit/mcp"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// Register wires all MCP tools into the server.
func Register(s *mcp.Server) {
	registerSetupTools(s)
	registerStorageTools(s)
	registerScheduleTools(s)
}

// toolText returns a successful text result.
func toolText(text string) (*mcpgo.CallToolResult, error) {
	return mcpgo.NewToolResultText(text), nil
}

// toolError returns a tool-level error result (not a Go error — keeps MCP connection alive).
func toolError(msg string) (*mcpgo.CallToolResult, error) {
	return mcpgo.NewToolResultError(msg), nil
}
