# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /deploymonster ./cmd/deploymonster

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    && addgroup -S monster \
    && adduser -S monster -G monster

COPY --from=builder /deploymonster /usr/local/bin/deploymonster

# Data directory
RUN mkdir -p /var/lib/deploymonster && chown monster:monster /var/lib/deploymonster
VOLUME /var/lib/deploymonster

# Switch to non-root user
USER monster

# Ports: API, HTTP, HTTPS
EXPOSE 8443 80 443

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f -k https://localhost:8443/api/v1/health || exit 1

ENTRYPOINT ["deploymonster"]
CMD ["serve"]
