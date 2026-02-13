package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type config struct {
	Server string
	Token  string
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "config":
		handleConfig(os.Args[2:])
	case "status":
		cfg := loadConfig()
		handleStatus(cfg)
	case "state":
		cfg := loadConfig()
		handleState(cfg, os.Args[2:])
	case "specs":
		cfg := loadConfig()
		handleSpecs(cfg, os.Args[2:])
	case "events":
		cfg := loadConfig()
		handleEvents(cfg, os.Args[2:])
	case "instances":
		cfg := loadConfig()
		handleInstances(cfg, os.Args[2:])
	case "rules":
		cfg := loadConfig()
		handleRules(cfg, os.Args[2:])
	case "contract":
		cfg := loadConfig()
		handleContract(cfg, os.Args[2:])
	case "backup":
		cfg := loadConfig()
		handleBackup(cfg, os.Args[2:])
	case "restore":
		cfg := loadConfig()
		handleRestore(cfg, os.Args[2:])
	case "register":
		cfg := loadConfig()
		handleRegister(cfg, os.Args[2:])
	case "activate":
		cfg := loadConfig()
		handleActivate(cfg, os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: koor-cli <command> [args]

Commands:
  config set server <url>         Set server URL
  config set token <token>        Set auth token
  status                          Check server health

  state list                      List all state keys
  state get <key>                 Get state value
  state set <key> --file <path>   Set state from file
  state set <key> --data <json>   Set state from inline data
  state delete <key>              Delete state key

  specs list <project>            List specs for a project
  specs get <project>/<name>      Get a spec
  specs set <project>/<name> --file <path>   Set spec from file
  specs set <project>/<name> --data <json>   Set spec from inline data
  specs delete <project>/<name>   Delete a spec

  events publish <topic> --data <json>   Publish an event
  events history [--last N] [--topic pattern]   Recent events
  events subscribe [pattern]     Stream events via WebSocket

  contract set <project>/<name> --file <path>   Store a contract
  contract get <project>/<name>                Get a contract
  contract validate <project>/<name> --endpoint "POST /api/x" --direction request --payload '{"k":"v"}'
  contract test <project>/<name> --target http://localhost:8080

  rules import --file <path>     Import rules from JSON file
  rules export [--source <s>] [--output <path>]   Export rules as JSON

  backup --output <path>         Backup all data to JSON file
  restore --file <path>          Restore data from backup file

  register <name> [--workspace <path>] [--intent <text>]   Register this agent
  activate <instance-id>         Activate agent (confirms CLI connectivity)
  instances list                 List registered instances
  instances get <id>             Get instance details

Flags:
  --pretty                        Pretty-print JSON output

Environment:
  KOOR_SERVER                     Server URL (overrides config)
  KOOR_TOKEN                      Auth token (overrides config)`)
}

// --- Config management ---

func configPath() string {
	return "settings.json"
}

func loadConfig() *config {
	cfg := &config{
		Server: "http://localhost:9800",
	}

	// Environment variables take priority.
	if v := os.Getenv("KOOR_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("KOOR_TOKEN"); v != "" {
		cfg.Token = v
	}

	// Load config file for any values not set by env.
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}

	var fileCfg struct {
		Server string `json:"server"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return cfg
	}

	if os.Getenv("KOOR_SERVER") == "" && fileCfg.Server != "" {
		cfg.Server = fileCfg.Server
	}
	if os.Getenv("KOOR_TOKEN") == "" && fileCfg.Token != "" {
		cfg.Token = fileCfg.Token
	}

	return cfg
}

func handleConfig(args []string) {
	if len(args) < 3 || args[0] != "set" {
		fmt.Fprintln(os.Stderr, "usage: koor-cli config set <server|token> <value>")
		os.Exit(1)
	}
	key := args[1]
	value := args[2]

	if key != "server" && key != "token" {
		fmt.Fprintf(os.Stderr, "unknown config key: %s (valid: server, token)\n", key)
		os.Exit(1)
	}

	// Read existing config file to preserve other keys.
	existing := map[string]any{}
	if data, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(data, &existing)
	}

	existing[key] = value

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configPath(), data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("config %s set to %s\n", key, value)
}

// --- Status ---

