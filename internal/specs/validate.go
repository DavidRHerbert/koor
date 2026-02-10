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
	Project    string   `json:"project"`
	RuleID     string   `json:"rule_id"`
	Severity   string   `json:"severity"`
	MatchType  string   `json:"match_type"`
	Pattern    string   `json:"pattern"`
	Message    string   `json:"message"`
	Stack      string   `json:"stack"`
	AppliesTo  []string `json:"applies_to"`
	Source     string   `json:"source"`
	Status     string   `json:"status"`
	ProposedBy string   `json:"proposed_by,omitempty"`
	Context    string   `json:"context,omitempty"`
	CreatedAt  string   `json:"created_at,omitempty"`
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
		`SELECT project, rule_id, severity, match_type, pattern, message, stack, applies_to,
		        source, status, proposed_by, context, created_at
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
			&rule.Pattern, &rule.Message, &rule.Stack, &appliesTo,
			&rule.Source, &rule.Status, &rule.ProposedBy, &rule.Context, &rule.CreatedAt); err != nil {
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

		source := rule.Source
		if source == "" {
			source = "local"
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO validation_rules (project, rule_id, severity, match_type, pattern, message, stack, applies_to, source, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'accepted')`,
			project, rule.RuleID, rule.Severity, rule.MatchType, rule.Pattern, rule.Message, rule.Stack, string(appliesTo), source)
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
	Stack    string `json:"stack"`
}

