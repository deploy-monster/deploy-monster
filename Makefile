.PHONY: build dev test lint clean docker fmt vet tidy

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

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(GOBIN)
	$(GOBUILD) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME) ./cmd/deploymonster

## dev: Run in development mode with live reload
dev:
	@echo "Starting development server..."
	$(GOCMD) run $(LDFLAGS) ./cmd/deploymonster

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

## test-short: Run short tests only
test-short:
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

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
	docker build -t deploymonster:$(VERSION) -f deployments/Dockerfile .

## coverage: Generate HTML coverage report
coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## help: Show this help
help:
	@echo "Available targets:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## /  /'
