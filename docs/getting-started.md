# Getting Started

Deploy your first application in under 5 minutes.

## Prerequisites

- A Linux server (Ubuntu 22.04+ recommended) or macOS
- Docker installed and running
- Ports 80, 443, and 8443 available

## Installation

### Option 1: Quick Install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.1.8/scripts/install.sh \
  | bash -s -- --version=v0.1.8
```

For a multi-server master, pass a stable join token during install:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.1.8/scripts/install.sh \
  | bash -s -- --version=v0.1.8 --token=JOIN_TOKEN
```

### Agent Node Install

Run this on each worker server after the master is reachable and you have the shared join token:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.1.8/scripts/install.sh \
  | bash -s -- --version=v0.1.8 --agent \
      --master=http://MASTER_HOST:8443 \
      --token=JOIN_TOKEN \
      --server-id=worker-1
```

The installer creates the same `deploymonster` systemd service, but starts the binary in `serve --agent` mode and stores the master URL/join token in `/etc/deploymonster/deploymonster.env`.

### Option 2: Download Binary

```bash
# Linux (amd64)
wget https://github.com/deploy-monster/deploy-monster/releases/latest/download/deploymonster_linux_amd64.tar.gz
tar xzf deploymonster_linux_amd64.tar.gz
sudo mv deploymonster /usr/local/bin/
```

### Option 3: Docker

```bash
docker run -d \
  --name deploymonster \
  -p 8443:8443 -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dm-data:/var/lib/deploymonster \
  ghcr.io/deploy-monster/deploy-monster:v0.1.8
```

### Option 4: Build from Source

```bash
git clone https://github.com/deploy-monster/deploy-monster.git
cd deploy-monster
bash scripts/build.sh
./bin/deploymonster
```

## First Run

### Quick Start (IP / local test)

```bash
deploymonster
```

Open `http://SERVER_IP:8443` in your browser. Default admin credentials are printed in the console if not pre-configured.

### Production Setup (custom domain + automatic SSL)

Run the interactive wizard and follow the prompts:

```bash
deploymonster setup
```

This will ask for your domain, Let's Encrypt email, admin credentials, and write the configuration to `/var/lib/deploymonster/monster.yaml`. The platform UI/API stays on `http://<domain>:8443`; ports `80` and `443` are used by application ingress and automatic certificates for deployed apps.

After setup:

1. Point your DNS A record to the server IP.
2. Open ports `80` and `443` in your firewall.
3. Restart the service:

```bash
sudo systemctl restart deploymonster
```

4. Open `http://your-domain.com:8443` for the platform UI. Deployed app domains use ports `80` and `443`; the first TLS handshake for an app domain triggers automatic certificate provisioning via Let's Encrypt (HTTP-01 challenge).

## Understanding Admin Roles

DeployMonster has **two levels of administration**:

| Role | Access Level | What They Manage |
|------|--------------|------------------|
| **System Admin** | Platform-wide | Tenants, servers, VPS providers, system settings, all resources |
| **Client Admin** | Tenant-level | Own projects, apps, databases, domains, team members, billing |

### System Admin (Platform Owner)

The first login is always a **System Admin**. They can:
- Create and manage tenants (organizations)
- Provision VPS servers from Hetzner, DigitalOcean, Vultr, Linode
- Configure DNS providers (Cloudflare)
- Set up system-wide backups
- View all resources across all tenants
- Manage system settings and security

### Client Admin (Tenant Owner)

When a System Admin creates a tenant, they can assign a **Client Admin** who can:
- Create and manage projects
- Deploy applications from Git, Docker images, or marketplace
- Manage databases (PostgreSQL, MySQL, Redis, MongoDB)
- Configure custom domains with SSL
- Invite team members with role-based access
- View billing and upgrade plans

## Deploy Your First App

### From Docker Image

1. Go to **Applications** → **Deploy New App**
2. Select **Docker Image**
3. Enter image: `nginx:alpine`
4. Click **Deploy**

Your app is live at `https://<app-name>.<tenant>.deploy.example.com`

### From Git Repository

1. Go to **Applications** → **Deploy New App**
2. Select **Git Repository**
3. Enter your repo URL: `https://github.com/user/app.git`
4. Select branch: `main`
5. DeployMonster auto-detects the project type and builds it

### From Marketplace

1. Go to **Marketplace**
2. Click **Deploy** on WordPress, Ghost, n8n, or any template
3. Configure variables (database password, etc.)
4. One click — your stack is running

## Multi-Tenancy Example

```bash
# As System Admin, list/manage tenants through the admin API/UI.
GET /api/v1/admin/tenants

# Invite a user into the current tenant through the team API/UI.
POST /api/v1/team/invites
{
  "email": "admin@acme.com",
  "role_id": "role_admin"
}

# Client Admin logs in and sees only their tenant-scoped resources.
# They can create projects, deploy apps, and invite team members subject to RBAC.
```

The exact tenant-admin mutation payloads are defined in the generated OpenAPI
spec (`docs/openapi.yaml` / `GET /api/v1/openapi.json`).

## Next Steps

- [Add a custom domain](./deployment-guide.md#custom-domains)
- [Connect a Git provider](./deployment-guide.md#git-providers) for auto-deploy on push
- [Set up backups](./deployment-guide.md#backups)
- [Invite team members](./deployment-guide.md#team-management)
- [Configure notifications](./deployment-guide.md#notifications) (Slack, Discord, Telegram)
- [Provision a VPS](./deployment-guide.md#vps-provisioning) from Hetzner, DigitalOcean, or others
