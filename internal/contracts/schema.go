package contracts

import (
	"encoding/json"
	"fmt"
)

// Contract is a structured API contract stored as a Koor spec.
// It defines the exact JSON field names, types, and constraints
// for each endpoint — machine-readable, language-agnostic.
type Contract struct {
	Kind      string              `json:"kind"`      // must be "contract"
	Version   int                 `json:"version"`
	Endpoints map[string]Endpoint `json:"endpoints"` // key: "METHOD /path"
}

// Endpoint defines the request/response schema for a single API endpoint.
type Endpoint struct {
	Query          map[string]Field `json:"query,omitempty"`
	Request        map[string]Field `json:"request,omitempty"`
	Response       map[string]Field `json:"response,omitempty"`
	ResponseArray  map[string]Field `json:"response_array,omitempty"`
	ResponseStatus int              `json:"response_status,omitempty"`
	Error          map[string]Field `json:"error,omitempty"`
}

// Field describes a single JSON field in a contract.
// Recursive: object fields contain sub-fields, array fields contain item schema.
type Field struct {
	Type     string           `json:"type"`
	Required bool             `json:"required,omitempty"`
	Nullable bool             `json:"nullable,omitempty"`
	Enum     []string         `json:"enum,omitempty"`
	Fields   map[string]Field `json:"fields,omitempty"` // sub-fields when type=object
	Items    *Field           `json:"items,omitempty"`   // item schema when type=array
}

// Violation is a contract validation failure.
type Violation struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Parse decodes JSON bytes into a Contract, validating the kind field.
func Parse(data []byte) (*Contract, error) {
	var c Contract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("invalid contract JSON: %w", err)
	}
	if c.Kind != "contract" {
		return nil, fmt.Errorf("expected kind \"contract\", got %q", c.Kind)
	}
	if len(c.Endpoints) == 0 {
		return nil, fmt.Errorf("contract has no endpoints")
	}
	return &c, nil
}
