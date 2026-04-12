# Architecture Decision Records

This directory captures the major architectural decisions behind DeployMonster,
so contributors and operators understand *why* the codebase looks the way it
does — not just *what* it does.

Each ADR follows the [Michael Nygard template](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions):
Context → Decision → Consequences. They are immutable once accepted; if a
decision is later reversed, write a new ADR that supersedes it instead of
editing the old one.

## Index

| # | Title | Status |
|---|---|---|
| [0001](0001-sqlite-default-database.md) | Use SQLite as the default database | Accepted |
| [0002](0002-modular-monolith.md) | Modular monolith, not microservices | Accepted |
| [0003](0003-no-kubernetes.md) | Target Docker directly, not Kubernetes | Accepted |
| [0004](0004-pure-go-sqlite.md) | Use pure-Go SQLite driver (modernc.org/sqlite) | Accepted |
| [0005](0005-embedded-react-ui.md) | Embed the React UI into the Go binary | Accepted |
| [0006](0006-event-bus-in-process.md) | In-process pub/sub, not a message broker | Accepted |
| [0007](0007-master-agent-same-binary.md) | Master and agent are the same binary | Accepted |
| [0008](0008-encryption-key-strategy.md) | Encryption key strategy (Argon2id + AES-256-GCM vault) | Accepted |
| [0009](0009-store-interface-composition.md) | 12-way Store interface composition for backend portability | Accepted |

## Writing a new ADR

1. Copy the template from ADR 0001 (keep the frontmatter block).
2. Number sequentially — do not reuse numbers even for deleted ADRs.
3. Start in `Proposed` status, then move to `Accepted` when merged.
4. A new ADR that reverses an older one sets the older one's status to
   `Superseded by ADR NNNN` in its own file.
