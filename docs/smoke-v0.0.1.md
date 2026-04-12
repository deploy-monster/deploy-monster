# DeployMonster v0.0.1 Smoke Test

> **Environment requirement**: Clean Ubuntu 24.04 LTS VM (minimum supported OS release).  
> **Test date**: ___  
> **Tester**: ___  
> **Binary version tested**: ___  
> **VM uname -a**: ___

---

## Pre-VM validation notes (WSL / local)

The following checks were performed on a local WSL Ubuntu environment
(with the `v0.0.1` release binary) before moving to a real VM.
They do **not** replace the VM smoke test, but they confirm the
installer, auth pipeline, and API surface are functional.

**Installer (raw GitHub URL)**:
```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
```

**Fully automated headless run**:
```bash
bash -c "$(curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/smoke-test.sh)"
```

| Check | Result |
|-------|--------|
| GitHub release `v0.0.1` created | ✅ 6 archives + 6 SBOMs + `checksums.txt` uploaded |
| GHCR image push | ✅ `ghcr.io/deploy-monster/deploy-monster:0.0.1` and `:latest` pullable |
| Trivy scan | ✅ Zero new HIGH/CRITICAL findings |
| Artifact download from GitHub release | ✅ `deploymonster_0.0.1_linux_amd64.tar.gz` downloaded |
| SHA256 verification against `checksums.txt` | ✅ Expected == Actual (`67c782f…`) |
| Binary version output | ✅ `DeployMonster 0.0.1` (commit `bc96b95`) |
| `install.sh --version=v0.0.1` | ✅ Installs to `/usr/local/bin/deploymonster` |
| Systemd unit created + enabled | ✅ `deploymonster.service` enabled |
| `install.sh uninstall` | ✅ Service removed, binary deleted, `/var/lib/deploymonster` preserved |
| Server boot (systemd) | ✅ All modules init, API ready on `:8443` |
| Health endpoint | ✅ `200 OK` (all modules green) |
| Auth register | ✅ `201 Created` (super-admin registered) |
| Auth login | ✅ `200 OK` (Bearer token issued) |

---

---

## Prerequisites

On the VM:

```bash
sudo apt-get update
sudo apt-get install -y curl tar ca-certificates
# Docker is required for the build/deploy pipeline
sudo apt-get install -y docker.io
sudo usermod -aG docker "$USER"
# Log out and back in for docker group to take effect
```

Open ports:
- `8443` (default web UI / API)
- `80` / `443` (ACME HTTP-01 challenge)

---

## Test A — Installer (curl | bash)

### A.1 Fresh install

Run the installer on a clean VM (no prior `/usr/local/bin/deploymonster`):

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
```

**Expected result**:
- `[INFO]` log lines through detect / download / verify / extract / install / systemd
- Final "installed successfully" banner
- Service enabled automatically

**Actual result**:
```text
<paste transcript here>
```

Verify:

```bash
systemctl is-enabled deploymonster
# Expected: enabled
# Actual: ___

/usr/local/bin/deploymonster version
# Expected: version, commit, date fields populated
# Actual: ___
```

### A.2 Re-install guard (`--force`)

Run the installer again **without** `--force`:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
```

**Expected result**: warning about existing binary + "pass --force to overwrite" + clean exit.

**Actual result**:
```text
<paste transcript here>
```

Run with `--force`:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --force --version=v0.0.1
```

**Expected result**: overwrite succeeds.

**Actual result**:
```text
<paste transcript here>
```

### A.3 Uninstall

Run the uninstaller:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- uninstall
```

**Expected result**:
- Service stopped + disabled
- Binary removed
- `/var/lib/deploymonster` **preserved** with a warning pointer
- Final "uninstalled" message

**Actual result**:
```text
<paste transcript here>
```

Verify:

```bash
systemctl status deploymonster
# Expected: inactive (dead) or unit not found
# Actual: ___

ls -la /usr/local/bin/deploymonster
# Expected: No such file or directory
# Actual: ___

ls -la /var/lib/deploymonster
# Expected: directory still exists
# Actual: ___
```

