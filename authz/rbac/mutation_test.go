package rbac

import (
	"context"
	"strings"
	"testing"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// dbruntimeDB is the package's test database handle.
func dbruntimeDB() *gorm.DB { return dbruntime.DB }

// storedRBAC pairs the in-memory enforcer with a real adapter over a real
// table, so a test can assert what a write left in storage as well as in memory.
func storedRBAC(tb testing.TB, table string) (*rbac, *adapter) {
	tb.Helper()
	store := newPolicyTable(tb, table)
	return &rbac{
		enforcer: newTestEnforcer(tb, 0),
		adapter:  store,
		mu:       &enforcerMu,
	}, store
}

// storedRules returns every rule the table holds, as the loader sees it.
func storedRules(tb testing.TB, store *adapter) []string {
	tb.Helper()
	m, err := casbinmodel.NewModelFromString(string(modelData))
	require.NoError(tb, err)
	require.NoError(tb, store.LoadPolicyCtx(context.Background(), m))

	rules := make([]string, 0)
	for _, sec := range []string{"p", "g"} {
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
