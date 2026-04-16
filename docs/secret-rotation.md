# JWT Secret Rotation

This runbook covers rotating the JWT signing secret used for access and
refresh tokens. It applies to every DeployMonster deployment — `master`,
`agent`, single-node, and HA.

> **When to rotate.** After a suspected or confirmed secret compromise,
> when a privileged engineer leaves the team, or on a periodic hygiene
> cadence (recommended: every 90 days). There is no online way to
> invalidate every issued token short of rotating the secret, so treat
> rotation as the canonical "revoke everything" lever.

## How rotation works

DeployMonster signs every token with one **active** secret and accepts
tokens signed by any key in a short-lived **previous-keys** list. The
implementation is in `internal/auth/jwt.go`:

- `server.secret_key` — the active key. Every new token is signed with
  this value.
- `server.previous_secret_keys` — zero or more recently-rotated keys.
  Tokens signed with any of these still validate, but only for
  `RotationGracePeriod = 20 * time.Minute` from when the server picked
  them up. After the grace period expires, the server purges them from
  memory on the next validation; a config reload would reinstate them
  for another 20 minutes, which is usually not what you want.
- Access tokens live 15 minutes; refresh tokens live 7 days. The 20-min
  grace covers "access token issued at `T-0` just before rotation"
  plus clock skew. Refresh tokens span the grace window, so a client
  whose refresh token was minted under the old key must refresh within
  20 minutes of cutover or re-authenticate.

Key enforcement constraints (`internal/auth/jwt.go`):

- Minimum secret length is **32 characters** (256 bits). Anything
  shorter panics on startup. Generate entropy from `/dev/urandom` or
  `openssl rand -hex 32`.
- HS256 is enforced both at parse time (`WithValidMethods`) and
  belt-and-suspenders after parse. `alg=none` and alg-confusion attacks
  fail closed.
- Token lookup tries the active key first, then every non-expired
  previous key in registration order.

## Rotation procedure

### 1. Generate a new secret

```bash
openssl rand -hex 32
# e.g. d3b07384d113edec49eaa6238ad5ff00a8b7f0ac35f6a5de7aa58c2f63b3b8d2
```

Store the value in your secret manager. Never commit it to git.

### 2. Move the current active key into the previous list

In `monster.yaml`:

```yaml
server:
  secret_key: <NEW-SECRET-FROM-STEP-1>
  previous_secret_keys:
    - <OLD-SECRET-BEING-ROTATED-OUT>
    # keep any other still-in-grace keys below
```

Or via environment:

```bash
export MONSTER_SECRET=<NEW-SECRET-FROM-STEP-1>
export MONSTER_PREVIOUS_SECRET_KEYS=<OLD-SECRET-BEING-ROTATED-OUT>
# comma-separate to carry multiple previous keys:
# MONSTER_PREVIOUS_SECRET_KEYS=key1,key2
```

### 3. Reload or restart every server

On a single node: restart the `deploymonster` process. The new key is
loaded at startup; every new token is signed with it, and tokens issued
in the last 20 minutes still validate against the previous key.

On a multi-node deployment (`master` + `agent` nodes): restart each
node one at a time. The signing secret is master-side only — agents
don't sign tokens — but if you share `monster.yaml` across nodes,
keep the configs in sync to avoid drift when an agent is promoted.

### 4. Wait out the grace window

After **20 minutes + access-token TTL (15 min) = ~35 minutes**, every
still-valid token in the wild has been re-signed with the new key.
Clients that refresh during the window swap to the new key
transparently. Clients that never touch the API for 35+ minutes will
fail authentication and must log in again.

### 5. Remove the rotated-out key on the next config edit

```yaml
server:
  secret_key: <NEW-SECRET>
  previous_secret_keys: []  # or drop the list entirely
```

Leaving the old key in `previous_secret_keys` past the grace window is
harmless — the server purges expired entries on every token validation
— but the YAML still exposes the old secret to anyone who reads the
config, which defeats the point of rotation. Clean it up.

## Emergency rotation (compromise response)

When a secret is confirmed compromised, the grace period is a
liability, not a feature: a stolen token is still accepted for 20
minutes. Collapse that window:

1. Generate the new secret (step 1 above).
2. Set `server.secret_key` to the new value and leave
   `server.previous_secret_keys` **empty**. Do not carry the
   compromised key forward.
3. Restart the server. Every issued token becomes invalid immediately
   — all users see 401 on their next request and must log in again.
4. Audit the `audit_log` table for activity during the compromise
   window. The `UserID` + `TenantID` columns are indexed; filter by
   time and any principal that was active.
5. Post a user-visible notice (status page, in-app banner) explaining
   the forced logout so support doesn't drown in tickets.

Expect this to log every user out at once. That is the point.

## Staging rehearsal

Rotate on staging before production. Verify:

- The process comes back up cleanly with the new config.
- A token issued **just before** the restart still works for a few
  minutes after (proof the previous-keys list is wired).
- A token issued 25+ minutes before the restart is rejected (proof the
  grace period is enforced).
- A token issued after the restart works with the new key (proof
  signing switched over).

A useful smoke script:

```bash
# Before rotation — capture a token
TOK=$(curl -s -X POST http://staging.example:8443/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"..."}' \
  | jq -r .access_token)

# Rotate monster.yaml, restart the server, then:
# Within 20 min: should return 200
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $TOK" \
  http://staging.example:8443/api/v1/me

# After 20+ min: should return 401
```

## Related

- `internal/auth/jwt.go` — `NewJWTService`, `AddPreviousKey`,
  `RotationGracePeriod`, `ValidateAccessToken`, `ValidateRefreshToken`.
- `internal/core/config.go` — `Server.SecretKey`,
  `Server.PreviousSecretKeys`, `MONSTER_SECRET`,
  `MONSTER_PREVIOUS_SECRET_KEYS`.
- `docs/upgrade-guide.md` — general upgrade/backup guidance; run
  rotations during low-traffic windows the same way.
