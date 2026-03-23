package main

import (
	"context"
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
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := core.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	app, err := core.NewApp(cfg, core.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init error: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
