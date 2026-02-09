# Chapter 2: Landscape Research

## Methodology

Before designing anything, I surveyed every existing system that claimed to coordinate AI coding agents. The goal was to find the gap — what's already solved and what isn't. Eight systems were evaluated against five criteria: coordination, shared state, shared specifications, cross-LLM support, and standalone deployment.

## Systems Evaluated

### 1. Claude Agent Teams (Anthropic)

Anthropic's first-party multi-agent system. A team lead assigns tasks to teammates, which are independent Claude Code sessions.

**Strengths:** Native integration, shared task list with dependencies, direct messaging via mailbox, hooks for quality gates.

**Limitations:** Claude-only. No shared state layer beyond the task list. No pub/sub events. Requires Anthropic infrastructure — no self-hosted option.

### 2. Claude-Flow

Third-party orchestration framework built on Claude Code. ~250,000 lines of code. Supports shared state via an in-memory store.

**Strengths:** Rich orchestration patterns, shared state between agents.

**Limitations:** Claude-only. In-process (not a standalone server). No cross-LLM support. Heavy — the opposite of the lightweight coordination server I needed.

### 3. LangGraph (LangChain)

Graph-based agent orchestration in Python. Defines agent workflows as directed graphs with state management.

**Strengths:** Mature state management, well-documented, large community.

**Limitations:** In-process Python framework, not a standalone server. No cross-LLM coordination between separate processes. Focused on pipeline orchestration, not multi-agent coordination across IDEs.

### 4. Microsoft AutoGen

Multi-agent conversation framework. Agents communicate through conversation patterns (sequential, group chat, nested).

**Strengths:** Flexible conversation patterns, good at multi-agent dialogue.

**Limitations:** In-process Python. Designed for agent conversations, not cross-IDE coordination. No standalone deployment.

### 5. OpenAI Swarm

Lightweight multi-agent framework. Agents hand off tasks to each other through function calls.

**Strengths:** Simple, minimal abstraction.

**Limitations:** Stateless — everything routes through the LLM. No external state. No standalone deployment. OpenAI-only.

### 6. Google A2A Protocol

Agent-to-Agent protocol specification. Defines how agents discover and communicate with each other.

**Strengths:** Cross-vendor by design, standard protocol, agent cards for discovery.

**Limitations:** Transport layer, not a coordination hub. No shared state. No spec storage. No standalone implementation to deploy.

### 7. MCP Gateways

Proxy servers that sit between MCP clients and servers, adding caching, rate limiting, and multi-provider support.

**Strengths:** Cross-LLM, standalone, can serve multiple providers.

**Limitations:** Don't change the fundamental architecture — all data still routes through the LLM. No shared state or coordination primitives. Caching helps but doesn't solve the token tax.

### 8. W2C AI MCP Server (My Own)

The existing MCP server I built for the W2C-DaCss01 component library. 11 tools for component discovery, schema lookup, validation, and code generation.

**Strengths:** Cross-LLM via MCP, standalone binary, shared specs for UI components.

**Limitations:** Scoped to one UI library. No general-purpose coordination. All data routes through MCP (token tax applies).

## The Competitive Matrix

| System | Coordination | Shared State | Shared Specs | Cross-LLM | Standalone |
|--------|:---:|:---:|:---:|:---:|:---:|
| Claude Agent Teams | Yes | No | No | No | No |
| Claude-Flow | Yes | Yes | No | No | No |
| LangGraph | Yes | Yes | No | No | No |
| AutoGen | Yes | Yes | No | No | No |
| A2A Protocol | Yes | No | No | Yes | No |
| MCP Gateways | No | No | No | Yes | Yes |
| W2C AI MCP | No | No | Yes | Yes* | Yes |
| **Koor** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |

No single existing system covers all five criteria. The frameworks (Claude-Flow, LangGraph, AutoGen) are in-process and vendor-locked. The protocols (A2A) lack implementations. The gateways are passthrough, not coordination hubs.

## The Innovation Map

Plotting systems on two axes — token-awareness and deployment model — revealed a clear empty quadrant:

```
                    Token-Aware?
              No         │        Yes
         ┌───────────────┼──────────────────┐
Framework│ OpenAI Swarm  │ Claude-Flow      │
(in-proc)│ AutoGen       │ LangGraph        │
         ├───────────────┼──────────────────┤
Standalone│ A2A Protocol │                  │
(server) │ MCP Gateways  │  KOOR ← here    │
         └───────────────┴──────────────────┘
```

Token-aware + standalone server + cross-LLM = the quadrant nobody occupied.

## Key Insight

The gap wasn't "better orchestration" — LangGraph, AutoGen, and Agent Teams all do orchestration well. The gap was **lightweight, token-aware coordination primitives that work across any LLM and deploy as a standalone binary**. Not a framework to embed, but a server to connect to.

This framing — coordination primitives rather than orchestration framework — defined Koor's entire design philosophy.

[Next: Chapter 3 — Token Tax Discovery](03-token-tax-discovery.md)
