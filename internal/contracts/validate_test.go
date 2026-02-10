package contracts

import (
	"encoding/json"
	"testing"
)

var testContract = &Contract{
	Kind:    "contract",
	Version: 1,
	Endpoints: map[string]Endpoint{
		"POST /api/trucks": {
			Request: map[string]Field{
				"plate":   {Type: "string", Required: true},
				"company": {Type: "string", Required: true},
				"type":    {Type: "string", Required: true, Enum: []string{"semi", "tanker", "flatbed"}},
				"color":   {Type: "string"},
			},
			ResponseStatus: 201,
			Response: map[string]Field{
				"id":         {Type: "string", Required: true},
				"plate":      {Type: "string"},
				"company":    {Type: "string"},
				"type":       {Type: "string"},
				"color":      {Type: "string"},
				"created_at": {Type: "string"},
			},
			Error: map[string]Field{
				"message": {Type: "string", Required: true},
			},
		},
		"GET /api/trucks": {
			Query: map[string]Field{
				"status": {Type: "string", Enum: []string{"active", "all"}},
				"page":   {Type: "number"},
			},
			ResponseStatus: 200,
			ResponseArray: map[string]Field{
				"id":    {Type: "string"},
				"plate": {Type: "string"},
			},
		},
		"GET /api/trucks/{id}": {
			ResponseStatus: 200,
			Response: map[string]Field{
				"id":    {Type: "string", Required: true},
				"plate": {Type: "string"},
				"address": {
					Type: "object",
					Fields: map[string]Field{
						"street": {Type: "string"},
						"city":   {Type: "string", Required: true},
					},
				},
				"washes": {
					Type: "array",
					Items: &Field{
						Type: "object",
						Fields: map[string]Field{
							"id":        {Type: "string"},
							"wash_type": {Type: "string"},
							"status":    {Type: "string"},
						},
					},
				},
				"completed_at": {Type: "string", Nullable: true},
			},
		},
	},
}

// --- Parse tests ---

func TestParseValid(t *testing.T) {
	data := `{"kind":"contract","version":1,"endpoints":{"GET /api/test":{"response":{"id":{"type":"string"}}}}}`
	c, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Kind != "contract" {
		t.Errorf("expected kind=contract, got %s", c.Kind)
	}
	if len(c.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(c.Endpoints))
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse([]byte(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseMissingKind(t *testing.T) {
	_, err := Parse([]byte(`{"kind":"spec","version":1,"endpoints":{"GET /x":{"response":{"id":{"type":"string"}}}}}`))
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestParseNoEndpoints(t *testing.T) {
	_, err := Parse([]byte(`{"kind":"contract","version":1,"endpoints":{}}`))
	if err == nil {
		t.Fatal("expected error for empty endpoints")
	}
}

// --- ValidatePayload tests ---

func TestValidPayload(t *testing.T) {
	payload := map[string]any{
		"plate":   "ABC-123",
		"company": "Acme",
		"type":    "semi",
		"color":   "red",
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestUnknownField(t *testing.T) {
	payload := map[string]any{
		"plate_number": "ABC-123",
		"company":      "Acme",
		"type":         "semi",
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "unexpected field") && containsStr(v.Message, "plate_number") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected violation for unknown field plate_number, got: %v", violations)
	}
}

func TestMissingRequired(t *testing.T) {
	payload := map[string]any{
		"company": "Acme",
		"type":    "semi",
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "missing required") && containsStr(v.Message, "plate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected violation for missing required field plate, got: %v", violations)
	}
}

func TestWrongType(t *testing.T) {
	payload := map[string]any{
		"plate":   123.0, // should be string
		"company": "Acme",
		"type":    "semi",
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "expected string") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected type violation, got: %v", violations)
	}
}

func TestInvalidEnum(t *testing.T) {
	payload := map[string]any{
		"plate":   "ABC",
		"company": "Acme",
		"type":    "van", // not in enum
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "not in allowed enum") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected enum violation, got: %v", violations)
	}
}

func TestNullWithoutNullable(t *testing.T) {
	payload := map[string]any{
		"plate":   "ABC",
		"company": nil, // not nullable
		"type":    "semi",
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "null but not nullable") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected null violation, got: %v", violations)
	}
}

func TestNullWithNullable(t *testing.T) {
	payload := map[string]any{
		"id":           "123",
		"plate":        "ABC",
		"completed_at": nil, // nullable: true
	}
	violations := ValidatePayload(testContract, "GET /api/trucks/{id}", "response", payload)
	for _, v := range violations {
		if containsStr(v.Message, "null") && containsStr(v.Path, "completed_at") {
			t.Errorf("should not violate nullable field, got: %v", v)
		}
	}
}

func TestNestedObject(t *testing.T) {
	payload := map[string]any{
		"id":    "123",
		"plate": "ABC",
		"address": map[string]any{
			"street": "Main St",
			// missing required "city"
		},
	}
	violations := ValidatePayload(testContract, "GET /api/trucks/{id}", "response", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Path, "address") && containsStr(v.Message, "missing required") && containsStr(v.Message, "city") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected nested object violation for address.city, got: %v", violations)
	}
}

func TestNestedObjectUnknownField(t *testing.T) {
	payload := map[string]any{
		"id":    "123",
		"plate": "ABC",
		"address": map[string]any{
			"street":  "Main St",
			"city":    "LA",
			"zipcode": "90001", // unknown field
		},
	}
	violations := ValidatePayload(testContract, "GET /api/trucks/{id}", "response", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "unexpected field") && containsStr(v.Message, "zipcode") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown field violation for zipcode, got: %v", violations)
	}
}

