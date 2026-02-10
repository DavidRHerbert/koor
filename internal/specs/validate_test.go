package specs_test

import (
	"context"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/specs"
)

func testRegistryWithRules(t *testing.T) *specs.Registry {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return specs.New(database)
}

func TestPutAndListRules(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	rules := []specs.Rule{
		{RuleID: "no-eval", Severity: "error", MatchType: "regex", Pattern: `\beval\(`, Message: "eval is forbidden"},
		{RuleID: "require-strict", Severity: "warning", MatchType: "missing", Pattern: `"use strict"`, Message: "must use strict mode"},
	}

	err := reg.PutRules(ctx, "myproj", rules)
	if err != nil {
		t.Fatal(err)
	}

	got, err := reg.ListRules(ctx, "myproj")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
	if got[0].RuleID != "no-eval" {
		t.Errorf("expected no-eval, got %s", got[0].RuleID)
	}
}

func TestPutRulesReplaces(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	// Put initial rules.
	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "a", Pattern: "x"},
		{RuleID: "b", Pattern: "y"},
	})

	// Replace with different rules.
	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "c", Pattern: "z"},
	})

	got, _ := reg.ListRules(ctx, "proj")
	if len(got) != 1 {
		t.Fatalf("expected 1 rule after replace, got %d", len(got))
	}
	if got[0].RuleID != "c" {
		t.Errorf("expected rule c, got %s", got[0].RuleID)
	}
}

func TestValidateRegex(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "no-eval", Severity: "error", MatchType: "regex", Pattern: `\beval\(`, Message: "eval is forbidden"},
	})

	violations, err := reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "var x = 1;\nvar y = eval('code');\nvar z = 3;",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Line != 2 {
		t.Errorf("expected line 2, got %d", violations[0].Line)
	}
	if violations[0].RuleID != "no-eval" {
		t.Errorf("expected no-eval, got %s", violations[0].RuleID)
	}
}

func TestValidateMissing(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "require-strict", Severity: "warning", MatchType: "missing", Pattern: `"use strict"`, Message: "must use strict mode"},
	})

	// Content without "use strict" should violate.
	violations, _ := reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "var x = 1;",
	})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Severity != "warning" {
		t.Errorf("expected warning severity, got %s", violations[0].Severity)
	}

	// Content with "use strict" should pass.
	violations, _ = reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "\"use strict\";\nvar x = 1;",
	})
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(violations))
	}
}

func TestValidateAppliesTo(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "js-only", Severity: "error", MatchType: "regex", Pattern: `console\.log`, AppliesTo: []string{"*.js"}},
	})

	// Should match .js files.
	violations, _ := reg.Validate(ctx, "proj", specs.ValidateRequest{
		Filename: "app.js",
		Content:  "console.log('hello');",
	})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for .js file, got %d", len(violations))
	}

	// Should not match .go files.
	violations, _ = reg.Validate(ctx, "proj", specs.ValidateRequest{
		Filename: "main.go",
		Content:  "console.log('hello');",
	})
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for .go file, got %d", len(violations))
	}
}

func TestValidateStackFilter(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "goth-no-inline", Severity: "error", MatchType: "regex", Pattern: `style\s*=`, Message: "no inline styles", Stack: "goth"},
		{RuleID: "all-no-eval", Severity: "error", MatchType: "regex", Pattern: `\beval\(`, Message: "no eval"},
		{RuleID: "react-no-class", Severity: "warning", MatchType: "regex", Pattern: `class=`, Message: "use className", Stack: "react"},
	})

	// Validate with stack=goth: should see goth rule + universal rule, not react rule.
	violations, _ := reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "style = 'red'; eval('x'); class='foo';",
		Stack:   "goth",
	})
	ruleIDs := map[string]bool{}
	for _, v := range violations {
		ruleIDs[v.RuleID] = true
	}
	if !ruleIDs["goth-no-inline"] {
		t.Error("expected goth-no-inline violation for goth stack")
	}
	if !ruleIDs["all-no-eval"] {
		t.Error("expected all-no-eval violation for goth stack")
	}
	if ruleIDs["react-no-class"] {
		t.Error("did not expect react-no-class violation for goth stack")
	}

	// Validate with stack=react: should see react rule + universal rule, not goth rule.
	violations, _ = reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "style = 'red'; eval('x'); class='foo';",
		Stack:   "react",
	})
	ruleIDs = map[string]bool{}
	for _, v := range violations {
		ruleIDs[v.RuleID] = true
	}
	if ruleIDs["goth-no-inline"] {
		t.Error("did not expect goth-no-inline violation for react stack")
	}
	if !ruleIDs["all-no-eval"] {
		t.Error("expected all-no-eval violation for react stack")
	}
	if !ruleIDs["react-no-class"] {
		t.Error("expected react-no-class violation for react stack")
	}

	// Validate with no stack: should see ALL rules.
	violations, _ = reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "style = 'red'; eval('x'); class='foo';",
	})
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations with no stack filter, got %d", len(violations))
	}
}

