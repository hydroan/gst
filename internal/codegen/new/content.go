//nolint:predeclared
package new

import (
	"github.com/hydroan/gst/consts"
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

    gomoddirectives:
      # A replace pointing at a local path is how a project builds against a
      # framework checkout beside it while verifying a framework change — a
      # temporary state, removed once the change is released; only local
      # paths are allowed, a replace pointing at a remote module is still
      # refused.
      replace-local: true

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
      # Design, TableName, Purge and Indexes are stateless declaration
      # methods that use value receivers by framework convention, while
      # stateful hooks require pointer receivers.
      exclusions:
        - "*.Design"
        - "*.TableName"
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
      checks: ["all", "-QF1008"]
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
      # Constants in the configx package are named after the environment
      # variables they stand for and hold that name as their value, so the
      # ALL_CAPS spelling is deliberate and has to match the variable
      # exactly, and an explicit staticcheck checks list turns off golangci's
      # own ST1003 exclusion. The path is anchored so that packages merely
      # named configx elsewhere in the tree stay checked.
      - path: ^configx/
        linters:
          - staticcheck
        text: "ST1003"
      # G101 keys off names containing token/password/secret, which every
      # credential-related environment variable name in configx matches.
      - path: ^configx/
        linters:
          - gosec
        text: "G101"

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

const componentContent = `// Package component registers the application's long-running work: loops
// that run on every replica for the life of the process.
//
// Call component.Register(fn, name) in init below; the process starts fn
// once every table exists and is seeded, right before it starts serving,
// and stops it first at shutdown, before the clients it may use close. fn
// is a func(ctx context.Context) error expected to run until ctx ends: ctx
// ends when the process begins shutting down, and fn must return then —
// shutdown waits for it, for a bounded time. Returning before ctx ends is a
// failure, nil included, and so is a panic: each ends the process with the
// reason, for the orchestrator to restart it.
//
// Work the deployment must do once belongs elsewhere: on a schedule in
// cronjob, as a loop on one replica in leader, on demand under lock.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/hydroan/gst/component"
//	)
//
//	func consumeEvents(ctx context.Context) error {
//		for {
//			select {
//			case <-ctx.Done():
//				return nil
//			default:
//				// take the next batch and handle it; return on a failure
//			}
//		}
//	}
//
//	func init() {
//		component.Register(consumeEvents, "event-consumer")
//	}
package component

func init() {
	// TODO: register your long-running work here.
}
`

const cronjobContent = `// Package cronjob registers the application's scheduled tasks.
//
// Call cronjob.Register(fn, spec, name) in init below; the framework starts
// the scheduler once the process is ready to serve and stops it first at
// shutdown. fn is a func(ctx context.Context) error: ctx ends when the
// process begins shutting down or the round's lease is lost, so a long round
// must stop early — one still running 5 seconds after its lease was lost
// fails the process, which exits without waiting for it, since another
// replica may be running the next instant by then — and it carries the
// round's identity — the job name and a trace id of the round's own — and,
// with tracing on, the round's root span, so the statements and log lines
// the job produces are found again from any of them. Every run is logged
// under name, and panics are recovered.
//
// spec is a 6-field cron expression "second minute hour day month weekday",
// e.g. "0 0 2 * * *" (daily at 02:00 UTC), or a descriptor such as "@hourly"
// or "@every 5m". Schedules are read in UTC — prefix the expression with
// CRON_TZ=<zone> for another zone — and "@every" runs on the multiples of
// its period from the Unix epoch, so every replica computes the same
// instants. An instant that passes while the previous run is still in flight
// is skipped.
//
// A job runs once per instant across every replica of the deployment: the
// replicas share the instant's lease through the primary database, the first
// to claim it runs the round, the others skip it. Work that belongs to the
// process itself — refreshing a process-local cache, cleaning a local
// directory — registers with cronjob.RegisterPerInstance and runs on every
// replica. On startup a job's most recent instant is caught up once, on one
// replica, when the job has run before, that instant passed within the last
// day and no replica ran it; a job that has never run, or whose most recent
// instant was run, starts with its next instant.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside a job blocks the lease renewal: keep each transaction
// under 8 seconds — a longer one may end the round with the lease counted as
// lost, one over 10 seconds always does — or register work that only ever
// runs in one process with cronjob.RegisterPerInstance.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/hydroan/gst/cronjob"
//	)
//
//	func cleanup(ctx context.Context) error { return nil }
//
//	func init() {
//		cronjob.Register(cleanup, "0 0 2 * * *", "daily-cleanup")
//	}
package cronjob

func init() {
	// TODO: register your cron jobs here.
}
`

const leaderContent = `// Package leader registers the application's leader work: loops that run on
// exactly one replica of the deployment at a time.
//
// Call leader.Register(fn, name) in init below; every replica campaigns for
// the name once the process is ready to serve, the winner runs fn, and the
// others take over within seconds if it dies. fn is a func(ctx
// context.Context) error expected to run until ctx ends: ctx ends when the
// process begins shutting down or the lease behind the leadership is lost,
// and fn must stop then — one still running 5 seconds after its lease was
// lost fails the process, which exits without waiting for it, since another
// replica may be leading by then. fn runs again from scratch on the replica
// that takes over, so what it must not repeat it keeps in the database. fn
// that returns hands the leadership back, and the campaign resumes after a
// few seconds. Panics are recovered and logged, and every tenure is logged
// under name in leader.log.
//
// Work that runs on a schedule belongs in cronjob instead: a job registered
// there already runs once per instant across the deployment.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside fn blocks the lease renewal: keep each transaction
// under 8 seconds — a longer one may end the work with the lease counted as
// lost, one over 10 seconds always does.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/hydroan/gst/leader"
//	)
//
//	func relayOutbox(ctx context.Context) error {
//		for {
//			select {
//			case <-ctx.Done():
//				return nil
//			default:
//				// forward the next batch, then wait; return on a failure
//			}
//		}
//	}
//
//	func init() {
//		leader.Register(relayOutbox, "outbox-relay")
//	}
package leader

func init() {
	// TODO: register your leader work here.
}
`

const lockContent = `// Package lock declares the application's locks: one for each piece of work
// that must not run twice at once across the deployment and is done when it
// returns — an administrator's "rebuild the report", a refresh of a
// credential every replica shares.
//
// Declare each lock in a package variable with lock.New(name) and try it
// from the code that triggers the work with TryRun(ctx, fn). The try never
// waits: a lock held elsewhere is refused at once with lock.ErrHeld, and the
// caller answers accordingly — a conflict to the client, a skipped run to
// the log. fn receives a context that ends when the lease behind the lock is
// lost or ctx ends, and must stop then — fn still running 5 seconds after
// the loss fails the process, which exits without waiting for it; a lost
// lease is reported as lock.ErrLost even when fn returned nothing, since
// another holder may have started the same work since. A lock protects a
// piece of work, not rows: two requests writing the same row are kept apart
// by a transaction and a row lock.
//
// Try a lock outside any database transaction and open the transactions
// inside fn: the lock is given back as soon as fn returns, before a
// transaction around the try — a database.Transaction closure, a model
// hook — commits fn's writes, so such a try is refused with
// lock.ErrInTransaction.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside fn blocks the lease renewal: keep each transaction
// under 8 seconds — a longer one may end the work with the lease counted as
// lost, one over 10 seconds always does.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/cockroachdb/errors"
//		"github.com/hydroan/gst/lock"
//	)
//
//	var rebuildReport = lock.New("rebuild-report")
//
//	func rebuild(ctx context.Context) error {
//		err := rebuildReport.TryRun(ctx, rebuildReportRows)
//		if errors.Is(err, lock.ErrHeld) {
//			return errors.New("a rebuild is already running")
//		}
//		return err
//	}
package lock

// TODO: declare your locks here, one package variable each.
`

const middlewareContent = `// Package middleware registers the application's custom HTTP middleware.
//
// middleware.Register applies to every API route; middleware.RegisterAuth
// applies only to the API routes behind authentication. Both take one or more
// gin.HandlerFunc and wrap each with tracing automatically.
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
//		// Runs before each handler and simply returns: gin carries the chain
//		// on by itself, and the tracing span wrapped around this middleware
//		// then covers only its own work. Call c.Next() only when code has to
//		// run after the handler, such as reading the response status; the
//		// span then covers the handler as well.
//		//
//		// Refuse a request with response.Abort: it answers in the API
//		// envelope every other response carries, so one client reads them
//		// all the same way and can quote back the trace id that explains
//		// this one.
//		if c.GetHeader("X-Sample") == "" {
//			response.Abort(c, http.StatusForbidden, "sample header required")
//			return
//		}
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
