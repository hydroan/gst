.PHONY: check build format vet lint selfcheck ggcheck test test-short testv testvv generate fix install uninstall help testcachefix

# Tool versions - must match go.mod exactly
GOLANGCI_LINT_VERSION := $(shell go list -m -f '{{.Version}}' github.com/golangci/golangci-lint/v2)
GOFUMPT_VERSION := $(shell go list -m -f '{{.Version}}' mvdan.cc/gofumpt)
GOTESTSUM_VERSION := $(shell go list -m -f '{{.Version}}' gotest.tools/gotestsum)
GOBIN := $(shell go env GOBIN)
# GOPATH is pinned for every go command make runs, the way TEST_PATH pins PATH
# for the tests: gvm keeps a module cache per Go version beside its GOROOT, and
# a shell started without gvm's GOPATH would build the same packages into a
# second cache under ~/go. That doubles the build cache, and it splits the test
# result cache between two environments, one of which then relinks every test
# binary on every run. A GOROOT outside gvm keeps the GOPATH the environment
# provides.
GVM_GOPATH := $(subst /gos/,/pkgsets/,$(shell go env GOROOT))/global
GOPATH := $(if $(wildcard $(GVM_GOPATH)),$(GVM_GOPATH),$(shell go env GOPATH))
export GOPATH
GO_BIN_DIR := $(if $(GOBIN),$(GOBIN),$(GOPATH)/bin)

GOLANGCI_LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOFUMPT_PKG := mvdan.cc/gofumpt
GOTESTSUM_PKG := gotest.tools/gotestsum

# The tools make runs, each the binary of the version go.mod pins, installed
# under GO_BIN_DIR in a directory named after the tool and its version, as
# golangci-lint-v2.14.0/golangci-lint. A tool on PATH is never run, whatever
# its version, so a run here and a run in CI judge the code by the same tool;
# an upgrade installs the new version beside the old one.
GOLANGCI_LINT := $(GO_BIN_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)/golangci-lint
GOFUMPT := $(GO_BIN_DIR)/gofumpt-$(GOFUMPT_VERSION)/gofumpt
GOTESTSUM := $(GO_BIN_DIR)/gotestsum-$(GOTESTSUM_VERSION)/gotestsum

# install_tool installs the tool at the path $(1), the binary of the package
# $(2) at the version $(3), when it is not there yet. go install names the
# binary after its package, so the version goes on the directory, which is
# the GOBIN of the install alone: the module and build caches serve the
# build like any other, and nothing else of the environment changes.
define install_tool
	@[ -x "$(1)" ] || { \
		echo "Installing $(2)@$(3)..."; \
		mkdir -p "$(dir $(1))"; \
		GOBIN="$(dir $(1))" go install $(2)@$(3); \
	}
endef

# run_tool runs the tool at the path $(1), with the environment TOOL_ENV
# sets, empty but for test.
define run_tool
	@echo "$(notdir $(1)) $(2)"; \
		$(TOOL_ENV) "$(1)" $(2)
endef

# run_tool_in runs the tool at the path $(1) from another directory. Neither
# tool it serves has a flag of its own for this: golangci-lint discovers its
# configuration by walking up from the working directory, so an example is
# only linted against its own .golangci.yml when the tool actually runs inside
# the example -- which is also what gg lint does in a generated project -- and
# gotestsum tests the module of the directory it runs in.
define run_tool_in
	@echo "$(notdir $(1)) $(3) ($(2))"; \
		cd $(2) && $(TOOL_ENV) "$(1)" $(3)
endef

