package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TestResult holds the results of testing a live endpoint against a contract.
type TestResult struct {
	Endpoint           string      `json:"endpoint"`
	StatusCode         int         `json:"status_code,omitempty"`
	RequestViolations  []Violation `json:"request_violations"`
	ResponseViolations []Violation `json:"response_violations"`
	Error              string      `json:"error,omitempty"`
}

// TestEndpoint sends an HTTP request to a live service and validates
// both the request payload and response against the contract.
func TestEndpoint(c *Contract, endpoint string, baseURL string, testPayload map[string]any) (*TestResult, error) {
	ep, ok := c.Endpoints[endpoint]
	if !ok {
		return nil, fmt.Errorf("endpoint %q not in contract", endpoint)
	}

	result := &TestResult{
		Endpoint:           endpoint,
		RequestViolations:  []Violation{},
		ResponseViolations: []Violation{},
	}

	// Parse "METHOD /path" from the endpoint key.
	parts := strings.SplitN(endpoint, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid endpoint format %q (expected \"METHOD /path\")", endpoint)
	}
	method, path := parts[0], parts[1]

	// Validate request payload before sending (if applicable).
	if testPayload != nil && ep.Request != nil {
		result.RequestViolations = ValidatePayload(c, endpoint, "request", testPayload)
	}

	// Build the HTTP request.
	url := strings.TrimRight(baseURL, "/") + path
	var body io.Reader
	if testPayload != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		data, err := json.Marshal(testPayload)
		if err != nil {
			return nil, fmt.Errorf("marshal test payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("HTTP request failed: %v", err)
		return result, nil
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	// Validate status code.
	if v := ValidateStatus(c, endpoint, resp.StatusCode); v != nil {
		result.ResponseViolations = append(result.ResponseViolations, *v)
	}

	// Read and validate response body.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		result.Error = fmt.Sprintf("read response body: %v", err)
		return result, nil
	}

	if len(respBody) == 0 {
		return result, nil
	}

	// Try to determine if response is array or object.
	if ep.ResponseArray != nil {
		var items []any
		if err := json.Unmarshal(respBody, &items); err == nil {
			result.ResponseViolations = append(result.ResponseViolations, ValidateResponseArray(c, endpoint, items)...)
		}
	} else if ep.Response != nil {
		var obj map[string]any
		if err := json.Unmarshal(respBody, &obj); err == nil {
			result.ResponseViolations = append(result.ResponseViolations, ValidatePayload(c, endpoint, "response", obj)...)
		}
	}

	return result, nil
}
