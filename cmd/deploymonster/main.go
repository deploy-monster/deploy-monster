package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/deploy-monster/deploy-monster/internal/core"

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
	configPath := fs.String("config", "", "path to monster.yaml config file")
	fs.Parse(os.Args[1:])

	_ = configPath // will be used when custom config path is supported

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := core.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	bi := core.BuildInfo{Version: version, Commit: commit, Date: date}

	if *agentMode {
		fmt.Printf("DeployMonster Agent %s (%s)\n", version, commit)
		fmt.Println("Agent mode not yet implemented — run in master mode for now.")
		os.Exit(0)
	}

	app, err := core.NewApp(cfg, bi)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init error: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
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
	cfg, err := core.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
	fmt.Println("\nConfig OK")
}

func printUsage() {
	fmt.Printf(`DeployMonster %s — Tame Your Deployments

Usage:
  deploymonster [command] [flags]

Commands:
  serve       Start the DeployMonster server (default)
  version     Print version information
  config      Validate and display current configuration
  help        Show this help

Flags (serve):
  --agent     Run in agent mode (worker node)
  --config    Path to monster.yaml config file

Examples:
  deploymonster                      Start server with defaults
  deploymonster serve --agent        Start as agent/worker node
  deploymonster version              Show version
  deploymonster config               Check configuration

Documentation: https://deploy.monster/docs
`, version)
}
