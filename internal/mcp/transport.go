package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/server/serverconfig"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Transport wraps the MCP server and exposes it as an http.Handler.
type Transport struct {
	registry *instances.Registry
	config   serverconfig.Endpoints
	handler  http.Handler
}

// New creates the MCP transport with 4 discovery tools.
func New(registry *instances.Registry, endpoints serverconfig.Endpoints) *Transport {
	t := &Transport{
		registry: registry,
		config:   endpoints,
	}

	srv := mcpserver.NewMCPServer(
		"koor",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
	)

	// Tool 1: register_instance
	srv.AddTool(
		mcplib.NewTool("register_instance",
			mcplib.WithDescription("Register this agent instance with the Koor coordination server. Returns an instance ID and token for subsequent requests."),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Agent name (e.g. 'claude-frontend')")),
			mcplib.WithString("workspace", mcplib.Description("Workspace path or identifier")),
			mcplib.WithString("intent", mcplib.Description("Current intent or task description")),
		),
		t.handleRegisterInstance,
	)

	// Tool 2: discover_instances
	srv.AddTool(
		mcplib.NewTool("discover_instances",
			mcplib.WithDescription("Discover other registered agent instances. Optionally filter by name or workspace."),
			mcplib.WithString("name", mcplib.Description("Filter by agent name")),
			mcplib.WithString("workspace", mcplib.Description("Filter by workspace")),
		),
		t.handleDiscoverInstances,
	)

	// Tool 3: set_intent
	srv.AddTool(
		mcplib.NewTool("set_intent",
			mcplib.WithDescription("Update the current intent/task for a registered instance. Also refreshes the last_seen timestamp."),
			mcplib.WithString("instance_id", mcplib.Required(), mcplib.Description("Instance ID from register_instance")),
			mcplib.WithString("intent", mcplib.Required(), mcplib.Description("New intent or task description")),
		),
		t.handleSetIntent,
	)

	// Tool 4: get_endpoints
	srv.AddTool(
		mcplib.NewTool("get_endpoints",
			mcplib.WithDescription("Get the REST API and CLI endpoints for direct data access. Use these endpoints with curl or koor-cli instead of MCP for data operations."),
		),
		t.handleGetEndpoints,
	)

	streamable := mcpserver.NewStreamableHTTPServer(srv)
	t.handler = streamable

	return t
}

// ServeHTTP implements http.Handler.
func (t *Transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.handler.ServeHTTP(w, r)
}

// getArg extracts a string argument from the tool request.
func getArg(req mcplib.CallToolRequest, key string) string {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return ""
	}
	v, _ := args[key].(string)
	return v
}

func (t *Transport) handleRegisterInstance(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name := getArg(req, "name")
	workspace := getArg(req, "workspace")
	intent := getArg(req, "intent")

	if name == "" {
		return mcplib.NewToolResultError("name is required"), nil
	}

	inst, err := t.registry.Register(ctx, name, workspace, intent)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("registration failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(map[string]any{
		"instance_id":   inst.ID,
		"token":         inst.Token,
		"name":          inst.Name,
		"workspace":     inst.Workspace,
		"intent":        inst.Intent,
		"registered_at": inst.RegisteredAt,
		"message":       "Registered successfully. Use the token for authenticated requests. Use REST API or koor-cli for data operations.",
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}

func (t *Transport) handleDiscoverInstances(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name := getArg(req, "name")
	workspace := getArg(req, "workspace")

	items, err := t.registry.Discover(ctx, name, workspace)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("discovery failed: %v", err)), nil
	}

	if items == nil {
		items = []instances.Summary{}
	}

	data, _ := json.MarshalIndent(map[string]any{
		"count":     len(items),
		"instances": items,
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}

func (t *Transport) handleSetIntent(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	instanceID := getArg(req, "instance_id")
	intent := getArg(req, "intent")

	if instanceID == "" {
		return mcplib.NewToolResultError("instance_id is required"), nil
	}
	if intent == "" {
		return mcplib.NewToolResultError("intent is required"), nil
	}

	if err := t.registry.SetIntent(ctx, instanceID, intent); err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("set intent failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(map[string]any{
		"instance_id": instanceID,
		"intent":      intent,
		"message":     "Intent updated successfully.",
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}

func (t *Transport) handleGetEndpoints(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	data, _ := json.MarshalIndent(map[string]any{
		"api_base": t.config.APIBase,
		"endpoints": map[string]string{
			"state_list":     "GET /api/state",
			"state_get":      "GET /api/state/{key}",
			"state_put":      "PUT /api/state/{key}",
			"state_delete":   "DELETE /api/state/{key}",
			"specs_list":     "GET /api/specs/{project}",
			"specs_get":      "GET /api/specs/{project}/{name}",
			"specs_put":      "PUT /api/specs/{project}/{name}",
			"specs_delete":   "DELETE /api/specs/{project}/{name}",
			"events_publish": "POST /api/events/publish",
			"events_history": "GET /api/events/history",
			"events_ws":      "GET /api/events/subscribe",
			"instances_list": "GET /api/instances",
			"instance_get":   "GET /api/instances/{id}",
		},
		"cli": map[string]string{
			"install": "go install github.com/DavidRHerbert/koor/cmd/koor-cli@latest",
			"usage":   "koor-cli --help",
		},
		"message": "Use these REST endpoints or koor-cli for data operations. MCP is for discovery only.",
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}
