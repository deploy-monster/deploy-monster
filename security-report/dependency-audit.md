# Dependency Audit — DeployMonster

## Go Dependencies (go.mod)

### Direct Dependencies
| Package | Version | Type | Notes |
|---------|---------|------|-------|
| github.com/golang-jwt/jwt/v5 | v5.3.1 | JWT | HS256, stable API |
| github.com/gorilla/websocket | v1.5.3 | WebSocket | - |
| github.com/docker/docker | v28.5.2+incompatible | Docker client | - |
| github.com/jackc/pgx/v5 | v5.9.1 | PostgreSQL driver | for future Postgres support |
| go.etcd.io/bbolt | v1.4.3 | KV store | - |
| golang.org/x/crypto | v0.49.0 | bcrypt, crypto | - |
| modernc.org/sqlite | v1.48.0 | Pure-Go SQLite | - |
| gopkg.in/yaml.v3 | v3.0.1 | Config parsing | - |

### Indirect Dependencies (Notable)
| Package | Version | Risk | Notes |
|---------|---------|------|-------|
| golang.org/x/net | v0.52.0 | Low | std net |
| golang.org/x/sys | v0.42.0 | Low | OS abstraction |
| golang.org/x/text | v0.35.0 | Low | Unicode |
| github.com/google/uuid | v1.6.0 | Low | UUID generation |
| go.opentelemetry.io/* | 1.43.0 | Medium | Telemetry SDK |
| github.com/docker/docker | 28.5.2 | See below | Docker client |

## Node Dependencies (web/package.json)

### Production
| Package | Version | Risk | Notes |
|---------|---------|------|-------|
| react | 19.2.4 | Low | - |
| react-router | 7.13.2 | Low | - |
| zustand | 5.0.12 | Low | State management |
| @xyflow/react | 12.10.2 | Low | Topology canvas |
| tailwindcss | 4.2.2 | Low | - |
| lucide-react | 1.7.0 | Low | Icons |

### Development
| Package | Version | Risk | Notes |
|---------|---------|------|-------|
| @playwright/test | 1.59.1 | Low | E2E testing |
| vite | 8.0.5 | Low | Build tool |
| typescript | 5.9.3 | Low | - |

## Supply Chain Assessment

1. **All direct Go deps are reputable** — golang-jwt (JWT), gorilla (WebSocket), docker (Docker), bbolt (KV), crypto (bcrypt)
2. **No transitive dep on unknown/unofficial packages** — all come from established orgs
3. **Node deps are all mainstream** — React, Vite, Tailwind, Zustand, React Router
4. **No known vulnerable versions detected** — checked against CISA known exploit catalog
5. **Go version pinning** — go 1.26.1 with toolchain go1.26.2 explicit
6. **No internal dependencies that could be compromised**

## Risks

- `modernc.org/sqlite` — pure Go implementation, no C binding risk
- `github.com/docker/docker` — Docker client (incompatible tag) is widespread, keep Docker host secured
- OTel packages — adds observability, no known CVEs at current versions
