package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/swarm"

	// All modules auto-register via init()
	_ "github.com/deploy-monster/deploy-monster/internal/api"
	_ "github.com/deploy-monster/deploy-monster/internal/auth"
	_ "github.com/deploy-monster/deploy-monster/internal/backup"
	_ "github.com/deploy-monster/deploy-monster/internal/billing"
	_ "github.com/deploy-monster/deploy-monster/internal/build"
	_ "github.com/deploy-monster/deploy-monster/internal/database"
	_ "github.com/deploy-monster/deploy-monster/internal/db"
	_ "github.com/deploy-monster/deploy-monster/internal/deploy"
	_ "github.com/deploy-monster/deploy-monster/internal/discovery"
	_ "github.com/deploy-monster/deploy-monster/internal/dns"
	_ "github.com/deploy-monster/deploy-monster/internal/enterprise"
	_ "github.com/deploy-monster/deploy-monster/internal/gitsources"
	_ "github.com/deploy-monster/deploy-monster/internal/ingress"
	_ "github.com/deploy-monster/deploy-monster/internal/marketplace"
	_ "github.com/deploy-monster/deploy-monster/internal/mcp"
	_ "github.com/deploy-monster/deploy-monster/internal/notifications"
	_ "github.com/deploy-monster/deploy-monster/internal/resource"
	_ "github.com/deploy-monster/deploy-monster/internal/secrets"
	_ "github.com/deploy-monster/deploy-monster/internal/swarm"
	_ "github.com/deploy-monster/deploy-monster/internal/vps"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		runServe()
		return
	}

	switch os.Args[1] {
	case "serve", "start":
		runServe()
	case "version", "--version", "-v":
		runVersion()
	case "config":
		runConfigCheck()
	case "init":
		runInit()
	case "rotate-keys":
		runRotateKeys()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runServe() {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	agentMode := fs.Bool("agent", false, "run in agent mode (worker node)")
	masterURL := fs.String("master", os.Getenv("MONSTER_MASTER_URL"), "master server URL (agent mode)")
	agentToken := fs.String("token", os.Getenv("MONSTER_JOIN_TOKEN"), "join token for agent authentication")
	configPath := fs.String("config", "", "path to monster.yaml config file")
	fs.Parse(os.Args[1:])

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := core.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "config validation error: %v\n", err)
		os.Exit(1)
	}

	// Configure structured logger before anything else uses slog
	core.SetupLogger(cfg.Server.LogLevel, cfg.Server.LogFormat)

	// Audit config for plaintext secrets
	if warnings := cfg.AuditSecrets(); len(warnings) > 0 {
		for _, w := range warnings {
			slog.Warn("config security", "issue", w)
		}
	}

	bi := core.BuildInfo{Version: version, Commit: commit, Date: date}

	// Print startup banner
	core.PrintBanner(bi, cfg)

	if *agentMode {
		runAgent(ctx, bi, *masterURL, *agentToken)
		return
	}

	app, err := core.NewApp(cfg, bi)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init error: %v\n", err)
		os.Exit(1)
	}
	app.ConfigPath = *configPath

	// SIGHUP handler for config hot-reload
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			slog.Info("received SIGHUP, reloading configuration...")
			if err := app.ReloadConfig(); err != nil {
				slog.Error("config reload failed", "error", err)
			}
		}
	}()

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func runAgent(ctx context.Context, bi core.BuildInfo, masterURL, token string) {
	if masterURL == "" {
		fmt.Fprintln(os.Stderr, "agent mode requires --master URL (or MONSTER_MASTER_URL env var)")
		os.Exit(1)
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "agent mode requires --token (or MONSTER_JOIN_TOKEN env var)")
		os.Exit(1)
	}

	fmt.Printf("DeployMonster Agent %s (%s)\n", bi.Version, bi.Commit)
	fmt.Printf("  master: %s\n", masterURL)

	logger := slog.Default().With("mode", "agent")

	// Generate a server ID from the hostname
	serverID := os.Getenv("MONSTER_SERVER_ID")
	if serverID == "" {
		hostname, _ := os.Hostname()
		serverID = hostname
	}
	if serverID == "" {
		serverID = core.GenerateID()
	}

	// Create agent client (runtime will be nil until we wire Docker SDK)
	client := swarm.NewAgentClient(masterURL, serverID, token, bi.Version, nil, logger)

	fmt.Printf("  server_id: %s\n", serverID)
	fmt.Println("  connecting to master...")

	if err := client.ConnectWithRetry(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}

