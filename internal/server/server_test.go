package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DavidRHerbert/koor/internal/audit"
	"github.com/DavidRHerbert/koor/internal/compliance"
	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/observability"
	"github.com/DavidRHerbert/koor/internal/server"
	"github.com/DavidRHerbert/koor/internal/specs"
	"github.com/DavidRHerbert/koor/internal/state"
	"github.com/DavidRHerbert/koor/internal/templates"
	"github.com/DavidRHerbert/koor/internal/webhooks"
)

func testServer(t *testing.T, authToken string) *httptest.Server {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	stateStore := state.New(database)
	specReg := specs.New(database)
	eventBus := events.New(database, 1000)
	instanceReg := instances.New(database)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := server.Config{
		Bind:      "localhost:0",
		AuthToken: authToken,
	}
	srv := server.New(cfg, stateStore, specReg, eventBus, instanceReg, nil, logger)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHealth(t *testing.T) {
	ts := testServer(t, "")
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestStateRoundTrip(t *testing.T) {
	ts := testServer(t, "")

	// PUT state.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/my-key", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("PUT: expected 200, got %d", resp.StatusCode)
	}

	// GET state.
	resp, err = http.Get(ts.URL + "/api/state/my-key")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"hello":"world"}` {
		t.Errorf("unexpected value: %s", body)
	}
	if resp.Header.Get("ETag") == "" {
		t.Error("expected ETag header")
	}
}

func TestStateList(t *testing.T) {
	ts := testServer(t, "")

	// Put two keys.
	for _, key := range []string{"alpha", "beta"} {
		req, _ := http.NewRequest("PUT", ts.URL+"/api/state/"+key, strings.NewReader(`"val"`))
		http.DefaultClient.Do(req)
	}

	resp, _ := http.Get(ts.URL + "/api/state")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "alpha") || !strings.Contains(string(body), "beta") {
		t.Errorf("list should contain both keys: %s", body)
	}
}

func TestStateDelete(t *testing.T) {
	ts := testServer(t, "")

	// Put then delete.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/temp", strings.NewReader(`"data"`))
	http.DefaultClient.Do(req)

	req, _ = http.NewRequest("DELETE", ts.URL+"/api/state/temp", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("DELETE: expected 200, got %d", resp.StatusCode)
	}

	// GET should now 404.
	resp, _ = http.Get(ts.URL + "/api/state/temp")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("GET after DELETE: expected 404, got %d", resp.StatusCode)
	}
}

func TestStateGetNotFound(t *testing.T) {
	ts := testServer(t, "")
	resp, _ := http.Get(ts.URL + "/api/state/nonexistent")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSpecsRoundTrip(t *testing.T) {
	ts := testServer(t, "")

	// PUT spec.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/specs/myproj/states",
		strings.NewReader(`{"open":{"transitions":["closed"]}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("PUT: expected 200, got %d", resp.StatusCode)
	}

	// GET spec.
	resp, _ = http.Get(ts.URL + "/api/specs/myproj/states")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"open":{"transitions":["closed"]}}` {
		t.Errorf("unexpected spec data: %s", body)
	}
}

func TestSpecsList(t *testing.T) {
	ts := testServer(t, "")

	req, _ := http.NewRequest("PUT", ts.URL+"/api/specs/proj/alpha", strings.NewReader(`"a"`))
	http.DefaultClient.Do(req)
	req, _ = http.NewRequest("PUT", ts.URL+"/api/specs/proj/beta", strings.NewReader(`"b"`))
	http.DefaultClient.Do(req)

	resp, _ := http.Get(ts.URL + "/api/specs/proj")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "alpha") || !strings.Contains(string(body), "beta") {
		t.Errorf("specs list should contain both: %s", body)
	}
}

func TestAuthRequired(t *testing.T) {
	ts := testServer(t, "secret123")

	// Without token: 401.
	resp, _ := http.Get(ts.URL + "/api/state")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401 without token, got %d", resp.StatusCode)
	}

	// With token: 200.
	req, _ := http.NewRequest("GET", ts.URL+"/api/state", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 with token, got %d", resp.StatusCode)
	}

	// Health is always public.
	resp, _ = http.Get(ts.URL + "/health")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("health should be public, got %d", resp.StatusCode)
	}
}

func TestETagCaching(t *testing.T) {
	ts := testServer(t, "")

	// Put a value.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/etag-test", strings.NewReader(`"cached"`))
	http.DefaultClient.Do(req)

	// Get and capture ETag.
	resp, _ := http.Get(ts.URL + "/api/state/etag-test")
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	// Request with matching ETag should get 304.
	req, _ = http.NewRequest("GET", ts.URL+"/api/state/etag-test", nil)
	req.Header.Set("If-None-Match", etag)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 304 {
		t.Errorf("expected 304, got %d", resp.StatusCode)
	}
}

func TestEventsPublishAndHistory(t *testing.T) {
	ts := testServer(t, "")

	// Publish an event.
	req, _ := http.NewRequest("POST", ts.URL+"/api/events/publish",
		strings.NewReader(`{"topic":"api.change","data":{"field":"name"}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("publish: expected 200, got %d", resp.StatusCode)
	}

	// Get history.
	resp, err = http.Get(ts.URL + "/api/events/history?last=10")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "api.change") {
		t.Errorf("history should contain event: %s", body)
	}
}

