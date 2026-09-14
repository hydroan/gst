package bootstrap

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCleanupsRunSeriallyInReverseOrder proves cleanups run one at a time in
// reverse registration order, like a defer stack: what was brought up last
// (the HTTP listener draining in-flight requests) is torn down first, and
// what everything else depends on (connections, log writers) is torn down
// last. Concurrent teardown would rip the infrastructure out from under the
// drain.
func TestCleanupsRunSeriallyInReverseOrder(t *testing.T) {
	originalCleanups := cleanups
	cleanups = nil
	t.Cleanup(func() { cleanups = originalCleanups })

	var order []int
	var active, maxActive int32
	for i := range 3 {
		registerCleanup(func() {
			now := atomic.AddInt32(&active, 1)
			if now > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, now)
			}
			// Long enough that concurrently started cleanups would overlap.
			time.Sleep(20 * time.Millisecond)
			order = append(order, i)
			atomic.AddInt32(&active, -1)
		})
	}

	runCleanups()

	require.Equal(t, []int{2, 1, 0}, order, "cleanups must run in reverse registration order")
	require.EqualValues(t, 1, maxActive, "cleanups must never overlap")
}