func handleStatus(cfg *config) {
	resp, err := doRequest(cfg, "GET", "/health", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	printResponse(resp)
}

// --- State commands ---

func handleState(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli state <list|get|set|delete> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		resp, err := doRequest(cfg, "GET", "/api/state", nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli state get <key>")
			os.Exit(1)
		}
		resp, err := doRequest(cfg, "GET", "/api/state/"+args[1], nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "set":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli state set <key> --file <path> | --data <json>")
			os.Exit(1)
		}
		key := args[1]
		body, err := readBodyArg(args[2:])
		if err != nil {
			fatal(err)
		}
		resp, err := doRequest(cfg, "PUT", "/api/state/"+key, strings.NewReader(string(body)))
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli state delete <key>")
			os.Exit(1)
		}
		resp, err := doRequest(cfg, "DELETE", "/api/state/"+args[1], nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	default:
		fmt.Fprintf(os.Stderr, "unknown state command: %s\n", args[0])
		os.Exit(1)
	}
}

// --- Specs commands ---

func handleSpecs(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli specs <list|get|set|delete> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli specs list <project>")
			os.Exit(1)
		}
		resp, err := doRequest(cfg, "GET", "/api/specs/"+args[1], nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli specs get <project>/<name>")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])
		resp, err := doRequest(cfg, "GET", "/api/specs/"+project+"/"+name, nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "set":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli specs set <project>/<name> --file <path> | --data <json>")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])
		body, err := readBodyArg(args[2:])
		if err != nil {
			fatal(err)
		}
		resp, err := doRequest(cfg, "PUT", "/api/specs/"+project+"/"+name, strings.NewReader(string(body)))
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli specs delete <project>/<name>")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])
		resp, err := doRequest(cfg, "DELETE", "/api/specs/"+project+"/"+name, nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	default:
		fmt.Fprintf(os.Stderr, "unknown specs command: %s\n", args[0])
		os.Exit(1)
	}
}

// --- Events commands ---

func handleEvents(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli events <publish|history|subscribe> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "publish":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli events publish <topic> --data <json>")
			os.Exit(1)
		}
		topic := args[1]
		body, err := readBodyArg(args[2:])
		if err != nil {
			fatal(err)
		}
		payload := fmt.Sprintf(`{"topic":%q,"data":%s}`, topic, string(body))
		resp, err := doRequest(cfg, "POST", "/api/events/publish", strings.NewReader(payload))
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "history":
		path := "/api/events/history"
		params := []string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--last":
				if i+1 < len(args) {
					params = append(params, "last="+args[i+1])
					i++
				}
			case "--topic":
				if i+1 < len(args) {
					params = append(params, "topic="+args[i+1])
					i++
				}
			}
		}
		if len(params) > 0 {
			path += "?" + strings.Join(params, "&")
		}
		resp, err := doRequest(cfg, "GET", path, nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "subscribe":
		pattern := "*"
		if len(args) >= 2 {
			pattern = args[1]
		}
		wsURL := strings.Replace(cfg.Server, "http://", "ws://", 1)
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
		wsURL = strings.TrimRight(wsURL, "/") + "/api/events/subscribe?pattern=" + pattern

		fmt.Fprintf(os.Stderr, "subscribing to %s (pattern: %s)...\n", wsURL, pattern)
		streamWebSocket(cfg, wsURL)

	default:
		fmt.Fprintf(os.Stderr, "unknown events command: %s\n", args[0])
		os.Exit(1)
	}
}

