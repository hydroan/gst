package bootstrap

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunHandlersIsSerialAndLIFO proves cleanup handlers run one at a time in
// reverse registration order, like a defer stack: what was brought up last
// (the HTTP listener draining in-flight requests) is torn down first, and
// what everything else depends on (connections, log writers) is torn down
// last. Concurrent teardown would rip the infrastructure out from under the
// drain.
func TestRunHandlersIsSerialAndLIFO(t *testing.T) {
	originalHandlers := handlers
	handlers = nil
	t.Cleanup(func() { handlers = originalHandlers })

	var order []int
	var active, maxActive int32
	for i := range 3 {
		registerCleanup(func() {
			now := atomic.AddInt32(&active, 1)
			if now > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, now)
			}
			// Long enough that concurrently started handlers would overlap.
			time.Sleep(20 * time.Millisecond)
			order = append(order, i)
			atomic.AddInt32(&active, -1)
		})
	}

	runHandlers()

	require.Equal(t, []int{2, 1, 0}, order, "handlers must run in reverse registration order")
	require.EqualValues(t, 1, maxActive, "handlers must never overlap")
}
