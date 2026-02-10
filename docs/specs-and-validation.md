# Specs and Validation

Koor provides per-project specification storage and rule-based content validation. Specs are shared data blobs (JSON schemas, API contracts, component definitions). Validation rules check content against patterns.

## Specs

### What Are Specs?

Specs are named data blobs stored under a project. They are the "shared truth" that multiple agents reference — API schemas, component definitions, coding standards, configuration templates.

Key properties:
- Keyed by `{project}/{name}` (composite primary key)
- Any JSON content
- Auto-incrementing version on each update
- SHA-256 hash for ETag-based caching
- Lightweight list operation (names and versions only, no data blobs)

### Storing Specs

**Via REST:**

```bash
curl -X PUT http://localhost:9800/api/specs/w2c-forms/button-schema \
  -H "Content-Type: application/json" \
  -d '{
    "states": ["idle", "hover", "active", "disabled"],
    "props": {"variant": ["primary", "secondary", "ghost"]},
    "css_class": "c-button"
  }'
```

**Via CLI:**

```bash
koor-cli specs set w2c-forms/button-schema --data '{"states":["idle","hover","active"]}'
koor-cli specs set w2c-forms/modal-schema --file ./schemas/modal.json
```

### Reading Specs

**List specs for a project:**

```bash
curl http://localhost:9800/api/specs/w2c-forms
```

```json
{
  "project": "w2c-forms",
  "specs": [
    {"name": "button-schema", "version": 3, "updated_at": "2026-02-09T14:30:00Z"},
    {"name": "modal-schema", "version": 1, "updated_at": "2026-02-09T12:00:00Z"}
  ]
}
```

**Get a specific spec:**

```bash
curl http://localhost:9800/api/specs/w2c-forms/button-schema
```

Returns the raw JSON data.

**ETag caching:** Send `If-None-Match` with the ETag to get `304 Not Modified` if the spec hasn't changed. The ETag and version are returned in response headers:

```
ETag: "a1b2c3d4..."
X-Koor-Version: 3
```

### Deleting Specs

```bash
curl -X DELETE http://localhost:9800/api/specs/w2c-forms/button-schema
```

```bash
koor-cli specs delete w2c-forms/button-schema
```

---

## Validation Rules

Validation rules check content against patterns. Rules are stored per-project and run against submitted content via the validation endpoint. Rules can be scoped to a technology stack so that stack-specific rules only fire when validating content for that stack.

### Rule Structure

```json
{
  "rule_id": "no-inline-style",
  "severity": "error",
  "match_type": "regex",
  "pattern": "style\\s*=",
  "message": "Inline styles are not allowed",
  "applies_to": ["*.html", "*.templ"],
  "stack": "goth"
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `rule_id` | Yes | — | Unique identifier within the project |
| `severity` | No | `error` | `error` or `warning` |
| `match_type` | No | `regex` | `regex`, `missing`, or `custom` |
| `pattern` | Yes | — | Regex pattern, or custom check name |
| `message` | No | Auto-generated | Human-readable message shown on violation |
| `applies_to` | No | `["*"]` | Glob patterns for filename filtering |
| `stack` | No | `""` (all stacks) | Technology stack this rule applies to (e.g. `goth`, `react`). Empty means universal. |

### Match Types

**regex** — Flags each line where the pattern matches. Reports the line number and matched text.

```json
{
  "rule_id": "no-inline-style",
  "match_type": "regex",
  "pattern": "style\\s*=",
  "message": "Inline styles are not allowed"
}
```

Violation output includes `line` and `match`:

```json
{
  "rule_id": "no-inline-style",
  "severity": "error",
  "message": "Inline styles are not allowed",
  "line": 5,
  "match": "style=\"color: red\""
}
```

**missing** — Flags a violation if the pattern is NOT found anywhere in the content. Used for enforcing required patterns.

```json
{
  "rule_id": "require-data-ai-id",
  "match_type": "missing",
  "pattern": "data-ai-id",
  "message": "Components must have data-ai-id attributes"
}
```

**custom** — Built-in checks referenced by name. Currently supported:

| Pattern Name | What It Checks |
|-------------|----------------|
| `no-console-log` | Flags `console.log(` statements |

Unknown custom patterns fall back to regex behaviour.

### Stack-Scoped Rules

Rules can target a specific technology stack via the `stack` field. When validating content with a `stack` parameter:

- Rules with a matching `stack` are applied
- Rules with an empty `stack` (universal rules) are always applied
- Rules with a different `stack` are skipped

This enables three-dimensional rule targeting: **project** + **filename** (`applies_to`) + **stack**.

```bash
# Only GOTH-stack rules + universal rules fire
curl -X POST http://localhost:9800/api/validate/w2c-forms \
  -H "Content-Type: application/json" \
  -d '{"content": "style=\"red\"", "stack": "goth"}'

# No stack filter — all rules fire
curl -X POST http://localhost:9800/api/validate/w2c-forms \
  -H "Content-Type: application/json" \
  -d '{"content": "style=\"red\""}'
```

### Setting Rules

Rules are set per-project. A PUT replaces all existing rules for that project.

```bash
curl -X PUT http://localhost:9800/api/validate/w2c-forms/rules \
  -H "Content-Type: application/json" \
  -d '[
    {
      "rule_id": "no-inline-style",
      "severity": "error",
      "match_type": "regex",
      "pattern": "style\\s*=",
      "message": "Inline styles are not allowed",
      "applies_to": ["*.html", "*.templ"],
      "stack": "goth"
    },
    {
      "rule_id": "require-data-ai-id",
      "severity": "warning",
      "match_type": "missing",
      "pattern": "data-ai-id",
      "message": "Components should have data-ai-id attributes",
      "applies_to": ["*.templ"]
    },
    {
      "rule_id": "no-console-log",
      "severity": "error",
      "match_type": "custom",
      "pattern": "no-console-log",
      "message": "Remove console.log statements",
      "applies_to": ["*.js", "*.ts"],
      "stack": "react"
    }
  ]'
