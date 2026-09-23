.PHONY: check build format vet lint testplacement test testv testvv generate fix install uninstall help

# Tool versions - must match go.mod exactly
GOLANGCI_LINT_VERSION := $(shell go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2)
GOFUMPT_VERSION := $(shell go list -m -f '{{.Version}}' mvdan.cc/gofumpt)
GOTESTSUM_VERSION := $(shell go list -m -f '{{.Version}}' gotest.tools/gotestsum)
GOBIN := $(shell go env GOBIN)
GOPATH := $(shell go env GOPATH)
GO_BIN_DIR := $(if $(GOBIN),$(GOBIN),$(GOPATH)/bin)

GOLANGCI_LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOFUMPT_PKG := mvdan.cc/gofumpt
GOTESTSUM_PKG := gotest.tools/gotestsum

INSTALL_BINS := golangci-lint gofumpt gotestsum gg
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

# run_tool_in runs a resolved tool from another directory. Neither tool it
# serves has a flag of its own for this: golangci-lint discovers its
# configuration by walking up from the working directory, so an example is
# only linted against its own .golangci.yml when the tool actually runs inside
# the example -- which is also what gg lint does in a generated project -- and
# gotestsum tests the module of the directory it runs in.
define run_tool_in
	@tool="$$(command -v $(1) 2>/dev/null || printf '%s' "$(GO_BIN_DIR)/$(1)")"; \
		echo "$(1) $(3) ($(2))"; \
		cd $(2) && "$$tool" $(3)
endef

# Default target
help:
	@echo "Available commands:"
	@echo "  check          - Run all code quality checks"
	@echo "  build          - Build the project"
	@echo "  format         - Format code with gofumpt"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run golangci-lint (includes modernize, nilness and shadow)"
	@echo "  testplacement  - Check that every test file named as an internal test has to be one"
	@echo "  test           - Run unit tests, a line per package"
	@echo "  testv          - Run unit tests, a line per test"
	@echo "  testvv         - Run unit tests with the full go test -v output"
	@echo "  generate       - Regenerate the framework's own generated sources"
	@echo "  fix            - Auto-fix code issues (gofumpt, golangci-lint)"
	@echo "  install        - Install gg command and development tools"
	@echo "  uninstall      - Uninstall gg command and development tools"
	@echo "  help           - Show this help message"

# Run all code quality checks
# Order matches make install tool installation order
check: build lint testplacement format vet
	@echo "All checks passed successfully!"

# Build the project and the example modules: each example is a module of its
# own, which go build ./... does not reach into, and an API change that breaks
# one must fail the check rather than the next person to open the example.
build:
	@echo "Running go build..."
	go build ./...
	go -C examples/demo build ./...
	go -C examples/cluster build ./...
	go -C examples/bench build ./...

format:
	$(call install_tool_if_missing,gofumpt,$(GOFUMPT_VERSION),$(GOFUMPT_PKG))
	@echo "Running gofumpt..."
	$(call run_tool,gofumpt,-l -w .)

# Run go vet, on the example modules too (see build)
vet:
	@echo "Running go vet..."
	go vet ./...
	go -C examples/demo vet ./...
	go -C examples/cluster vet ./...
	go -C examples/bench vet ./...

# Run golangci-lint (modernize, nilness and shadow run inside it, see .golangci.yml)
# The example modules are linted too (see build), each against the .golangci.yml
# gg new writes into a project, so the rules a generated project is held to are
# the rules the examples demonstrate.
lint:
	$(call install_tool_if_missing,golangci-lint,$(GOLANGCI_LINT_VERSION),$(GOLANGCI_LINT_PKG))
	@echo "Running golangci-lint..."
	$(call run_tool,golangci-lint,run ./...)
	$(call run_tool_in,golangci-lint,examples/demo,run ./...)
	$(call run_tool_in,golangci-lint,examples/cluster,run ./...)
	$(call run_tool_in,golangci-lint,examples/bench,run ./...)

# Check that every test file named as an internal test has to be one (see
# internal/testplacement): testpackage in golangci-lint makes a test file that
# joins its package carry the _internal_test.go suffix, and this check makes
# the suffix true.
testplacement:
	@echo "Running the test placement check..."
	go run ./internal/testplacement/cmd/testplacementcheck

