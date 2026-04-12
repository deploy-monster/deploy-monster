package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/swarm"
	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v3"

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
	case "setup":
		runSetup()
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
	masterPort := fs.Int("master-port", envInt("MONSTER_MASTER_PORT", 0), "fallback port for --master if URL has none (agent mode; 0 = 8443)")
	configPath := fs.String("config", "", "path to monster.yaml config file")
	_ = fs.Parse(os.Args[1:])

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

	if *agentMode {
		runAgent(ctx, bi, *masterURL, *agentToken, *masterPort)
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

func runAgent(ctx context.Context, bi core.BuildInfo, masterURL, token string, masterPort int) {
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
	if masterPort > 0 {
		client.SetDefaultPort(masterPort)
	}

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
	_ = fs.Parse(os.Args[2:])

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

// envInt reads an int-valued environment variable with a default fallback.
// Non-integer values fall back to def rather than failing flag parsing.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func prompt(r *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	s, _ := r.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

func promptBool(r *bufio.Reader, label string, def bool) bool {
	defStr := "N"
	if def {
		defStr = "Y"
	}
	fmt.Printf("%s [%s]: ", label, defStr)
	s, _ := r.ReadString('\n')
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return def
	}
	return s == "y" || s == "yes"
}

func runSetup() {
	cfg, err := core.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "setup requires an interactive terminal.")
		fmt.Fprintln(os.Stderr, "Tip: run `deploymonster setup` directly on the server.")
		os.Exit(1)
	}

	r := bufio.NewReader(os.Stdin)

	fmt.Println("\n=== DeployMonster Interactive Setup ===")
	fmt.Println("Leave a field empty to keep the current/default value.")
	fmt.Println()

	domain := prompt(r, "Domain (e.g. deploy.example.com)", cfg.Server.Domain)
	email := prompt(r, "ACME / Let's Encrypt email", cfg.ACME.Email)
	httpPortStr := prompt(r, "HTTP port", strconv.Itoa(cfg.Ingress.HTTPPort))
	httpsPortStr := prompt(r, "HTTPS port", strconv.Itoa(cfg.Ingress.HTTPSPort))
	staging := promptBool(r, "Use Let's Encrypt staging (recommended for first test)", cfg.ACME.Staging)
	forceHTTPS := promptBool(r, "Force HTTPS redirect from HTTP", cfg.Ingress.ForceHTTPS)

	if domain != "" {
		cfg.Server.Domain = domain
		cfg.Server.CORSOrigins = "" // derive later
		if cfg.Server.Port == 8443 {
			cfg.Server.Port = 443
		}
	}
	if email != "" {
		cfg.ACME.Email = email
		cfg.ACME.Provider = "http-01"
	}
	if p, err := strconv.Atoi(httpPortStr); err == nil {
		cfg.Ingress.HTTPPort = p
	}
	if p, err := strconv.Atoi(httpsPortStr); err == nil {
		cfg.Ingress.HTTPSPort = p
	}
	cfg.ACME.Staging = staging
	cfg.Ingress.ForceHTTPS = forceHTTPS

	fmt.Println("\n--- Admin Credentials ---")
	adminEmail := prompt(r, "Admin email", "admin@local.host")
	adminPassword := prompt(r, "Admin password (empty = auto-generate)", "")
	if adminPassword == "" {
		adminPassword = core.GenerateSecret(16)
		fmt.Printf("Auto-generated admin password: %s\n", adminPassword)
	}

	// Derive CORS origins from new domain/port
	if cfg.Server.CORSOrigins == "" && cfg.Server.Domain != "" {
		origin := "https://" + cfg.Server.Domain
		if cfg.Ingress.HTTPSPort != 443 {
			origin = fmt.Sprintf("https://%s:%d", cfg.Server.Domain, cfg.Ingress.HTTPSPort)
		}
		cfg.Server.CORSOrigins = origin
	}

	if err := core.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "validation error: %v\n", err)
		os.Exit(1)
	}

	configPath := "/var/lib/deploymonster/monster.yaml"
	if _, err := os.Stat(configPath); err == nil {
		backup := configPath + ".bak." + strconv.FormatInt(time.Now().Unix(), 10)
		if err := os.Rename(configPath, backup); err != nil {
			fmt.Fprintf(os.Stderr, "failed to backup config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Backed up existing config to %s\n", backup)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal config: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll("/var/lib/deploymonster", 0750); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create data dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(configPath, data, 0640); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nConfig written to %s\n", configPath)

	// Update systemd unit env vars if the unit exists
	unitUpdated := false
	if _, err := os.Stat("/etc/systemd/system/deploymonster.service"); err == nil {
		content, _ := os.ReadFile("/etc/systemd/system/deploymonster.service")
		if strings.Contains(string(content), "ExecStart=") {
			lines := strings.Split(string(content), "\n")
			var out []string
			for _, line := range lines {
				if strings.HasPrefix(line, "Environment=MONSTER_ADMIN_EMAIL=") ||
					strings.HasPrefix(line, "Environment=MONSTER_ADMIN_PASSWORD=") {
					continue
				}
				out = append(out, line)
			}
			// Re-insert env vars before [Install]
			var final []string
			for _, line := range out {
				final = append(final, line)
				if strings.HasPrefix(line, "[Install]") {
					final = final[:len(final)-1]
					final = append(final, fmt.Sprintf("Environment=MONSTER_ADMIN_EMAIL=%s", adminEmail))
					final = append(final, fmt.Sprintf("Environment=MONSTER_ADMIN_PASSWORD=%s", adminPassword))
					final = append(final, "[Install]")
				}
			}
			_ = os.WriteFile("/etc/systemd/system/deploymonster.service", []byte(strings.Join(final, "\n")), 0644)
			unitUpdated = true
		}
	}

	fmt.Println("\nNext steps:")
	if cfg.Server.Domain != "" {
		fmt.Println("  1. Ensure your domain's A/AAAA record points to this server's public IP")
		fmt.Println("  2. Open ports 80 and 443 in your firewall")
	}
	if unitUpdated {
		fmt.Println("  3. Run: sudo systemctl daemon-reload && sudo systemctl restart deploymonster")
	} else {
		fmt.Println("  3. Run: deploymonster serve --config=" + configPath)
	}
	fmt.Printf("\nAdmin login: %s / %s\n", adminEmail, adminPassword)
}

func printUsage() {
	fmt.Printf(`DeployMonster %s — Tame Your Deployments

Usage:
  deploymonster [command] [flags]

Commands:
  serve        Start the DeployMonster server (default)
  version      Print version information
  config       Validate and display current configuration
  setup        Interactive setup wizard (domain, SSL, admin credentials)
  rotate-keys  Re-encrypt all secrets with a new encryption key
  help         Show this help

Flags (serve):
  --agent         Run in agent mode (worker node)
  --master        Master server URL (agent mode, or MONSTER_MASTER_URL)
  --token         Join token for agent auth (agent mode, or MONSTER_JOIN_TOKEN)
  --master-port   Fallback port if --master URL has none (agent mode, or MONSTER_MASTER_PORT)
  --config        Path to monster.yaml config file

Examples:
  deploymonster                                          Start server with defaults
  deploymonster serve --agent --master=host:8443 --token=xxx  Start as agent
  deploymonster version                                  Show version
  deploymonster config                                   Check configuration

Documentation: https://github.com/deploy-monster/deploy-monster/tree/master/docs
`, version)
}
