.PHONY: build dev test lint clean docker docker-compose fmt vet tidy bench coverage release install help test-e2e

# Variables
BINARY_NAME := deploymonster
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Go
GOBIN := bin
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := gofmt

# Platforms
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

## build: Build the binary for current platform
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(GOBIN)
	$(GOBUILD) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME) ./cmd/deploymonster

## build-all: Build for all platforms (linux, darwin, windows)
build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(GOBIN)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "  → $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) \
			-o $(GOBIN)/$(BINARY_NAME)-$$os-$$arch$$ext ./cmd/deploymonster; \
	done
	@echo "Done. Binaries in $(GOBIN)/"

## dev: Run in development mode
dev:
	@echo "Starting development server..."
	$(GOCMD) run $(LDFLAGS) ./cmd/deploymonster

## test: Run all tests with race detection and coverage
test:
	@echo "Running tests..."
	$(GOTEST) -race -coverprofile=coverage.out ./...

## test-short: Run short tests only
test-short:
	@echo "Running short tests..."
	$(GOTEST) -short ./...

## test-cover: Run tests and show per-package coverage
test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -cover ./... | sort -t':' -k2 -n

## test-integration: Run integration tests (requires Docker)
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -tags integration -v ./...

## test-integration-postgres: Run Postgres store integration test (requires TEST_POSTGRES_DSN)
test-integration-postgres:
	@echo "Running Postgres integration test (TEST_POSTGRES_DSN=$${TEST_POSTGRES_DSN:-unset})..."
	$(GOTEST) -tags pgintegration -run TestPostgresIntegration -v ./internal/db/...

## bench: Run all benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## test-e2e: Run Playwright end-to-end tests (requires running server)
test-e2e:
	@echo "Running Playwright E2E tests (ensure server is running on :8443)..."
	cd web && pnpm test:e2e

## loadtest: Run HTTP load test against a running instance
loadtest:
	@echo "Running load test (ensure server is running)..."
	go run ./tests/loadtest -url http://localhost:8443 -duration 10s -concurrency 10

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

## vet: Run go vet
vet:
	@echo "Running vet..."
	$(GOVET) ./...

## tidy: Tidy go modules
tidy:
	@echo "Tidying modules..."
	$(GOCMD) mod tidy

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(GOBIN) coverage.out coverage.html dist/

## docker: Build Docker image
docker:
	@echo "Building Docker image..."
	docker build -t deploymonster:$(VERSION) -t deploymonster:latest \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .

## docker-compose: Start with docker-compose
docker-compose:
	docker compose up -d

## coverage: Generate HTML coverage report
coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in your browser"

## release: Create a release with goreleaser
release:
	@echo "Creating release..."
	goreleaser release --clean

## release-snapshot: Test release without publishing
release-snapshot:
	@echo "Creating snapshot release..."
	goreleaser release --snapshot --clean

## install: Install binary to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install $(LDFLAGS) ./cmd/deploymonster

## check: Run all checks (vet, test, build)
check: vet test build
	@echo "All checks passed!"

## stats: Show project statistics
stats:
	@echo "=== DeployMonster Stats ==="
	@echo "Version:    $(VERSION)"
	@echo "Go files:   $$(find internal cmd -name '*.go' | wc -l)"
	@echo "Go LOC:     $$(find internal cmd -name '*.go' | xargs wc -l 2>/dev/null | tail -1 | awk '{print $$1}')"
	@echo "Test files: $$(find internal -name '*_test.go' | wc -l)"
	@echo "Endpoints:  $$(grep -c 'r.mux.Handle' internal/api/router.go)"
	@echo "Modules:    $$(grep -r 'core.RegisterModule' internal/ --include='*.go' -l | wc -l)"
	@echo "Binary:     $$(ls -lh $(GOBIN)/$(BINARY_NAME)* 2>/dev/null | awk '{print $$5}' | head -1)"

## help: Show this help
help:
	@echo "DeployMonster — Available targets:"
	@echo ""
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## /  /'
