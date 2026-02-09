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