func TestEventsPublishValidation(t *testing.T) {
	ts := testServer(t, "")

	// Missing topic.
	req, _ := http.NewRequest("POST", ts.URL+"/api/events/publish",
		strings.NewReader(`{"data":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing topic, got %d", resp.StatusCode)
	}
}

func TestEventsHistoryEmpty(t *testing.T) {
	ts := testServer(t, "")
	resp, _ := http.Get(ts.URL + "/api/events/history")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "[]\n" {
		t.Errorf("expected empty array, got: %s", body)
	}
}

func TestInstanceRegisterAndList(t *testing.T) {
	ts := testServer(t, "")

	// Register an instance with stack.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/register",
		strings.NewReader(`{"name":"claude-test","workspace":"/tmp","intent":"testing","stack":"goth"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("register: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "claude-test") {
		t.Errorf("register response should contain name: %s", body)
	}
	if !strings.Contains(string(body), `"token"`) {
		t.Errorf("register response should contain token: %s", body)
	}
	if !strings.Contains(string(body), `"stack":"goth"`) {
		t.Errorf("register response should contain stack: %s", body)
	}

	// List instances.
	resp2, _ := http.Get(ts.URL + "/api/instances")
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), "claude-test") {
		t.Errorf("list should contain registered instance: %s", body2)
	}
}

func TestInstanceListByStack(t *testing.T) {
	ts := testServer(t, "")

	// Register goth instance.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/register",
		strings.NewReader(`{"name":"goth-agent","stack":"goth"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Register react instance.
	req, _ = http.NewRequest("POST", ts.URL+"/api/instances/register",
		strings.NewReader(`{"name":"react-agent","stack":"react"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()

	// Filter by stack=goth.
	resp, _ = http.Get(ts.URL + "/api/instances?stack=goth")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "goth-agent") {
		t.Errorf("stack filter should contain goth-agent: %s", body)
	}
	if strings.Contains(string(body), "react-agent") {
		t.Errorf("stack filter should not contain react-agent: %s", body)
	}
}

func TestInstanceRegisterValidation(t *testing.T) {
	ts := testServer(t, "")

	// Missing name.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/register",
		strings.NewReader(`{"workspace":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing name, got %d", resp.StatusCode)
	}
}

func TestInstanceGetNotFound(t *testing.T) {
	ts := testServer(t, "")
	resp, _ := http.Get(ts.URL + "/api/instances/nonexistent")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestInstanceDeregister(t *testing.T) {
	ts := testServer(t, "")

	// Register first.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/register",
		strings.NewReader(`{"name":"temp-agent"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Extract ID from response.
	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &result)

	// Deregister.
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/instances/"+result.ID, nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("deregister: expected 200, got %d", resp.StatusCode)
	}

	// Get should now 404.
	resp, _ = http.Get(ts.URL + "/api/instances/" + result.ID)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("get after deregister: expected 404, got %d", resp.StatusCode)
	}
}

func registerInstance(t *testing.T, tsURL, name string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", tsURL+"/api/instances/register",
		strings.NewReader(fmt.Sprintf(`{"name":%q}`, name)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &result)
	return result.ID
}

func TestInstanceActivate(t *testing.T) {
	ts := testServer(t, "")

	// Register — should be pending.
	id := registerInstance(t, ts.URL, "activate-test")

	resp, _ := http.Get(ts.URL + "/api/instances/" + id)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"status":"pending"`) {
		t.Errorf("expected pending status: %s", body)
	}

	// Activate.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/"+id+"/activate", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("activate: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"status":"active"`) {
		t.Errorf("activate response should contain active status: %s", body)
	}

	// Verify via GET.
	resp, _ = http.Get(ts.URL + "/api/instances/" + id)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"status":"active"`) {
		t.Errorf("get after activate should show active: %s", body)
	}
}

func TestInstanceActivateNotFound(t *testing.T) {
	ts := testServer(t, "")
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/nonexistent/activate", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestInstanceListShowsStatus(t *testing.T) {
	ts := testServer(t, "")

	// Register two.
	id1 := registerInstance(t, ts.URL, "agent-pending")
	id2 := registerInstance(t, ts.URL, "agent-active")

	// Activate one.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/"+id2+"/activate", nil)
	http.DefaultClient.Do(req)

	// List.
	resp, _ := http.Get(ts.URL + "/api/instances")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	bodyStr := string(body)
	_ = id1 // used for registration

	if !strings.Contains(bodyStr, `"status":"pending"`) {
		t.Errorf("list should contain pending status: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"status":"active"`) {
		t.Errorf("list should contain active status: %s", bodyStr)
	}
}

