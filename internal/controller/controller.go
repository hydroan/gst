// Package controller contains framework-owned HTTP handlers for registered routes.
//
// Application code should register routes through package router or generated
// code. Keeping controller internal prevents external projects from depending on
// handler factories or mutating controller-owned audit state directly.
package controller

import (
	"sync"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/pkg/auditmanager"
)

// TODO: Record failed operations.

var (
	// Global audit manager instance.
	am *auditmanager.AuditManager

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

	am = auditmanager.New(&config.App.Audit)

	initialized = true

	return nil
}
