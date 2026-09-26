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
	"github.com/hydroan/gst/pkg/auditmanager"
)

// TODO: Record failed operations.

var (
	// Global audit manager instance.
	audit *auditmanager.AuditManager

	initMu      sync.Mutex
	initialized bool
)

// Init initializes controller-owned audit logging state.
//
// Init is idempotent after a successful initialization. Failed initialization is
// not cached, so a later call can retry after configuration has been fixed.
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
