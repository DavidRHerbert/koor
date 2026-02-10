package contracts

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ValidatePayload checks a JSON payload against a contract endpoint definition.
// direction must be "request", "response", "query", or "error".
func ValidatePayload(c *Contract, endpoint, direction string, payload map[string]any) []Violation {
	ep, ok := c.Endpoints[endpoint]
	if !ok {
		return []Violation{{Path: endpoint, Message: fmt.Sprintf("endpoint %q not in contract", endpoint)}}
	}

	var schema map[string]Field
	switch direction {
	case "request":
		schema = ep.Request
	case "response":
		schema = ep.Response
		if schema == nil {
			schema = ep.ResponseArray
		}
	case "query":
		schema = ep.Query
	case "error":
		schema = ep.Error
	default:
		return []Violation{{Path: direction, Message: fmt.Sprintf("unknown direction %q (use request, response, query, or error)", direction)}}
	}

	if schema == nil {
		return []Violation{{Path: endpoint, Message: fmt.Sprintf("endpoint %q has no %s definition", endpoint, direction)}}
	}

	return validateFields(schema, payload, direction)
}

// ValidateResponseArray checks an array response where each element should match the schema.
func ValidateResponseArray(c *Contract, endpoint string, items []any) []Violation {
	ep, ok := c.Endpoints[endpoint]
	if !ok {
		return []Violation{{Path: endpoint, Message: fmt.Sprintf("endpoint %q not in contract", endpoint)}}
	}

	schema := ep.ResponseArray
	if schema == nil {
		return []Violation{{Path: endpoint, Message: fmt.Sprintf("endpoint %q has no response_array definition", endpoint)}}
	}

	var violations []Violation
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			violations = append(violations, Violation{
				Path:    fmt.Sprintf("response[%d]", i),
				Message: "expected object, got non-object",
			})
			continue
		}
		for _, v := range validateFields(schema, obj, fmt.Sprintf("response[%d]", i)) {
			violations = append(violations, v)
		}
	}
	return violations
}

// ValidateStatus checks the HTTP status code matches the contract.
func ValidateStatus(c *Contract, endpoint string, got int) *Violation {
	ep, ok := c.Endpoints[endpoint]
	if !ok {
		return nil
	}
	if ep.ResponseStatus == 0 {
		return nil // no status constraint
	}
	if got != ep.ResponseStatus {
		return &Violation{
			Path:    endpoint,
			Message: fmt.Sprintf("expected status %d, got %d", ep.ResponseStatus, got),
		}
	}
	return nil
}

// validateFields is the recursive core that walks the schema and payload.
func validateFields(schema map[string]Field, payload map[string]any, path string) []Violation {
	var violations []Violation

	// Check for unknown fields in payload.
	for key := range payload {
		if _, ok := schema[key]; !ok {
			known := fieldNames(schema)
			violations = append(violations, Violation{
				Path:    joinPath(path, key),
				Message: fmt.Sprintf("unexpected field %q (contract defines: %s)", key, strings.Join(known, ", ")),
			})
		}
	}

	// Check each field in the schema.
	for name, field := range schema {
		val, exists := payload[name]

		// Required check.
		if field.Required && !exists {
			violations = append(violations, Violation{
				Path:    joinPath(path, name),
				Message: fmt.Sprintf("missing required field %q", name),
			})
			continue
		}

		if !exists {
			continue
		}

		// Null check.
		if val == nil {
			if !field.Nullable {
				violations = append(violations, Violation{
					Path:    joinPath(path, name),
					Message: fmt.Sprintf("field %q is null but not nullable", name),
				})
			}
			continue
		}

		// Type check.
		if v := checkType(field, val, joinPath(path, name)); v != nil {
			violations = append(violations, *v)
			continue // skip deeper checks if type is wrong
		}

		// Enum check.
		if len(field.Enum) > 0 {
			if v := checkEnum(field, val, joinPath(path, name)); v != nil {
				violations = append(violations, *v)
			}
		}

		// Recurse into nested objects.
		if field.Type == "object" && field.Fields != nil {
			if obj, ok := val.(map[string]any); ok {
				violations = append(violations, validateFields(field.Fields, obj, joinPath(path, name))...)
			}
		}

		// Recurse into arrays.
		if field.Type == "array" && field.Items != nil {
			if arr, ok := val.([]any); ok {
				violations = append(violations, validateArray(field.Items, arr, joinPath(path, name))...)
			}
		}
	}

	return violations
}

// validateArray validates each element in an array against the items schema.
func validateArray(itemSchema *Field, arr []any, path string) []Violation {
	var violations []Violation
	for i, item := range arr {
		elemPath := fmt.Sprintf("%s[%d]", path, i)

		if item == nil {
			if !itemSchema.Nullable {
				violations = append(violations, Violation{
					Path:    elemPath,
					Message: "array element is null but not nullable",
				})
			}
			continue
		}

		// Type check on the element.
		if v := checkType(*itemSchema, item, elemPath); v != nil {
			violations = append(violations, *v)
			continue
		}

		// If items are objects, recurse.
		if itemSchema.Type == "object" && itemSchema.Fields != nil {
			if obj, ok := item.(map[string]any); ok {
				violations = append(violations, validateFields(itemSchema.Fields, obj, elemPath)...)
			}
		}
	}
	return violations
}

// checkType validates that a JSON value matches the expected type.
func checkType(field Field, val any, path string) *Violation {
	ok := false
	switch field.Type {
	case "string":
		_, ok = val.(string)
	case "number":
		_, ok = val.(float64)
		if !ok {
			// JSON numbers from Go encoding/json are always float64,
			// but be safe with json.Number too.
			_, ok = val.(json.Number)
		}
	case "boolean":
		_, ok = val.(bool)
	case "object":
		_, ok = val.(map[string]any)
	case "array":
		_, ok = val.([]any)
	case "":
		ok = true // no type constraint
	default:
		return &Violation{Path: path, Message: fmt.Sprintf("unknown type %q in contract schema", field.Type)}
	}
	if !ok {
		return &Violation{
			Path:    path,
			Message: fmt.Sprintf("expected %s, got %T", field.Type, val),
		}
	}
	return nil
}

// checkEnum validates that a value is in the allowed enum list.
func checkEnum(field Field, val any, path string) *Violation {
	s, ok := val.(string)
	if !ok {
		return nil // enum only applies to strings
	}
	for _, allowed := range field.Enum {
		if s == allowed {
			return nil
		}
	}
	return &Violation{
		Path:    path,
		Message: fmt.Sprintf("value %q not in allowed enum: [%s]", s, strings.Join(field.Enum, ", ")),
	}
}

// fieldNames returns sorted field names from a schema.
func fieldNames(schema map[string]Field) []string {
	names := make([]string, 0, len(schema))
	for k := range schema {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// joinPath creates a dotted path like "request.address.city".
func joinPath(base, field string) string {
	if base == "" {
		return field
	}
	return base + "." + field
}
