// Package controller contains framework-owned HTTP handlers for registered routes.
//
// Application code should register routes through package router or generated
// code. Keeping controller internal prevents external projects from depending on
// handler factories or mutating controller-owned audit state directly.
package controller

import (
	"context"
	"sync"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/ds/queue/circularbuffer"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/pkg/auditmanager"
	"go.uber.org/zap"
)

// TODO: Record failed operations.

// ErrRequestBodyEmpty is returned when an action requires a request body but the
// client sends an empty body.
const ErrRequestBodyEmpty = "request body is empty"

var (
	// Global circular buffer for async operation logs.
	cb *circularbuffer.CircularBuffer[*modellogmgmt.OperationLog]

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

	// Initialize circular buffer.
	cb, err = circularbuffer.New(int(config.App.Server.CircularBuffer.SizeOperationLog), circularbuffer.WithSafe[*modellogmgmt.OperationLog]())
	if err != nil {
		return err
	}

	// Initialize audit manager.
	am = auditmanager.New(&config.App.Audit, cb)

	// Consume operation log.
	go am.Consume()

	initialized = true

	return nil
}

// Clean flushes buffered operation logs during shutdown.
func Clean() {
	operationLogs := make([]*modellogmgmt.OperationLog, 0, config.App.Server.CircularBuffer.SizeOperationLog)
	for !cb.IsEmpty() {
		ol, _ := cb.Dequeue()
		operationLogs = append(operationLogs, ol)
	}
	if len(operationLogs) > 0 {
		if err := database.Database[*modellogmgmt.OperationLog](context.Background()).WithLimit(-1).WithBatchSize(100).Create(operationLogs...); err != nil {
			zap.S().Error(err)
		}
	}
}