# Run unit tests
# Every package is tested, so a newly added package is covered without editing
# this file. Tests bring up whatever they need in containers, so a container
# runtime is the only thing the machine has to provide.
# DIALECT_PACKAGES must behave the same on every supported dialect -- the
# database package, and the capabilities built on its leases -- so their
# suites run once per dialect: the full run already covers MySQL (their
# TestMain default), and the remaining dialects repeat them under the build
# tag naming the dialect, which only internal/testutil reads -- the public
# testutil knows nothing about it. A tag makes each dialect a test binary of
# its own, so go's test cache keeps a result per dialect and replays each one
# only while nothing its suite depends on has changed (see
# testutil.DatabaseUnderTest).
# Every run carries the race detector, through TEST_FLAGS. The framework runs
# the concurrency a project relies on -- renewal goroutines, campaign loops,
# scheduler loops, component and lifecycle startup -- and a data race there
# surfaces as a failure somewhere else entirely, which review does not catch.
# One package is built without the instrumentation:
# golang.org/x/crypto/blowfish, the cipher under bcrypt. bcrypt is slow by
# design, instrumenting its key schedule makes every password hash and check
# ten times slower on top, and that is where the suites signing accounts in
# spent most of their time. The cipher shares nothing between calls, so there
# is no race of the framework's to find inside it.
# The examples are modules of their own and go test ./... does not reach into
# them: examples/demo is what keeps the public testutil contract honest, using
# the suite the way a project generated by gg does, and examples/cluster
# exercises the multi-replica capabilities the way a deployment does.
# Every run goes through gotestsum, which reads the JSON go test writes. test
# prints a line per package and none for a package without tests, and ends
# with its totals and the output of the tests that failed, leaving the
# expected skips to their count. A machine without gotestsum gets the version
# go.mod pins installed on the first run.
DIALECT_PACKAGES := ./database/... ./internal/lease/... ./cronjob/... ./leader/... ./lock/...
TEST_FLAGS := -race -gcflags=golang.org/x/crypto/blowfish=-race=false
TEST_OUTPUT := --format pkgname --format-hide-empty-pkg --hide-summary=skipped

test:
	$(call install_tool_if_missing,gotestsum,$(GOTESTSUM_VERSION),$(GOTESTSUM_PKG))
	@echo "Running unit tests (the per-dialect suites run against mysql here)..."
	$(call run_tool,gotestsum,$(TEST_OUTPUT) -- $(TEST_FLAGS) ./...)
	@echo "Running the per-dialect suites against postgres..."
	$(call run_tool,gotestsum,$(TEST_OUTPUT) -- $(TEST_FLAGS) -tags gsttest_postgres $(DIALECT_PACKAGES))
	@echo "Running the per-dialect suites against sqlite..."
	$(call run_tool,gotestsum,$(TEST_OUTPUT) -- $(TEST_FLAGS) -tags gsttest_sqlite $(DIALECT_PACKAGES))
	@echo "Running example project tests..."
	$(call run_tool_in,gotestsum,examples/demo,$(TEST_OUTPUT) -- $(TEST_FLAGS) ./...)
	$(call run_tool_in,gotestsum,examples/cluster,$(TEST_OUTPUT) -- $(TEST_FLAGS) ./...)

# Run unit tests with more output: testv and testvv run what test runs, with
# the output switched to a line per test, or to what go test -v prints.
testv: TEST_OUTPUT := --format testname --format-hide-empty-pkg --hide-summary=skipped
testv: test

testvv: TEST_OUTPUT := --format standard-verbose
testvv: test

# Regenerate the framework's own generated sources.
# The framework registers its model doc comments at build time, the same way a
# generated project registers its own, so nothing parses Go sources at runtime.
# A forgotten run is caught by the test suite, not by review.
generate:
	@echo "Regenerating framework sources..."
	go run ./internal/codegen/cmd/apidocgen

# Auto-fix code issues
# The example modules are fixed too (see lint): golangci-lint reaches only the
# module it runs in, so each example is fixed from inside it, against its own
# .golangci.yml. gofumpt walks paths rather than modules, so the run at the
# root already covers them.
fix:
	@echo "Running auto-fix tools..."
	$(call install_tool_if_missing,gofumpt,$(GOFUMPT_VERSION),$(GOFUMPT_PKG))
	@echo "Running gofumpt..."
	$(call run_tool,gofumpt,-l -w .)
	$(call install_tool_if_missing,golangci-lint,$(GOLANGCI_LINT_VERSION),$(GOLANGCI_LINT_PKG))
	@echo "Running golangci-lint --fix..."
	$(call run_tool,golangci-lint,run --fix ./...)
	$(call run_tool_in,golangci-lint,examples/demo,run --fix ./...)
	$(call run_tool_in,golangci-lint,examples/cluster,run --fix ./...)
	$(call run_tool_in,golangci-lint,examples/bench,run --fix ./...)
	@echo "All auto-fix operations completed!"

# Install gg command and development tools
# Versions are defined at the top of this file and must match go.mod exactly
install:
	@echo "Installing development tools from go.mod..."
	@echo "Installing golangci-lint@$(GOLANGCI_LINT_VERSION)..."
	@go install $(GOLANGCI_LINT_PKG)@$(GOLANGCI_LINT_VERSION)
	@echo "Installing gofumpt@$(GOFUMPT_VERSION)..."
	@go install $(GOFUMPT_PKG)@$(GOFUMPT_VERSION)
	@echo "Installing gotestsum@$(GOTESTSUM_VERSION)..."
	@go install $(GOTESTSUM_PKG)@$(GOTESTSUM_VERSION)
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