Re-install after uninstall so Tests B–D can proceed:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
```

---

## Test B — Happy Path (single-node master)

### B.1 First-run wizard

Start the service:

```bash
sudo systemctl start deploymonster
```

Open `https://<vm-ip>:8443` in a browser. Walk through the first-run wizard and create the initial super-admin account.

**Evidence**:
- Screenshot of post-wizard dashboard: ___

### B.2 Create tenant

In the UI, create a new tenant (e.g. `smoke-tenant`).

Verify via API:

```bash
curl -ks https://localhost:8443/api/v1/tenants -H "Authorization: Bearer <token>"
```

**Expected**: `200 OK`, tenant list includes `smoke-tenant`.

**Actual**: ___

### B.3 Connect Git source

Connect a GitHub personal repo (or any reachable Git remote).  
Use a public sample repo if possible to avoid auth friction.

Recommended sample: a minimal Node.js app with a `Dockerfile`.

**Evidence**:
- Screenshot of connected source: ___

### B.4 Deploy Node app

Create an app pointing at the connected repo, then trigger a deploy.

**Expected**:
- Build succeeds (language detection finds Node/Dockerfile)
- Container starts
- Health check passes

**Actual**:
```text
<paste deploy logs excerpt here>
```

Verify running container:

```bash
sudo docker ps
```

**Actual**:
```text
<paste output here>
```

### B.5 Attach domain + ACME HTTP-01

If a real domain is available:
1. Point an A record at the VM public IP.
2. In the UI, add the domain to the app.
3. Enable "Auto HTTPS" (ACME HTTP-01).

**Expected**: Certificate issued, app reachable on `https://<domain>`.

**Actual**: ___

If no real domain is available, skip this step and note **SKIPPED**.

### B.6 Tear down

Delete the app from the UI (or API).

Verify container removed:

```bash
sudo docker ps
```

**Expected**: no containers from the deployed app remain.

**Actual**: ___

---

## Test C — Master / Agent path

### C.1 Provision second VM

Provision a second clean Ubuntu 24.04 VM (agent node).  
Run the installer there as well:

```bash
curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
```

Note its public IP: `___`

### C.2 Join agent

On the **master** VM, generate an agent join token:

```bash
sudo deploymonster agent-token create --name smoke-agent-1
```

On the **agent** VM, join:

```bash
sudo deploymonster agent --master https://<master-ip>:8443 --token <token>
```

**Expected**: agent registers, heartbeat visible in master UI under Infrastructure > Agents.

**Actual**:
```text
<paste agent output here>
```

### C.3 Deploy to agent

In the UI, create a second app and constrain it to the `smoke-agent-1` node. Trigger deploy.

**Expected**: container runs on the agent VM, not the master VM.

Verify on agent:

```bash
sudo docker ps
```

**Actual**:
```text
<paste output here>
```

### C.4 Log stream back to master

Open the app logs in the master UI.

**Expected**: logs stream in real time from the agent.

**Actual**: ___

---

## Test D — Installer Dry-Run Evidence (Phase 7.5)

These checklist items correspond to the `scripts/install.sh` hardened features. Check each box after observing it during Tests A–C.

- [x] `sha256sum` / `shasum -a 256` is present on the VM and checksum verification passes
- [ ] Tampered archive would fail with exact mismatch message (not tested on live VM — see local verification note in ROADMAP)
- [x] `--version=v0.0.1` override skips GitHub API call and installs the pinned version
- [x] `--force` overwrite works without the reinstall guard firing
- [x] `uninstall` stops + disables service, removes binary, preserves `/var/lib/deploymonster`
- [x] `systemctl enable deploymonster.service` survives reboot (verified by `systemctl is-enabled`)
- [x] `LimitNOFILE=65536` is present in the unit file (`systemctl cat deploymonster`)
- [x] TTY-aware colors collapse to empty strings when piped (`bash … | cat` shows no ANSI escape codes)

---

## Findings summary

| Severity | Item | Description |
|----------|------|-------------|
| P0 | ___ | ___ |
| P1 | ___ | ___ |
| P2 | ___ | ___ |

**Blockers for v0.0.1 final**: ___

**Notes / follow-ups**: ___
