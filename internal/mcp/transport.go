package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/DavidRHerbert/koor/internal/contracts"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/server/serverconfig"
	"github.com/DavidRHerbert/koor/internal/specs"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Transport wraps the MCP server and exposes it as an http.Handler.
type Transport struct {
	registry *instances.Registry
	specReg  *specs.Registry
	config   serverconfig.Endpoints
	handler  http.Handler
}

// New creates the MCP transport with 5 discovery/proposal tools.
func New(registry *instances.Registry, specReg *specs.Registry, endpoints serverconfig.Endpoints) *Transport {
	t := &Transport{
		registry: registry,
		specReg:  specReg,
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
			mcplib.WithString("stack", mcplib.Description("Technology stack identifier (e.g. 'goth', 'react')")),
			mcplib.WithString("capabilities", mcplib.Description("Comma-separated list of capabilities (e.g. 'code-review,testing,deployment')")),
		),
		t.handleRegisterInstance,
	)

	// Tool 2: discover_instances
	srv.AddTool(
		mcplib.NewTool("discover_instances",
			mcplib.WithDescription("Discover other registered agent instances. Optionally filter by name, workspace, stack, or capability."),
			mcplib.WithString("name", mcplib.Description("Filter by agent name")),
			mcplib.WithString("workspace", mcplib.Description("Filter by workspace")),
			mcplib.WithString("stack", mcplib.Description("Filter by technology stack (e.g. 'goth', 'react')")),
			mcplib.WithString("capability", mcplib.Description("Filter by capability (e.g. 'code-review')")),
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

	// Tool 5: propose_rule
	srv.AddTool(
		mcplib.NewTool("propose_rule",
			mcplib.WithDescription("Propose a validation rule based on a problem you solved. The rule will be reviewed by the user before activation."),
			mcplib.WithString("project", mcplib.Required(), mcplib.Description("Project the rule applies to")),
			mcplib.WithString("rule_id", mcplib.Required(), mcplib.Description("Unique rule identifier (e.g. 'no-hardcoded-colors')")),
			mcplib.WithString("pattern", mcplib.Required(), mcplib.Description("Regex pattern or custom check name")),
			mcplib.WithString("message", mcplib.Required(), mcplib.Description("Human-readable violation message")),
			mcplib.WithString("severity", mcplib.Description("'error' or 'warning' (default: error)")),
			mcplib.WithString("match_type", mcplib.Description("'regex', 'missing', or 'custom' (default: regex)")),
			mcplib.WithString("stack", mcplib.Description("Technology stack this rule targets (empty = universal)")),
			mcplib.WithString("proposed_by", mcplib.Description("Instance ID of the proposing agent")),
			mcplib.WithString("context", mcplib.Description("Description of the issue that led to this rule")),
		),
		t.handleProposeRule,
	)

	// Tool 6: validate_contract
	srv.AddTool(
		mcplib.NewTool("validate_contract",
			mcplib.WithDescription("Validate a JSON payload against a stored API contract. Use this to check that your request/response matches the agreed contract before sending."),
			mcplib.WithString("project", mcplib.Required(), mcplib.Description("Project name (e.g. 'Truck-Wash')")),
			mcplib.WithString("contract", mcplib.Required(), mcplib.Description("Contract spec name (e.g. 'api-contract')")),
			mcplib.WithString("endpoint", mcplib.Required(), mcplib.Description("Endpoint key (e.g. 'POST /api/trucks')")),
			mcplib.WithString("direction", mcplib.Required(), mcplib.Description("'request', 'response', 'query', or 'error'")),
			mcplib.WithString("payload", mcplib.Required(), mcplib.Description("JSON payload to validate (as a string)")),
		),
		t.handleValidateContract,
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
	stack := getArg(req, "stack")
	capsStr := getArg(req, "capabilities")

	if name == "" {
		return mcplib.NewToolResultError("name is required"), nil
	}

	inst, err := t.registry.Register(ctx, name, workspace, intent, stack)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("registration failed: %v", err)), nil
	}

	// Set capabilities if provided (comma-separated string).
	if capsStr != "" {
		caps := strings.Split(capsStr, ",")
		for i := range caps {
			caps[i] = strings.TrimSpace(caps[i])
		}
		t.registry.SetCapabilities(ctx, inst.ID, caps)
		inst.Capabilities = caps
	}

	data, _ := json.MarshalIndent(map[string]any{
		"instance_id":   inst.ID,
		"token":         inst.Token,
		"name":          inst.Name,
		"workspace":     inst.Workspace,
		"intent":        inst.Intent,
		"stack":         inst.Stack,
		"capabilities":  inst.Capabilities,
		"status":        inst.Status,
		"registered_at": inst.RegisteredAt,
		"message":       "Registered (status: pending). Activate via CLI: ./koor-cli activate " + inst.ID,
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}

func (t *Transport) handleDiscoverInstances(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name := getArg(req, "name")
	workspace := getArg(req, "workspace")
	stack := getArg(req, "stack")
	capability := getArg(req, "capability")

	items, err := t.registry.Discover(ctx, name, workspace, stack, capability)
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
			"state_get":      "GET /api/state/{key...}",
			"state_put":      "PUT /api/state/{key...}",
			"state_delete":   "DELETE /api/state/{key...}",
			"specs_list":     "GET /api/specs/{project}",
			"specs_get":      "GET /api/specs/{project}/{name}",
			"specs_put":      "PUT /api/specs/{project}/{name}",
			"specs_delete":   "DELETE /api/specs/{project}/{name}",
			"events_publish": "POST /api/events/publish",
			"events_history": "GET /api/events/history",
			"events_ws":      "GET /api/events/subscribe",
			"instances_list": "GET /api/instances",
			"instance_get":   "GET /api/instances/{id}",
			"rules_export":      "GET /api/rules/export",
			"rules_import":      "POST /api/rules/import",
			"instance_activate": "POST /api/instances/{id}/activate",
		},
		"cli": map[string]string{
			"install": "go install github.com/DavidRHerbert/koor/cmd/koor-cli@latest",
			"usage":   "koor-cli --help",
		},
		"message": "Use these REST endpoints or ./koor-cli for data operations. MCP is for discovery and rule proposals only. Activate your instance first: ./koor-cli activate <instance-id>",
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}

func (t *Transport) handleProposeRule(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	project := getArg(req, "project")
	ruleID := getArg(req, "rule_id")
	pattern := getArg(req, "pattern")
	message := getArg(req, "message")

	if project == "" {
		return mcplib.NewToolResultError("project is required"), nil
	}
	if ruleID == "" {
		return mcplib.NewToolResultError("rule_id is required"), nil
	}
	if pattern == "" {
		return mcplib.NewToolResultError("pattern is required"), nil
	}

	rule := specs.Rule{
		Project:    project,
		RuleID:     ruleID,
		Severity:   getArg(req, "severity"),
		MatchType:  getArg(req, "match_type"),
		Pattern:    pattern,
		Message:    message,
		Stack:      getArg(req, "stack"),
		ProposedBy: getArg(req, "proposed_by"),
		Context:    getArg(req, "context"),
	}

	if err := t.specReg.ProposeRule(ctx, rule); err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("propose rule failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(map[string]any{
		"project": project,
		"rule_id": ruleID,
		"status":  "proposed",
		"message": "Rule proposed successfully. It will be reviewed by the user before activation.",
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}

func (t *Transport) handleValidateContract(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	project := getArg(req, "project")
	contractName := getArg(req, "contract")
	endpoint := getArg(req, "endpoint")
	direction := getArg(req, "direction")
	payloadStr := getArg(req, "payload")

	if project == "" || contractName == "" || endpoint == "" || direction == "" {
		return mcplib.NewToolResultError("project, contract, endpoint, and direction are all required"), nil
	}

	// Load contract from specs.
	spec, err := t.specReg.Get(ctx, project, contractName)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("contract not found: %s/%s (%v)", project, contractName, err)), nil
	}

	contract, err := contracts.Parse(spec.Data)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("stored spec is not a valid contract: %v", err)), nil
	}

	// Parse the payload JSON.
	var payload map[string]any
	if payloadStr != "" {
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("invalid payload JSON: %v", err)), nil
		}
	} else {
		payload = map[string]any{}
	}

	violations := contracts.ValidatePayload(contract, endpoint, direction, payload)
	if violations == nil {
		violations = []contracts.Violation{}
	}

	data, _ := json.MarshalIndent(map[string]any{
		"valid":      len(violations) == 0,
		"endpoint":   endpoint,
		"direction":  direction,
		"violations": violations,
	}, "", "  ")

	return mcplib.NewToolResultText(string(data)), nil
}
