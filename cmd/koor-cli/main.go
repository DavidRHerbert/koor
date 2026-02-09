package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

Flags:
  --pretty                        Pretty-print JSON output

Environment:
  KOOR_SERVER                     Server URL (overrides config)
  KOOR_TOKEN                      Auth token (overrides config)`)
}

// --- Config management ---

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".koor", "config.toml")
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
	f, err := os.Open(configPath())
	if err != nil {
		return cfg
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Remove surrounding quotes if present.
		val = strings.Trim(val, `"'`)

		switch key {
		case "server":
			if os.Getenv("KOOR_SERVER") == "" {
				cfg.Server = val
			}
		case "token":
			if os.Getenv("KOOR_TOKEN") == "" {
				cfg.Token = val
			}
		}
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

	// Read existing config.
	cfg := loadConfig()
	switch key {
	case "server":
		cfg.Server = value
	case "token":
		cfg.Token = value
	}

	// Write config file.
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error creating config dir: %v\n", err)
		os.Exit(1)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("server = %q\n", cfg.Server))
	if cfg.Token != "" {
		sb.WriteString(fmt.Sprintf("token = %q\n", cfg.Token))
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
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
