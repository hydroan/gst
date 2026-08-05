package rbac

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failLoadAdapter delegates to a real adapter and fails every policy read, so a
// test can observe a process that cannot put itself back in step with storage.
type failLoadAdapter struct{ *adapter }

var errAdapterLoad = errors.New("adapter load failed")

func (a *failLoadAdapter) loadPolicies(context.Context) (*policySet, error) {
	return nil, errAdapterLoad
}

// recoveringLoadAdapter fails a fixed number of policy reads and then answers
// like the real adapter, which is what a database blip looks like to a reload.
// The counter is atomic because the reads come from the retry goroutine.
type recoveringLoadAdapter struct {
	*adapter
	remaining atomic.Int32
}

func (a *recoveringLoadAdapter) loadPolicies(ctx context.Context) (*policySet, error) {
	if a.remaining.Add(-1) >= 0 {
		return nil, errAdapterLoad
	}
	return a.adapter.loadPolicies(ctx)
}

// TestRecoveryOutlivesTheRequestThatTriggeredIt covers the context an
// after-commit action is handed: the one from before the transaction opened,
// which in an HTTP handler is the request's.
//
// A client that has already disconnected would otherwise cancel the one read
// that can put this process back in step with storage — and it disconnects
// exactly while that read is the only thing still to happen.
func TestRecoveryOutlivesTheRequestThatTriggeredIt(t *testing.T) {
	r, store := storedRBAC(t, "policy_canceled_trigger")

	// Storage holds a rule memory does not, which is the state a reload repairs.
	_, err := r.applyToStore(context.Background(), []policyMutation{
		addRules("p", []string{"tenant_a", "role_a", "/api/things", "GET", "allow"}),
	})
	require.NoError(t, err)
	require.Len(t, storedRules(t, store), 1)
	require.Empty(t, memoryRules(t, r), "the memory half has not run")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// Without this the test would pass on a database that ignores the context,
	// proving nothing about the reload.
	_, err = store.loadPolicies(canceled)
	require.Error(t, err, "the trigger context has to be dead for this test to mean anything")

	require.NoError(t, r.recoverPolicies(canceled, nil))
	assert.Equal(t, storedRules(t, store), memoryRules(t, r),
		"a canceled trigger must not cancel the reload it triggered")
}

// TestReloadKeepsDeciding covers what a reload has to leave working: the set,
// the index and the role graph are swapped together, so a decision after a
// reload answers exactly as before it — templates matched, inheritance
// resolved — from the reloaded rules.
func TestReloadKeepsDeciding(t *testing.T) {
	r, _ := storedRBAC(t, "policy_reload_invariants")
	ctx := context.Background()

	require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", []types.Permission{
		{Object: "/api/things/{id}", Action: "GET"},
	}))
	require.NoError(t, r.AssignRole(ctx, "tenant_a", "u1", "role_a"))
	require.NoError(t, r.ReloadPolicies(ctx))

	decision, err := r.Authorize(ctx, "tenant_a", "u1", "/api/things/1", "GET")
	require.NoError(t, err)
	assert.True(t, decision.Allowed, "a reloaded process has to keep deciding as before")
}

// TestDivergedProcessRetriesUntilStorageAnswers covers what happens after the
// reload a recovery needed has failed: without a retry, the divergence that was
// published and logged is also final — a process that missed one revocation
// while the database blipped would keep allowing it for the rest of its life.
// The retry has to be driven by the divergence state, so it ends the moment a
// reload succeeds and never runs on a process that is in step.
func TestDivergedProcessRetriesUntilStorageAnswers(t *testing.T) {
	prev := reloadRetryInterval
	reloadRetryInterval = time.Millisecond
	t.Cleanup(func() {
		reloadRetryInterval = prev
		publishPolicyDivergence(false)
	})

	store := newPolicyTable(t, "policy_divergence_retry")
	flaky := &recoveringLoadAdapter{adapter: store}
	flaky.remaining.Store(3)
	installTestSet(t)
	r := &rbac{adapter: flaky, mu: &policyMu}

	// Storage holds a rule memory does not, which is what the retry has to
	// repair once the adapter answers again.
	_, err := r.applyToStore(context.Background(), []policyMutation{
		addRules("p", []string{"tenant_a", "role_a", "/api/things", "GET", "allow"}),
	})
	require.NoError(t, err)

	require.ErrorIs(t, r.recoverPolicies(context.Background(), nil), errAdapterLoad)
	require.True(t, policiesDiverged.Load(), "the failed recovery has to publish the divergence")

	require.Eventually(t, func() bool {
		return !policiesDiverged.Load() && !reloadRetryRunning.Load()
	}, 5*time.Second, time.Millisecond,
		"the retry has to converge once storage answers, and stop once it has")
	assert.Equal(t, storedRules(t, store), memoryRules(t, r),
		"the converged process has to decide from what storage holds")
}