func TestValidateRulesRoundTrip(t *testing.T) {
	ts := testServer(t, "")

	// PUT rules.
	rules := `[{"rule_id":"no-eval","severity":"error","match_type":"regex","pattern":"\\beval\\(","message":"eval is forbidden"}]`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/validate/myproj/rules", strings.NewReader(rules))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("PUT rules: expected 200, got %d", resp.StatusCode)
	}

	// GET rules.
	resp, _ = http.Get(ts.URL + "/api/validate/myproj/rules")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no-eval") {
		t.Errorf("rules should contain no-eval: %s", body)
	}
}

func TestValidateEndpoint(t *testing.T) {
	ts := testServer(t, "")

	// Setup a rule.
	rules := `[{"rule_id":"no-eval","severity":"error","match_type":"regex","pattern":"\\beval\\(","message":"eval bad"}]`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/validate/proj/rules", strings.NewReader(rules))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	// Validate code with a violation.
	vReq, _ := http.NewRequest("POST", ts.URL+"/api/validate/proj",
		strings.NewReader(`{"content":"var x = eval('bad');","filename":"app.js"}`))
	vReq.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(vReq)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("validate: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "no-eval") {
		t.Errorf("should contain violation: %s", body)
	}
	if !strings.Contains(string(body), `"count":1`) {
		t.Errorf("should have count 1: %s", body)
	}
}

func TestRulesProposeAcceptReject(t *testing.T) {
	ts := testServer(t, "")

	// Propose a rule.
	resp, _ := http.Post(ts.URL+"/api/rules/propose", "application/json",
		strings.NewReader(`{"project":"proj","rule_id":"no-eval","pattern":"\\beval\\(","message":"no eval","proposed_by":"inst-1","context":"found eval usage"}`))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("propose: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"status":"proposed"`) {
		t.Errorf("propose should return proposed status: %s", body)
	}

	// Proposed rule should not fire during validation.
	resp, _ = http.Post(ts.URL+"/api/validate/proj", "application/json",
		strings.NewReader(`{"content":"eval('x')"}`))
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"count":0`) {
		t.Errorf("proposed rule should not fire: %s", body)
	}

	// Accept the rule.
	resp, _ = http.Post(ts.URL+"/api/rules/proj/no-eval/accept", "application/json", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("accept: expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Now rule should fire.
	resp, _ = http.Post(ts.URL+"/api/validate/proj", "application/json",
		strings.NewReader(`{"content":"eval('x')"}`))
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"count":1`) {
		t.Errorf("accepted rule should fire: %s", body)
	}

	// Propose another rule and reject it.
	http.Post(ts.URL+"/api/rules/propose", "application/json",
		strings.NewReader(`{"project":"proj","rule_id":"no-foo","pattern":"foo","message":"no foo"}`))
	resp, _ = http.Post(ts.URL+"/api/rules/proj/no-foo/reject", "application/json", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("reject: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"status":"rejected"`) {
		t.Errorf("reject should return rejected status: %s", body)
	}
}