# Default target
help:
	@echo "Available commands:"
	@echo "  check          - Run all code quality checks"
	@echo "  build          - Build the project"
	@echo "  format         - Format code with gofumpt"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run golangci-lint (includes modernize, nilness, shadow and goimports)"
	@echo "  selfcheck      - Check the framework's source against the rules golangci-lint cannot express"
	@echo "  ggcheck        - Run gg gen and gg check on the example projects with the gg built from the tree"
	@echo "  test           - Run unit tests, a line per package"
	@echo "  test-short     - Run the main module's unit tests alone, the gg end-to-end cases skipped"
	@echo "  testv          - Run unit tests, a line per test"
	@echo "  testvv         - Run unit tests with the full go test -v output"
	@echo "  generate       - Regenerate the framework's own generated sources"
	@echo "  fix            - Auto-fix code issues (golangci-lint, then gofumpt)"
	@echo "  install        - Install gg command and development tools"
	@echo "  uninstall      - Uninstall gg command and development tools"
	@echo "  help           - Show this help message"

# Run all code quality checks
# format runs before lint: gofumpt rewrites what it can, and lint, goimports
# among its checks, then judges the code gofumpt leaves.
check: build format lint selfcheck vet ggcheck
	@echo "All checks passed successfully!"

# Build the project, then build it again with cgo off, the way a project
# builds a static binary for a minimal image: the framework compiles into
# every project binary, so it has to compile without cgo too. Built without
# cgo, the sqlite driver is a stub declaring little more than the driver and
# its connection, so code using anything else the driver declares, such as
# its error type and codes, lives in a file built only with cgo, beside a
# file built only without cgo that stands in for it (see database/sqlite).
# The example modules are not built: vet type-checks each of them, which
# already fails the check on an API change that breaks one, and building them
# would add only the link of each example's program, which go build never
# caches and so would redo on every check.
build:
	@echo "Running go build..."
	go build ./...
	CGO_ENABLED=0 go build ./...

format:
	$(call install_tool,$(GOFUMPT),$(GOFUMPT_PKG),$(GOFUMPT_VERSION))
	@echo "Running gofumpt..."
	$(call run_tool,$(GOFUMPT),-l -w .)

# Run go vet, on the example modules too: each example is a module of its own,
# which go vet ./... does not reach into, and an API change that breaks one
# must fail the check rather than the next person to open the example.
vet:
	@echo "Running go vet..."
	go vet ./...
	go -C examples/demo vet ./...
	go -C examples/cluster vet ./...
	go -C examples/bench vet ./...

# Run golangci-lint: modernize, nilness and shadow run inside it, and goimports
# checks the imports are organized the way gopls saves them (see .golangci.yml).
# The example modules are linted too (see vet), each against the .golangci.yml
# gg new writes into a project, so the rules a generated project is held to are
# the rules the examples demonstrate.
lint:
	$(call install_tool,$(GOLANGCI_LINT),$(GOLANGCI_LINT_PKG),$(GOLANGCI_LINT_VERSION))
	@echo "Running golangci-lint..."
	$(call run_tool,$(GOLANGCI_LINT),run ./...)
	$(call run_tool_in,$(GOLANGCI_LINT),examples/demo,run ./...)
	$(call run_tool_in,$(GOLANGCI_LINT),examples/cluster,run ./...)
	$(call run_tool_in,$(GOLANGCI_LINT),examples/bench,run ./...)

# Hold the framework's own source to the rules golangci-lint cannot express
# (see internal/cmd/selfcheck): testpackage in golangci-lint, for one, makes a
# test file that joins its package carry the _internal_test.go suffix, and the
# test placement check makes the suffix true.
selfcheck:
	@echo "Running the framework's self check..."
	go run ./internal/cmd/selfcheck