func TestProposeAndAcceptRule(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	rule := specs.Rule{
		Project:    "proj",
		RuleID:     "no-hardcoded-colors",
		Severity:   "warning",
		MatchType:  "regex",
		Pattern:    `#[0-9a-fA-F]{3,8}`,
		Message:    "Use CSS custom properties",
		Stack:      "goth",
		ProposedBy: "instance-123",
		Context:    "Found hardcoded hex colors",
	}

	// Propose the rule.
	err := reg.ProposeRule(ctx, rule)
	if err != nil {
		t.Fatal(err)
	}

	// Verify it exists with status=proposed.
	got, err := reg.GetRule(ctx, "proj", "no-hardcoded-colors")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "proposed" {
		t.Errorf("expected proposed status, got %s", got.Status)
	}
	if got.Source != "learned" {
		t.Errorf("expected learned source, got %s", got.Source)
	}
	if got.ProposedBy != "instance-123" {
		t.Errorf("expected proposed_by instance-123, got %s", got.ProposedBy)
	}

	// Proposed rule should NOT fire during validation.
	violations, _ := reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "color: #ff0000;",
		Stack:   "goth",
	})
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for proposed rule, got %d", len(violations))
	}

	// Accept the rule.
	err = reg.AcceptRule(ctx, "proj", "no-hardcoded-colors")
	if err != nil {
		t.Fatal(err)
	}

	// Accepted rule should now fire during validation.
	violations, _ = reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "color: #ff0000;",
		Stack:   "goth",
	})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for accepted rule, got %d", len(violations))
	}
}

func TestProposeAndRejectRule(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	reg.ProposeRule(ctx, specs.Rule{
		Project: "proj", RuleID: "bad-idea", Pattern: "foo",
	})

	err := reg.RejectRule(ctx, "proj", "bad-idea")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := reg.GetRule(ctx, "proj", "bad-idea")
	if got.Status != "rejected" {
		t.Errorf("expected rejected status, got %s", got.Status)
	}

	// Rejected rule should not fire.
	violations, _ := reg.Validate(ctx, "proj", specs.ValidateRequest{Content: "foo bar"})
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for rejected rule, got %d", len(violations))
	}
}

func TestAcceptRejectNotFound(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	err := reg.AcceptRule(ctx, "proj", "nonexistent")
	if err == nil {
		t.Error("expected error for accept nonexistent")
	}

	err = reg.RejectRule(ctx, "proj", "nonexistent")
	if err == nil {
		t.Error("expected error for reject nonexistent")
	}
}