func TestRulesProposeValidation(t *testing.T) {
	ts := testServer(t, "")

	// Missing project.
	resp, _ := http.Post(ts.URL+"/api/rules/propose", "application/json",
		strings.NewReader(`{"rule_id":"x","pattern":"y"}`))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing project, got %d", resp.StatusCode)
	}

	// Missing rule_id.
	resp, _ = http.Post(ts.URL+"/api/rules/propose", "application/json",
		strings.NewReader(`{"project":"p","pattern":"y"}`))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing rule_id, got %d", resp.StatusCode)
	}

	// Missing pattern.
	resp, _ = http.Post(ts.URL+"/api/rules/propose", "application/json",
		strings.NewReader(`{"project":"p","rule_id":"x"}`))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing pattern, got %d", resp.StatusCode)
	}
}

func TestRulesAcceptRejectNotFound(t *testing.T) {
	ts := testServer(t, "")

	resp, _ := http.Post(ts.URL+"/api/rules/proj/nonexistent/accept", "application/json", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("accept nonexistent: expected 404, got %d", resp.StatusCode)
	}

	resp, _ = http.Post(ts.URL+"/api/rules/proj/nonexistent/reject", "application/json", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("reject nonexistent: expected 404, got %d", resp.StatusCode)
	}
}

func TestRulesExportImport(t *testing.T) {
	ts := testServer(t, "")

	// Import rules.
	resp, _ := http.Post(ts.URL+"/api/rules/import", "application/json",
		strings.NewReader(`[{"project":"proj","rule_id":"ext-1","pattern":"console\\.log","source":"external","message":"no console.log"},{"project":"proj","rule_id":"ext-2","pattern":"debugger","source":"external","message":"no debugger"}]`))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("import: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"imported":2`) {
		t.Errorf("should report 2 imported: %s", body)
	}

	// Imported rules should fire.
	resp, _ = http.Post(ts.URL+"/api/validate/proj", "application/json",
		strings.NewReader(`{"content":"console.log('x'); debugger;"}`))
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"count":2`) {
		t.Errorf("imported rules should fire: %s", body)
	}

	// Export — default sources (local+learned), should not include external.
	resp, _ = http.Get(ts.URL + "/api/rules/export")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("export: expected 200, got %d", resp.StatusCode)
	}
	var exported []json.RawMessage
	json.Unmarshal(body, &exported)
	if len(exported) != 0 {
		t.Errorf("default export should not include external rules, got %d", len(exported))
	}

	// Export external.
	resp, _ = http.Get(ts.URL + "/api/rules/export?source=external")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	json.Unmarshal(body, &exported)
	if len(exported) != 2 {
		t.Errorf("external export should have 2 rules, got %d", len(exported))
	}
}

func TestRulesImportEmpty(t *testing.T) {
	ts := testServer(t, "")

	resp, _ := http.Post(ts.URL+"/api/rules/import", "application/json",
		strings.NewReader(`[]`))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for empty import, got %d", resp.StatusCode)
	}
}

// --- Contract validation integration tests ---

func TestContractValidatePass(t *testing.T) {
	ts := testServer(t, "")

	// Store a contract as a spec.
	contract := `{"kind":"contract","version":1,"endpoints":{"POST /api/trucks":{"request":{"plate":{"type":"string","required":true},"company":{"type":"string","required":true},"type":{"type":"string","required":true,"enum":["semi","tanker","flatbed"]}},"response_status":201}}}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/specs/Truck-Wash/api-contract", strings.NewReader(contract))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("PUT spec: expected 200, got %d", resp.StatusCode)
	}

	// Validate a correct payload.
	vBody := `{"endpoint":"POST /api/trucks","direction":"request","payload":{"plate":"ABC-123","company":"Acme","type":"semi"}}`
	resp, _ = http.Post(ts.URL+"/api/contracts/Truck-Wash/api-contract/validate", "application/json", strings.NewReader(vBody))
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("validate: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"valid":true`) {
		t.Errorf("expected valid:true, got: %s", body)
	}
}

func TestContractValidateFail(t *testing.T) {
	ts := testServer(t, "")

	// Store a contract.
	contract := `{"kind":"contract","version":1,"endpoints":{"POST /api/trucks":{"request":{"plate":{"type":"string","required":true},"company":{"type":"string","required":true},"type":{"type":"string","required":true}}}}}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/specs/Truck-Wash/api-contract", strings.NewReader(contract))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Validate with wrong field names (the exact Truck-Wash bug).
	vBody := `{"endpoint":"POST /api/trucks","direction":"request","payload":{"plate_number":"ABC-123","company":"Acme","truck_type":"semi"}}`
	resp, _ = http.Post(ts.URL+"/api/contracts/Truck-Wash/api-contract/validate", "application/json", strings.NewReader(vBody))
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"valid":false`) {
		t.Errorf("expected valid:false, got: %s", body)
	}
	if !strings.Contains(string(body), "plate_number") {
		t.Errorf("should mention plate_number: %s", body)
	}
	if !strings.Contains(string(body), "truck_type") {
		t.Errorf("should mention truck_type: %s", body)
	}
}