// TestPeriodicReloadReconcilesWithStorage covers the schedule that bounds
// every staleness a process cannot see for itself: a write another replica
// made, a manual repair, a restore. None of those raises an error, publishes
// divergence, or touches a removal's row counts on this process, so nothing
// event-driven ever fires — only the schedule brings the process back in step.
func TestPeriodicReloadReconcilesWithStorage(t *testing.T) {
	prev := reloadInterval
	reloadInterval = time.Millisecond
	t.Cleanup(func() { reloadInterval = prev })

	r, store := storedRBAC(t, "policy_periodic_reload")
	// The loop resolves its target through RBAC on every tick, so the adapter
	// under test has to be the installed one.
	policyMu.Lock()
	policyStore = store
	policyMu.Unlock()

	stop := startPeriodicReload()
	t.Cleanup(stop)

	// A write this process never saw: storage only, no memory half, no
	// divergence published.
	_, err := r.applyToStore(context.Background(), []policyMutation{
		addRules("p", []string{"tenant_a", "role_a", "/api/things", "GET", "allow"}),
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		r.mu.RLock()
		defer r.mu.RUnlock()
		return len(policyRules.all("p")) == 1
	}, 5*time.Second, time.Millisecond,
		"the schedule has to pick up a write this process never made")
	assert.Equal(t, storedRules(t, store), memoryRules(t, r))
}

// TestPolicyDivergenceIsPublished covers the state a process enters when the
// reload that had to succeed did not. Nothing else can see it: the write is
// already durable, the request that made it has returned, and comparing stored
// rules against their records cannot see a disagreement that exists only in one
// process's memory.
func TestPolicyDivergenceIsPublished(t *testing.T) {
	// Each case sets the state it needs rather than inheriting it, so that any
	// one of them can be run on its own.
	t.Run("a recovery that cannot reload says so", func(t *testing.T) {
		publishPolicyDivergence(false)
		t.Cleanup(func() { publishPolicyDivergence(false) })

		store := newPolicyTable(t, "policy_divergence_failing")
		failing := &failLoadAdapter{adapter: store}
		installTestSet(t)
		r := &rbac{adapter: failing, mu: &policyMu}

		require.ErrorIs(t, r.recoverPolicies(context.Background(), nil), errAdapterLoad)
		assert.True(t, policiesDiverged.Load(),
			"a process that could not reload has to say it is out of step")
	})

	t.Run("a reload that works clears it", func(t *testing.T) {
		publishPolicyDivergence(true)
		t.Cleanup(func() { publishPolicyDivergence(false) })

		r, _ := storedRBAC(t, "policy_divergence_recovered")
		require.NoError(t, r.ReloadPolicies(context.Background()))
		assert.False(t, policiesDiverged.Load())
	})

	t.Run("the gauge follows the state", func(t *testing.T) {
		// The gauge is set directly rather than through prommetrics.Init, which
		// registers with the default registry and so cannot run twice. Leaving
		// it nil again afterwards also restores what the other cases exercise:
		// a process that never ran bootstrap still has to be able to reload.
		prommetrics.AuthzPolicyDiverged = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "authz_policy_diverged_probe",
		})
		t.Cleanup(func() {
			prommetrics.AuthzPolicyDiverged = nil
			publishPolicyDivergence(false)
		})

		publishPolicyDivergence(true)
		assert.InDelta(t, 1.0, gaugeValue(t, prommetrics.AuthzPolicyDiverged), 0,
			"a diverged process has to be visible to whoever is watching it")

		publishPolicyDivergence(false)
		assert.InDelta(t, 0.0, gaugeValue(t, prommetrics.AuthzPolicyDiverged), 0)
	})
}