func TestNestedArray(t *testing.T) {
	payload := map[string]any{
		"id":    "123",
		"plate": "ABC",
		"washes": []any{
			map[string]any{"id": "w1", "wash_type": "full", "status": "done"},
			map[string]any{"id": "w2", "wash_type": "exterior", "status": "queued"},
		},
	}
	violations := ValidatePayload(testContract, "GET /api/trucks/{id}", "response", payload)
	for _, v := range violations {
		if containsStr(v.Path, "washes") {
			t.Errorf("unexpected violation on valid array: %v", v)
		}
	}
}

func TestNestedArrayBadItem(t *testing.T) {
	payload := map[string]any{
		"id":    "123",
		"plate": "ABC",
		"washes": []any{
			map[string]any{"id": "w1", "wash_type": "full", "bogus_field": "x"},
		},
	}
	violations := ValidatePayload(testContract, "GET /api/trucks/{id}", "response", payload)
	found := false
	for _, v := range violations {
		if containsStr(v.Path, "washes[0]") && containsStr(v.Message, "bogus_field") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected violation for bogus_field in array item, got: %v", violations)
	}
}

func TestResponseArrayTopLevel(t *testing.T) {
	items := []any{
		map[string]any{"id": "1", "plate": "ABC"},
		map[string]any{"id": "2", "plate": "DEF", "unknown": "bad"},
	}
	violations := ValidateResponseArray(testContract, "GET /api/trucks", items)
	found := false
	for _, v := range violations {
		if containsStr(v.Path, "response[1]") && containsStr(v.Message, "unknown") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected violation for unknown field in array item 1, got: %v", violations)
	}
}

func TestQueryParams(t *testing.T) {
	query := map[string]any{
		"status": "invalid", // not in enum
	}
	violations := ValidatePayload(testContract, "GET /api/trucks", "query", query)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "not in allowed enum") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected enum violation for query param, got: %v", violations)
	}
}

func TestQueryParamsValid(t *testing.T) {
	query := map[string]any{
		"status": "active",
		"page":   1.0, // JSON numbers are float64
	}
	violations := ValidatePayload(testContract, "GET /api/trucks", "query", query)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got: %v", violations)
	}
}

func TestErrorResponseShape(t *testing.T) {
	errPayload := map[string]any{} // missing required "message"
	violations := ValidatePayload(testContract, "POST /api/trucks", "error", errPayload)
	found := false
	for _, v := range violations {
		if containsStr(v.Message, "missing required") && containsStr(v.Message, "message") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected violation for missing error message, got: %v", violations)
	}
}

// --- ValidateStatus tests ---

func TestStatusCorrect(t *testing.T) {
	v := ValidateStatus(testContract, "POST /api/trucks", 201)
	if v != nil {
		t.Errorf("expected no violation, got: %v", v)
	}
}

func TestStatusWrong(t *testing.T) {
	v := ValidateStatus(testContract, "POST /api/trucks", 200)
	if v == nil {
		t.Fatal("expected violation for wrong status")
	}
	if !containsStr(v.Message, "expected status 201") {
		t.Errorf("unexpected message: %s", v.Message)
	}
}

func TestStatusNoConstraint(t *testing.T) {
	// Add an endpoint with no status constraint
	c := &Contract{
		Kind:    "contract",
		Version: 1,
		Endpoints: map[string]Endpoint{
			"GET /test": {Response: map[string]Field{"ok": {Type: "boolean"}}},
		},
	}
	v := ValidateStatus(c, "GET /test", 500)
	if v != nil {
		t.Errorf("expected no violation when no status constraint, got: %v", v)
	}
}

// --- Unknown endpoint ---

func TestUnknownEndpoint(t *testing.T) {
	violations := ValidatePayload(testContract, "DELETE /api/trucks", "request", map[string]any{})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if !containsStr(violations[0].Message, "not in contract") {
		t.Errorf("unexpected message: %s", violations[0].Message)
	}
}

// --- Parse round-trip ---

func TestParseRoundTrip(t *testing.T) {
	data, err := json.Marshal(testContract)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	c, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(c.Endpoints) != len(testContract.Endpoints) {
		t.Errorf("endpoint count mismatch: %d vs %d", len(c.Endpoints), len(testContract.Endpoints))
	}
}

// --- Truck-Wash exact scenario ---

func TestTruckWashFieldMismatch(t *testing.T) {
	// This is the exact bug from the demo: frontend sends plate_number, backend expects plate.
	payload := map[string]any{
		"plate_number": "ABC-123",
		"company":      "Acme",
		"truck_type":   "semi",
	}
	violations := ValidatePayload(testContract, "POST /api/trucks", "request", payload)

	// Should have: unexpected plate_number, unexpected truck_type, missing plate, missing type
	if len(violations) < 4 {
		t.Errorf("expected at least 4 violations for the field mismatch scenario, got %d: %v", len(violations), violations)
	}

	messages := ""
	for _, v := range violations {
		messages += v.Message + "\n"
	}
	for _, expected := range []string{"plate_number", "truck_type", "missing required", "plate", "type"} {
		if !containsStr(messages, expected) {
			t.Errorf("expected violation mentioning %q, full output:\n%s", expected, messages)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAny(s, sub))
}

func containsAny(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
