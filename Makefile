.PHONY: check build format vet lint test testv fix install uninstall help

# Tool versions - must match go.mod exactly
GOLANGCI_LINT_VERSION := $(shell go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2)
GOFUMPT_VERSION := $(shell go list -m -f '{{.Version}}' mvdan.cc/gofumpt)
GOBIN := $(shell go env GOBIN)
GOPATH := $(shell go env GOPATH)
GO_BIN_DIR := $(if $(GOBIN),$(GOBIN),$(GOPATH)/bin)

GOLANGCI_LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOFUMPT_PKG := mvdan.cc/gofumpt

INSTALL_BINS := golangci-lint gofumpt gg
# install_tool_if_missing installs a Makefile-managed tool only when it is unavailable.
define install_tool_if_missing
	@if ! command -v $(1) >/dev/null 2>&1 && [ ! -x "$(GO_BIN_DIR)/$(1)" ]; then \
		echo "Installing $(1)@$(2)..."; \
		mkdir -p "$(GO_BIN_DIR)"; \
		go install $(3)@$(2); \
	fi
endef

# run_tool resolves tools installed during the current make invocation before running them.
define run_tool
	@tool="$$(command -v $(1) 2>/dev/null || printf '%s' "$(GO_BIN_DIR)/$(1)")"; \
		echo "$(1) $(2)"; \
		"$$tool" $(2)
endef

# Default target
help:
	@echo "Available commands:"
	@echo "  check          - Run all code quality checks"
	@echo "  build          - Build the project"
	@echo "  format         - Format code with gofumpt"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run golangci-lint (includes modernize, nilness and shadow)"
	@echo "  test           - Run unit tests (simple output)"
	@echo "  testv          - Run unit tests with verbose output"
	@echo "  fix            - Auto-fix code issues (gofumpt, golangci-lint)"
	@echo "  install        - Install gg command and development tools"
	@echo "  uninstall      - Uninstall gg command and development tools"
	@echo "  help           - Show this help message"

# Run all code quality checks
# Order matches make install tool installation order
check: build lint format vet
	@echo "All checks passed successfully!"

# Build the project
build:
	@echo "Running go build..."
	go build ./...

format:
	$(call install_tool_if_missing,gofumpt,$(GOFUMPT_VERSION),$(GOFUMPT_PKG))
	@echo "Running gofumpt..."
	$(call run_tool,gofumpt,-l -w .)

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run golangci-lint (modernize, nilness and shadow run inside it, see .golangci.yml)
lint:
	$(call install_tool_if_missing,golangci-lint,$(GOLANGCI_LINT_VERSION),$(GOLANGCI_LINT_PKG))
	@echo "Running golangci-lint..."
	$(call run_tool,golangci-lint,run ./...)

# Run unit tests
test:
	@echo "Running unit tests..."
	go test ./model/...
	go test ./service
	go test ./util/...
	go test ./dsl
	go test ./client
	go test ./database/...
	go test ./ds/...
	go test ./internal/codegen/gen/
	go test ./internal/ggmodule
	go test ./internal/model/...
	go test ./internal/service/...
	go test ./internal/modelregistry/...
	go test ./internal/serviceregistry/...
	go test ./module/...
	go test ./dbmigrate

# Run unit tests with verbose output
testv:
	@echo "Running unit tests with verbose output..."
	go test -v ./model/...
	go test -v ./service
	go test -v ./util/...
	go test -v ./dsl
	go test -v ./client
	go test -v ./database/...
	go test -v ./ds/...
	go test -v ./internal/codegen/gen/
	go test -v ./internal/ggmodule
	go test -v ./internal/model/...
	go test -v ./internal/service/...
	go test -v ./internal/modelregistry/...
	go test -v ./internal/serviceregistry/...
	go test -v ./module/...
	go test -v ./dbmigrate

# Auto-fix code issues
fix:
	@echo "Running auto-fix tools..."
	@echo "Running gofumpt..."
	gofumpt -l -w .
	@echo "Running golangci-lint --fix..."
	golangci-lint run --fix ./...
	@echo "All auto-fix operations completed!"

# Install gg command and development tools
# Versions are defined at the top of this file and must match go.mod exactly
install:
	@echo "Installing development tools from go.mod..."
	@echo "Installing golangci-lint@$(GOLANGCI_LINT_VERSION)..."
	@go install $(GOLANGCI_LINT_PKG)@$(GOLANGCI_LINT_VERSION)
	@echo "Installing gofumpt@$(GOFUMPT_VERSION)..."
	@go install $(GOFUMPT_PKG)@$(GOFUMPT_VERSION)
	@echo "Installing gg command..."
	@go install ./cmd/gg
	@echo "Installation completed!"

# Uninstall gg command and development tools
uninstall:
	@echo "Uninstalling development tools from $(GO_BIN_DIR)..."
	@for bin in $(INSTALL_BINS); do \
		path="$(GO_BIN_DIR)/$$bin"; \
		if [ -e "$$path" ]; then \
			rm -f "$$path"; \
			echo "Removed $$path"; \
		else \
			echo "Skipped $$path (not installed)"; \
		fi; \
	done
	@echo "Uninstallation completed!"