func TestExportRules(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	// Add local rules via PutRules.
	reg.PutRules(ctx, "proj-a", []specs.Rule{
		{RuleID: "local-1", Pattern: "x", Source: "local"},
	})
	reg.PutRules(ctx, "proj-b", []specs.Rule{
		{RuleID: "local-2", Pattern: "y"},
	})

	// Add a proposed (learned) rule — should NOT appear in export (not accepted).
	reg.ProposeRule(ctx, specs.Rule{
		Project: "proj-a", RuleID: "proposed-1", Pattern: "z",
	})

	// Export local+learned — only accepted rules.
	rules, err := reg.ExportRules(ctx, []string{"local", "learned"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 exported rules, got %d", len(rules))
	}

	// Export with default (nil) sources — same result.
	rules, err = reg.ExportRules(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 default-exported rules, got %d", len(rules))
	}
}

func TestImportRules(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	rules := []specs.Rule{
		{Project: "proj", RuleID: "ext-1", Pattern: `console\.log`, Source: "external", Message: "no console.log"},
		{Project: "proj", RuleID: "ext-2", Pattern: `debugger`, Source: "external", Message: "no debugger"},
		{Project: "", RuleID: "bad", Pattern: "x"}, // invalid — no project, should be skipped
	}

	count, err := reg.ImportRules(ctx, rules)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported, got %d", count)
	}

	// Imported rules should be accepted and fire during validation.
	violations, _ := reg.Validate(ctx, "proj", specs.ValidateRequest{
		Content: "console.log('test'); debugger;",
	})
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations from imported rules, got %d", len(violations))
	}

	// Re-import should upsert without error.
	count, err = reg.ImportRules(ctx, []specs.Rule{
		{Project: "proj", RuleID: "ext-1", Pattern: `console\.log`, Source: "external", Message: "updated message"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 re-imported, got %d", count)
	}

	got, _ := reg.GetRule(ctx, "proj", "ext-1")
	if got.Message != "updated message" {
		t.Errorf("expected updated message, got %s", got.Message)
	}
}

func TestPutRulesDefaultSource(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	reg.PutRules(ctx, "proj", []specs.Rule{
		{RuleID: "r1", Pattern: "x"},
	})

	got, _ := reg.GetRule(ctx, "proj", "r1")
	if got.Source != "local" {
		t.Errorf("expected local source, got %s", got.Source)
	}
	if got.Status != "accepted" {
		t.Errorf("expected accepted status, got %s", got.Status)
	}
}

func TestListAllRules(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	// Add rules to multiple projects.
	reg.PutRules(ctx, "proj-a", []specs.Rule{
		{RuleID: "a1", Pattern: "x", Stack: "goth", Source: "local"},
		{RuleID: "a2", Pattern: "y", Source: "local"},
	})
	reg.PutRules(ctx, "proj-b", []specs.Rule{
		{RuleID: "b1", Pattern: "z", Stack: "react"},
	})
	reg.ProposeRule(ctx, specs.Rule{
		Project: "proj-a", RuleID: "proposed-1", Pattern: "w",
	})

	// No filters — returns all 4 rules (3 accepted + 1 proposed).
	all, err := reg.ListAllRules(ctx, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 rules total, got %d", len(all))
	}

	// Filter by project.
	byProj, _ := reg.ListAllRules(ctx, "proj-a", "", "", "")
	if len(byProj) != 3 {
		t.Fatalf("expected 3 rules for proj-a, got %d", len(byProj))
	}

	// Filter by stack.
	byStack, _ := reg.ListAllRules(ctx, "", "goth", "", "")
	if len(byStack) != 1 {
		t.Fatalf("expected 1 goth rule, got %d", len(byStack))
	}
	if byStack[0].RuleID != "a1" {
		t.Errorf("expected a1, got %s", byStack[0].RuleID)
	}

	// Filter by status.
	proposed, _ := reg.ListAllRules(ctx, "", "", "", "proposed")
	if len(proposed) != 1 {
		t.Fatalf("expected 1 proposed, got %d", len(proposed))
	}

	accepted, _ := reg.ListAllRules(ctx, "", "", "", "accepted")
	if len(accepted) != 3 {
		t.Fatalf("expected 3 accepted, got %d", len(accepted))
	}

	// Filter by source.
	local, _ := reg.ListAllRules(ctx, "", "", "local", "")
	if len(local) != 3 {
		t.Fatalf("expected 3 local rules, got %d", len(local))
	}
}

func TestValidateGlobalRules(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	// Add a _global external rule.
	reg.ImportRules(ctx, []specs.Rule{
		{Project: "_global", RuleID: "ext-no-eval", Pattern: `\beval\(`, Message: "no eval", Source: "external"},
	})

	// Add a project-specific rule.
	reg.PutRules(ctx, "myproj", []specs.Rule{
		{RuleID: "no-foo", Pattern: `\bfoo\b`, Message: "no foo"},
	})

	// Validate myproj: should see both project rule AND _global rule.
	violations, err := reg.Validate(ctx, "myproj", specs.ValidateRequest{
		Content: "foo eval('x')",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations (project + global), got %d", len(violations))
	}
	ruleIDs := map[string]bool{}
	for _, v := range violations {
		ruleIDs[v.RuleID] = true
	}
	if !ruleIDs["no-foo"] {
		t.Error("expected no-foo violation from project rule")
	}
	if !ruleIDs["ext-no-eval"] {
		t.Error("expected ext-no-eval violation from _global rule")
	}

	// Validate _global directly: should NOT double-include _global rules.
	violations, _ = reg.Validate(ctx, "_global", specs.ValidateRequest{
		Content: "eval('x')",
	})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for _global project, got %d", len(violations))
	}

	// Validate a project with no rules: should still get _global violations.
	violations, _ = reg.Validate(ctx, "empty-proj", specs.ValidateRequest{
		Content: "eval('bad')",
	})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation from _global rule on empty project, got %d", len(violations))
	}
}

func TestValidateNoRules(t *testing.T) {
	reg := testRegistryWithRules(t)
	ctx := context.Background()

	violations, err := reg.Validate(ctx, "empty-proj", specs.ValidateRequest{
		Content: "anything",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Errorf("expected 0 violations with no rules, got %d", len(violations))
	}
}