```

### Listing Rules

```bash
# All rules for the project
curl http://localhost:9800/api/validate/w2c-forms/rules

# Only goth-stack rules
curl http://localhost:9800/api/validate/w2c-forms/rules?stack=goth
```

```json
{
  "project": "w2c-forms",
  "rules": [
    {
      "project": "w2c-forms",
      "rule_id": "no-inline-style",
      "severity": "error",
      "match_type": "regex",
      "pattern": "style\\s*=",
      "message": "Inline styles are not allowed",
      "applies_to": ["*.html", "*.templ"],
      "stack": "goth"
    }
  ]
}
```

### Running Validation

Submit content to validate against all rules for a project. Optionally include `stack` to filter rules by technology stack.

```bash
curl -X POST http://localhost:9800/api/validate/w2c-forms \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "button.templ",
    "content": "<div style=\"color: red\" class=\"c-button\">\n  <span>Click me</span>\n</div>",
    "stack": "goth"
  }'
```

**Response:**

```json
{
  "project": "w2c-forms",
  "violations": [
    {
      "rule_id": "no-inline-style",
      "severity": "error",
      "message": "Inline styles are not allowed",
      "line": 1,
      "match": "style=\"color: red\""
    },
    {
      "rule_id": "require-data-ai-id",
      "severity": "warning",
      "message": "Components should have data-ai-id attributes"
    }
  ],
  "count": 2
}
```

When content passes all rules:

```json
{"project": "w2c-forms", "violations": [], "count": 0}
```

### Filename Filtering

The `applies_to` field uses glob patterns to filter which rules run against which files:

| Pattern | Matches |
|---------|---------|
| `*` | All files |
| `*.templ` | Go templ files |
| `*.js` | JavaScript files |
| `*.html` | HTML files |

Both the full path and the base filename are checked. If `filename` is omitted from the validation request, all rules run regardless of `applies_to`.

## Worked Example: W2C Component Rules

A scanner that pushes W2C-DaCss01 component specs to Koor could set up stack-scoped validation rules. The `stack: "goth"` rules only fire when a GOTH-stack agent validates content, while universal rules (no stack) apply to all agents.

```json
[
  {
    "rule_id": "w2c-no-inline-style",
    "severity": "error",
    "match_type": "regex",
    "pattern": "style\\s*=",
    "message": "Use W2C utility classes (u-*) instead of inline styles",
    "stack": "goth"
  },
  {
    "rule_id": "w2c-require-ai-id",
    "severity": "warning",
    "match_type": "missing",
    "pattern": "data-ai-id",
    "message": "All W2C components require data-ai-id for LLM navigation",
    "stack": "goth"
  },
  {
    "rule_id": "w2c-no-js-recreation",
    "severity": "error",
    "match_type": "regex",
    "pattern": "createElement.*c-",
    "message": "Do not recreate W2C components in JavaScript. Use HTMX to fetch server-rendered components.",
    "stack": "goth"
  },
  {
    "rule_id": "w2c-require-css-prefix",
    "severity": "warning",
    "match_type": "missing",
    "pattern": "(c-|l-|u-|is-|r-)",
    "message": "W2C components should use the standard CSS naming convention (c-*, l-*, u-*, is-*, r-*)"
  }
]
```

These rules enforce the W2C-DaCss01 coding standards across any agent that validates its output through Koor. The first three are GOTH-specific; the last is universal and applies regardless of stack.

---

## Rule Sources and Lifecycle

Rules have three sources and a lifecycle status. Only **accepted** rules participate in validation.

### Sources

| Source | Description |
|--------|-------------|
| `local` | Created manually via `PUT /api/validate/{project}/rules`. Always accepted. |
| `learned` | Proposed by LLM agents via `POST /api/rules/propose` or MCP `propose_rule`. Starts as `proposed`, must be accepted by a user. |
| `external` | Imported from community rule sets via `POST /api/rules/import`. Always accepted on import. |

### Status Lifecycle

```
proposed ──accept──> accepted  (rule fires during validation)
proposed ──reject──> rejected  (rule stored but never fires)
```

Local and external rules are always created with `status=accepted`. Only learned (proposed) rules go through the accept/reject workflow.

### Proposing Rules (LLM Learning)

When an LLM agent solves a problem, it can propose a validation rule to prevent the same issue in the future:

**Via MCP:**

The `propose_rule` MCP tool lets agents propose rules directly:

```
propose_rule({
  project: "w2c-forms",
  rule_id: "no-hardcoded-colors",
  pattern: "#[0-9a-fA-F]{3,8}",
  message: "Use CSS custom properties instead of hardcoded colors",
  stack: "goth",
  proposed_by: "<instance-id>",
  context: "Found hardcoded hex colors causing theme inconsistency."
})
```

**Via REST:**

```bash
curl -X POST http://localhost:9800/api/rules/propose \
  -H "Content-Type: application/json" \
  -d '{
    "project": "w2c-forms",
    "rule_id": "no-hardcoded-colors",
    "pattern": "#[0-9a-fA-F]{3,8}",
    "message": "Use CSS custom properties instead of hardcoded colors",
    "stack": "goth",
    "proposed_by": "instance-uuid",
    "context": "Found hardcoded hex colors causing theme inconsistency."
  }'
