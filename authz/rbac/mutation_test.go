package rbac

import (
	"context"
	"strings"
	"testing"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/types"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// dbruntimeDB is the package's test database handle.
func dbruntimeDB() *gorm.DB { return dbruntime.DB }

// storedRBAC pairs an enforcer with a real adapter over a real table, so a test
// can assert what a write left in storage as well as in memory.
//
// The enforcer is given the same adapter rather than a null one, so that
// reloading the model actually reads the table back.
func storedRBAC(tb testing.TB, table string) (*rbac, *adapter) {
	tb.Helper()
	store := newPolicyTable(tb, table)

	enforcer, err := newEnforcer(store)
	require.NoError(tb, err)

	return &rbac{enforcer: enforcer, adapter: store, mu: &enforcerMu}, store
}

// memoryRules returns every rule the in-memory model holds, in the same shape
// storedRules reports, so the two can be compared directly.
func memoryRules(tb testing.TB, r *rbac) []string {
	tb.Helper()
	rules := make([]string, 0)
	for _, sec := range policySections {
		for ptype, ast := range r.enforcer.GetModel()[sec] {
			for _, rule := range ast.Policy {
				rules = append(rules, ptype+":"+strings.Join(rule, ","))
			}
		}
	}
	return rules
}

// storedRules returns every rule the table holds, as the loader sees it.
func storedRules(tb testing.TB, store *adapter) []string {
	tb.Helper()
	m, err := casbinmodel.NewModelFromString(string(modelData))
	require.NoError(tb, err)
	require.NoError(tb, store.LoadPolicyCtx(context.Background(), m))

	rules := make([]string, 0)
	for _, sec := range policySections {
		for ptype, ast := range m[sec] {
			for _, rule := range ast.Policy {
				rules = append(rules, ptype+":"+strings.Join(rule, ","))
			}
		}
	}
	return rules
}

// TestMutateRejectsEmptyValues covers the case where a caller passes an empty
// identifier. Storage and Casbin both read an empty filter field as "match
// anything", so the mutation would delete every rule of its kind rather than
// one, and it has to be refused before it reaches either.
func TestMutateRejectsEmptyValues(t *testing.T) {
	r, store := storedRBAC(t, "policy_reject_empty")
	ctx := context.Background()

	require.NoError(t, r.AssignRole(ctx, "tenant_a", "u1", "role_a"))
	require.NoError(t, r.AssignSystemRole(ctx, "u2", "system_root"))
	before := storedRules(t, store)
	require.Len(t, before, 2)

	t.Run("remove subject with no subject", func(t *testing.T) {
		err := r.RemoveSubject(ctx, "")
		require.ErrorIs(t, err, ErrEmptyPolicyValue)
		assert.ElementsMatch(t, before, storedRules(t, store),
			"an empty subject must not delete every assignment in the deployment")
	})

	t.Run("remove role with no role", func(t *testing.T) {
		err := r.RemoveRole(ctx, "tenant_a", "")
		require.ErrorIs(t, err, ErrEmptyPolicyValue)
		assert.ElementsMatch(t, before, storedRules(t, store))
	})

	t.Run("assign role with no role", func(t *testing.T) {
		err := r.AssignRole(ctx, "tenant_a", "u3", "")
		require.ErrorIs(t, err, ErrEmptyPolicyValue)
		assert.ElementsMatch(t, before, storedRules(t, store))
	})
}

// TestMutateNormalizesEmptyTenant covers the case where a caller omits the
// tenant. An omitted tenant means the default domain on every read, so a write
// has to store it under that same name: storing the empty string would leave a
// rule that no read looks for and that the loader rejects for being one token
// short of what its assertion declares.
func TestMutateNormalizesEmptyTenant(t *testing.T) {
	r, store := storedRBAC(t, "policy_default_tenant")
	ctx := context.Background()

	require.NoError(t, r.AssignRole(ctx, "", "u1", "role_a"))
	require.NoError(t, r.SetRolePermissions(ctx, "", "role_a", []types.Permission{
		{Object: "/api/things", Action: "GET"},
	}))

	assert.ElementsMatch(t, []string{
		"g:u1,role_a," + DefaultTenant,
		"p:" + DefaultTenant + ",role_a,/api/things,GET,allow",
	}, storedRules(t, store), "an omitted tenant must be stored as the default one")

	// The rules load back into a fresh model, which is what a restart does.
	m, err := casbinmodel.NewModelFromString(string(modelData))
	require.NoError(t, err)
	require.NoError(t, store.LoadPolicyCtx(ctx, m),
		"rules written with an omitted tenant must survive a reload")
}

// TestLoadPolicyReportsUnusableRowsInsteadOfPanicking covers a policy table
// holding a row the model cannot place. The ptype column is NOT NULL with an
// empty default and nothing constrains it to the declared ptypes, so both cases
// are reachable from storage alone — through a restore, a migration, or a manual
// repair. Casbin derives the section from the first byte of the ptype, which
// panics on an empty one, so the row has to be caught before it gets there.
func TestLoadPolicyReportsUnusableRowsInsteadOfPanicking(t *testing.T) {
	_, store := storedRBAC(t, "policy_unusable_rows")
	ctx := context.Background()

	insert := func(ptype string) {
		require.NoError(t, dbruntimeDB().Table(store.table).
			Create(map[string]any{"ptype": ptype, "v0": "u1", "v1": "role_a", "v2": "default"}).Error)
	}

	for name, ptype := range map[string]string{"empty ptype": "", "unknown ptype": "zz"} {
		t.Run(name, func(t *testing.T) {
			insert(ptype)
			m, err := casbinmodel.NewModelFromString(string(modelData))
			require.NoError(t, err)

			err = store.LoadPolicyCtx(ctx, m)
			require.Error(t, err, "an unusable row must fail the load rather than crash it")
			assert.Contains(t, err.Error(), "casbin policy row",
				"the error must name the row so it can be repaired")

			require.NoError(t, dbruntimeDB().Table(store.table).Where("ptype = ?", ptype).
				Delete(map[string]any{}).Error)
		})
	}
}

// failAfterAdapter delegates to a real adapter and fails a chosen write, so a
// test can observe what a partly applied mutation set leaves behind.
type failAfterAdapter struct {
	*adapter

	// failOn is the 1-based position of the write that fails.
	failOn int
	calls  int
}

var errAdapterWrite = errors.New("adapter write failed")

func (a *failAfterAdapter) fails() bool {
	a.calls++
	return a.calls == a.failOn
}

func (a *failAfterAdapter) AddPoliciesCtx(ctx context.Context, sec, ptype string, rules [][]string) error {
	if a.fails() {
		return errAdapterWrite
	}
	return a.adapter.AddPoliciesCtx(ctx, sec, ptype, rules)
}

func (a *failAfterAdapter) RemovePoliciesCtx(ctx context.Context, sec, ptype string, rules [][]string) error {
	if a.fails() {
		return errAdapterWrite
	}
	return a.adapter.RemovePoliciesCtx(ctx, sec, ptype, rules)
}

func (a *failAfterAdapter) RemoveFilteredPolicyCtx(
	ctx context.Context, sec, ptype string, fieldIndex int, fieldValues ...string,
) error {
	if a.fails() {
		return errAdapterWrite
	}
	return a.adapter.RemoveFilteredPolicyCtx(ctx, sec, ptype, fieldIndex, fieldValues...)
}

// TestMutateIsAtomicWithoutACallerTransaction covers a caller that writes
// policies outside any transaction, which is what seeding and the
// application-declared permission set both do.
//
// Replacing a permission set is a delete and an insert. Left to autocommit
// separately, a failed insert commits the delete on its own and strips the role
// of every permission, while the in-memory half never runs and keeps serving the
// old set — a divergence that outlives the process and that comparing stored
// rules against their records cannot see.
func TestMutateIsAtomicWithoutACallerTransaction(t *testing.T) {
	r, store := storedRBAC(t, "policy_atomic_write")
	ctx := context.Background()

	granted := []types.Permission{{Object: "/api/things", Action: "GET"}}
	require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", granted))
	before := storedRules(t, store)
	require.Len(t, before, 1)

	// The replacement deletes first and inserts second; the insert fails.
	r.adapter = &failAfterAdapter{adapter: store, failOn: 2}
	err := r.SetRolePermissions(ctx, "tenant_a", "role_a", []types.Permission{
		{Object: "/api/things", Action: "POST"},
	})

	require.ErrorIs(t, err, errAdapterWrite)
	assert.ElementsMatch(t, before, storedRules(t, store),
		"a failed replacement must not leave the role stripped of its permissions")

	allowed, err := r.Authorize(ctx, "tenant_a", "u1", "/api/things", "GET")
	require.NoError(t, err)
	assert.False(t, allowed, "no subject holds the role, so nothing changed in memory either")
}

// failLoadAdapter delegates to a real adapter and fails every policy read, so a
// test can observe a process that cannot put itself back in step with storage.
type failLoadAdapter struct{ *adapter }

var errAdapterLoad = errors.New("adapter load failed")

func (a *failLoadAdapter) LoadPolicyCtx(context.Context, casbinmodel.Model) error {
	return errAdapterLoad
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
	require.NoError(t, r.applyToStore(context.Background(), []policyMutation{
		addRules("p", "p", []string{"tenant_a", "role_a", "/api/things", "GET", "allow"}),
	}))
	require.Len(t, storedRules(t, store), 1)
	require.Empty(t, memoryRules(t, r), "the memory half has not run")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// Without this the test would pass on a database that ignores the context,
	// proving nothing about the reload.
	probe, err := casbinmodel.NewModelFromString(string(modelData))
	require.NoError(t, err)
	require.Error(t, store.LoadPolicyCtx(canceled, probe),
		"the trigger context has to be dead for this test to mean anything")

	require.NoError(t, r.recover(canceled, nil))
	assert.Equal(t, storedRules(t, store), memoryRules(t, r),
		"a canceled trigger must not cancel the reload it triggered")
}

// TestReloadKeepsTheEnforcerInvariants guards the enforcer state a reload has to
// leave alone.
//
// The reload goes through the enforcer's own load, which swaps the model
// through applyModifiedModel. Rebuilding it any way that reaches SetModel
// re-initializes the enforcer instead: the function map is rebuilt, dropping
// the matcher function this package registers so that no decision can resolve
// it, and autosave comes back on, letting Casbin write policies behind
// mutate's back. Both faults are silent where they are introduced.
func TestReloadKeepsTheEnforcerInvariants(t *testing.T) {
	r, store := storedRBAC(t, "policy_reload_invariants")
	ctx := context.Background()

	require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", []types.Permission{
		{Object: "/api/things/{id}", Action: "GET"},
	}))
	require.NoError(t, r.AssignRole(ctx, "tenant_a", "u1", "role_a"))
	require.NoError(t, r.ReloadPolicies(ctx))

	allowed, err := r.Authorize(ctx, "tenant_a", "u1", "/api/things/1", "GET")
	require.NoError(t, err)
	assert.True(t, allowed, "the matcher function has to survive a reload")

	before := storedRules(t, store)
	_, err = r.enforcer.AddPolicy("tenant_a", "role_b", "/api/other", "GET", "allow")
	require.NoError(t, err)
	assert.Equal(t, before, storedRules(t, store), "autosave has to stay off after a reload")
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
		enforcer, err := newEnforcer(failing)
		require.NoError(t, err)
		r := &rbac{enforcer: enforcer, adapter: failing, mu: &enforcerMu}

		require.ErrorIs(t, r.recover(context.Background(), nil), errAdapterLoad)
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

