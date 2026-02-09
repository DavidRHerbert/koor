# Koor: From Problem Statement to Coordination Server

## Executive Summary

This portfolio documents the complete journey of building **Koor**, a lightweight coordination server for AI coding agents, from initial problem identification through research, architecture, and implementation.

The project started with a straightforward frustration: I run multiple LLM coding agents across different IDEs — Claude Code, Cursor, Kilo Code, and others — and I was copying and pasting between them. I wanted them to coordinate automatically.

The first attempt, **MCP-ChattTeam**, tried to solve this with MCP hubs, WebSocket bridges, and file watchers. It worked, but was fundamentally a set of workarounds for missing platform APIs.

Then two things happened. Anthropic shipped Agent SDK and Agent Teams, making ~80% of MCP-ChattTeam obsolete. And I discovered the **MCP Token Tax Problem** — the insight that MCP routes all coordination data through the LLM's context window, burning tokens on data the LLM doesn't need to reason about.

That insight led to Koor's core architecture: split **control** (MCP, thin discovery layer) from **data** (REST + CLI, direct access). The result is approximately 35x cheaper per coordination cycle compared to pure-MCP approaches.

Koor occupies a unique position in the landscape. As of February 2026, no other system combines: coordination + shared state + shared specifications + cross-LLM support + standalone binary.

## Key Numbers

| Metric | Value |
|--------|-------|
| Codebase | ~3,800 lines of Go |
| Tests | 52, all passing |
| Dependencies | 3 (modernc.org/sqlite, nhooyr.io/websocket, mark3labs/mcp-go) |
| Build targets | 6 (Linux/macOS/Windows, amd64/arm64) |
| MCP discovery tools | 4 (~750 tokens total) |
| REST endpoints | 21 |
| Token reduction | ~35x vs pure-MCP coordination |
| Time to first use | 30 seconds (download binary, run it) |

## Journey Map

| Chapter | What It Covers |
|---------|---------------|
| [01 — Problem Statement](01-problem-statement.md) | The original 4 blockers. What I needed and why existing tools fell short. |
| [02 — Landscape Research](02-landscape-research.md) | Agent SDK, Agent Teams, Claude-Flow, LangGraph, AutoGen, A2A Protocol, MCP Gateways. What exists, what's missing. |
| [03 — Token Tax Discovery](03-token-tax-discovery.md) | The MCP Token Tax Problem. How all data routes through the LLM context. The control/data plane insight. |
| [04 — Architecture Design](04-architecture-design.md) | The three shared layers. Technology choices. SQLite rationale. API surface design. |
| [05 — Ecosystem Design](05-ecosystem-design.md) | The W2C-DaCss01 constraint chain. Why the scanner must be separate. Language-agnostic core + optional companions. |
| [06 — Naming and Positioning](06-naming-and-positioning.md) | Name candidates, conflict research, final choice. "Redis for AI coding agents" positioning. |
| [07 — Implementation](07-implementation.md) | The 5-phase build. What was built in each phase. 52 tests. Cross-platform binaries. |
| [08 — Results and Reflection](08-results-and-reflection.md) | What works. What was learned. What would be done differently. Future directions. |

## Tools Used

- **Research and analysis:** Claude Opus 4.6 as research partner
- **Implementation:** Claude Opus 4.6 as pair programmer
- **Language:** Go 1.21+
- **IDE:** VS Code with Claude Code extension
- **Build:** GoReleaser for cross-platform binaries
