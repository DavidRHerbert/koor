# Chapter 1: The Problem Statement

## The Working Context

I run LLMs on each of my codebases at project level: one for frontend, one for backend, plus test projects and reference projects — up to 12 open at once. I use multiple IDE-based agents: Cursor, Kilo Code, Claude Code, Antigravity, and others, evaluating each for their strengths.

The workflow was: write code in one IDE, copy the output, paste it into another IDE as context, explain what happened, and ask the next agent to continue. This was manual, error-prone, and slow. I wanted a programmatic API — an external system that could send prompts to, and receive structured responses from, running IDE agents.

## The Four Blockers (Original Problem Statement)

In early 2026, I documented four fundamental blockers preventing automated coordination between IDE-based LLM agents:

### 1. No Claude Code API

There was no way to programmatically send prompts to Claude Code or receive structured responses. The only interface was the interactive CLI. To automate coordination, I needed:

- Inbound prompt injection (send tasks to a running agent)
- Structured response output (get results as JSON, not chat text)
- Event-driven callbacks (know when things happen)
- Context persistence (resume sessions with full history)

### 2. MCP Can't Push Events to the LLM

MCP (Model Context Protocol) was client-initiated only. The server could respond to tool calls but couldn't proactively notify the LLM. If Agent A changed an API contract, there was no way to alert Agent B through MCP.

### 3. Can't Orchestrate Across Workspaces

Each LLM instance was confined to its own workspace. There was no mechanism to coordinate work across projects — no shared task list, no inter-agent messaging, no awareness of what other agents were doing.

### 4. Workarounds All Failed

Every attempted workaround had fatal limitations:

| Workaround | Problem |
|------------|---------|
| File-based polling | Required human intervention to trigger reads |
| Spawning CLI instances | Lost persistent context between sessions |
| Raw Anthropic API | Lost Claude Code's tool capabilities (file editing, terminal, etc.) |
| Sub-agents (Task tool) | Confined to one workspace, no cross-project coordination |

## The First Attempt: MCP-ChattTeam

I built MCP-ChattTeam to work around these limitations. It used MCP hubs, WebSocket bridges, and file watchers to create a coordination layer between LLM instances. The architecture included:

- A central hub server that agents connected to via MCP
- WebSocket bridges for real-time event delivery
- File watchers for detecting changes across workspaces
- A shared state store for API contracts and configuration

It worked — agents could share data and react to changes. But it was fundamentally a set of workarounds. Every component existed because a platform capability was missing, not because the architecture demanded it.

## What Changed

Between the original problem statement and the start of the Koor project, Anthropic shipped two major features:

**Agent SDK** — Solved blocker #1 completely. Provides programmatic access to Claude Code: `claude -p "task"`, `--output-format json`, `--resume <session_id>`, streaming events.

**Agent Teams** — Solved blocker #3. Team lead coordinates, teammates are independent sessions, shared task list with dependencies, direct inter-agent messaging via mailbox.

**MCP server-initiated requests** — Partially solved blocker #2. Sampling (server asks client LLM to generate) and elicitation (server asks user for input) are now in the spec.

This made ~80% of MCP-ChattTeam obsolete overnight.

## The Remaining 20%

What Agent SDK and Agent Teams don't cover:

1. **Cross-IDE, cross-LLM coordination** — Agent Teams is Claude-to-Claude only. A Claude Code instance can't coordinate with a Cursor+GPT instance.

2. **Shared state store** — Agent Teams has a task list but not a general-purpose shared state layer for API contracts, schemas, and configuration.

3. **Pub/sub events with pattern matching** — Agent Teams has direct messaging but not topic-based pub/sub with history.

4. **Shared specifications with validation** — No system centralises coding standards and validation rules across multiple agents.

5. **Standalone, self-hosted operation** — Agent Teams requires Anthropic's infrastructure. No offline or self-hosted option.

These remaining gaps defined the scope for Koor.

[Next: Chapter 2 — Landscape Research](02-landscape-research.md)