func TestContractValidateNotFound(t *testing.T) {
	ts := testServer(t, "")
	resp, _ := http.Post(ts.URL+"/api/contracts/NoProj/no-contract/validate", "application/json",
		strings.NewReader(`{"endpoint":"GET /test"}`))
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestContractTestLive(t *testing.T) {
	ts := testServer(t, "")

	// Create a mock backend that returns a JSON response.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "1", "plate": "ABC"},
			{"id": "2", "plate": "DEF"},
		})
	}))
	defer backend.Close()

	// Store a contract.
	contract := `{"kind":"contract","version":1,"endpoints":{"GET /api/trucks":{"response_status":200,"response_array":{"id":{"type":"string"},"plate":{"type":"string"}}}}}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/specs/TW/api", strings.NewReader(contract))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Test the live endpoint.
	testBody := fmt.Sprintf(`{"endpoint":"GET /api/trucks","base_url":"%s"}`, backend.URL)
	resp, _ = http.Post(ts.URL+"/api/contracts/TW/api/test", "application/json", strings.NewReader(testBody))
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("test: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"valid":true`) {
		t.Errorf("expected valid:true, got: %s", body)
	}
	if !strings.Contains(string(body), `"status_code":200`) {
		t.Errorf("expected status_code 200: %s", body)
	}
}

func TestContractTestLiveFail(t *testing.T) {
	ts := testServer(t, "")

	// Backend returns wrong field names.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{
			"id":           "1",
			"plate_number": "ABC", // wrong — contract says "plate"
		})
	}))
	defer backend.Close()

	contract := `{"kind":"contract","version":1,"endpoints":{"POST /api/trucks":{"request":{"plate":{"type":"string","required":true}},"response_status":201,"response":{"id":{"type":"string","required":true},"plate":{"type":"string"}}}}}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/specs/TW/api", strings.NewReader(contract))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	testBody := fmt.Sprintf(`{"endpoint":"POST /api/trucks","base_url":"%s","test_data":{"plate":"ABC"}}`, backend.URL)
	resp, _ = http.Post(ts.URL+"/api/contracts/TW/api/test", "application/json", strings.NewReader(testBody))
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"valid":false`) {
		t.Errorf("expected valid:false for wrong response fields: %s", body)
	}
	if !strings.Contains(string(body), "plate_number") {
		t.Errorf("should mention unexpected plate_number in response: %s", body)
	}
}

func TestMetrics(t *testing.T) {
	ts := testServer(t, "")
	resp, _ := http.Get(ts.URL + "/api/metrics")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "uptime") {
		t.Errorf("metrics should contain uptime: %s", body)
	}
	if !strings.Contains(string(body), "state_keys") {
		t.Errorf("metrics should contain state_keys: %s", body)
	}
	if !strings.Contains(string(body), "token_tax") {
		t.Errorf("metrics should contain token_tax: %s", body)
	}
}

func TestTokenTaxCounting(t *testing.T) {
	ts := testServer(t, "")

	// Make some REST calls.
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("PUT", ts.URL+"/api/state/test-key", strings.NewReader(`{"x":1}`))
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// Check metrics reflect the REST calls.
	resp, _ := http.Get(ts.URL + "/api/metrics")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var metrics struct {
		TokenTax struct {
			MCPCalls        int64   `json:"mcp_calls"`
			RESTCalls       int64   `json:"rest_calls"`
			TotalCalls      int64   `json:"total_calls"`
			RESTTokensSaved int64   `json:"rest_tokens_saved"`
			SavingsPercent  float64 `json:"savings_percent"`
		} `json:"token_tax"`
	}
	if err := json.Unmarshal(body, &metrics); err != nil {
		t.Fatalf("unmarshal metrics: %v", err)
	}

	if metrics.TokenTax.RESTCalls < 3 {
		t.Errorf("expected at least 3 REST calls, got %d", metrics.TokenTax.RESTCalls)
	}
	if metrics.TokenTax.MCPCalls != 0 {
		t.Errorf("expected 0 MCP calls, got %d", metrics.TokenTax.MCPCalls)
	}
	if metrics.TokenTax.RESTTokensSaved != metrics.TokenTax.RESTCalls*300 {
		t.Errorf("tokens saved mismatch: %d vs %d*300", metrics.TokenTax.RESTTokensSaved, metrics.TokenTax.RESTCalls)
	}
	if metrics.TokenTax.SavingsPercent != 100.0 {
		t.Errorf("expected 100%% savings (no MCP calls), got %.1f%%", metrics.TokenTax.SavingsPercent)
	}
}

