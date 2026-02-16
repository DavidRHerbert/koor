package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DavidRHerbert/koor/internal/audit"
	"github.com/DavidRHerbert/koor/internal/compliance"
	"github.com/DavidRHerbert/koor/internal/db"
	"github.com/DavidRHerbert/koor/internal/events"
	"github.com/DavidRHerbert/koor/internal/instances"
	"github.com/DavidRHerbert/koor/internal/liveness"
	koormcp "github.com/DavidRHerbert/koor/internal/mcp"
	"github.com/DavidRHerbert/koor/internal/server"
	"github.com/DavidRHerbert/koor/internal/server/serverconfig"
	"github.com/DavidRHerbert/koor/internal/observability"
	"github.com/DavidRHerbert/koor/internal/specs"
	"github.com/DavidRHerbert/koor/internal/state"
	"github.com/DavidRHerbert/koor/internal/templates"
	"github.com/DavidRHerbert/koor/internal/webhooks"
)

// fileConfig mirrors the JSON structure in settings.json.
type fileConfig struct {
	Bind          string `json:"bind"`
	DashboardBind string `json:"dashboard_bind"`
	DataDir       string `json:"data_dir"`
	AuthToken     string `json:"auth_token"`
	LogLevel      string `json:"log_level"`
}

func main() {
	defaultDataDir := "."

	// 1. Load config file (base layer).
	fc := loadConfigFile(defaultDataDir)

	// 2. Register flags with config-file values as defaults.
	bind := flag.String("bind", fc.Bind, "API listen address")
	dashBind := flag.String("dashboard-bind", fc.DashboardBind, "dashboard listen address (empty = disabled)")
	dataDir := flag.String("data-dir", fc.DataDir, "SQLite database directory (default: current directory)")
	authToken := flag.String("auth-token", fc.AuthToken, "bearer token (empty = no auth)")
	logLevel := flag.String("log-level", fc.LogLevel, "log level: debug|info|warn|error")
	configFile := flag.String("config", "", "path to config file (default: ./settings.json)")
	flag.Parse()

	// If --config was explicitly provided, reload from that path.
	if *configFile != "" {
		fc = loadConfigFileFrom(*configFile, defaultDataDir)
		// Re-apply file values for any flags not explicitly set on CLI.
		applyFileDefaults(fc, bind, dashBind, dataDir, authToken, logLevel)
	}

	// 3. Environment variables override config file + flag defaults.
	if v := os.Getenv("KOOR_BIND"); v != "" {
		*bind = v
	}
	if v := os.Getenv("KOOR_DATA_DIR"); v != "" {
		*dataDir = v
	}
	if v := os.Getenv("KOOR_AUTH_TOKEN"); v != "" {
		*authToken = v
	}
	if v := os.Getenv("KOOR_DASHBOARD_BIND"); v != "" {
		*dashBind = v
	}
	if v := os.Getenv("KOOR_LOG_LEVEL"); v != "" {
		*logLevel = v
	}

	// 4. CLI flags (if explicitly set) win over everything — already handled
	//    by flag.Parse() above since explicitly-set flags overwrite the pointer.

	// Setup structured logging.
	var level slog.Level
	level.UnmarshalText([]byte(*logLevel))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// Open database.
	database, err := db.Open(*dataDir)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create stores.
	stateStore := state.New(database)
	specReg := specs.New(database)
	eventBus := events.New(database, 1000)
	instanceReg := instances.New(database)

	// Create MCP transport.
	mcpTransport := koormcp.New(instanceReg, specReg, serverconfig.Endpoints{
		APIBase: "http://" + *bind,
	})

	// Create server.
	cfg := server.Config{
		Bind:          *bind,
		DashboardBind: *dashBind,
		DataDir:       *dataDir,
		AuthToken:     *authToken,
	}
	srv := server.New(cfg, stateStore, specReg, eventBus, instanceReg, mcpTransport, logger)

	// Start liveness monitor (checks every 60s, marks stale after 5m of no heartbeat).
	liveMon := liveness.New(instanceReg, eventBus, 5*time.Minute, 60*time.Second, logger)
	liveMon.Start()
	defer liveMon.Stop()
	srv.SetLiveness(liveMon)

	// Start webhook dispatcher (subscribes to all events, dispatches to registered URLs).
	webhookDisp := webhooks.New(database, eventBus, logger)
	webhookDisp.Start()
	defer webhookDisp.Stop()
	srv.SetWebhooks(webhookDisp)

	// Start compliance scheduler (checks active agents every 5 minutes).
	compSched := compliance.New(database, instanceReg, specReg, eventBus, 5*time.Minute, logger)
	compSched.Start()
	defer compSched.Stop()
	srv.SetCompliance(compSched)

	// Create template store.
	templateStore := templates.New(database)
	srv.SetTemplates(templateStore)

	// Create audit log and observability metrics.
	auditLog := audit.New(database)
	srv.SetAudit(auditLog)
	metricsStore := observability.New(database)
	srv.SetObservability(metricsStore)

	// Start background event pruning (every 60 seconds).
	eventBus.StartPruning(60 * time.Second)
	defer eventBus.Stop()

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("koor server starting",
		"api", *bind,
		"dashboard", *dashBind,
		"data_dir", *dataDir,
		"auth", *authToken != "",
	)

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

// loadConfigFile tries ./settings.json.
func loadConfigFile(defaultDataDir string) fileConfig {
	if fc, ok := readConfigFile("settings.json", defaultDataDir); ok {
		return fc
	}
	return defaults(defaultDataDir)
}

// loadConfigFileFrom loads from an explicit path.
func loadConfigFileFrom(path, defaultDataDir string) fileConfig {
	if fc, ok := readConfigFile(path, defaultDataDir); ok {
		return fc
	}
	return defaults(defaultDataDir)
}

func readConfigFile(path, defaultDataDir string) (fileConfig, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, false
	}
	fc := defaults(defaultDataDir)
	if err := json.Unmarshal(data, &fc); err != nil {
		return fileConfig{}, false
	}
	// If data_dir is empty in the file, use the default.
	if fc.DataDir == "" {
		fc.DataDir = defaultDataDir
	}
	return fc, true
}

func defaults(defaultDataDir string) fileConfig {
	return fileConfig{
		Bind:          "localhost:9800",
		DashboardBind: "localhost:9847",
		DataDir:       defaultDataDir,
		AuthToken:     "",
		LogLevel:      "info",
	}
}

// applyFileDefaults sets flag values from the config file for any flags
// that were NOT explicitly set on the command line.
func applyFileDefaults(fc fileConfig, bind, dashBind, dataDir, authToken, logLevel *string) {
	explicitly := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicitly[f.Name] = true })

	if !explicitly["bind"] {
		*bind = fc.Bind
	}
	if !explicitly["dashboard-bind"] {
		*dashBind = fc.DashboardBind
	}
	if !explicitly["data-dir"] {
		*dataDir = fc.DataDir
	}
	if !explicitly["auth-token"] {
		*authToken = fc.AuthToken
	}
	if !explicitly["log-level"] {
		*logLevel = fc.LogLevel
	}
}
