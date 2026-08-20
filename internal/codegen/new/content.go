//nolint:predeclared
package new

import (
	"github.com/hydroan/gst/types/consts"
)

var modelContent = consts.CodeGeneratedComment() + `
package model

func init() {
}
`

var serviceContent = consts.CodeGeneratedComment() + `
package service

func init() {
}
`

var routerContent = consts.CodeGeneratedComment() + `
package router

func Init() error {
	return nil
}
`

const golangciLintContent = `version: "2"

run:
  timeout: 5m
  modules-download-mode: readonly

severity:
  default: error

linters:
  default: none
  enable:
    # Core correctness.
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused

    # Error handling.
    - errorlint
    - errname
    - errchkjson
    - nilerr
    - nilnil
    - nilnesserr

    # Backend resource safety.
    - bidichk
    - bodyclose
    - rowserrcheck
    - sqlclosecheck
    - durationcheck
    - fatcontext
    - gosec

    # Code hygiene.
    - asciicheck
    - dupword
    - gocheckcompilerdirectives
    - iface
    - makezero
    - mirror
    - misspell
    - musttag
    - nolintlint
    - predeclared
    - reassign
    - recvcheck
    - revive
    - unconvert
    - unparam
    - wastedassign

    # Go modernization.
    - copyloopvar
    - exptostd
    - intrange
    - modernize
    - perfsprint
    - usestdlibvars

    # Broad complementary suite (diagnostic, style, performance).
    - gocritic

    # Test quality.
    - testifylint
    - thelper
    - tparallel
    - usetesting

    # Project constraints.
    - depguard
    - forbidigo
    - gomoddirectives

    # Project-specific checks.
    # canonicalheader v1.1.2 panics on parameterized method calls (go1.27);
    # re-enable once an upstream release handles them.
    # - canonicalheader
    - loggercheck
    - spancheck

    # Documentation.
    - godoclint

  settings:
    depguard:
      rules:
        main:
          deny:
            - pkg: "errors"
              desc: "Use github.com/cockroachdb/errors instead"
            - pkg: "github.com/pkg/errors"
              desc: "Use github.com/cockroachdb/errors instead"

    errcheck:
      check-type-assertions: true
      exclude-functions:
        - io.Copy(*bytes.Buffer)
        - io.Copy(os.Stdout)

    forbidigo:
      # Refusing a request outside the controller path goes through
      # response.Abort, which is the single place the API envelope is written.
      # gin's own aborts stay reachable on *gin.Context forever, so nothing but
      # this rule stops the next middleware from writing a second envelope by
      # hand.
      forbid:
        - pattern: '^.*\.AbortWithStatusJSON$'
          msg: "write the envelope through response.Abort instead of building it by hand"
        - pattern: '^.*\.AbortWithStatus$'
          msg: "an empty body carries neither a code nor a trace id; refuse through response.Abort"

    godoclint:
      # Default set of rules to enable.
      # Possible values are: 'basic', 'all' or 'none'.
      # Default: 'basic' (enables 'pkg-doc', 'single-pkg-doc', 'start-with-name', and 'deprecated')
      default: basic
      # List of rules to enable in addition to the default set.
      enable:
        # Check proper package-level godoc, if any.
        - pkg-doc
        # Assert at most one godoc per package.
        - single-pkg-doc
        # Check godocs start with the corresponding symbol name.
        - start-with-name
        # Check deprecated symbols have proper deprecation notice.
        - deprecated

    govet:
      # These analyzers are not part of the default go vet analyzer set.
      enable:
        - shadow
        - nilness
        - unusedwrite
        - reflectvaluecompare
        - deepequalerrors
        - sortslice

    misspell:
      locale: US

    recvcheck:
      # Design, GetTableName, Purge and Indexes are stateless declaration
      # methods that use value receivers by framework convention, while
      # stateful hooks require pointer receivers.
      exclusions:
        - "*.Design"
        - "*.GetTableName"
        - "*.Purge"
        - "*.Indexes"

    revive:
      rules:
        - name: blank-imports
        - name: dot-imports
          arguments:
            - allowedPackages:
                - github.com/hydroan/gst/dsl
        - name: context-as-argument
        - name: context-keys-type
        - name: error-naming
        - name: error-return
        - name: error-strings
        - name: errorf
        - name: indent-error-flow
        - name: range
        - name: receiver-naming
        - name: time-naming
        - name: unexported-return
        - name: var-declaration
        - name: var-naming

    staticcheck:
      # SA4023 is the only check requiring the nilness fact, whose analyzer in
      # staticcheck v0.8.0-rc.1 panics on getsentry/sentry-go ("unhandled
      # builtin recover"); dropping the check prunes the fact from the run.
      # govet's nilness analyzer still covers nil-consistency independently.
      # Re-enable once a stable staticcheck release fixes the fact analyzer.
      checks: ["all", "-QF1008", "-SA4023"]
      dot-import-whitelist:
        - github.com/hydroan/gst/dsl

  exclusions:
    generated: lax
    presets:
      - comments
      - std-error-handling
      - common-false-positives
      - legacy
    rules:
      # unparam is excluded in tests because test helpers commonly keep
      # parameters for signature symmetry across cases.
      - path: _test\.go
        linters:
          - gosec
          - godoclint
          - unparam
      # The gofix //go:fix directive is valid but not yet in
      # gocheckcompilerdirectives' known directive list.
      - linters:
          - gocheckcompilerdirectives
        text: "//go:fix"
      # Revive var-naming: ignore ALL_CAPS (redundant with staticcheck) and underscores in names.
      - linters:
          - revive
        text: "don't use (ALL_CAPS|underscores) in Go names"
      # Allow package names "types" and "util" (meaningful in our context).
      - linters:
          - revive
        text: "avoid meaningless package names"
        path: "^(types|util)/|.*/(types|util)/"

issues:
  max-same-issues: 100
`

