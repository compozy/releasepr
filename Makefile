-include .env
# Makefile for Compozy Go Project

# -----------------------------------------------------------------------------
# Go Parameters & Setup
# -----------------------------------------------------------------------------
GOCMD=$(shell which go)
GOVERSION ?= $(shell awk '/^go /{print $$2}' go.mod 2>/dev/null || echo "1.25")
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOFMT=gofmt -s -w
BINARY_NAME=compozy
BINARY_DIR=bin
SRC_DIRS=./...
LINTCMD=golangci-lint

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
NC := \033[0m # No Color

# -----------------------------------------------------------------------------
# Build Variables
# -----------------------------------------------------------------------------
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION := $(shell git describe --tags --match="v*" --always 2>/dev/null || echo "unknown")

# Build flags for injecting version info (aligned with GoReleaser format)
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X github.com/compozy/releasepr/pkg/version.Version=$(VERSION) -X github.com/compozy/releasepr/pkg/version.CommitHash=$(GIT_COMMIT) -X github.com/compozy/releasepr/pkg/version.BuildDate=$(BUILD_DATE)

# -----------------------------------------------------------------------------
# Swagger/OpenAPI
# -----------------------------------------------------------------------------
SWAGGER_DIR=./docs
SWAGGER_OUTPUT=$(SWAGGER_DIR)/swagger.json

.PHONY: all test lint fmt modernize clean build dev deps schemagen schemagen-watch help integration-test
.PHONY: tidy test-go start-docker stop-docker clean-docker reset-docker
.PHONY: swagger swagger-deps swagger-gen swagger-serve check-go-version setup clean-go-cache

# -----------------------------------------------------------------------------
# Setup & Version Checks
# -----------------------------------------------------------------------------
check-go-version:
	@echo "Checking Go version..."
	@GO_VERSION=$$($(GOCMD) version 2>/dev/null | awk '{print $$3}' | sed 's/go//'); \
	REQUIRED_VERSION=$(GOVERSION); \
	if [ -z "$$GO_VERSION" ]; then \
		echo "$(RED)Error: Go is not available$(NC)"; \
		echo "Please ensure Go $(GOVERSION) is installed via mise"; \
		exit 1; \
	elif [ "$$(printf '%s\n' "$$REQUIRED_VERSION" "$$GO_VERSION" | sort -V | head -n1)" != "$$REQUIRED_VERSION" ]; then \
		echo "$(YELLOW)Warning: Go version $$GO_VERSION found, but $(GOVERSION) is required$(NC)"; \
		echo "Please update Go to version $(GOVERSION) with: mise use go@$(GOVERSION)"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ Go version $$GO_VERSION is compatible$(NC)"; \
	fi

setup: check-go-version deps
	@echo "$(GREEN)✓ Setup complete! You can now run 'make build' or 'make dev'$(NC)"

# -----------------------------------------------------------------------------
# Main Targets
# -----------------------------------------------------------------------------
all: swagger test lint fmt

clean:
	rm -rf $(BINARY_DIR)/
	rm -rf $(SWAGGER_DIR)/
	$(GOCMD) clean

build: check-go-version swagger
	mkdir -p $(BINARY_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/$(BINARY_NAME) ./
	chmod +x $(BINARY_DIR)/$(BINARY_NAME)

# -----------------------------------------------------------------------------
# Code Quality & Formatting
# -----------------------------------------------------------------------------
lint:
	$(LINTCMD) run --fix --allow-parallel-runners
	@echo "Running static driver import guard..."
	@./scripts/check-driver-imports.sh
	@echo "Scanning docs for legacy tool references... (make scan-docs to enforce)"
	@echo "Running modernize analyzer for min/max suggestions..."
	@echo "Linting completed successfully"

fmt:
	@echo "Formatting code..."
	$(LINTCMD) fmt
	@echo "Formatting completed successfully"

modernize:
	$(GOCMD) run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix ./...

# -----------------------------------------------------------------------------
# Development & Dependencies
# -----------------------------------------------------------------------------

dev: EXAMPLE=weather
dev:
	gow run . dev --cwd examples/$(EXAMPLE) --env-file .env --debug --watch

tidy:
	@echo "Tidying modules..."
	$(GOCMD) mod tidy

deps: check-go-version clean-go-cache swagger-deps
	@echo "Installing Go dependencies..."
	@echo "Installing gotestsum..."
	@$(GOCMD) install gotest.tools/gotestsum@latest
	@echo "Installing gow for hot reload..."
	@$(GOCMD) install github.com/mitranim/gow@latest
	@echo "Installing goose for migrations..."
	@$(GOCMD) install github.com/pressly/goose/v3/cmd/goose@latest
	@echo "Installing golangci-lint v2..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$($(GOCMD) env GOPATH)/bin v2.4.0
	@echo "$(GREEN)✓ All dependencies installed successfully$(NC)"

clean-go-cache:
	@echo "Cleaning Go build cache for fresh setup..."
	@$(GOCMD) clean -cache -testcache -modcache 2>/dev/null || true
	@echo "$(GREEN)✓ Go cache cleaned$(NC)"

# -----------------------------------------------------------------------------
# Release Management
# -----------------------------------------------------------------------------
.PHONY: compozy-release

# Build the compozy-release binary
compozy-release:
	mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(BINARY_DIR)/compozy-release .

# -----------------------------------------------------------------------------
# Testing
# -----------------------------------------------------------------------------

test:
	@gotestsum --format pkgname  -- -race -parallel=4 ./...

test-coverage:
	@gotestsum --format pkgname -- -race -parallel=4 -coverprofile=coverage.out -covermode=atomic ./...

test-nocache:
	@gotestsum --format pkgname -- -race -count=1 -parallel=4 ./...
