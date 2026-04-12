package vps

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// BootstrapScript generates a cloud-init / shell script that:
// 1. Updates the system
// 2. Installs Docker
// 3. Configures firewall
// 4. Downloads and starts the DeployMonster agent
func BootstrapScript(masterURL, joinToken, serverID string) string {
	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

echo "==> DeployMonster Server Bootstrap"
echo "==> Server ID: %s"

# Update system
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get upgrade -y -qq

# Install Docker
if ! command -v docker &>/dev/null; then
    echo "==> Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
    echo "==> Docker installed: $(docker --version)"
fi

# Configure firewall (ufw)
if command -v ufw &>/dev/null; then
    ufw allow 22/tcp    # SSH
    ufw allow 80/tcp    # HTTP
    ufw allow 443/tcp   # HTTPS
    ufw allow 2377/tcp  # Docker Swarm
    ufw allow 7946/tcp  # Swarm node communication
    ufw allow 7946/udp
    ufw allow 4789/udp  # Overlay network
    ufw --force enable
    echo "==> Firewall configured"
fi

# Create DeployMonster data directory
mkdir -p /var/lib/deploymonster

# Download DeployMonster agent binary
echo "==> Downloading DeployMonster agent..."
ARCH=$(dpkg --print-architecture)
curl -fsSL "%s/api/v1/agent/binary?arch=${ARCH}" -o /usr/local/bin/deploymonster
chmod +x /usr/local/bin/deploymonster

# Create systemd service for agent
cat > /etc/systemd/system/deploymonster-agent.service << 'UNIT'
[Unit]
Description=DeployMonster Agent
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/deploymonster serve --agent
Restart=always
RestartSec=5
Environment=MONSTER_MASTER_URL=%s
Environment=MONSTER_JOIN_TOKEN=%s
Environment=MONSTER_SERVER_ID=%s

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable deploymonster-agent
systemctl start deploymonster-agent

echo "==> DeployMonster agent started"
echo "==> Bootstrap complete"
`, serverID, masterURL, masterURL, joinToken, serverID)
}

// Bootstrap connects to a remote server via SSH and installs the DeployMonster
// agent. It checks for Docker, downloads the binary, creates a systemd service,
// and starts the agent pointing at the master.
func Bootstrap(ctx context.Context, pool *SSHPool, host string, port int, user string, key []byte, masterURL, token string, logger *slog.Logger) error {
	run := func(cmd string) (string, error) {
		return pool.Execute(ctx, host, port, user, key, cmd)
	}

	logger.Info("bootstrapping remote server", "host", host)

	// 1. Check if Docker is installed, install if not
	if _, err := run("command -v docker"); err != nil {
		logger.Info("installing Docker", "host", host)
		if _, err := run("curl -fsSL https://get.docker.com | sh && systemctl enable docker && systemctl start docker"); err != nil {
			return fmt.Errorf("install docker on %s: %w", host, err)
		}
	}

	// 2. Download DeployMonster binary
	logger.Info("downloading DeployMonster binary", "host", host)
	downloadCmd := fmt.Sprintf(
		`ARCH=$(dpkg --print-architecture 2>/dev/null || echo amd64) && ` +
			`curl -fsSL "https://github.com/deploy-monster/deploy-monster/releases/latest/download/deploymonster-linux-${ARCH}" ` +
			`-o /usr/local/bin/deploymonster && chmod +x /usr/local/bin/deploymonster`,
	)
	if _, err := run(downloadCmd); err != nil {
		return fmt.Errorf("download binary on %s: %w", host, err)
	}

	// 3. Create systemd service file
	logger.Info("creating systemd service", "host", host)
	serviceFile := fmt.Sprintf(`[Unit]
Description=DeployMonster Agent
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/deploymonster serve --agent --master=%s --token=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`, masterURL, token)

	writeCmd := fmt.Sprintf("cat > /etc/systemd/system/deploymonster-agent.service << 'SERVICEFILE'\n%s\nSERVICEFILE", serviceFile)
	if _, err := run(writeCmd); err != nil {
		return fmt.Errorf("create service file on %s: %w", host, err)
	}

	// 4. Start the agent
	logger.Info("starting DeployMonster agent", "host", host)
	if _, err := run("systemctl daemon-reload && systemctl enable deploymonster-agent && systemctl start deploymonster-agent"); err != nil {
		return fmt.Errorf("start agent on %s: %w", host, err)
	}

	logger.Info("bootstrap complete", "host", host)
	return nil
}

// CloudInitConfig generates a cloud-init YAML for VPS providers.
func CloudInitConfig(masterURL, joinToken, serverID string) string {
	script := BootstrapScript(masterURL, joinToken, serverID)

	// Escape for YAML
	indented := ""
	for _, line := range strings.Split(script, "\n") {
		indented += "    " + line + "\n"
	}

	return fmt.Sprintf(`#cloud-config
package_update: true
packages:
  - curl
  - ca-certificates

runcmd:
  - |
%s
`, indented)
}