// --- Phase 10: State History + Rollback endpoint tests ---

func TestStateHistory(t *testing.T) {
	ts := testServer(t, "")

	// Write 3 versions.
	for i := 1; i <= 3; i++ {
		body := fmt.Sprintf(`{"v":%d}`, i)
		req, _ := http.NewRequest("PUT", ts.URL+"/api/state/my-key", strings.NewReader(body))
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// Get history.
	resp, _ := http.Get(ts.URL + "/api/state/my-key?history=1")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Key      string `json:"key"`
		Versions []struct {
			Version int64 `json:"version"`
		} `json:"versions"`
	}
	json.Unmarshal(body, &result)
	if result.Key != "my-key" {
		t.Errorf("expected key my-key, got %s", result.Key)
	}
	if len(result.Versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(result.Versions))
	}
	if result.Versions[0].Version != 3 {
		t.Errorf("expected latest version 3, got %d", result.Versions[0].Version)
	}
}

func TestStateGetVersion(t *testing.T) {
	ts := testServer(t, "")

	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"v":1}`))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	req, _ = http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"v":2}`))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()

	// Get version 1.
	resp, _ = http.Get(ts.URL + "/api/state/k?version=1")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if string(body) != `{"v":1}` {
		t.Errorf("expected v1 value, got %s", body)
	}

	// Get non-existent version.
	resp2, _ := http.Get(ts.URL + "/api/state/k?version=99")
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Errorf("expected 404 for version 99, got %d", resp2.StatusCode)
	}
}

func TestStateRollback(t *testing.T) {
	ts := testServer(t, "")

	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"v":1}`))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	req, _ = http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"v":2}`))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()

	req, _ = http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"bad":"data"}`))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()

	// Rollback to version 1.
	resp, _ = http.Post(ts.URL+"/api/state/k?rollback=1", "application/json", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("rollback: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"rolled_back":1`) {
		t.Errorf("should report rolled_back version: %s", body)
	}

	// Verify current value is the rolled-back version.
	resp, _ = http.Get(ts.URL + "/api/state/k")
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if string(body) != `{"v":1}` {
		t.Errorf("expected rolled-back value, got %s", body)
	}
}

func TestStateRollbackNotFound(t *testing.T) {
	ts := testServer(t, "")

	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"v":1}`))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	resp, _ = http.Post(ts.URL+"/api/state/k?rollback=99", "application/json", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for rollback to nonexistent version, got %d", resp.StatusCode)
	}
}

func TestStateDiff(t *testing.T) {
	ts := testServer(t, "")

	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"name":"Alice","age":30}`))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	req, _ = http.NewRequest("PUT", ts.URL+"/api/state/k", strings.NewReader(`{"name":"Alice","age":31,"city":"London"}`))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()

	resp, _ = http.Get(ts.URL + "/api/state/k?diff=1,2")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("diff: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		V1    int64 `json:"v1"`
		V2    int64 `json:"v2"`
		Diffs []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"diffs"`
	}
	json.Unmarshal(body, &result)
	if len(result.Diffs) != 2 {
		t.Fatalf("expected 2 diffs (age changed, city added), got %d: %s", len(result.Diffs), body)
	}
}

// --- Phase 11: Webhooks + Compliance endpoint tests ---

func testServerWithPhase11(t *testing.T) *httptest.Server {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	stateStore := state.New(database)
	specReg := specs.New(database)
	eventBus := events.New(database, 1000)
	instanceReg := instances.New(database)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := server.Config{Bind: "localhost:0"}
	srv := server.New(cfg, stateStore, specReg, eventBus, instanceReg, nil, logger)

	webhookDisp := webhooks.New(database, eventBus, logger)
	srv.SetWebhooks(webhookDisp)

	compSched := compliance.New(database, instanceReg, specReg, eventBus, 1*time.Hour, logger)
	srv.SetCompliance(compSched)

	templateStore := templates.New(database)
	srv.SetTemplates(templateStore)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestWebhookCreateAndList(t *testing.T) {
	ts := testServerWithPhase11(t)

	// Create a webhook.
	resp, _ := http.Post(ts.URL+"/api/webhooks", "application/json",
		strings.NewReader(`{"id":"wh-1","url":"http://example.com/hook","patterns":["agent.*"]}`))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"id":"wh-1"`) {
		t.Errorf("create response should contain id: %s", body)
	}

	// List webhooks.
	resp, _ = http.Get(ts.URL + "/api/webhooks")
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "wh-1") {
		t.Errorf("list should contain wh-1: %s", body)
	}
}

