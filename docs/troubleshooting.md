# Troubleshooting Guide

Common issues and solutions when running DeployMonster.

## Startup Issues

### "config validation error: server.secret_key must be at least 16 characters"

DeployMonster requires a secret key of at least 16 characters for JWT signing.

**Fix:** Set a strong secret key in `monster.yaml` or via environment variable:

```yaml
server:
  secret_key: "your-secret-key-at-least-16-chars"
```

```bash
export MONSTER_SECRET_KEY="your-secret-key-at-least-16-chars"
```

On first run without a config file, a random key is generated automatically.

### "config validation error: server.port N out of range (1-65535)"

The configured port is invalid.

**Fix:** Use a valid port number. Default is `8443`:

```yaml
server:
  port: 8443
```

### "bind: address already in use"

Another process is using the configured port (default 8443, 80, or 443).

**Fix:** Either stop the conflicting process or change ports:

```bash
# Find what's using the port
lsof -i :8443   # Linux/macOS
netstat -tlnp | grep 8443  # Linux

# Or change the port
export MONSTER_PORT=9443
```

### "config error: open monster.yaml: no such file or directory"

DeployMonster can't find the config file.

**Fix:** Either create one with `deploymonster init` or specify a path:

```bash
deploymonster init              # Creates monster.yaml in current dir
deploymonster serve --config /path/to/monster.yaml
```

### "dependency resolution: circular dependency"

A module dependency cycle was detected during startup.

**Fix:** This is a bug — please report it with the full error message at the project's issue tracker.

## Database Issues

### "database is locked"

SQLite concurrent write contention. This is rare with WAL mode but can happen under heavy load.

**Fix:**
- Ensure only one DeployMonster instance is using the database file
- Check for zombie processes: `ps aux | grep deploymonster`
- If the issue persists, stop DeployMonster and remove the `-wal` and `-shm` files (data in the main `.db` is safe)

### Migration errors on startup

If a migration fails, DeployMonster will refuse to start.

**Fix:**
1. Check the error message for the failing migration number
2. Back up the database file
3. Roll back the problematic migration if needed:
   ```bash
   # Programmatic rollback is available via the Rollback(steps) API
   # For manual recovery, restore from backup
   ```

## Docker / Container Issues

### "Cannot connect to the Docker daemon"

DeployMonster can't reach Docker.

**Fix:**
```bash
# Verify Docker is running
docker info

# Check the configured Docker host
export MONSTER_DOCKER_HOST=unix:///var/run/docker.sock  # Linux
export MONSTER_DOCKER_HOST=npipe:////./pipe/docker_engine  # Windows

# Ensure the user has Docker permissions
sudo usermod -aG docker $USER  # Then re-login
```

### Builds stuck or timing out

A build exceeds the configured timeout (default 30 minutes).

**Fix:**
```yaml
limits:
  max_build_minutes: 60  # Increase if builds are legitimately large
```

Also check:
- Network connectivity from the server (git clone needs to reach the repo)
- Disk space: `df -h`
- Docker disk usage: `docker system df`

### Container starts but app is unreachable

The container is running but the ingress proxy can't route to it.

**Fix:**
1. Check container health: the app must listen on the expected port
2. Verify the domain is pointed to the server's IP
3. Check ingress logs for routing errors
4. Verify HTTPS is configured if `enable_https: true`

## Authentication Issues

### "invalid credentials" on login

**Fix:**
- Verify email and password are correct
- Passwords are bcrypt-hashed — there's no way to recover them. Use password reset if available.
- On first run, an admin account is created. Check startup logs for the initial credentials.

### JWT token expired / 401 on every request

Access tokens expire after 15 minutes. The frontend auto-refreshes via httpOnly cookies.

**Fix for API users:**
```bash
# Refresh tokens manually
curl -X POST https://your-server/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "your-refresh-token"}'
```

If refresh also fails, re-authenticate with login.

### CORS errors in browser console

Requests from the browser are blocked by CORS policy.

**Fix:** Configure allowed origins:

```yaml
server:
  domain: deploy.example.com  # Auto-derives CORS origins
```

Or explicitly:
```bash
export MONSTER_CORS_ORIGINS="https://deploy.example.com,https://admin.example.com"
```

## SSL / HTTPS Issues

### ACME certificate issuance fails

Let's Encrypt HTTP-01 challenge can't reach your server.

**Fix:**
1. Ensure port 80 is open and reachable from the internet
2. Ensure the domain DNS points to this server
3. Check that no other service is binding port 80
4. For testing, use staging mode first:
   ```yaml
   acme:
     email: admin@example.com
     staging: true
   ```

### Self-signed certificate warnings

On first startup without ACME, DeployMonster uses a self-signed certificate.

**Fix:** Configure ACME for automatic Let's Encrypt certificates, or provide your own certificate.

## Performance Issues

### High memory usage

**Diagnosis:**
```bash
# Enable pprof for profiling
export MONSTER_ENABLE_PPROF=true

# Then access (requires authentication):
curl -H "Authorization: Bearer $TOKEN" https://localhost:8443/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

### Slow API responses

**Diagnosis:**
1. Check `/metrics/api` for latency percentiles
2. Enable debug logging: `export MONSTER_LOG_LEVEL=debug`
3. Run the load test suite: `make loadtest`
4. Check SQLite performance — consider vacuum: `sqlite3 deploymonster.db "VACUUM;"`

## Agent Mode Issues

### Agent can't connect to master

```
agent error: websocket: bad handshake
```

**Fix:**
- Verify the master URL is correct and reachable
- Verify the join token matches
- Ensure the master is running and the WebSocket endpoint is accessible
- Check firewall rules between agent and master

```bash
# Test connectivity
curl -v https://master-host:8443/health
```

## Logging

### Change log verbosity

```bash
# Environment variable
export MONSTER_LOG_LEVEL=debug    # debug, info, warn, error

# JSON format for log aggregators
export MONSTER_LOG_FORMAT=json

# In monster.yaml
server:
  log_level: debug
  log_format: json
```

### Finding relevant logs

DeployMonster uses structured logging with `log/slog`. Every log entry includes a `module` field:

```
level=INFO msg="deploy completed" module=deploy app_id=abc123 duration=45.2s
level=ERROR msg="build failed" module=build app_id=abc123 error="Dockerfile not found"
```

Filter by module to isolate issues:
```bash
deploymonster 2>&1 | grep 'module=deploy'
```

With JSON format, use `jq`:
```bash
deploymonster 2>&1 | jq 'select(.module == "deploy")'
```
