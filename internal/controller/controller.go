// Package controller serves the actions of the registered routes on both
// transports: over HTTP through the handlers the router mounts (CreateHandler
// and its kind), over gRPC through the call functions the generated pb
// package runs (CreateCall and its kind). An action is one flow — the
// service hooks, the database access, the operation log — with a binding
// and an answer per transport around it.
//
// Application code registers routes through package router or the generated
// code and services through package service. Keeping controller internal
// keeps projects from depending on the handlers or the calls directly, or
// from touching the audit state the package owns.
package controller

import (
	"sync"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/auditmanager"
)

// TODO: Record failed operations.

var (
	// audit is the global audit manager, which records the operation log.
	audit *auditmanager.AuditManager

	// initMu guards initialized, which records that Init built audit.
	initMu      sync.Mutex
	initialized bool
)

// Init builds the audit manager the operation log is recorded through, from
// config.App.Audit, once: a later call leaves it as it is. Building it cannot
// fail; the error is always nil.
func Init() (err error) {
	initMu.Lock()
	defer initMu.Unlock()

	if initialized {
		return nil
	}

	audit = auditmanager.New(&config.App.Audit)

	initialized = true

	return nil
}
