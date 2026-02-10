package wizard

import (
	"encoding/json"
	"strings"
)

// mcpConfig is the structure of .claude/mcp.json.
type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	URL string `json:"url"`
}

// GenerateMCPJSON returns the bytes for .claude/mcp.json.
func GenerateMCPJSON(serverURL string) ([]byte, error) {
	cfg := mcpConfig{
		MCPServers: map[string]mcpServerEntry{
			"koor": {
				URL: strings.TrimRight(serverURL, "/") + "/mcp",
			},
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}