func TestWebhookDeleteAndNotFound(t *testing.T) {
	ts := testServerWithPhase11(t)

	// Create then delete.
	http.Post(ts.URL+"/api/webhooks", "application/json",
		strings.NewReader(`{"id":"wh-del","url":"http://example.com/hook"}`))

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/webhooks/wh-del", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("delete: expected 200, got %d", resp.StatusCode)
	}

	// Delete nonexistent.
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/webhooks/nonexistent", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("delete nonexistent: expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookCreateValidation(t *testing.T) {
	ts := testServerWithPhase11(t)

	// Missing id.
	resp, _ := http.Post(ts.URL+"/api/webhooks", "application/json",
		strings.NewReader(`{"url":"http://example.com"}`))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing id, got %d", resp.StatusCode)
	}

	// Missing url.
	resp, _ = http.Post(ts.URL+"/api/webhooks", "application/json",
		strings.NewReader(`{"id":"wh-x"}`))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing url, got %d", resp.StatusCode)
	}
}

func TestComplianceHistoryAndRun(t *testing.T) {
	ts := testServerWithPhase11(t)

	// History should be empty initially.
	resp, _ := http.Get(ts.URL + "/api/compliance/history")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("history: expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "[]\n" {
		t.Errorf("expected empty array, got: %s", body)
	}

	// Force a compliance run (no active instances, so 0 runs).
	resp, _ = http.Post(ts.URL+"/api/compliance/run", "application/json", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("run: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"checked":true`) {
		t.Errorf("run response should contain checked:true: %s", body)
	}
	if !strings.Contains(string(body), `"count":0`) {
		t.Errorf("run response should contain count:0 with no active agents: %s", body)
	}
}

// --- Phase 12: Capabilities + Templates endpoint tests ---

func TestInstanceSetCapabilities(t *testing.T) {
	ts := testServerWithPhase11(t)

	// Register an instance.
	id := registerInstance(t, ts.URL, "cap-agent")

	// Set capabilities.
	resp, _ := http.Post(ts.URL+"/api/instances/"+id+"/capabilities", "application/json",
		strings.NewReader(`{"capabilities":["code-review","testing"]}`))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("set capabilities: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "code-review") {
		t.Errorf("response should contain code-review: %s", body)
	}

	// Verify via GET.
	resp, _ = http.Get(ts.URL + "/api/instances/" + id)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "code-review") {
		t.Errorf("instance GET should include capabilities: %s", body)
	}
}

func TestInstanceFilterByCapability(t *testing.T) {
	ts := testServerWithPhase11(t)

	id1 := registerInstance(t, ts.URL, "agent-cap-a")
	id2 := registerInstance(t, ts.URL, "agent-cap-b")

	http.Post(ts.URL+"/api/instances/"+id1+"/capabilities", "application/json",
		strings.NewReader(`{"capabilities":["code-review"]}`))
	http.Post(ts.URL+"/api/instances/"+id2+"/capabilities", "application/json",
		strings.NewReader(`{"capabilities":["deployment"]}`))

	// Filter by capability.
	resp, _ := http.Get(ts.URL + "/api/instances?capability=code-review")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "agent-cap-a") {
		t.Errorf("should contain agent-cap-a: %s", body)
	}
	if strings.Contains(string(body), "agent-cap-b") {
		t.Errorf("should not contain agent-cap-b: %s", body)
	}
}

