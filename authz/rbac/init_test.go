package rbac

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnforcerAndAdapterArePublishedTogether covers what a reader may observe
// while Init installs the enforcer.
//
// A caller reaches both package variables through RBAC the moment either one is
// assigned. Assigned one at a time, there is a window in which the enforcer is
// published and the adapter its writes go through is still nil, and any write
// taken from that window dereferences it. Read without the lock they are
// assigned under, the two are a data race besides.
func TestEnforcerAndAdapterArePublishedTogether(t *testing.T) {
	store := newPolicyTable(t, "policy_install_enforcer")
	t.Cleanup(func() { installEnforcer(nil, nil) })

	policyEnforcer, err := newEnforcer(store)
	require.NoError(t, err)

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if published, ok := RBAC().(*rbac); ok {
					assert.NotNil(t, published.adapter,
						"an enforcer must not be reachable before the adapter its writes go through")
				}
			}
		})
	}
	for range 100 {
		installEnforcer(policyEnforcer, store)
		installEnforcer(nil, nil)
	}
	close(stop)
	readers.Wait()
}
