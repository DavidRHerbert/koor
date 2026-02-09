package specs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Rule is a validation rule.
type Rule struct {
	Project   string   `json:"project"`
	RuleID    string   `json:"rule_id"`
	Severity  string   `json:"severity"`
	MatchType string   `json:"match_type"`
	Pattern   string   `json:"pattern"`
	Message   string   `json:"message"`
	AppliesTo []string `json:"applies_to"`
}

// Violation is a single rule violation found during validation.
type Violation struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
	Match    string `json:"match,omitempty"`
}

// ListRules returns all validation rules for a project.
func (r *Registry) ListRules(ctx context.Context, project string) ([]Rule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT project, rule_id, severity, match_type, pattern, message, applies_to
		 FROM validation_rules WHERE project = ? ORDER BY rule_id`, project)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		var appliesTo string
		if err := rows.Scan(&rule.Project, &rule.RuleID, &rule.Severity, &rule.MatchType,
			&rule.Pattern, &rule.Message, &appliesTo); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		json.Unmarshal([]byte(appliesTo), &rule.AppliesTo)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// PutRules replaces all validation rules for a project.
func (r *Registry) PutRules(ctx context.Context, project string, rules []Rule) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing rules for this project.
	if _, err := tx.ExecContext(ctx, `DELETE FROM validation_rules WHERE project = ?`, project); err != nil {
		return fmt.Errorf("delete old rules: %w", err)
	}

	// Insert new rules.
	for _, rule := range rules {
		appliesTo, _ := json.Marshal(rule.AppliesTo)
		if rule.AppliesTo == nil {
			appliesTo = []byte(`["*"]`)
		}
		if rule.Severity == "" {
			rule.Severity = "error"
		}
		if rule.MatchType == "" {
			rule.MatchType = "regex"
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO validation_rules (project, rule_id, severity, match_type, pattern, message, applies_to)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			project, rule.RuleID, rule.Severity, rule.MatchType, rule.Pattern, rule.Message, string(appliesTo))
		if err != nil {
			return fmt.Errorf("insert rule %s: %w", rule.RuleID, err)
		}
	}

	return tx.Commit()
}

// DeleteRule removes a single validation rule.
func (r *Registry) DeleteRule(ctx context.Context, project, ruleID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM validation_rules WHERE project = ? AND rule_id = ?`, project, ruleID)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ValidateRequest holds the content to validate.
type ValidateRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// Validate runs all rules for a project against the given content.
func (r *Registry) Validate(ctx context.Context, project string, req ValidateRequest) ([]Violation, error) {
	rules, err := r.ListRules(ctx, project)
	if err != nil {
		return nil, err
	}

	var violations []Violation
	for _, rule := range rules {
		// Check if this rule applies to the given filename.
		if req.Filename != "" && !matchesGlobs(req.Filename, rule.AppliesTo) {
			continue
		}

		switch rule.MatchType {
		case "regex":
			violations = append(violations, validateRegex(rule, req.Content)...)
		case "missing":
			violations = append(violations, validateMissing(rule, req.Content)...)
		case "custom":
			violations = append(violations, validateCustom(rule, req.Content)...)
		}
	}

	return violations, nil
}

// validateRegex checks if the content matches a forbidden pattern.
func validateRegex(rule Rule, content string) []Violation {
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return []Violation{{
			RuleID:   rule.RuleID,
			Severity: "error",
			Message:  fmt.Sprintf("invalid regex pattern: %v", err),
		}}
	}

	lines := strings.Split(content, "\n")
	var violations []Violation
	for i, line := range lines {
		if loc := re.FindString(line); loc != "" {
			msg := rule.Message
			if msg == "" {
				msg = fmt.Sprintf("pattern '%s' matched", rule.Pattern)
			}
			violations = append(violations, Violation{
				RuleID:   rule.RuleID,
				Severity: rule.Severity,
				Message:  msg,
				Line:     i + 1,
				Match:    loc,
			})
		}
	}
	return violations
}

// validateMissing checks if a required pattern is absent from the content.
func validateMissing(rule Rule, content string) []Violation {
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return []Violation{{
			RuleID:   rule.RuleID,
			Severity: "error",
			Message:  fmt.Sprintf("invalid regex pattern: %v", err),
		}}
	}

	if !re.MatchString(content) {
		msg := rule.Message
		if msg == "" {
			msg = fmt.Sprintf("required pattern '%s' not found", rule.Pattern)
		}
		return []Violation{{
			RuleID:   rule.RuleID,
			Severity: rule.Severity,
			Message:  msg,
		}}
	}
	return nil
}

// validateCustom handles custom match types. Currently supports:
// - "no-console-log": flags console.log statements
// - Falls back to regex for unknown custom types.
func validateCustom(rule Rule, content string) []Violation {
	switch rule.Pattern {
	case "no-console-log":
		return validateRegex(Rule{
			RuleID:   rule.RuleID,
			Severity: rule.Severity,
			Message:  rule.Message,
			Pattern:  `console\.log\(`,
		}, content)
	default:
		// Treat unknown custom patterns as regex.
		return validateRegex(rule, content)
	}
}

// matchesGlobs checks if a filename matches any of the given glob patterns.
func matchesGlobs(filename string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if p == "*" || p == "" {
			return true
		}
		if matched, _ := path.Match(p, filename); matched {
			return true
		}
		// Also try matching just the base name.
		if matched, _ := path.Match(p, path.Base(filename)); matched {
			return true
		}
	}
	return false
}
