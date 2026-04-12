package vps

import "fmt"

// GenerateCloudInit returns a cloud-init YAML that provisions a new server as
// a DeployMonster agent. It installs Docker, downloads the DeployMonster
// binary, and starts the agent service pointing at the given master.
func GenerateCloudInit(masterURL, token, version string) string {
	return fmt.Sprintf(`#cloud-config
package_update: true
packages:
  - curl
  - ca-certificates

write_files:
  - path: /etc/systemd/system/deploymonster-agent.service
    permissions: "0644"
    content: |
      [Unit]
      Description=DeployMonster Agent
      After=docker.service
      Requires=docker.service

      [Service]
      Type=simple
      ExecStart=/usr/local/bin/deploymonster serve --agent --master=%s --token=%s
      Restart=always
      RestartSec=5
      Environment=MONSTER_MASTER_URL=%s
      Environment=MONSTER_JOIN_TOKEN=%s

      [Install]
      WantedBy=multi-user.target

runcmd:
  # Install Docker
  - |
    if ! command -v docker &>/dev/null; then
      curl -fsSL https://get.docker.com | sh
      systemctl enable docker
      systemctl start docker
    fi
  # Configure firewall
  - |
    if command -v ufw &>/dev/null; then
      ufw allow 22/tcp
      ufw allow 80/tcp
      ufw allow 443/tcp
      ufw --force enable
    fi
  # Create data directory
  - mkdir -p /var/lib/deploymonster
  # Download DeployMonster binary
  - |
    ARCH=$(dpkg --print-architecture 2>/dev/null || echo amd64)
    curl -fsSL "https://github.com/deploy-monster/deploy-monster/releases/download/v%s/deploymonster-linux-${ARCH}" \
      -o /usr/local/bin/deploymonster
    chmod +x /usr/local/bin/deploymonster
  # Start agent
  - systemctl daemon-reload
  - systemctl enable deploymonster-agent
  - systemctl start deploymonster-agent
`, masterURL, token, masterURL, token, version)
}