func streamWebSocket(cfg *config, wsURL string) {
	// Use nhooyr.io/websocket via the server's WS endpoint.
	// The CLI uses a simple HTTP-upgrade approach with stdlib for portability.
	dialer := &http.Client{}
	req, err := http.NewRequest("GET", wsURL, nil)
	if err != nil {
		fatal(err)
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	// For WebSocket, we need the actual websocket library.
	// Since koor-cli should stay dependency-free, we use a raw HTTP approach:
	// connect and read the server-sent JSON lines.
	// However, the server uses real WebSocket frames, so we need the library.
	// Instead, use golang.org/x/net or just inform the user to use wscat/websocat.
	//
	// For now, fall back to polling history as a simple subscribe mechanism.
	_ = dialer
	_ = req
	fmt.Fprintln(os.Stderr, "live WebSocket streaming requires a WebSocket client.")
	fmt.Fprintln(os.Stderr, "use: websocat "+wsURL)
	fmt.Fprintln(os.Stderr, "or:  wscat -c "+wsURL)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "falling back to polling history every 2 seconds...")

	seen := map[int64]bool{}
	for {
		resp, err := doRequest(cfg, "GET", "/api/events/history?last=10", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "poll error: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var rawEvts []json.RawMessage
		json.Unmarshal(data, &rawEvts)
		for _, raw := range rawEvts {
			var ev struct {
				ID int64 `json:"id"`
			}
			json.Unmarshal(raw, &ev)
			if !seen[ev.ID] {
				seen[ev.ID] = true
				fmt.Println(string(raw))
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// --- Instance commands ---

func handleRegister(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli register <name> [--workspace <path>] [--intent <text>]")
		os.Exit(1)
	}
	name := args[0]
	workspace := ""
	intent := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 < len(args) {
				workspace = args[i+1]
				i++
			}
		case "--intent":
			if i+1 < len(args) {
				intent = args[i+1]
				i++
			}
		}
	}

	payload := fmt.Sprintf(`{"name":%q,"workspace":%q,"intent":%q}`, name, workspace, intent)
	resp, err := doRequest(cfg, "POST", "/api/instances/register", strings.NewReader(payload))
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()
	printResponse(resp)
}

func handleActivate(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli activate <instance-id>")
		os.Exit(1)
	}
	id := args[0]
	resp, err := doRequest(cfg, "POST", "/api/instances/"+id+"/activate", nil)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()
	printResponse(resp)
}

func handleInstances(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli instances <list|get> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		resp, err := doRequest(cfg, "GET", "/api/instances", nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli instances get <id>")
			os.Exit(1)
		}
		resp, err := doRequest(cfg, "GET", "/api/instances/"+args[1], nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	default:
		fmt.Fprintf(os.Stderr, "unknown instances command: %s\n", args[0])
		os.Exit(1)
	}
}

// --- Rules commands ---

func handleRules(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli rules <import|export> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		filePath := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--file" && i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		}
		if filePath == "" {
			fmt.Fprintln(os.Stderr, "usage: koor-cli rules import --file <path>")
			os.Exit(1)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			fatal(fmt.Errorf("read file %s: %w", filePath, err))
		}

		// Validate it's a JSON array.
		var rules []json.RawMessage
		if err := json.Unmarshal(data, &rules); err != nil {
			fatal(fmt.Errorf("invalid JSON in %s: %w", filePath, err))
		}

		resp, err := doRequest(cfg, "POST", "/api/rules/import", strings.NewReader(string(data)))
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "export":
		source := ""
		output := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--source":
				if i+1 < len(args) {
					source = args[i+1]
					i++
				}
			case "--output":
				if i+1 < len(args) {
					output = args[i+1]
					i++
				}
			}
		}

		path := "/api/rules/export"
		if source != "" {
			path += "?source=" + source
		}

		resp, err := doRequest(cfg, "GET", path, nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fatal(fmt.Errorf("read response: %w", err))
		}

		// Pretty-print the JSON.
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			body, _ = json.MarshalIndent(v, "", "  ")
		}

		if output != "" {
			if err := os.WriteFile(output, append(body, '\n'), 0o644); err != nil {
				fatal(fmt.Errorf("write file %s: %w", output, err))
			}
			fmt.Fprintf(os.Stderr, "exported to %s\n", output)
		} else {
			fmt.Println(string(body))
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown rules command: %s\n", args[0])
		os.Exit(1)
	}
}

// --- Contract commands ---

