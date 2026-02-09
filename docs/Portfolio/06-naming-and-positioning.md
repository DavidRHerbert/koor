# Chapter 6: Naming and Positioning

## The Naming Process

A name needed to work across multiple contexts: CLI commands (`koor-server`, `koor-cli`), package imports (`github.com/DavidRHerbert/koor`), documentation, marketing, and conversation. It also needed to be unique in the AI/agent space with no trademark conflicts.

## Names Already Taken

The AI agent landscape has claimed many obvious names:

| Name | Taken By |
|------|----------|
| Loom | Screen recording tool |
| Swarm | OpenAI's agent framework |
| Flow | Generic term, Claude-Flow exists |
| AgentCore | Various agent frameworks |
| AgentMesh | Various agent frameworks |
| CrewAI | Multi-agent framework |
| Nexus | Overloaded across tech |
| Plex | Media server |
| Hive | Multiple projects |

## Candidates Evaluated

### ai2s (AI-to-Server)

Follows the W2C naming pattern (Work-to-Cloud). Short, descriptive.

**Problem found:** KDD workshop "AI2S" (ai2sdata.github.io) occupies search results. Usable but noisy — every search returns the academic workshop first.

**Verdict:** Usable but not clean.

### ai2c (AI-to-Cloud)

Also follows the W2C pattern.

**Problem found:** US Army Futures Command has an organisation called AFC-AI2C (Army Artificial Intelligence Integration Center). 18 active GitHub repositories. Active military organisation.

**Verdict:** Dead. Can't share a name with a military organisation.

### Koor

Short for "coordinate." 4 letters, 1 syllable. No conflicts found in the AI/agent space.

**GitHub search:** No competing projects in the relevant space.

**CLI ergonomics:** `koor-server` and `koor-cli` feel natural. `koor state get api-contract` reads well.

**Package path:** `github.com/DavidRHerbert/koor` is clean and memorable.

**Verdict:** Zero conflicts, good ergonomics. Chosen.

## Market Positioning

### The One-Liner

After testing several formulations, the positioning crystallised as:

> **"Koor is Redis for AI coding agents."**

This works because Redis is universally understood by developers. It immediately communicates: shared data layer, lightweight, fast, multi-client. The analogy isn't perfect (Koor uses SQLite, not memory; Koor has specs and events, Redis doesn't), but it communicates the role accurately in five words.

### The Elevator Pitch

> Koor is a lightweight coordination server for AI coding agents. Shared state, events, and coding specs. Any LLM that can run curl has a client. One binary, zero configuration, deploy in 30 seconds.

### The Technical Hook: MCP Token Tax

> MCP routes all coordination data through the LLM's context window. That's thousands of tokens burned just to move an API contract between agents. Koor splits control (MCP, ~750 tokens) from data (REST, 0 tokens). ~35x cheaper per coordination cycle.

This hook resonates with developers who've noticed their LLM agents getting slow or expensive during multi-agent workflows. It names a problem they've felt but haven't articulated.

## The Competitive Position Statement

> As of February 2026, no single system combines: coordination + shared state + shared specifications + cross-LLM support + standalone binary. The frameworks (LangGraph, AutoGen, Claude-Flow) are in-process and vendor-locked. The protocols (A2A) lack implementations. The gateways are passthrough, not coordination hubs. Koor fills the gap.

## What the Name Communicates

**Koor** doesn't try to be clever or branded. It's functional: short for "coordinate," which is exactly what the server does. In a space full of metaphorical names (Swarm, Hive, Flow, Nexus), a direct name stands out.

The pairing with "Redis for AI coding agents" gives people two handles: the name (Koor) and the concept (Redis-like coordination layer). Either one is sufficient for them to find the project and understand what it does.

[Next: Chapter 7 — Implementation](07-implementation.md)
