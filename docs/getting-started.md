# Getting Started

Deploy your first application in under 5 minutes.

## Prerequisites

- A Linux server (Ubuntu 22.04+ recommended) or macOS
- Docker installed and running
- Ports 80, 443, and 8443 available

## Installation

### Option 1: Quick Install (recommended)

```bash
curl -fsSL https://get.deploy.monster | bash
```

### Option 2: Download Binary

```bash
# Linux (amd64)
wget https://github.com/Deploy-Monster/DeployMonster_GO/releases/latest/download/deploymonster_linux_amd64.tar.gz
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
  deploymonster/deploymonster:latest
```

### Option 4: Build from Source

```bash
git clone https://github.com/Deploy-Monster/DeployMonster_GO.git
cd deploy-monster
bash scripts/build.sh
./bin/deploymonster
```

## First Run

1. Start the server:

```bash
deploymonster
```

2. On first startup, DeployMonster will:
   - Create a SQLite database
   - Run all migrations (25+ tables)
   - Seed RBAC roles (Super Admin, Owner, Admin, Developer, Operator, Viewer)
   - Auto-generate a super admin account
   - Print credentials to the console

3. Open your browser: `https://localhost:8443`

4. Log in with the printed credentials.

## Configuration

Generate a config file:

```bash
deploymonster init
```

This creates `monster.yaml` with all available settings. Key options:

```yaml
server:
  port: 8443
  domain: deploy.example.com

database:
  driver: sqlite        # or "postgres" for enterprise
  path: deploymonster.db

ingress:
  http_port: 80
  https_port: 443

acme:
  email: admin@example.com  # For Let's Encrypt SSL

registration:
  mode: open  # open, invite_only, approval, disabled
```

Or use environment variables:

```bash
export MONSTER_PORT=8443
export MONSTER_DOMAIN=deploy.example.com
export MONSTER_ADMIN_EMAIL=admin@example.com
export MONSTER_ADMIN_PASSWORD=your-secure-password
```

## Deploy Your First App

### From Docker Image

1. Go to **Applications** → **Deploy New App**
2. Select **Docker Image**
3. Enter: `nginx:alpine`
4. Name it: `my-first-app`
5. Click **Deploy**

Your app is live at `https://my-first-app.deploy.example.com` (if domain is configured).

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

## Next Steps

- [Add a custom domain](./deployment-guide.md#custom-domains)
- [Connect a Git provider](./deployment-guide.md#git-providers) for auto-deploy on push
- [Set up backups](./deployment-guide.md#backups)
- [Invite team members](./deployment-guide.md#team-management)
- [Configure notifications](./deployment-guide.md#notifications) (Slack, Discord, Telegram)