func handleContract(cfg *config, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: koor-cli contract <set|get|validate|test> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "set":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli contract set <project>/<name> --file <path>")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])
		body, err := readBodyArg(args[2:])
		if err != nil {
			fatal(err)
		}

		// Validate it's a valid contract before storing.
		var contract map[string]any
		if err := json.Unmarshal(body, &contract); err != nil {
			fatal(fmt.Errorf("invalid JSON: %w", err))
		}
		if contract["kind"] != "contract" {
			fatal(fmt.Errorf("JSON must have \"kind\": \"contract\""))
		}

		resp, err := doRequest(cfg, "PUT", "/api/specs/"+project+"/"+name, strings.NewReader(string(body)))
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()
		printResponse(resp)

	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: koor-cli contract get <project>/<name>")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])
		resp, err := doRequest(cfg, "GET", "/api/specs/"+project+"/"+name, nil)
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()

		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			fmt.Print(string(data))
			os.Exit(1)
		}

		// Always pretty-print contracts for readability.
		var v any
		if err := json.Unmarshal(data, &v); err == nil {
			formatted, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(formatted))
		} else {
			fmt.Print(string(data))
		}

	case "validate":
		// Parse flags: --endpoint, --direction, --payload, --file
		endpoint := ""
		direction := "request"
		var payload string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--endpoint":
				if i+1 < len(args) {
					endpoint = args[i+1]
					i++
				}
			case "--direction":
				if i+1 < len(args) {
					direction = args[i+1]
					i++
				}
			case "--payload":
				if i+1 < len(args) {
					payload = args[i+1]
					i++
				}
			case "--file":
				if i+1 < len(args) {
					data, err := os.ReadFile(args[i+1])
					if err != nil {
						fatal(fmt.Errorf("read payload file: %w", err))
					}
					payload = string(data)
					i++
				}
			}
		}
		if len(args) < 2 || endpoint == "" {
			fmt.Fprintln(os.Stderr, "usage: koor-cli contract validate <project>/<name> --endpoint \"POST /api/x\" [--direction request] --payload '{...}'")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])

		// Build the validation request.
		reqBody := map[string]any{
			"endpoint":  endpoint,
			"direction": direction,
		}
		if payload != "" {
			var p map[string]any
			if err := json.Unmarshal([]byte(payload), &p); err != nil {
				fatal(fmt.Errorf("invalid payload JSON: %w", err))
			}
			reqBody["payload"] = p
		} else {
			reqBody["payload"] = map[string]any{}
		}

		reqJSON, _ := json.Marshal(reqBody)
		resp, err := doRequest(cfg, "POST", "/api/contracts/"+project+"/"+name+"/validate", strings.NewReader(string(reqJSON)))
		if err != nil {
			fatal(err)
		}
		defer resp.Body.Close()

		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			fmt.Print(string(data))
			os.Exit(1)
		}

		// Parse and print results.
		var result struct {
			Valid      bool `json:"valid"`
			Violations []struct {
				Path    string `json:"path"`
				Message string `json:"message"`
			} `json:"violations"`
		}
		json.Unmarshal(data, &result)

		if result.Valid {
			fmt.Printf("PASS  %s %s\n", direction, endpoint)
		} else {
			fmt.Printf("FAIL  %s %s\n", direction, endpoint)
			for _, v := range result.Violations {
				fmt.Printf("  - [%s] %s\n", v.Path, v.Message)
			}
			os.Exit(1)
		}

	case "test":
		// Parse flags: --target
		target := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--target" && i+1 < len(args) {
				target = args[i+1]
				i++
			}
		}
		if len(args) < 2 || target == "" {
			fmt.Fprintln(os.Stderr, "usage: koor-cli contract test <project>/<name> --target http://localhost:8080")
			os.Exit(1)
		}
		project, name := parseSpecPath(args[1])

		// First, fetch the contract to get the list of endpoints.
		resp, err := doRequest(cfg, "GET", "/api/specs/"+project+"/"+name, nil)
		if err != nil {
			fatal(err)
		}
		contractData, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			fmt.Print(string(contractData))
			os.Exit(1)
		}

		var contract struct {
			Endpoints map[string]json.RawMessage `json:"endpoints"`
		}
		if err := json.Unmarshal(contractData, &contract); err != nil {
			fatal(fmt.Errorf("parse contract: %w", err))
		}

		pass := 0
		fail := 0
		for ep := range contract.Endpoints {
			reqBody, _ := json.Marshal(map[string]any{
				"endpoint": ep,
				"base_url": target,
			})
			resp, err := doRequest(cfg, "POST", "/api/contracts/"+project+"/"+name+"/test", strings.NewReader(string(reqBody)))
			if err != nil {
				fmt.Printf("FAIL  %s (request error: %v)\n", ep, err)
				fail++
				continue
			}
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var result struct {
				Valid              bool `json:"valid"`
				StatusCode         int  `json:"status_code"`
				Error              string `json:"error"`
				RequestViolations  []struct {
					Path    string `json:"path"`
					Message string `json:"message"`
				} `json:"request_violations"`
				ResponseViolations []struct {
					Path    string `json:"path"`
					Message string `json:"message"`
				} `json:"response_violations"`
			}
			json.Unmarshal(data, &result)

			if result.Valid {
				fmt.Printf("PASS  %s (status: %d)\n", ep, result.StatusCode)
				pass++
			} else {
				fmt.Printf("FAIL  %s (status: %d)\n", ep, result.StatusCode)
				if result.Error != "" {
					fmt.Printf("  - error: %s\n", result.Error)
				}
				for _, v := range result.RequestViolations {
					fmt.Printf("  - [req] [%s] %s\n", v.Path, v.Message)
				}
				for _, v := range result.ResponseViolations {
					fmt.Printf("  - [resp] [%s] %s\n", v.Path, v.Message)
				}
				fail++
			}
		}

		total := pass + fail
		fmt.Printf("\n%d/%d endpoints PASS", pass, total)
		if fail > 0 {
			fmt.Printf(", %d FAIL", fail)
		}
		fmt.Println()
		if fail > 0 {
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown contract command: %s\n", args[0])
		os.Exit(1)
	}
}