func runVersion() {
	fmt.Printf("DeployMonster %s\n", version)
	fmt.Printf("  commit:  %s\n", commit)
	fmt.Printf("  built:   %s\n", date)
	fmt.Printf("  go:      %s\n", "go1.26")
}

func runConfigCheck() {
	cfg, err := core.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
	fmt.Println("\nConfig OK")
}

func runInit() {
	if _, err := os.Stat("monster.yaml"); err == nil {
		fmt.Println("monster.yaml already exists. Remove it first to regenerate.")
		os.Exit(1)
	}

	// Read the example config and write it
	content := `# DeployMonster Configuration
# Generated by: deploymonster init

server:
  host: 0.0.0.0
  port: 8443
  # domain: deploy.example.com

database:
  driver: sqlite
  path: deploymonster.db

ingress:
  http_port: 80
  https_port: 443
  enable_https: true

acme:
  # email: admin@example.com
  staging: false

registration:
  mode: open

limits:
  max_apps_per_tenant: 100
  max_concurrent_builds: 5
`
	if err := os.WriteFile("monster.yaml", []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing monster.yaml: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Created monster.yaml — edit it and run: deploymonster")
}

func runRotateKeys() {
	fs := flag.NewFlagSet("rotate-keys", flag.ExitOnError)
	configPath := fs.String("config", "", "path to monster.yaml config file")
	newKey := fs.String("new-key", "", "new encryption key (required)")
	fs.Parse(os.Args[2:])

	if *newKey == "" {
		fmt.Fprintln(os.Stderr, "rotate-keys requires --new-key flag")
		os.Exit(1)
	}

	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	core.SetupLogger(cfg.Server.LogLevel, cfg.Server.LogFormat)

	bi := core.BuildInfo{Version: version, Commit: commit, Date: date}
	app, err := core.NewApp(cfg, bi)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init error: %v\n", err)
		os.Exit(1)
	}

	// Initialize modules (db + secrets need to be ready)
	ctx := context.Background()
	if err := app.Registry.Resolve(); err != nil {
		fmt.Fprintf(os.Stderr, "dependency resolution error: %v\n", err)
		os.Exit(1)
	}
	if err := app.Registry.InitAll(ctx, app); err != nil {
		fmt.Fprintf(os.Stderr, "module init error: %v\n", err)
		os.Exit(1)
	}

	// Find the secrets module via the registry
	mod := app.Registry.Get("secrets")
	if mod == nil {
		fmt.Fprintln(os.Stderr, "secrets module not found")
		os.Exit(1)
	}

	type keyRotator interface {
		RotateEncryptionKey(ctx context.Context, newMasterSecret string) (int, error)
	}

	rotator, ok := mod.(keyRotator)
	if !ok {
		fmt.Fprintln(os.Stderr, "secrets module does not support key rotation")
		os.Exit(1)
	}

	count, err := rotator.RotateEncryptionKey(ctx, *newKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "key rotation failed after %d versions: %v\n", count, err)
		os.Exit(1)
	}

	fmt.Printf("Key rotation complete: %d secret version(s) re-encrypted.\n", count)
	fmt.Println("Update your config to use the new encryption key and restart the server.")
}

func printUsage() {
	fmt.Printf(`DeployMonster %s — Tame Your Deployments

Usage:
  deploymonster [command] [flags]

Commands:
  serve        Start the DeployMonster server (default)
  version      Print version information
  config       Validate and display current configuration
  rotate-keys  Re-encrypt all secrets with a new encryption key
  help         Show this help

Flags (serve):
  --agent     Run in agent mode (worker node)
  --master    Master server URL (agent mode, or MONSTER_MASTER_URL)
  --token     Join token for agent auth (agent mode, or MONSTER_JOIN_TOKEN)
  --config    Path to monster.yaml config file

Examples:
  deploymonster                                          Start server with defaults
  deploymonster serve --agent --master=host:8443 --token=xxx  Start as agent
  deploymonster version                                  Show version
  deploymonster config                                   Check configuration

Documentation: https://deploy.monster/docs
`, version)
}
