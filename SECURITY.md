# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.2.x   | Yes                |
| 0.1.x   | Security fixes only|
| < 0.1   | No                 |

## Reporting a Vulnerability

**Do NOT open a public issue for security vulnerabilities.**

Instead, please report them privately:

1. **Email**: security@ecostack.dev
2. **Subject**: `[DeployMonster Security] <brief description>`
3. **Include**:
   - Description of the vulnerability
   - Steps to reproduce
   - Impact assessment
   - Suggested fix (if any)

We will acknowledge your report within **48 hours** and provide a detailed response within **5 business days**.

## Security Measures

DeployMonster implements the following security measures:

### Authentication & Authorization
- JWT tokens with configurable expiry
- bcrypt password hashing (cost 12)
- RBAC with 6 built-in roles
- API key authentication (SHA-256 hashed)
- 2FA via TOTP
- SSO via OAuth (Google, GitHub)

### Encryption
- AES-256-GCM for secret vault
- Argon2id for key derivation
- TLS 1.2+ for all HTTPS traffic
- HMAC-SHA256 for webhook signatures

### API Security
- Rate limiting (per-IP and per-tenant)
- Request body size limiting (10MB default)
- Request timeout (30s default)
- CORS with configurable allowed origins
- Security headers (X-Frame-Options, X-Content-Type-Options, etc.)
- Audit logging for all state-changing operations
- Request ID tracing

### Infrastructure
- Non-root Docker container
- Read-only filesystem (except data volume)
- No shell in production image
- Health check endpoint
- Graceful shutdown with connection draining

## Responsible Disclosure

We follow responsible disclosure practices. If you discover a vulnerability:

1. Report it privately (see above)
2. Allow us reasonable time to fix it before public disclosure
3. We will credit you in the security advisory (unless you prefer anonymity)

## Bug Bounty

We do not currently have a bug bounty program, but we deeply appreciate security researchers who report vulnerabilities responsibly.
