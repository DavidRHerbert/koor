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

Validation rules check content against patterns. Rules are stored per-project and run against submitted content via the validation endpoint.

### Rule Structure

```json
{
  "rule_id": "no-inline-style",
  "severity": "error",
  "match_type": "regex",
  "pattern": "style\\s*=",
  "message": "Inline styles are not allowed",
  "applies_to": ["*.html", "*.templ"]
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
      "applies_to": ["*.html", "*.templ"]
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
      "applies_to": ["*.js", "*.ts"]
    }
  ]'
```

### Listing Rules

```bash
curl http://localhost:9800/api/validate/w2c-forms/rules
```

```json
{
  "project": "w2c-forms",
  "rules": [
    {
      "project": "w2c-forms",
      "rule_id": "no-console-log",
      "severity": "error",
      "match_type": "custom",
      "pattern": "no-console-log",
      "message": "Remove console.log statements",
      "applies_to": ["*.js", "*.ts"]
    }
  ]
}
```

### Running Validation

Submit content to validate against all rules for a project:

```bash
curl -X POST http://localhost:9800/api/validate/w2c-forms \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "button.templ",
    "content": "<div style=\"color: red\" class=\"c-button\">\n  <span>Click me</span>\n</div>"
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

A scanner that pushes W2C-DaCss01 component specs to Koor could set up these validation rules:

```json
[
  {
    "rule_id": "w2c-no-inline-style",
    "severity": "error",
    "match_type": "regex",
    "pattern": "style\\s*=",
    "message": "Use W2C utility classes (u-*) instead of inline styles"
  },
  {
    "rule_id": "w2c-require-ai-id",
    "severity": "warning",
    "match_type": "missing",
    "pattern": "data-ai-id",
    "message": "All W2C components require data-ai-id for LLM navigation"
  },
  {
    "rule_id": "w2c-no-js-recreation",
    "severity": "error",
    "match_type": "regex",
    "pattern": "createElement.*c-",
    "message": "Do not recreate W2C components in JavaScript. Use HTMX to fetch server-rendered components."
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

These rules enforce the W2C-DaCss01 coding standards across any agent that validates its output through Koor.
