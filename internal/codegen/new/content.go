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
    - nilerr
    - nilnil
    - nilnesserr

    # Backend resource safety.
    - bodyclose
    - rowserrcheck
    - sqlclosecheck
    - durationcheck
    - gosec

    # Code hygiene.
    - asciicheck
    - misspell
    - nolintlint
    - predeclared
    - revive
    - unconvert
    - wastedassign

    # Go modernization.
    - intrange
    - perfsprint
    - usestdlibvars

    # Test quality.
    - testifylint
    - thelper
    - usetesting

    # Module hygiene.
    - gomoddirectives

  settings:
    errcheck:
      check-type-assertions: true
      exclude-functions:
        - io.Copy(*bytes.Buffer)
        - io.Copy(os.Stdout)

    misspell:
      locale: US

    staticcheck:
      dot-import-whitelist:
        - github.com/hydroan/gst/dsl

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

  exclusions:
    generated: lax
    presets:
      - comments
      - std-error-handling
      - common-false-positives
      - legacy
    rules:
      - path: _test\.go
        linters:
          - gosec

issues:
  max-same-issues: 100
`

var moduleContent = `// Package module provides business logic modules for the application.
//
// Recommended pattern:
//   - Organize each resource into its own subpackage under module/, e.g., module/user.
//   - Inside each subpackage, expose a Register() function that calls module.Use.
//   - Call these Register() functions from module.Init() to centralize startup.
//
// See module/helloworld for a complete example.
package module

func init() {
	// TODO: Call your module Register() functions here
	// Example:
	//   user.Register()
	//   order.Register()
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

const configxContent = `// Package configx provides custom configuration extensions for the application.
//
// Define your custom configuration structs and register them using config.Register.
// See config.Register documentation for details on configuration loading priority
// and struct tag usage.
//
// Example:
//
//	import "github.com/hydroan/gst/config"
//
//	type Payment struct {
//		Provider string ` + "`json:\"provider\" mapstructure:\"provider\" default:\"alipay\"`" + `
//		Enabled bool   ` + "`json:\"enabled\" mapstructure:\"enabled\" default:\"false\"`" + `
//	}
//
//	func init() {
//		config.Register[Payment]()
//	}
package configx

func init() {
	// TODO: Register your custom configurations here
	// Example:
	//   config.Register[YourCustomConfig]()
}
`

const cronjobContent = `// Package cronjob provides scheduled task management for the application.
//
// Cron spec format: "second minute hour day month weekday" (6 fields)
// Examples: "0 0 2 * * *" (daily at 2:00 AM), "0 0 * * * *" (hourly)
//
// Example:
//
//	import "github.com/hydroan/gst/cronjob"
//
//	func cleanup() error {
//		// Implementation here
//		return nil
//	}
//
//	func init() {
//		cronjob.Register(cleanup, "0 0 2 * * *", "daily-cleanup")
//		// Optional: run immediately after registration
//		// cronjob.Register(cleanup, "0 0 2 * * *", "daily-cleanup", cronjob.Config{
//		//     RunImmediately: true,
//		// })
//	}
package cronjob

func init() {
	// TODO: Register your cron jobs here
	// Example:
	//   cronjob.Register(yourFunc, "0 0 * * * *", "hourly-task")
}
`

const middlewareContent = `// Package middleware provides custom HTTP middleware for the application.
//
// Register global middleware (applied to all routes) or authentication middleware
// (applied to authenticated routes only). Middlewares are automatically wrapped
// with tracing for performance monitoring.
//
// Example:
//
//	import (
//		"github.com/gin-gonic/gin"
//		"github.com/hydroan/gst/middleware"
//	)
//
//	func customMiddleware() gin.HandlerFunc {
//		return func(c *gin.Context) {
//			// Your middleware logic here
//			c.Next()
//		}
//	}
//
//	func init() {
//		// Register global middleware (applied to all routes)
//		middleware.Register(customMiddleware())
//
//		// Register authentication middleware (applied to authenticated routes only)
//		middleware.RegisterAuth(customMiddleware())
//	}
package middleware

func init() {
	// TODO: Register your custom middlewares here
	// Example:
	//   middleware.Register(yourMiddleware())
	//   middleware.RegisterAuth(yourAuthMiddleware())
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
