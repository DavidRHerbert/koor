package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/server"
	"github.com/DavidRHerbert/koor/internal/specs"
	"github.com/DavidRHerbert/koor/internal/state"
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

	// Register an instance.
	req, _ := http.NewRequest("POST", ts.URL+"/api/instances/register",
		strings.NewReader(`{"name":"claude-test","workspace":"/tmp","intent":"testing"}`))
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

	// List instances.
	resp2, _ := http.Get(ts.URL + "/api/instances")
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), "claude-test") {
		t.Errorf("list should contain registered instance: %s", body2)
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