// Validate runs all rules for a project against the given content.
// It also includes _global rules (external rules that apply across all projects).
func (r *Registry) Validate(ctx context.Context, project string, req ValidateRequest) ([]Violation, error) {
	rules, err := r.ListRules(ctx, project)
	if err != nil {
		return nil, err
	}

	// Include _global rules (external rules that apply to all projects).
	if project != "_global" {
		globalRules, err := r.ListRules(ctx, "_global")
		if err == nil {
			rules = append(rules, globalRules...)
		}
	}

	var violations []Violation
	for _, rule := range rules {
		// Only accepted rules participate in validation.
		if rule.Status != "" && rule.Status != "accepted" {
			continue
		}

		// Skip if rule targets a specific stack and request stack doesn't match.
		if rule.Stack != "" && req.Stack != "" && rule.Stack != req.Stack {
			continue
		}

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

// ProposeRule inserts a rule with source=learned, status=proposed.
func (r *Registry) ProposeRule(ctx context.Context, rule Rule) error {
	if rule.Project == "" || rule.RuleID == "" || rule.Pattern == "" {
		return fmt.Errorf("project, rule_id, and pattern are required")
	}
	if rule.Severity == "" {
		rule.Severity = "error"
	}
	if rule.MatchType == "" {
		rule.MatchType = "regex"
	}
	appliesTo, _ := json.Marshal(rule.AppliesTo)
	if rule.AppliesTo == nil {
		appliesTo = []byte(`["*"]`)
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO validation_rules (project, rule_id, severity, match_type, pattern, message, stack, applies_to, source, status, proposed_by, context)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'learned', 'proposed', ?, ?)`,
		rule.Project, rule.RuleID, rule.Severity, rule.MatchType, rule.Pattern, rule.Message,
		rule.Stack, string(appliesTo), rule.ProposedBy, rule.Context)
	if err != nil {
		return fmt.Errorf("propose rule: %w", err)
	}
	return nil
}

// AcceptRule sets a proposed rule's status to accepted.
func (r *Registry) AcceptRule(ctx context.Context, project, ruleID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE validation_rules SET status = 'accepted' WHERE project = ? AND rule_id = ? AND status = 'proposed'`,
		project, ruleID)
	if err != nil {
		return fmt.Errorf("accept rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RejectRule sets a proposed rule's status to rejected.
func (r *Registry) RejectRule(ctx context.Context, project, ruleID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE validation_rules SET status = 'rejected' WHERE project = ? AND rule_id = ? AND status = 'proposed'`,
		project, ruleID)
	if err != nil {
		return fmt.Errorf("reject rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ExportRules returns all rules matching the given sources across all projects.
func (r *Registry) ExportRules(ctx context.Context, sources []string) ([]Rule, error) {
	if len(sources) == 0 {
		sources = []string{"local", "learned"}
	}

	// Build placeholders for IN clause.
	placeholders := make([]string, len(sources))
	args := make([]any, len(sources))
	for i, s := range sources {
		placeholders[i] = "?"
		args[i] = s
	}

	query := fmt.Sprintf(
		`SELECT project, rule_id, severity, match_type, pattern, message, stack, applies_to,
		        source, status, proposed_by, context, created_at
		 FROM validation_rules WHERE source IN (%s) AND status = 'accepted' ORDER BY project, rule_id`,
		strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("export rules: %w", err)
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		var appliesTo string
		if err := rows.Scan(&rule.Project, &rule.RuleID, &rule.Severity, &rule.MatchType,
			&rule.Pattern, &rule.Message, &rule.Stack, &appliesTo,
			&rule.Source, &rule.Status, &rule.ProposedBy, &rule.Context, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan export rule: %w", err)
		}
		json.Unmarshal([]byte(appliesTo), &rule.AppliesTo)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// ImportRules bulk-inserts rules, using UPSERT to avoid conflicts.
func (r *Registry) ImportRules(ctx context.Context, rules []Rule) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	count := 0
	for _, rule := range rules {
		if rule.Project == "" || rule.RuleID == "" || rule.Pattern == "" {
			continue
		}
		if rule.Severity == "" {
			rule.Severity = "error"
		}
		if rule.MatchType == "" {
			rule.MatchType = "regex"
		}
		if rule.Source == "" {
			rule.Source = "external"
		}
		appliesTo, _ := json.Marshal(rule.AppliesTo)
		if rule.AppliesTo == nil {
			appliesTo = []byte(`["*"]`)
		}

		_, err := tx.ExecContext(ctx,
			`INSERT INTO validation_rules (project, rule_id, severity, match_type, pattern, message, stack, applies_to, source, status, proposed_by, context)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'accepted', ?, ?)
			 ON CONFLICT (project, rule_id) DO UPDATE SET
			   severity = excluded.severity, match_type = excluded.match_type,
			   pattern = excluded.pattern, message = excluded.message,
			   stack = excluded.stack, applies_to = excluded.applies_to,
			   source = excluded.source, status = 'accepted'`,
			rule.Project, rule.RuleID, rule.Severity, rule.MatchType, rule.Pattern,
			rule.Message, rule.Stack, string(appliesTo), rule.Source, rule.ProposedBy, rule.Context)
		if err != nil {
			return 0, fmt.Errorf("import rule %s/%s: %w", rule.Project, rule.RuleID, err)
		}
		count++
	}

	return count, tx.Commit()
}

// ListAllRules returns rules across all projects with optional filters.
func (r *Registry) ListAllRules(ctx context.Context, project, stack, source, status string) ([]Rule, error) {
	query := `SELECT project, rule_id, severity, match_type, pattern, message, stack, applies_to,
	          source, status, proposed_by, context, created_at
	          FROM validation_rules WHERE 1=1`
	var args []any

	if project != "" {
		query += ` AND project LIKE ?`
		args = append(args, "%"+project+"%")
	}
	if stack != "" {
		query += ` AND stack = ?`
		args = append(args, stack)
	}
	if source != "" {
		query += ` AND source = ?`
		args = append(args, source)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY project, rule_id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all rules: %w", err)
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		var appliesTo string
		if err := rows.Scan(&rule.Project, &rule.RuleID, &rule.Severity, &rule.MatchType,
			&rule.Pattern, &rule.Message, &rule.Stack, &appliesTo,
			&rule.Source, &rule.Status, &rule.ProposedBy, &rule.Context, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		json.Unmarshal([]byte(appliesTo), &rule.AppliesTo)
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// GetRule returns a single rule by project and rule_id.
func (r *Registry) GetRule(ctx context.Context, project, ruleID string) (*Rule, error) {
	var rule Rule
	var appliesTo string
	err := r.db.QueryRowContext(ctx,
		`SELECT project, rule_id, severity, match_type, pattern, message, stack, applies_to,
		        source, status, proposed_by, context, created_at
		 FROM validation_rules WHERE project = ? AND rule_id = ?`, project, ruleID).
		Scan(&rule.Project, &rule.RuleID, &rule.Severity, &rule.MatchType,
			&rule.Pattern, &rule.Message, &rule.Stack, &appliesTo,
			&rule.Source, &rule.Status, &rule.ProposedBy, &rule.Context, &rule.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(appliesTo), &rule.AppliesTo)
	return &rule, nil
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