// gaugeValue reads what a gauge currently holds, which is the only way to tell
// that the polarity of a published state is the right way round.
func gaugeValue(tb testing.TB, gauge prometheus.Gauge) float64 {
	tb.Helper()
	var metric dto.Metric
	require.NoError(tb, gauge.Write(&metric))
	return metric.GetGauge().GetValue()
}

// TestApplyToModelRebuildsWhenOvertaken covers two writes to the same role
// whose memory halves run in the opposite order from their commits.
//
// The commit settles which write storage kept; which goroutine then reaches the
// enforcer lock first does not, and nothing orders the two. Replaying the
// older batch at that point would leave the process deciding from a permission
// set storage has already replaced, for as long as it runs — and comparing
// stored rules against the records they come from cannot see it, because
// storage is not the half that is wrong.
func TestApplyToModelRebuildsWhenOvertaken(t *testing.T) {
	r, store := storedRBAC(t, "policy_overtaken")
	ctx := context.Background()

	// The older write, held back before its memory half runs.
	older := []policyMutation{
		removeFiltered("p", "p", 0, "tenant_a", "role_a"),
		addRules("p", "p", []string{"tenant_a", "role_a", "/api/old", "GET", "allow"}),
	}
	require.NoError(t, r.applyToStore(ctx, older))
	olderSequence := policySequence.Add(1)

	// The newer write lands in storage and in memory while the older one waits.
	require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", []types.Permission{
		{Object: "/api/new", Action: "GET"},
	}))
	require.Equal(t, []string{"p:tenant_a,role_a,/api/new,GET,allow"}, storedRules(t, store))
	require.Equal(t, []string{"p:tenant_a,role_a,/api/new,GET,allow"}, memoryRules(t, r))

	// The older memory half finally runs, out of order.
	require.NoError(t, r.applyToModel(ctx, olderSequence, older))

	assert.Equal(t, storedRules(t, store), memoryRules(t, r),
		"an overtaken write must not put memory back to a permission set storage has replaced")
}
