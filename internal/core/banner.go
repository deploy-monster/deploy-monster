package core

import (
	"fmt"
	"runtime"
)

// PrintBanner displays the startup banner with system information.
func PrintBanner(build BuildInfo, cfg *Config) {
	banner := `
  ____             _             __  __                 _
 |  _ \  ___ _ __ | | ___  _   _|  \/  | ___  _ __  ___| |_ ___ _ __
 | | | |/ _ \ '_ \| |/ _ \| | | | |\/| |/ _ \| '_ \/ __| __/ _ \ '__|
 | |_| |  __/ |_) | | (_) | |_| | |  | | (_) | | | \__ \ ||  __/ |
 |____/ \___| .__/|_|\___/ \__, |_|  |_|\___/|_| |_|___/\__\___|_|
            |_|            |___/
`
	fmt.Print(banner)
	fmt.Printf("  Version:    %s (%s)\n", build.Version, build.Commit)
	fmt.Printf("  Go:         %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  API:        https://%s:%d\n", cfg.Server.Host, cfg.Server.Port)

	if cfg.Ingress.EnableHTTPS {
		fmt.Printf("  Ingress:    http://:%d → https://:%d\n", cfg.Ingress.HTTPPort, cfg.Ingress.HTTPSPort)
	} else {
		fmt.Printf("  Ingress:    http://:%d\n", cfg.Ingress.HTTPPort)
	}

	fmt.Printf("  Database:   %s (%s)\n", cfg.Database.Driver, cfg.Database.Path)
	fmt.Println()
}
