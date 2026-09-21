package rbac

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicyStateIsPublishedTogether covers what a reader may observe while
// Init installs the policy set.
//
// A caller reaches the package state through RBAC the moment the set is
// assigned. Assigned one variable at a time, there is a window in which the
// set is published and the adapter its writes go through is still nil, and any
// write taken from that window dereferences it. Read without the lock they are
// assigned under, the pieces are a data race besides.
func TestPolicyStateIsPublishedTogether(t *testing.T) {
	store := newPolicyTable(t, "policy_install_set")
	set, err := store.loadPolicies(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { installPolicySet(nil, nil) })

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
						"a policy set must not be reachable before the adapter its writes go through")
				}
			}
		})
	}
	for range 100 {
		installPolicySet(set, store)
		installPolicySet(nil, nil)
	}
	close(stop)
	readers.Wait()
}
