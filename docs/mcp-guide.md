# MCP Guide

How to connect LLM coding agents to Koor via the Model Context Protocol.

## Overview

Koor exposes 4 MCP tools through a StreamableHTTP transport at `/mcp`. These tools handle **discovery only** — registration, finding other agents, updating intent, and getting REST endpoints. All data operations (state, specs, events) go through the REST API directly, bypassing the LLM context window.

This is the core of Koor's control plane / data plane split. MCP tools cost ~750 tokens total. A single state GET via REST costs 0 tokens (the LLM never sees it unless it needs to reason about the result).

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

**Returns** — Instance ID, token, and a message directing to REST for data operations.

### discover_instances

Find other registered agent instances.

**Parameters**

| Name | Required | Description |
|------|----------|-------------|
| `name` | No | Filter by agent name |
| `workspace` | No | Filter by workspace |

**Returns** — Count and list of matching instances with their IDs, names, workspaces, intents, and timestamps.

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

## IDE Configuration

### Claude Code

Add to your MCP settings (`.claude/settings.json` or via Claude Code MCP config):

```json
{
  "mcpServers": {
    "koor": {
      "url": "http://localhost:9800/mcp"
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json` in your workspace:

```json
{
  "mcpServers": {
    "koor": {
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

The key insight: steps 1-3 happen once (or rarely). Step 4 happens constantly and goes through REST, keeping the LLM context window clean.

## With Authentication

When the server runs with `--auth-token`, the MCP endpoint is protected by the same Bearer token. Configure your MCP client to send the token. In Claude Code:

```json
{
  "mcpServers": {
    "koor": {
      "url": "http://localhost:9800/mcp",
      "headers": {
        "Authorization": "Bearer secret123"
      }
    }
  }
}
```

## Cross-LLM Usage

Koor's MCP endpoint works with any LLM that supports MCP tools — Claude, GPT, Gemini, Ollama, or any other provider. The protocol is standardised; the agent just needs an MCP client that supports StreamableHTTP.

This is what makes Koor different from Claude Agent Teams (Claude-only): a Claude Code instance and a Cursor+GPT instance can both register with the same Koor server and coordinate through shared state and events.