# The example projects are generated and checked with the gg built from the
# tree, the way a project runs gg gen and gg check: a generator that fails on
# an example's models, or a project check or a DSL rule an example breaks,
# fails here, not first on a developer's gg gen. What gen rewrites stays in
# the tree, as gofumpt's rewrites do, for the developer to commit. The gg is
# built where make install puts it, so the installed one is the tree's.
GG := $(GO_BIN_DIR)/gg
ggcheck:
	@echo "Running gg gen and gg check on the examples..."
	@mkdir -p "$(GO_BIN_DIR)"
	@go build -o "$(GG)" ./cmd/gg
	cd examples/demo && "$(GG)" gen && "$(GG)" check
	cd examples/cluster && "$(GG)" gen && "$(GG)" check
	cd examples/bench && "$(GG)" gen && "$(GG)" check

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
# Nothing is compiled with DWARF debug information either. go test links a
# test binary it runs and throws away with -s -w, dropping the debug
# information every package brings along, but compiles only the binary's own
# main package without it (golang.org/issue/38777): the compiler still writes
# it into the build cache for every other package, a quarter of each object,
# for nothing. Stack traces and race reports read the runtime's own tables,
# not DWARF. A package matching more than one -gcflags pattern takes the
# flags of the last one, so blowfish's carries -dwarf=false again.
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
TEST_FLAGS := -race -gcflags=all=-dwarf=false -gcflags='golang.org/x/crypto/blowfish=-race=false -dwarf=false'
TEST_OUTPUT := --format pkgname --format-hide-empty-pkg --hide-summary=skipped

# go test keys a cached result on the value of each environment variable the
# test read, and the tests read PATH: they run go and git, and the test
# container library runs the Docker credential helper. Each shell builds its
# PATH its own way, so the tests run with this fixed one, and runs started
# from any terminal or agent share their cached results instead of each
# rerunning what another ran last.
TEST_PATH := $(shell go env GOROOT)/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin

# The build cache helper (see internal/cmd/testcachefix) keeps make test fast
# when everything is cached: go links a cached test's binary again on every
# run once a change the linker dropped from it moved its link key, and the
# helper files the result under that key after each suite. It adds to the
# cache and never removes from it, since the cache is the machine's. The
# helper is built from the tree on every run, so it is always the source's.
TESTCACHEFIX := $(GO_BIN_DIR)/testcachefix
testcachefix:
	@mkdir -p "$(GO_BIN_DIR)"
	@go build -o "$(TESTCACHEFIX)" ./internal/cmd/testcachefix

# run_test_suite runs one gotestsum suite, with the arguments after -- given,
# records every test binary the run links and files the results under their
# link keys afterwards, whatever the run's status, which it then reports. The
# write-back is given TEST_FLAGS too: the test flags among them, -short for
# one, are part of the keys.
define run_test_suite
	@record="$$(mktemp -d)"; \
		echo "gotestsum $(TEST_OUTPUT) -- $(TEST_FLAGS) $(1)"; \
		$(TOOL_ENV) "$(GOTESTSUM)" $(TEST_OUTPUT) -- $(TEST_FLAGS) -toolexec "$(TESTCACHEFIX) record-link $$record" $(1); status=$$?; \
		$(TOOL_ENV) "$(TESTCACHEFIX)" writeback "$$record" $(TEST_FLAGS); rm -rf "$$record"; exit $$status
endef

# run_test_suite_in runs a suite from another directory, the way run_tool_in
# does: gotestsum tests the module of the directory it runs in.
define run_test_suite_in
	@record="$$(mktemp -d)"; \
		echo "gotestsum $(TEST_OUTPUT) -- $(TEST_FLAGS) $(2) ($(1))"; \
		cd $(1) && $(TOOL_ENV) "$(GOTESTSUM)" $(TEST_OUTPUT) -- $(TEST_FLAGS) -toolexec "$(TESTCACHEFIX) record-link $$record" $(2); status=$$?; \
		$(TOOL_ENV) "$(TESTCACHEFIX)" writeback "$$record" $(TEST_FLAGS); rm -rf "$$record"; exit $$status
endef