```

The proposed rule is stored but does **not** fire during validation until a user accepts it:

```bash
# Accept
curl -X POST http://localhost:9800/api/rules/w2c-forms/no-hardcoded-colors/accept

# Or reject
curl -X POST http://localhost:9800/api/rules/w2c-forms/no-hardcoded-colors/reject
```

### Exporting and Importing Rules

Export your organisation's rules (local + learned) as a portable JSON file:

```bash
# Export local and learned rules (default)
curl http://localhost:9800/api/rules/export > my-org-rules.json

# Export only external rules
curl "http://localhost:9800/api/rules/export?source=external" > external-rules.json
```

Import rules from a JSON file (uses UPSERT — safe to re-run):

```bash
curl -X POST http://localhost:9800/api/rules/import \
  -H "Content-Type: application/json" \
  -d @my-org-rules.json
```

This separation means you can always export just your organisation's rules and learned procedures, independent of community/external rules.

**Via CLI:**

```bash
# Import external rules from a JSON file
koor-cli rules import --file rules/external/claude-code-rules.json

# Export local + learned rules (default)
koor-cli rules export --output my-org-rules.json

# Export specific sources
koor-cli rules export --source external --output external-rules.json
```

### Global Rules (`_global` project)

Rules stored under the special project name `_global` apply to **all projects** during validation. This is useful for external/community rules that should be universal:

```json
{
  "project": "_global",
  "rule_id": "ext-no-debugger",
  "pattern": "\\bdebugger\\b",
  "message": "Remove debugger statements",
  "source": "external"
}
```

When validating project `myproj`, Koor automatically includes both `myproj` rules and `_global` rules. Global rules do not duplicate when validating the `_global` project directly.

### Curated External Rules

Koor ships with a curated seed file at `rules/external/claude-code-rules.json` containing common code quality rules derived from community best practices. Import them with:

```bash
koor-cli rules import --file rules/external/claude-code-rules.json
```

The seed file includes rules for: no `console.log`, no `debugger`, no `eval()`, no hardcoded secrets, no hardcoded localhost URLs, no TODO/FIXME in production, and Go-specific patterns (no `fmt.Println`, no `panic`).

### Dashboard Rules UI

The Koor dashboard (default `http://localhost:9847/rules`) provides a visual interface for managing validation rules:

- **Filter** rules by project, stack, source, and status
- **Review proposed rules** — accept or reject rules proposed by LLM agents
- **Add/edit/delete rules** via inline forms
- **Export** local + learned rules as JSON

The dashboard uses HTMX for partial page updates without full page reloads. All rule operations are available through both the REST API and the dashboard.
