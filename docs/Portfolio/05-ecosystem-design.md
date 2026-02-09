# Chapter 5: Ecosystem Design

## The Constraint Chain

Koor doesn't exist in isolation. It's part of a chain of solutions, each built to solve a problem created by the previous one. Understanding this chain explains why Koor is language-agnostic and why the scanner must be separate.

| # | Problem | Solution Built | What Koor Replaces |
|---|---------|---------------|-------------------|
| 1 | No internet, long offline periods | W2C-DaCss01 CSS/JS library (zero dependencies) | Nothing — the library stays |
| 2 | LLMs don't know custom libraries | `data-ai-id` attributes on every component | Nothing — the pattern stays |
| 3 | LLMs break naming conventions | W2C AI MCP Server (11 tools for component spec lookup) | Koor serves specs/rules centrally |
| 4 | Rules are per-LLM, not centralised | CLAUDE.md, .cursorrules files | Koor centralises rules for all LLMs |
| 5 | Teams can't share context | MCP-ChattTeam coordination hub | Koor replaces the hub |
| 6 | Token cost of coordination | Identified but not solved | Koor's control/data plane split |

Each step in this chain constrained the next. The W2C-DaCss01 library exists because the product needs to work offline. The `data-ai-id` attributes exist because LLMs can't navigate custom CSS conventions without landmarks. The MCP server exists because even with landmarks, LLMs need schema data to use components correctly.

## Two Different Constraint Sets

A critical distinction emerged during the design:

**W2C-DaCss01 (the product):** Must work offline. Zero dependencies. 10+ year longevity requirement. Ships to end users. Can never require a network connection.

**Koor (the dev tool):** Developers using LLM coding agents are already online. Go dependencies are fine. Cloud deployment is fine. This is infrastructure for the development process, not the product.

This distinction freed Koor from the zero-dependency constraint that shaped W2C-DaCss01. It also meant Koor must know nothing about the W2C library specifically — it's a general-purpose coordination server, not a W2C-specific tool.

## The Integration Question

The existing W2C AI MCP server has 11 tools. The question was: can Koor absorb them all?

**Answer: No.** Half the tools require local filesystem access — scanning `.templ` files for `data-ai-id` attributes, listing components by directory, running `templ generate`, validating file content. These can't move to a remote server.

## The Clean Split

I evaluated three integration models:

**Model A (Side-by-Side):** Keep both servers running independently. Rejected — two separate processes with duplicated concerns.

**Model B (W2C as Koor Plugin):** Build W2C-specific code into Koor. Rejected — this violates the language-agnostic principle. Koor must know nothing about Go, templ, or W2C.

**Model C (Compose-Alongside):** A thin scanner handles local operations and pushes specs to Koor. Koor stores and serves them to all LLMs. The scanner is optional. **This was the chosen approach.**

### Tool Classification

| Category | Count | Tools | Where They Live |
|----------|-------|-------|----------------|
| Local-only (filesystem) | 3 | findByAIID, listComponents, runTemplGenerate | Scanner |
| Spec-based (can serve from Koor) | 6 | getComponentSchema, validateTransition, getValidTransitions, generateTemplCode, getComponentDocs, listAvailableComponents | Eliminated — Koor specs |
| Hybrid (local read + remote validate) | 1 | validateTemplFile | Scanner reads, Koor validates |
| Metrics | 1 | getMetrics | Both have their own |

The result: 11-tool MCP server becomes a 3-tool scanner + Koor.

## The Spec Provider Model

The scanner pushes specs to Koor on startup and on file change:

```
PUT /api/specs/w2c/states            ← 13 state machines from states.json
PUT /api/specs/w2c/components        ← component catalogue from directory scan
PUT /api/validate/w2c/rules          ← validation rules as structured JSON
PUT /api/specs/w2c/dom-contracts     ← data-* attribute mappings
PUT /api/specs/w2c/{component}       ← per-component docs
PUT /api/specs/w2c/tokens            ← design token definitions
```

Total: ~8-10 REST calls on startup, incremental on file change.

Once the specs are in Koor, any LLM can access them — not just the one connected to the scanner. A Claude Code instance working on the frontend and a Cursor+GPT instance working on the backend both get the same component schemas and validation rules.

## The Scanner Ecosystem

The compose-alongside model isn't limited to W2C. Any project can have a scanner that pushes specs to Koor:

```
koor-server                    (always runs, language-agnostic)
├── w2c-scanner               (optional, Go/templ)
├── react-scanner             (optional, React/TypeScript)
├── openapi-scanner           (optional, any project with OpenAPI specs)
└── dotnet-scanner            (optional, .NET)
```

Each scanner is small (a few hundred lines), project-specific, and optional. Koor doesn't know or care what pushes specs — it stores opaque JSON blobs and serves them on request.

## The W2C Scanner Specifically

Built fresh (not refactored from the 11-tool server) for clean separation:

**Dependencies:** 2 only — `mark3labs/mcp-go` (MCP server) and `fsnotify/fsnotify` (file watching).

**3 MCP tools:** `findByAIID` (scan for data-ai-id), `listComponents` (directory listing), `runTemplGenerate` (exec build).

**Background pusher:** Scans the library on startup, watches for changes, pushes specs to Koor via REST.

**Why build fresh:** The existing 11-tool server is monolithic — local file operations, schema lookups, validation, and code generation are all mixed together in one resolver. Extracting 3 tools while preserving the rest is harder than writing 3 new tools from scratch. The existing server runs unchanged during the transition and is archived once the scanner is verified.

## Key Design Decision: Language-Agnostic Core

The most important architectural decision in the ecosystem design was keeping Koor language-agnostic. Every pressure point pushed toward adding Go/templ-specific features (templ validation, component schema parsing, data-ai-id awareness). Resisting that pressure means Koor serves any language ecosystem equally.

The scanner handles language-specific concerns. Koor handles coordination. The boundary is clean.

[Next: Chapter 6 — Naming and Positioning](06-naming-and-positioning.md)
