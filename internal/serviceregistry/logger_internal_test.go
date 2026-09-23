package serviceregistry

import (
	"sync"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
)

// loggerTestRecord is the model of the services these tests register.
type loggerTestRecord struct {
	Name string
	modelregistry.Base
}

// pointerBaseService embeds Base by pointer: a shape gg gen never generates
// and rewrites to the value embedding wherever it finds one. The registry does
// not support it either, since logger injection cannot reach a Logger through
// a pointer field. The tests below pin where that surfaces, which is always at
// startup and never on a request.
type pointerBaseService struct {
	*Base[*loggerTestRecord, *loggerTestRecord, *loggerTestRecord]
}

// TestRegisterPanicsOnAPointerEmbeddedBaseOnceTheLoggerExists covers a
// registration made after the service logger is set, which injects the logger
// on the spot.
func TestRegisterPanicsOnAPointerEmbeddedBaseOnceTheLoggerExists(t *testing.T) {
	setServiceLogger(t, zap.Fallback("service"))

	require.Panics(t, func() {
		Register[*loggerTestRecord, *loggerTestRecord, *loggerTestRecord](
			consts.Phase("test_pointer_base_after_logger"), "samples", &pointerBaseService{})
	})
}

// TestInitPanicsOnAPointerEmbeddedBaseRegisteredBeforeTheLogger covers the
// usual order, a registration from an init function: it passes while no
// service logger exists yet, and the bootstrap pass that injects the logger
// into every earlier registration panics on it.
func TestInitPanicsOnAPointerEmbeddedBaseRegisteredBeforeTheLogger(t *testing.T) {
	setServiceLogger(t, nil)
	phase := consts.Phase("test_pointer_base_before_logger")
	Register[*loggerTestRecord, *loggerTestRecord, *loggerTestRecord](phase, "samples", &pointerBaseService{})
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		delete(services, Key(phase, "samples"))
	})

	// Init runs once per process. A fresh once lets the pass run here, and
	// again when the test is repeated.
	initOnce = sync.Once{}
	logger.Service = zap.Fallback("service")
	require.Panics(t, func() { _ = Init() })
}

// setServiceLogger sets logger.Service for one test and restores it after.
func setServiceLogger(t *testing.T, l types.Logger) {
	t.Helper()

	previous := logger.Service
	logger.Service = l
	t.Cleanup(func() { logger.Service = previous })
}
