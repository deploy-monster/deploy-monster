# Stage 1: Build React UI
FROM node:22-alpine AS ui-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci --silent
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.23-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev) -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo none) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /deploymonster \
    ./cmd/deploymonster

# Stage 3: Final minimal image
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-builder /deploymonster /usr/local/bin/deploymonster

# Create data directory
RUN mkdir -p /var/lib/deploymonster

EXPOSE 8443 80 443
VOLUME ["/var/lib/deploymonster"]
ENTRYPOINT ["deploymonster"]