// --- Backup/Restore commands ---

func handleBackup(cfg *config, args []string) {
	output := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--output" && i+1 < len(args) {
			output = args[i+1]
			i++
		}
	}
	if output == "" {
		fmt.Fprintln(os.Stderr, "usage: koor-cli backup --output <path>")
		os.Exit(1)
	}

	backup := map[string]any{}

	// Backup state.
	resp, err := doRequest(cfg, "GET", "/api/state", nil)
	if err != nil {
		fatal(fmt.Errorf("backup state list: %w", err))
	}
	stateListData, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var stateItems []struct {
		Key string `json:"key"`
	}
	json.Unmarshal(stateListData, &stateItems)

	stateBackup := map[string]json.RawMessage{}
	for _, item := range stateItems {
		resp, err := doRequest(cfg, "GET", "/api/state/"+item.Key, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not backup state key %s: %v\n", item.Key, err)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		stateBackup[item.Key] = json.RawMessage(data)
	}
	backup["state"] = stateBackup

	// Backup rules.
	resp, err = doRequest(cfg, "GET", "/api/rules/export?source=local,learned,external,user-rules", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not backup rules: %v\n", err)
	} else {
		rulesData, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var rules []json.RawMessage
		json.Unmarshal(rulesData, &rules)
		backup["rules"] = rules
	}

	data, _ := json.MarshalIndent(backup, "", "  ")
	if err := os.WriteFile(output, data, 0o644); err != nil {
		fatal(fmt.Errorf("write backup file: %w", err))
	}
	fmt.Printf("backup saved to %s\n", output)
	fmt.Printf("  state keys: %d\n", len(stateBackup))
	if rules, ok := backup["rules"].([]json.RawMessage); ok {
		fmt.Printf("  rules: %d\n", len(rules))
	}
}

func handleRestore(cfg *config, args []string) {
	filePath := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--file" && i+1 < len(args) {
			filePath = args[i+1]
			i++
		}
	}
	if filePath == "" {
		fmt.Fprintln(os.Stderr, "usage: koor-cli restore --file <path>")
		os.Exit(1)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		fatal(fmt.Errorf("read backup file: %w", err))
	}

	var backup struct {
		State map[string]json.RawMessage `json:"state"`
		Rules []json.RawMessage          `json:"rules"`
	}
	if err := json.Unmarshal(data, &backup); err != nil {
		fatal(fmt.Errorf("invalid backup JSON: %w", err))
	}

	// Restore state.
	stateCount := 0
	for key, val := range backup.State {
		resp, err := doRequest(cfg, "PUT", "/api/state/"+key, strings.NewReader(string(val)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not restore state key %s: %v\n", key, err)
			continue
		}
		resp.Body.Close()
		stateCount++
	}

	// Restore rules.
	rulesCount := 0
	if len(backup.Rules) > 0 {
		rulesJSON, _ := json.Marshal(backup.Rules)
		resp, err := doRequest(cfg, "POST", "/api/rules/import", strings.NewReader(string(rulesJSON)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not restore rules: %v\n", err)
		} else {
			respData, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var result struct {
				Imported int `json:"imported"`
			}
			json.Unmarshal(respData, &result)
			rulesCount = result.Imported
		}
	}

	fmt.Printf("restore complete from %s\n", filePath)
	fmt.Printf("  state keys: %d\n", stateCount)
	fmt.Printf("  rules: %d\n", rulesCount)
}

// --- HTTP client helpers ---

func doRequest(cfg *config, method, path string, body io.Reader) (*http.Response, error) {
	url := strings.TrimRight(cfg.Server, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func printResponse(resp *http.Response) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response: %v\n", err)
		os.Exit(1)
	}

	// Check if --pretty was passed anywhere in os.Args.
	pretty := false
	for _, arg := range os.Args {
		if arg == "--pretty" {
			pretty = true
			break
		}
	}

	if pretty {
		var v any
		if err := json.Unmarshal(data, &v); err == nil {
			formatted, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(formatted))
			return
		}
	}

	fmt.Print(string(data))
}

func readBodyArg(args []string) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("expected --file <path> or --data <json>")
	}
	switch args[0] {
	case "--file":
		data, err := os.ReadFile(args[1])
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", args[1], err)
		}
		return data, nil
	case "--data":
		return []byte(args[1]), nil
	default:
		return nil, fmt.Errorf("expected --file or --data, got %s", args[0])
	}
}

func parseSpecPath(s string) (project, name string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return s, ""
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
