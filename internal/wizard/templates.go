package wizard

import "sort"

// StackTemplate defines stack-specific content injected into CLAUDE.md files.
type StackTemplate struct {
	ID          string   // registry key, e.g. "goth"
	DisplayName string   // shown in select menu, e.g. "Go + templ + HTMX"
	Description string   // one-line summary for the wizard UI
	BuildCmd    string   // primary build command
	TestCmd     string   // primary test command
	DevCmd      string   // dev server command
	Instructions []string // stack-specific lines for CLAUDE.md "Your Job" section
	FilePatterns []string // glob patterns this stack works with
}

// Registry is the global map of all known stack templates.
var Registry = map[string]StackTemplate{
	"goth": {
		ID:          "goth",
		DisplayName: "Go + templ + HTMX",
		Description: "Full-stack Go with templ templates and HTMX interactivity",
		BuildCmd:    "templ generate && go build ./...",
		TestCmd:     "go test ./... -count=1",
		DevCmd:      "go run ./cmd/server",
		Instructions: []string{
			"Use templ for all HTML templates (.templ files)",
			"Use HTMX for interactivity — no custom JavaScript frameworks",
			"Run `templ generate` before `go build`",
			"Follow Go stdlib HTTP routing patterns (Go 1.22+ mux)",
			"Use SQLite via modernc.org/sqlite for data persistence if needed",
		},
		FilePatterns: []string{"*.go", "*.templ", "*.css"},
	},
	"go-api": {
		ID:          "go-api",
		DisplayName: "Go REST API",
		Description: "Backend Go API with SQLite",
		BuildCmd:    "go build ./...",
		TestCmd:     "go test ./... -count=1",
		DevCmd:      "go run ./cmd/server",
		Instructions: []string{
			"Build a REST API using Go stdlib net/http",
			"Use Go 1.22+ routing patterns with wildcards: mux.HandleFunc(\"GET /api/resource/{id}\", handler)",
			"Use SQLite via modernc.org/sqlite for data persistence",
			"Return JSON responses with proper Content-Type headers",
			"Write table-driven tests using the testing package",
		},
		FilePatterns: []string{"*.go"},
	},
	"react": {
		ID:          "react",
		DisplayName: "React (Vite)",
		Description: "React frontend with Vite build system",
		BuildCmd:    "npm run build",
		TestCmd:     "npm test",
		DevCmd:      "npm run dev",
		Instructions: []string{
			"Use React with TypeScript",
			"Use Vite as the build tool",
			"Use fetch() for API calls — configure the backend URL from environment",
			"Use React Router for client-side routing",
			"Write component tests with Vitest and React Testing Library",
		},
		FilePatterns: []string{"*.tsx", "*.ts", "*.css"},
	},
	"flutter": {
		ID:          "flutter",
		DisplayName: "Flutter",
		Description: "Flutter cross-platform application",
		BuildCmd:    "flutter build",
		TestCmd:     "flutter test",
		DevCmd:      "flutter run",
		Instructions: []string{
			"Use Flutter with Dart",
			"Structure as lib/screens/, lib/widgets/, lib/services/",
			"Use http package for API calls to the backend",
			"Write widget tests for key components",
			"Keep business logic in services, not widgets",
		},
		FilePatterns: []string{"*.dart"},
	},
	"c": {
		ID:          "c",
		DisplayName: "C (make/cmake)",
		Description: "C application with make or cmake build system",
		BuildCmd:    "make",
		TestCmd:     "make test",
		DevCmd:      "make run",
		Instructions: []string{
			"Use C with a Makefile or CMakeLists.txt",
			"Organize as src/, include/, tests/",
			"Use proper header guards in all .h files",
			"Compile with -Wall -Wextra -Werror",
			"Manage memory carefully — free all allocations",
		},
		FilePatterns: []string{"*.c", "*.h"},
	},
	"generic": {
		ID:          "generic",
		DisplayName: "Generic",
		Description: "Language-agnostic minimal template",
		BuildCmd:    "",
		TestCmd:     "",
		DevCmd:      "",
		Instructions: []string{
			"Follow the project conventions established by the Controller",
			"Keep code organized and well-documented",
			"Write tests for all major functionality",
		},
		FilePatterns: []string{},
	},
}

// StackIDs returns all registered stack IDs with pinned stacks first, rest sorted alphabetically.
func StackIDs() []string {
	pinned := []string{"goth", "go-api"}

	pinnedSet := map[string]bool{}
	for _, id := range pinned {
		pinnedSet[id] = true
	}

	ids := make([]string, 0, len(Registry))
	for id := range Registry {
		if !pinnedSet[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	result := make([]string, 0, len(Registry))
	for _, id := range pinned {
		if _, ok := Registry[id]; ok {
			result = append(result, id)
		}
	}
	return append(result, ids...)
}
