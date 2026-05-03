# Stage 1: Build React UI
FROM node:22-alpine AS ui-builder
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile --silent
COPY web/ ./
RUN pnpm run build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/web/dist ./web/dist
# Copy UI to embed directory
RUN mkdir -p internal/api/static && cp -r web/dist/* internal/api/static/
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev) -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo none) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /deploymonster \
    ./cmd/deploymonster

# Stage 3: Final minimal image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata curl \
    && addgroup -S monster \
    && adduser -S monster -G monster

COPY --from=go-builder /deploymonster /deploymonster
RUN ln -s /deploymonster /usr/local/bin/deploymonster

# Create data directory
RUN mkdir -p /var/lib/deploymonster && chown monster:monster /var/lib/deploymonster

# Switch to non-root user
USER monster

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD /deploymonster health

EXPOSE 8443 80 443
VOLUME ["/var/lib/deploymonster"]

LABEL org.opencontainers.image.title="DeployMonster" \
      org.opencontainers.image.description="Self-hosted PaaS — Tame Your Deployments" \
      org.opencontainers.image.url="https://deploy.monster" \
      org.opencontainers.image.source="https://github.com/deploy-monster/deploy-monster" \
      org.opencontainers.image.vendor="ECOSTACK TECHNOLOGY OÜ" \
      org.opencontainers.image.licenses="AGPL-3.0"

ENTRYPOINT ["deploymonster"]