func TestTemplateCreateAndList(t *testing.T) {
	ts := testServerWithPhase11(t)

	// Create a template.
	resp, _ := http.Post(ts.URL+"/api/templates", "application/json",
		strings.NewReader(`{"id":"tpl-1","name":"Security Rules","kind":"rules","data":[{"rule_id":"no-eval"}],"tags":["security"]}`))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"id":"tpl-1"`) {
		t.Errorf("create response should contain id: %s", body)
	}

	// List templates.
	resp, _ = http.Get(ts.URL + "/api/templates")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "tpl-1") {
		t.Errorf("list should contain tpl-1: %s", body)
	}

	// Get template.
	resp, _ = http.Get(ts.URL + "/api/templates/tpl-1")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Security Rules") {
		t.Errorf("get should contain name: %s", body)
	}
}

func TestTemplateDeleteAndNotFound(t *testing.T) {
	ts := testServerWithPhase11(t)

	// Create then delete.
	http.Post(ts.URL+"/api/templates", "application/json",
		strings.NewReader(`{"id":"tpl-del","name":"Temp","data":{}}`))

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/templates/tpl-del", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("delete: expected 200, got %d", resp.StatusCode)
	}

	// Delete nonexistent.
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/templates/nonexistent", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("delete nonexistent: expected 404, got %d", resp.StatusCode)
	}
}

// --- Phase 13 tests ---

func testServerWithPhase13(t *testing.T) *httptest.Server {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	stateStore := state.New(database)
	specReg := specs.New(database)
	eventBus := events.New(database, 1000)
	instanceReg := instances.New(database)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := server.Config{Bind: "localhost:0"}
	srv := server.New(cfg, stateStore, specReg, eventBus, instanceReg, nil, logger)

	webhookDisp := webhooks.New(database, eventBus, logger)
	srv.SetWebhooks(webhookDisp)

	compSched := compliance.New(database, instanceReg, specReg, eventBus, 1*time.Hour, logger)
	srv.SetCompliance(compSched)

	templateStore := templates.New(database)
	srv.SetTemplates(templateStore)

	auditLog := audit.New(database)
	srv.SetAudit(auditLog)

	metricsStore := observability.New(database)
	srv.SetObservability(metricsStore)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestAuditQueryEmpty(t *testing.T) {
	ts := testServerWithPhase13(t)

	resp, _ := http.Get(ts.URL + "/api/audit")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if string(body) != "[]\n" {
		t.Errorf("expected empty array, got %s", body)
	}
}

func TestAuditAfterStatePut(t *testing.T) {
	ts := testServerWithPhase13(t)

	// Put state to trigger audit.
	resp, _ := http.NewRequest("PUT", ts.URL+"/api/state/test-key", strings.NewReader(`{"hello":"world"}`))
	resp2, _ := http.DefaultClient.Do(resp)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("state put: expected 200, got %d", resp2.StatusCode)
	}

	// Query audit log.
	resp3, _ := http.Get(ts.URL + "/api/audit")
	body, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("audit: expected 200, got %d", resp3.StatusCode)
	}

	var entries []map[string]any
	json.Unmarshal(body, &entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0]["action"] != "state.put" {
		t.Errorf("expected action state.put, got %v", entries[0]["action"])
	}
	if entries[0]["resource"] != "test-key" {
		t.Errorf("expected resource test-key, got %v", entries[0]["resource"])
	}
}

func TestAuditSummary(t *testing.T) {
	ts := testServerWithPhase13(t)

	// Create some audit entries via mutations.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/state/key1", strings.NewReader(`{"a":1}`))
	r, _ := http.DefaultClient.Do(req)
	r.Body.Close()

	req, _ = http.NewRequest("DELETE", ts.URL+"/api/state/key1", nil)
	r, _ = http.DefaultClient.Do(req)
	r.Body.Close()

	// Get summary.
	resp, _ := http.Get(ts.URL + "/api/audit/summary")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var summary map[string]any
	json.Unmarshal(body, &summary)
	total := summary["total_entries"].(float64)
	if total != 2 {
		t.Errorf("expected 2 total entries, got %v", total)
	}
}

func TestAgentMetricsEmpty(t *testing.T) {
	ts := testServerWithPhase13(t)

	resp, _ := http.Get(ts.URL + "/api/metrics/agents")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if string(body) != "[]\n" {
		t.Errorf("expected empty array, got %s", body)
	}
}

func TestAgentMetricsGetById(t *testing.T) {
	ts := testServerWithPhase13(t)

	resp, _ := http.Get(ts.URL + "/api/metrics/agents/nonexistent")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if string(body) != "[]\n" {
		t.Errorf("expected empty array, got %s", body)
	}
}
