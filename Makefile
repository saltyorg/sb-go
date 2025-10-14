# Makefile for sb-go project
# Build, test, and development automation

# Variables
BINARY_NAME := sb
BUILD_DIR := build
MODULE := github.com/saltyorg/sb-go
VERSION ?= 0.0.0-dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DISABLE_SELF_UPDATE := true

# Go build flags
CGO_ENABLED := 0
GO_FLAGS := -trimpath
LDFLAGS := -w -s \
	-X '$(MODULE)/internal/runtime.Version=$(VERSION)' \
	-X '$(MODULE)/internal/runtime.GitCommit=$(GIT_COMMIT)' \
	-X '$(MODULE)/internal/runtime.DisableSelfUpdate=$(DISABLE_SELF_UPDATE)'

# Build output
BINARY_PATH := $(BUILD_DIR)/$(BINARY_NAME)

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Default target
.DEFAULT_GOAL := build

# Phony targets (don't produce files with these names)
.PHONY: all build test clean fmt vet lint run help version deps tidy check modernize

##@ General

help: ## Display this help message
	@echo "$(BLUE)sb-go Makefile$(NC)"
	@echo ""
	@echo "$(GREEN)Usage:$(NC)"
	@echo "  make [target]"
	@echo ""
	@echo "$(GREEN)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_-]+:.*?##/ { printf "  $(BLUE)%-15s$(NC) %s\n", $$1, $$2 } /^##@/ { printf "\n$(YELLOW)%s$(NC)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

all: clean fmt vet test build ## Run clean, fmt, vet, test, and build

build: ## Build the binary with development settings
	@echo "$(GREEN)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(GO_FLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY_PATH) .
	@echo "$(GREEN)Build complete: $(BINARY_PATH)$(NC)"
	@ls -lh $(BINARY_PATH)

build-release: ## Build optimized release binary (use VERSION= to override)
	@echo "$(GREEN)Building release binary (version: $(VERSION))...$(NC)"
	@$(MAKE) build VERSION=$(VERSION) DISABLE_SELF_UPDATE=false

##@ Testing

test: ## Run all tests with verbose output
	@echo "$(GREEN)Running tests...$(NC)"
	CGO_ENABLED=$(CGO_ENABLED) go test -v ./...

test-coverage: ## Run tests with coverage report
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go test -v -coverprofile=$(BUILD_DIR)/coverage.out ./...
	@go tool cover -func=$(BUILD_DIR)/coverage.out
	@echo "$(GREEN)Coverage report saved to: $(BUILD_DIR)/coverage.out$(NC)"
	@echo "Run 'go tool cover -html=$(BUILD_DIR)/coverage.out' to view HTML report"

test-race: ## Run tests with race detector
	@echo "$(GREEN)Running tests with race detector...$(NC)"
	go test -race -v ./...

bench: ## Run benchmarks
	@echo "$(GREEN)Running benchmarks...$(NC)"
	go test -bench=. -benchmem ./...

##@ Code Quality

fmt: ## Format Go code
	@echo "$(GREEN)Formatting code...$(NC)"
	go fmt ./...

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	go vet ./...

lint: ## Run linters (staticcheck or golangci-lint if available)
	@echo "$(GREEN)Running linters...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run ./...; \
	elif command -v staticcheck >/dev/null 2>&1; then \
		echo "Running staticcheck..."; \
		staticcheck ./...; \
	else \
		echo "$(YELLOW)No linter found. Install golangci-lint or staticcheck.$(NC)"; \
		echo "  brew install golangci-lint (macOS)"; \
		echo "  go install honnef.co/go/tools/cmd/staticcheck@latest"; \
	fi

check: fmt vet lint ## Run all code quality checks (fmt, vet, lint)

modernize: ## Run Go modernization tool to update code to latest patterns
	@echo "$(GREEN)Running Go modernization tool...$(NC)"
	go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix -test ./...
	@echo "$(GREEN)Modernization complete$(NC)"

##@ Dependencies

deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	go mod download

tidy: ## Tidy and verify dependencies
	@echo "$(GREEN)Tidying dependencies...$(NC)"
	go mod tidy
	go mod verify

##@ Running

run: build ## Build and run the binary
	@echo "$(GREEN)Running $(BINARY_NAME)...$(NC)"
	@$(BINARY_PATH)

version: build ## Display version information
	@echo "$(GREEN)Version information:$(NC)"
	@$(BINARY_PATH) version || echo "Version command not available"

##@ Cleanup

clean: ## Remove build artifacts
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	@rm -f $(BINARY_PATH)
	@echo "$(GREEN)Clean complete$(NC)"

clean-all: clean ## Remove build artifacts and Go cache
	@echo "$(GREEN)Cleaning Go cache...$(NC)"
	go clean -cache -testcache -modcache
	@echo "$(GREEN)Clean complete$(NC)"

##@ Information

info: ## Display build information
	@echo "$(BLUE)Build Configuration:$(NC)"
	@echo "  Binary Name:     $(BINARY_NAME)"
	@echo "  Build Directory: $(BUILD_DIR)"
	@echo "  Module:          $(MODULE)"
	@echo "  Version:         $(VERSION)"
	@echo "  Git Commit:      $(GIT_COMMIT)"
	@echo "  CGO Enabled:     $(CGO_ENABLED)"
	@echo "  Go Version:      $$(go version)"
