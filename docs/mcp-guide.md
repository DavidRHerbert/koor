# MCP Guide

How to connect LLM coding agents to Koor via the Model Context Protocol.

## Overview

Koor exposes 5 MCP tools through a StreamableHTTP transport at `/mcp`. These tools handle **discovery and rule proposals** — registration, finding other agents, updating intent, getting REST endpoints, and proposing validation rules. All data operations (state, specs, events) go through the REST API directly, bypassing the LLM context window.

This is the core of Koor's control plane / data plane split. MCP tools are lightweight. A single state GET via REST costs 0 tokens (the LLM never sees it unless it needs to reason about the result).

## MCP Endpoint

```
POST http://localhost:9800/mcp
```

Transport: StreamableHTTP (supported by MCP client libraries).

## Available Tools

### register_instance

Register this agent with Koor. Call this once when the agent starts.

**Parameters**

| Name | Required | Description |
|------|----------|-------------|
| `name` | Yes | Agent name (e.g. `claude-frontend`, `cursor-backend`) |
| `workspace` | No | Workspace path or project identifier |
| `intent` | No | Current task or goal description |
| `stack` | No | Technology stack identifier (e.g. `goth`, `react`) |

**Returns** — Instance ID, token, stack, and a message directing to REST for data operations.

### discover_instances

Find other registered agent instances.

**Parameters**

| Name | Required | Description |
|------|----------|-------------|
| `name` | No | Filter by agent name |
| `workspace` | No | Filter by workspace |
| `stack` | No | Filter by technology stack (e.g. `goth`, `react`) |

**Returns** — Count and list of matching instances with their IDs, names, workspaces, intents, stacks, and timestamps.

### set_intent

Update the current task/intent for a registered instance. Also refreshes the `last_seen` timestamp (acts as a heartbeat).

**Parameters**

| Name | Required | Description |
|------|----------|-------------|
| `instance_id` | Yes | ID from `register_instance` |
| `intent` | Yes | New task description |

### get_endpoints

Get the full list of REST API endpoints and CLI install instructions. No parameters. Call this to learn how to access Koor's data plane directly.

**Returns** — API base URL, all endpoint paths, CLI install command.

### propose_rule

Propose a validation rule after solving a problem. The rule is stored as `proposed` and must be accepted by the user before it activates. This enables LLM agents to learn from issues they fix and build a shared knowledge base of rules over time.

**Parameters**

| Name | Required | Description |
|------|----------|-------------|
| `project` | Yes | Project the rule applies to |
| `rule_id` | Yes | Unique rule identifier (e.g. `no-hardcoded-colors`) |
| `pattern` | Yes | Regex pattern or custom check name |
| `message` | Yes | Human-readable violation message |
| `severity` | No | `error` or `warning` (default: `error`) |
| `match_type` | No | `regex`, `missing`, or `custom` (default: `regex`) |
| `stack` | No | Technology stack this rule targets (empty = universal) |
| `proposed_by` | No | Instance ID of the proposing agent |
| `context` | No | Description of the issue that led to this rule |

**Returns** — Confirmation that the rule was proposed, with project, rule_id, and status.

## IDE Configuration

### Claude Code

Add to your MCP settings (`.claude/settings.json` or via Claude Code MCP config):

```json
{
  "mcpServers": {
    "koor": {
      "type": "http",
      "url": "http://localhost:9800/mcp"
    }
  }
}
```

> **Important:** The `"type": "http"` field is required for Claude Code to connect via StreamableHTTP transport. Without it, the MCP connection will silently fail.

### Cursor

Add to `.cursor/mcp.json` in your workspace:

```json
{
  "mcpServers": {
    "koor": {
      "type": "http",
      "url": "http://localhost:9800/mcp"
    }
  }
}
```

### Kilo Code

Add via the Kilo Code MCP settings panel:

- **Name:** `koor`
- **Transport:** StreamableHTTP
- **URL:** `http://localhost:9800/mcp`

### Other MCP Clients

Any MCP client that supports StreamableHTTP transport can connect. Point it at:

```
http://localhost:9800/mcp
```

If the server has auth enabled, the MCP endpoint is behind the same auth middleware. Ensure your MCP client sends the `Authorization: Bearer <token>` header.

## Typical Workflow

1. **Agent starts:** Calls `register_instance` to register itself
2. **Agent discovers peers:** Calls `discover_instances` to find other agents
3. **Agent gets endpoints:** Calls `get_endpoints` to learn the REST API
4. **Agent reads/writes data:** Uses REST directly (curl, HTTP client) — not MCP
5. **Agent updates intent:** Calls `set_intent` as tasks change
6. **Agent learns from fixes:** Calls `propose_rule` after solving an issue to suggest a new validation rule

The key insight: steps 1-3 happen once (or rarely). Steps 4-5 happen constantly and go through REST, keeping the LLM context window clean. Step 6 happens occasionally, building a knowledge base of learned rules over time.

## With Authentication

When the server runs with `--auth-token`, the MCP endpoint is protected by the same Bearer token. Configure your MCP client to send the token. In Claude Code:

```json
{
  "mcpServers": {
    "koor": {
      "type": "http",
      "url": "http://localhost:9800/mcp",
      "headers": {
        "Authorization": "Bearer secret123"
      }
    }
  }
}
```

## Multi-Agent Workflow

MCP registration is the foundation of Koor's multi-agent coordination pattern. In a typical multi-agent project:

1. **Controller agent** registers with Koor, reads the project plan, and writes task assignments to Koor state
2. **Frontend/Backend agents** register with Koor, read their tasks from state, and publish events when done
3. Agents communicate through Koor — never through the user's clipboard

The Controller generates tailored AGENTS.md files for each agent, including the MCP config and task-checking instructions. The user only sets up the Controller manually; everything else flows from it.

See **[Multi-Agent Workflow](multi-agent-workflow.md)** for the full guide.

## Cross-LLM Usage

Koor's MCP endpoint works with any LLM that supports MCP tools — Claude, GPT, Gemini, Ollama, or any other provider. The protocol is standardised; the agent just needs an MCP client that supports StreamableHTTP.

This is what makes Koor different from Claude Agent Teams (Claude-only): a Claude Code instance and a Cursor+GPT instance can both register with the same Koor server and coordinate through shared state and events.