test: TOOL_ENV = PATH="$(TEST_PATH)"
test: testcachefix
	$(call install_tool,$(GOTESTSUM),$(GOTESTSUM_PKG),$(GOTESTSUM_VERSION))
	@echo "Running unit tests (the per-dialect suites run against mysql here)..."
	$(call run_test_suite,./...)
	@echo "Running the per-dialect suites against postgres..."
	$(call run_test_suite,-tags gsttest_postgres $(DIALECT_PACKAGES))
	@echo "Running the per-dialect suites against sqlite..."
	$(call run_test_suite,-tags gsttest_sqlite $(DIALECT_PACKAGES))
	@echo "Running example project tests..."
	$(call run_test_suite_in,examples/demo,./...)
	$(call run_test_suite_in,examples/cluster,./...)

# test-short runs the main module alone with -short, the signal for a change
# in progress: the gg command's cases that build a project against the
# framework, a minute or more together, skip themselves (see newGenProject in
# cmd/gg), and the per-dialect suites and the example projects are left out.
# make test is the verdict. -short is a test flag, so the two share no cached
# results.
test-short: TOOL_ENV = PATH="$(TEST_PATH)"
test-short: TEST_FLAGS += -short
test-short: testcachefix
	$(call install_tool,$(GOTESTSUM),$(GOTESTSUM_PKG),$(GOTESTSUM_VERSION))
	@echo "Running unit tests in short mode (main module; the gg end-to-end cases skipped)..."
	$(call run_test_suite,./...)

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
	go run ./internal/cmd/apidocgen

# Auto-fix code issues
# golangci-lint runs first, its goimports organizing the imports the way gopls
# does on save, and gofumpt last, the order gopls saves a file in. The example
# modules are fixed too (see lint): golangci-lint reaches only the module it
# runs in, so each example is fixed from inside it, against its own
# .golangci.yml. gofumpt walks paths rather than modules, so the run at the
# root already covers them.
fix:
	@echo "Running auto-fix tools..."
	$(call install_tool,$(GOLANGCI_LINT),$(GOLANGCI_LINT_PKG),$(GOLANGCI_LINT_VERSION))
	@echo "Running golangci-lint --fix..."
	$(call run_tool,$(GOLANGCI_LINT),run --fix ./...)
	$(call run_tool_in,$(GOLANGCI_LINT),examples/demo,run --fix ./...)
	$(call run_tool_in,$(GOLANGCI_LINT),examples/cluster,run --fix ./...)
	$(call run_tool_in,$(GOLANGCI_LINT),examples/bench,run --fix ./...)
	$(call install_tool,$(GOFUMPT),$(GOFUMPT_PKG),$(GOFUMPT_VERSION))
	@echo "Running gofumpt..."
	$(call run_tool,$(GOFUMPT),-l -w .)
	@echo "All auto-fix operations completed!"

# Install gg and the tools make runs, at the versions go.mod pins (see
# GOLANGCI_LINT and the others above)
install:
	@echo "Installing development tools from go.mod..."
	$(call install_tool,$(GOLANGCI_LINT),$(GOLANGCI_LINT_PKG),$(GOLANGCI_LINT_VERSION))
	$(call install_tool,$(GOFUMPT),$(GOFUMPT_PKG),$(GOFUMPT_VERSION))
	$(call install_tool,$(GOTESTSUM),$(GOTESTSUM_PKG),$(GOTESTSUM_VERSION))
	@echo "Installing gg command..."
	@go install ./cmd/gg
	@echo "Installation completed!"

# Uninstall gg, the test cache helper and the tools make runs, at the
# versions go.mod pins: a version installed by an earlier go.mod is left
# where it is.
INSTALL_PATHS := $(GO_BIN_DIR)/gg $(GO_BIN_DIR)/testcachefix $(patsubst %/,%,$(dir $(GOLANGCI_LINT) $(GOFUMPT) $(GOTESTSUM)))
uninstall:
	@echo "Uninstalling development tools from $(GO_BIN_DIR)..."
	@for path in $(INSTALL_PATHS); do \
		if [ -e "$$path" ]; then \
			rm -rf "$$path"; \
			echo "Removed $$path"; \
		else \
			echo "Skipped $$path (not installed)"; \
		fi; \
	done
	@echo "Uninstallation completed!"
