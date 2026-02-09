# Chapter 3: The Token Tax Discovery

## The Problem No One Was Talking About

During the landscape research, I noticed something that none of the existing systems addressed: **MCP routes all coordination data through the LLM's context window**.

When an agent uses an MCP tool to read shared state — an API contract, a component schema, a configuration blob — the entire response flows through the LLM:

```
Agent asks MCP tool → MCP server reads data → Response flows into LLM context → LLM sees everything
```

Every byte of that data costs tokens. A 500-line API contract is roughly 4,000 tokens. If Agent A asks Agent B's MCP server for the current contract, those 4,000 tokens land in A's context window — even if A only needs to check whether the version number changed.

This is the **MCP Token Tax**: the cost of routing coordination data through a channel designed for reasoning, not data movement.

## The Economics

I calculated the token cost of a typical 10-cycle coordination workflow:

| Operation | Pure MCP | Direct REST |
|-----------|----------|-------------|
| Register + discover (one-time) | ~500 tokens | ~500 tokens (same — MCP is fine here) |
| Read 500-line contract | ~4,000 tokens | ~50 tokens (curl command + result check) |
| Push update to shared state | ~2,000 tokens | ~30 tokens (curl PUT) |
| Receive change notification | ~1,000 tokens | ~20 tokens (check event log) |
| **10 full coordination cycles** | **~70,000+ tokens** | **~2,000 tokens** |
| **Reduction** | Baseline | **~35x cheaper** |

The insight wasn't just about cost. Context window pollution is worse than the dollar amount suggests. Stuffing 4,000 tokens of API contract JSON into the context window pushes out other context the LLM actually needs for reasoning. It's not just expensive — it degrades quality.

## The Control Plane / Data Plane Split

The solution came from network engineering, where the concept is well-established: the **control plane** handles routing decisions (lightweight, infrequent), and the **data plane** handles actual traffic (heavyweight, frequent).

Applied to AI agent coordination:

**Control Plane (MCP, LLM-touched):**
- `register_instance` — Tell the hub who you are (~200 tokens)
- `discover_instances` — Find other agents (~150 tokens)
- `set_intent` — Update what you're working on (~100 tokens)
- `get_endpoints` — Learn the REST API URLs (~300 tokens)
- **Total: ~750 tokens**

**Data Plane (REST + CLI, LLM-bypassed):**
- `GET /api/state/{key}` — Read shared state (0 tokens to LLM)
- `PUT /api/state/{key}` — Write shared state (0 tokens to LLM)
- `POST /api/events/publish` — Publish event (0 tokens to LLM)
- `GET /api/events/subscribe` — Stream events (0 tokens to LLM)

The LLM uses MCP to discover what's available and where to find it. Then it uses curl (or any HTTP client) to actually move data. The data never enters the context window unless the LLM explicitly needs to reason about it.

## Why This Works

LLM coding agents already have shell access — that's how they edit files, run tests, and execute commands. A curl command is no different from a git command or a build command. The LLM decides what to read and what to write, but the data moves through the shell, not through the context window.

When the LLM does need to see the data (e.g., to understand an API contract before generating code), it reads it intentionally — the same way it would read a file from disk. The difference is that it's opt-in rather than forced.

## The 35x Reduction

The exact multiplier depends on the workload, but the principle is consistent: MCP discovery is a one-time cost (~750 tokens), while data operations happen constantly. Moving the frequent operations off the LLM channel produces dramatic savings.

For a session with 10 coordination cycles:
- **Pure MCP:** ~70,000 tokens (all data routes through context)
- **Control/data split:** ~2,000 tokens (discovery + intentional reads)
- **Ratio:** ~35x

For longer sessions with more coordination, the ratio gets even better because the one-time discovery cost is amortised across more data operations.

## What This Meant for the Design

The Token Tax insight became the architectural foundation for Koor:

1. **MCP serves exactly 4 tools** — registration, discovery, intent, endpoints. Nothing more.
2. **All data operations are REST** — state, specs, events, validation. Direct HTTP.
3. **The CLI is a first-class citizen** — `koor-cli state get api-contract` is as natural as `git status`.
4. **No MCP tools for data CRUD** — this was a deliberate exclusion, not an oversight.

This is what makes Koor architecturally distinct from every system in the competitive matrix. The others are either all-MCP (token expensive) or all-framework (vendor locked). Koor is the only system designed from the ground up around token-aware data movement.

[Next: Chapter 4 — Architecture Design](04-architecture-design.md)
