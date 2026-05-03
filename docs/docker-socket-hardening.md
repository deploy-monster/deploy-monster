# Docker Socket Hardening

DeployMonster needs the Docker API to manage containers, pull images,
and wire up networks. The default `docker-compose.yml` achieves this by
mounting `/var/run/docker.sock` straight into the container. **That
mount is equivalent to root on the host.** Anyone with code execution
inside the DeployMonster container can start a privileged sibling
container and break out.

For deployments where that blast radius is unacceptable — multi-tenant
hosts, regulated environments, or anywhere the threat model includes
DeployMonster itself being compromised — put the socket behind an
allowlist proxy. This page shows how.

> **`:ro` is not a mitigation.** Binding `/var/run/docker.sock` as
> read-only prevents filesystem writes, but the Docker API is
> bidirectional socket I/O and doesn't care about the mount's ro flag.
> An attacker with access to the mounted socket still gets full
> Docker API. Treat every `docker.sock` mount as read-write.

## The proxy pattern

[Tecnativa's `docker-socket-proxy`][tecnativa] is a small haproxy
container that owns the real socket and exposes a filtered TCP
endpoint. It denies everything by default, including all `POST` and
`DELETE`, and you opt in to the API sections your app actually needs.
DeployMonster's required endpoints are narrow:

| Endpoint | Why DeployMonster needs it |
|----------|----------------------------|
| `/containers/*` | Create, start, stop, remove, restart, logs, inspect, exec, stats |
| `/images/*` | Pull, list, remove |
| `/networks/*` | List, create, remove per-app networks |
| `/volumes/*` | List (for app storage) |
| `/_ping`, `/version`, `/info` | Health check and capability probing |

DeployMonster does **not** use Swarm, Services, Nodes, Tasks, Plugins,
Secrets, Configs, Sessions, or Distribution at the Docker API level as
of v0.1.x — leave those disabled.

[tecnativa]: https://github.com/Tecnativa/docker-socket-proxy

## Example compose

A ready-to-run version of this configuration is committed at
[`deployments/docker-compose.hardened.yaml`](../deployments/docker-compose.hardened.yaml).
Start it with:

```bash
docker compose -f deployments/docker-compose.hardened.yaml up -d
```

The file below is the same config inlined for reference — it replaces
the direct socket mount with a private bridge network on which only
the proxy can reach the real socket.

```yaml
services:
  docker-proxy:
    image: tecnativa/docker-socket-proxy:latest
    container_name: dm-docker-proxy
    restart: unless-stopped
    environment:
      # Allow the endpoints DeployMonster actually uses. All others
      # remain denied. See the table above for the rationale.
      CONTAINERS: 1
      IMAGES: 1
      NETWORKS: 1
      VOLUMES: 1
      INFO: 1
      VERSION: 1
      PING: 1
      # Allow the write verbs for the allow-listed endpoints above.
      # Without POST=1 you get "403 operation not permitted" on every
      # container create / image pull / network create.
      POST: 1
      # DeployMonster removes containers/images/networks on undeploy
      # and when cleaning up failed rollouts.
      DELETE: 1
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - dm-internal
    read_only: true
    tmpfs:
      - /run
    security_opt:
      - no-new-privileges:true
    # Do NOT publish the proxy port to the host. Leave it reachable
    # only on the internal bridge network.

  deploymonster:
    image: ghcr.io/deploy-monster/deploy-monster:latest
    container_name: deploymonster
    restart: unless-stopped
    depends_on:
      - docker-proxy
    ports:
      - "80:80"
      - "443:443"
      - "8443:8443"
    volumes:
      - dm-data:/var/lib/deploymonster
      # Note: no docker.sock mount here — the API comes via the proxy.
    environment:
      - MONSTER_DOMAIN=${MONSTER_DOMAIN:-localhost}
      - MONSTER_ACME_EMAIL=${MONSTER_ACME_EMAIL:-}
      - MONSTER_SECRET=${MONSTER_SECRET:-}
      # Point DeployMonster at the proxy instead of the real socket.
      - MONSTER_DOCKER_HOST=tcp://docker-proxy:2375
    networks:
      - dm-internal
      - default

volumes:
  dm-data:
    driver: local

networks:
  dm-internal:
    driver: bridge
    internal: true  # no outbound route for this network
```

The key fields:

- `MONSTER_DOCKER_HOST=tcp://docker-proxy:2375` — DeployMonster's
  Docker client talks to the proxy's internal address. The same knob
  is available as `docker.host` in `monster.yaml`.
- `networks.dm-internal.internal: true` — the proxy and DeployMonster
  share a private bridge with no outbound route. DeployMonster keeps a
  second network for its own internet access (image pulls happen via
  the proxy, not direct).
- `tecnativa/docker-socket-proxy` is `read_only: true` and drops
  `no-new-privileges` — the proxy itself is a narrow process, so we
  constrain it aggressively.

## Verifying the proxy is working

After `docker compose up -d`:

```bash
# From inside the deploymonster container — should return JSON
docker exec deploymonster wget -qO- http://docker-proxy:2375/_ping
# → OK

# Allowed: listing containers
docker exec deploymonster wget -qO- http://docker-proxy:2375/containers/json
# → [ {...}, {...} ]

# Blocked: asking for swarm info (DeployMonster never calls this)
docker exec deploymonster wget -qO- http://docker-proxy:2375/swarm
# → 403 Forbidden
```

And in the DeployMonster UI: navigate to the Apps tab, deploy a
template (e.g. Nginx), and confirm the container appears. If you see
`403 operation not permitted` in the logs on the first deploy, a
required API section is still denied — check the proxy env vars
against the table above.

## Tradeoffs

**What you give up:**

- One extra container (~10 MB haproxy image, negligible CPU).
- A new failure mode: if the proxy is down, DeployMonster can't talk
  to Docker at all. Mitigate with `restart: unless-stopped` on both
  services and alert on the proxy's health.
- Feature drift risk: if a future DeployMonster release calls a new
  Docker API section (e.g. `/services/*` for Swarm), it will fail
  closed until you flip the corresponding env var. Release notes
  call out any new socket requirements.

**What you gain:**

- A container compromise no longer implies host compromise via Docker
  API. An attacker still has whatever the allowed endpoints can do
  (which is still considerable — they can start a privileged sibling
  container if they can specify one via `/containers/create`). This is
  an attack-surface *reduction*, not elimination.
- Audit visibility: the proxy logs every allowed and denied request,
  so you can answer "what Docker operations did DeployMonster actually
  perform today" without going through Docker daemon logs.

## When *not* to use the proxy

- Single-user dev boxes. The ceremony isn't worth it; use the default
  `docker-compose.yml`.
- Environments where a compromised DeployMonster is already a
  catastrophic loss regardless of socket access (e.g. the box it runs
  on also holds production secrets). The proxy reduces one specific
  breakout path, not the overall impact.

## Further reading

- Tecnativa's [`docker-socket-proxy` README](https://github.com/Tecnativa/docker-socket-proxy#readme)
  lists every `ENV=0/1` toggle and their default values.
- OWASP's [Docker Security Cheat Sheet][owasp-docker] covers the
  general "don't mount the socket" guidance and alternatives.
- `docs/secret-rotation.md` is the other half of operational
  hardening: rotating JWT signing keys after a suspected compromise.

[owasp-docker]: https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html