var moduleContent = `// Package module assembles the application's business modules.
//
// Call each module's Register function in init below: built-in gst modules
// such as github.com/hydroan/gst/module/iam, and your own. For your own
// resources, create one subpackage per resource under module/ and expose a
// Register function that wires model, service and routes via module.Use.
//
// See github.com/hydroan/gst/module/helloworld for a complete example.
package module

func init() {
	// TODO: call your module Register functions here.
}
`

var mainContent = consts.CodeGeneratedComment() + `
package main

import (
	_ "%s/configx"
	_ "%s/cronjob"
	_ "%s/middleware"
	_ "%s/model"
	_ "%s/module"
	"%s/router"
	_ "%s/service"

	"github.com/hydroan/gst/bootstrap"
	. "github.com/hydroan/gst/util"
)

func main() {
	RunOrDie(bootstrap.Bootstrap)
	RunOrDie(router.Init)
	RunOrDie(bootstrap.Run)
}
`

const configxContent = `// Package configx registers the application's custom configuration sections.
//
// Declare a struct and call config.Register[T]() in init below. The section
// name is the snake_case of the struct name (Sample -> [sample] in
// config.ini). Fields resolve from environment variables (SAMPLE_ENDPOINT),
// then the config file, then "default" struct tags; see config.Register for
// details.
//
// Example:
//
//	import "github.com/hydroan/gst/config"
//
//	type Sample struct {
//		Endpoint string ` + "`json:\"endpoint\" mapstructure:\"endpoint\" default:\"127.0.0.1:8080\"`" + `
//		Enabled  bool   ` + "`json:\"enabled\" mapstructure:\"enabled\"`" + `
//	}
//
//	func init() {
//		config.Register[Sample]()
//	}
//
//	// Anywhere after startup:
//	cfg := config.Get[Sample]()
package configx

func init() {
	// TODO: register your custom configurations here.
}
`

const cronjobContent = `// Package cronjob registers the application's scheduled tasks.
//
// Call cronjob.Register(fn, spec, name) in init below; the framework starts
// all registered jobs on boot. fn is a func() error; every run is logged
// under name, and panics are recovered.
//
// spec is a 6-field cron expression "second minute hour day month weekday",
// e.g. "0 0 2 * * *" (daily at 02:00), or a descriptor such as "@hourly" or
// "@every 5m". Pass cronjob.Config{RunImmediately: true} as the optional
// fourth argument to also run the job once at startup.
//
// Example:
//
//	import "github.com/hydroan/gst/cronjob"
//
//	func cleanup() error { return nil }
//
//	func init() {
//		cronjob.Register(cleanup, "0 0 2 * * *", "daily-cleanup")
//	}
package cronjob

func init() {
	// TODO: register your cron jobs here.
}
`

const middlewareContent = `// Package middleware registers the application's custom HTTP middleware.
//
// middleware.Register applies to all routes; middleware.RegisterAuth applies
// only to routes behind authentication. Both take one or more gin.HandlerFunc
// and wrap each with tracing automatically.
//
// Example:
//
//	import (
//		"net/http"
//
//		"github.com/gin-gonic/gin"
//		"github.com/hydroan/gst/middleware"
//		"github.com/hydroan/gst/response"
//	)
//
//	func sample(c *gin.Context) {
//		// Runs before each handler. Refuse a request with response.Abort: it
//		// answers in the API envelope every other response carries, so one
//		// client reads them all the same way and can quote back the trace id
//		// that explains this one.
//		if c.GetHeader("X-Sample") == "" {
//			response.Abort(c, http.StatusForbidden, "sample header required")
//			return
//		}
//		c.Next()
//	}
//
//	func init() {
//		middleware.Register(sample)
//	}
package middleware

func init() {
	// TODO: register your custom middlewares here.
}
`

const gitignoreContent = `# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with 'go test -c'
*.test

# Output of the go coverage tool, specifically when used with LiteIDE
*.out

# Dependency directories (remove the comment below to include it)
# vendor/

# Go workspace file
go.work

# IDE files
.vscode/
.idea/
*.swp
*.swo
*~

# OS generated files
.DS_Store
.DS_Store?
._*
.Spotlight-V100
.Trashes
ehthumbs.db
Thumbs.db

# Log files
*.log
/logs/

# Temporary files
tmp/
temp/

# Build output
dist/
build/

# Generated files
generated/
`
