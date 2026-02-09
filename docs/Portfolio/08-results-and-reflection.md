# Chapter 8: Results and Reflection

## What Works

### The Core Architecture

The control plane / data plane split works exactly as designed. MCP handles discovery (~750 tokens), REST handles data (0 tokens to the LLM). The ~35x token reduction is real — agents coordinate through shared state and events without polluting their context windows.

### The Developer Experience

"Download binary, run it, use curl" delivers the 30-second time-to-first-value goal. No configuration, no database server, no Docker. `koor-server` starts and is ready. `koor-cli state set key --data '{}'` works immediately.

### The Cross-LLM Promise

Any LLM that can execute shell commands can use Koor. The MCP endpoint works with any MCP-compliant client (Claude Code, Cursor, Kilo Code). The REST API works with any HTTP client. This isn't theoretical — it's a natural consequence of using standard protocols rather than vendor-specific SDKs.

### The Validation System

The rule-based validation with regex, missing, and custom match types provides a flexible foundation for coding standard enforcement. The `applies_to` glob filtering means rules can target specific file types. This is the piece that makes shared specs actionable rather than just informational.

## What Was Learned

### Research Before Building Saves Everything

The 8 analysis documents (2,973 lines) took significant time, but they eliminated dead-end architectures before any code was written. The token tax insight, the competitive matrix, the scanner split — all emerged from analysis, not from coding and discovering problems.

Without the research phase, I would likely have built another all-MCP system (like MCP-ChattTeam v2) and hit the token tax problem months later.

### The Power of Constraint

Three constraints shaped the entire architecture:
1. **Token awareness** forced the control/data split
2. **Language agnosticism** forced the scanner separation
3. **Single binary** forced SQLite and go:embed

Each constraint eliminated options and simplified decisions. Without them, the design would have sprawled. With them, every choice had a clear justification.

### LLM-Assisted Development at Scale

The entire project — research, analysis, planning, implementation — was done with Claude Opus 4.6 as a research and development partner. This worked well for several reasons:

- **Analysis documents as context:** The 8 research docs served as persistent context across sessions. Each new session started by loading the relevant docs.
- **Plan-driven implementation:** The consolidated plan documents specified exact APIs, schemas, and test expectations. The LLM could implement from the plan without ambiguity.
- **Phased delivery:** Each phase had a clear milestone. This mapped well to LLM session boundaries.

The main challenge was context continuity across sessions. When a conversation ran out of context, the next session needed significant context restoration. Memory files and plan documents mitigated this but didn't eliminate it.

## What Would Be Done Differently

### Instance Token Auth

Instance tokens are generated on registration but not currently used for authentication. The server uses a single global Bearer token. In retrospect, per-instance tokens should authenticate requests, so each agent can only modify its own data. This would require a more nuanced auth model but is worth the complexity.

### Event Subscriptions via CLI

The CLI's polling fallback for event subscription works but isn't ideal. Adding a lightweight WebSocket client (even if it adds a dependency) would make the CLI a complete client for all operations.

### Dashboard Depth

The embedded dashboard shows basic metrics but doesn't expose the full power of the system. A richer dashboard with real-time event streaming, state editing, and instance management would make Koor more accessible to non-CLI users.

### Schema Validation on Specs

Specs are currently opaque blobs — Koor stores and serves them without understanding the content. Adding optional JSON Schema validation on spec writes would catch malformed specs early. This could be a v2 feature.

## The Unique Quadrant

Looking back at the competitive matrix from Chapter 2, Koor's position holds:

```
                    Token-Aware?
              No         │        Yes
         ┌───────────────┼──────────────────┐
Framework│ OpenAI Swarm  │ Claude-Flow      │
(in-proc)│ AutoGen       │ LangGraph        │
         ├───────────────┼──────────────────┤
Standalone│ A2A Protocol │                  │
(server) │ MCP Gateways  │  KOOR ← still   │
         │               │  alone here      │
         └───────────────┴──────────────────┘
```

No new system has appeared in this quadrant since the research was completed. The combination of token-awareness, standalone deployment, cross-LLM support, shared specs, and validation rules remains unique.

## Future Directions (v2 Considerations)

1. **A2A Protocol compatibility** — Koor could expose an A2A-compatible agent card for discovery by other A2A systems.

2. **Spec change webhooks** — Push notifications when specs or state change, rather than requiring polling or WebSocket subscription.

3. **RBAC beyond Bearer tokens** — Per-instance permissions, read-only vs read-write access, project-level scoping.

4. **Event replay and durable subscriptions** — Named subscriptions that remember their position, for agents that restart.

5. **Plugin/extension system** — Custom validation rules, custom event handlers, integration hooks.

6. **W2C Scanner** — The companion scanner for Go/templ projects, built as the first concrete example of the spec provider pattern.

## The Journey

This project started as frustration with copy-pasting between IDEs. It went through a false start (MCP-ChattTeam), a platform shift (Agent SDK and Agent Teams shipping), a research phase that produced genuine novel insights (the token tax problem), and a focused implementation that delivered a working system.

The most valuable outcome isn't the software itself — it's the architecture. The control/data plane split for LLM coordination is a pattern that applies beyond Koor. Any system that routes data through an LLM when it doesn't need to is paying the token tax. Koor shows what it looks like to not pay it.

[Back to Overview](00-overview.md)
