package rbac

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMutateRejectsEmptyValues covers the case where a caller passes an empty
// identifier. Storage and Casbin both read an empty filter field as "match
// anything", so the mutation would delete every rule of its kind rather than
// one, and it has to be refused before it reaches either.
//
// A permission missing its object or its action is the same fault reached
// through a whole-set replacement, and refusing it there matters twice over: the
// replacement deletes the role's current permissions before writing the new
// ones, so an entry passed over instead of refused would leave a caller whose
// entries are all empty having revoked everything and been told it succeeded.
func TestMutateRejectsEmptyValues(t *testing.T) {
	r, store := storedRBAC(t, "policy_reject_empty")
	ctx := context.Background()

	require.NoError(t, r.AssignRole(ctx, "tenant_a", "u1", "role_a"))
	require.NoError(t, r.AssignSystemRole(ctx, "u2", "system_root"))
	require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", []types.Permission{
		{Object: "/api/things", Action: "GET"},
	}))
	before := storedRules(t, store)
	require.Len(t, before, 3)

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

	t.Run("set role permissions with an empty entry", func(t *testing.T) {
		for name, permissions := range map[string][]types.Permission{
			"no object":                {{Action: "GET"}},
			"no action":                {{Object: "/api/other"}},
			"beside a populated entry": {{Object: "/api/other", Action: "POST"}, {Action: "GET"}},
		} {
			t.Run(name, func(t *testing.T) {
				err := r.SetRolePermissions(ctx, "tenant_a", "role_a", permissions)
				require.ErrorIs(t, err, ErrEmptyPolicyValue)
				assert.ElementsMatch(t, before, storedRules(t, store),
					"a refused replacement must not have revoked what the role already holds")
			})
		}
	})

	// Having no entry at all is not an empty value but the caller stating an
	// empty permission set, which the replacement has to keep honoring.
	t.Run("set role permissions with no entries", func(t *testing.T) {
		require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", nil))
		assert.NotContains(t, storedRules(t, store), "p:tenant_a,role_a,/api/things,GET,allow")
	})
}

// TestAssignRefusesASubjectNamedLikeItsRole covers an assignment that could
// never take effect: the matcher and the role-graph branches refuse a
// self-match, so storing the rule would report success for a grant that
// decides nothing.
func TestAssignRefusesASubjectNamedLikeItsRole(t *testing.T) {
	r, store := storedRBAC(t, "policy_subject_named_role")
	ctx := context.Background()

	require.ErrorIs(t, r.AssignRole(ctx, "tenant_a", "role_a", "role_a"), ErrSubjectIsRole)
	require.ErrorIs(t, r.AssignSystemRole(ctx, "system_root", "system_root"), ErrSubjectIsRole)
	assert.Empty(t, storedRules(t, store), "a refused assignment must not have stored a rule")
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
		"g:u1,role_a," + tenant.Default,
		"p:" + tenant.Default + ",role_a,/api/things,GET,allow",
	}, storedRules(t, store), "an omitted tenant must be stored as the default one")

	// The rules load back into a fresh set, which is what a restart does.
	_, err := store.loadPolicies(ctx)
	require.NoError(t, err, "rules written with an omitted tenant must survive a reload")
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

func (a *failAfterAdapter) addPolicies(ctx context.Context, ptype string, rules [][]string) error {
	if a.fails() {
		return errAdapterWrite
	}
	return a.adapter.addPolicies(ctx, ptype, rules)
}

// The removal overrides intercept the counted entry points, which are the ones
// mutate drives; intercepting the Casbin-shaped methods would leave the writes
// under test reaching storage through the embedded adapter.
func (a *failAfterAdapter) removePoliciesCount(
	ctx context.Context, ptype string, rules [][]string,
) (int64, error) {
	if a.fails() {
		return 0, errAdapterWrite
	}
	return a.adapter.removePoliciesCount(ctx, ptype, rules)
}

func (a *failAfterAdapter) removeFilteredPolicyCount(
	ctx context.Context, ptype string, fieldIndex int, fieldValues ...string,
) (int64, error) {
	if a.fails() {
		return 0, errAdapterWrite
	}
	return a.adapter.removeFilteredPolicyCount(ctx, ptype, fieldIndex, fieldValues...)
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

	decision, err := r.Authorize(ctx, "tenant_a", "u1", "/api/things", "GET")
	allowed := decision.Allowed
	require.NoError(t, err)
	assert.False(t, allowed, "no subject holds the role, so nothing changed in memory either")
}

// TestMutateReloadsWhenARemovalDisagreesWithStorage covers the one comparison
// the two halves of a removal admit: how many rules went. The halves can part
// company without either erring — storage holding rules memory never saw, or
// memory holding rules storage never kept — and the caller's own write is the
// moment the disagreement becomes visible. It must not fail that caller, whose
// write landed; it must leave the process deciding from what storage holds.
func TestMutateReloadsWhenARemovalDisagreesWithStorage(t *testing.T) {
	r, store := storedRBAC(t, "policy_removal_disagreement")
	ctx := context.Background()

	t.Run("storage ahead of memory", func(t *testing.T) {
		// Two assignments memory does not know about, which is what an external
		// write or a passed-by concurrent insert leaves behind.
		_, err := r.applyToStore(ctx, []policyMutation{
			addRules(tenantRoleGrouping, []string{"u1", "role_a", "tenant_a"}, []string{"u2", "role_a", "tenant_a"}),
		})
		require.NoError(t, err)
		require.Empty(t, memoryRules(t, r))

		// Removing one deletes a stored row and no in-memory rule.
		require.NoError(t, r.UnassignRole(ctx, "tenant_a", "u1", "role_a"))

		assert.Equal(t, storedRules(t, store), memoryRules(t, r),
			"the disagreement has to end in a reload that puts memory in step")
		assert.Contains(t, memoryRules(t, r), "g:u2,role_a,tenant_a",
			"the reload has to surface the rules memory was missing")
	})

	t.Run("memory ahead of storage", func(t *testing.T) {
		// Two assignments storage never kept: written through the enforcer,
		// whose autosave is off.
		for _, subject := range []string{"u3", "u4"} {
			seed(t, tenantRoleGrouping, []string{subject, "role_b", "tenant_a"})
		}

		// Removing one deletes an in-memory rule and no stored row.
		require.NoError(t, r.UnassignRole(ctx, "tenant_a", "u3", "role_b"))

		assert.Equal(t, storedRules(t, store), memoryRules(t, r),
			"the reload has to drop every rule storage does not hold")
	})
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
		removeFiltered("p", 0, "tenant_a", "role_a"),
		addRules("p", []string{"tenant_a", "role_a", "/api/old", "GET", "allow"}),
	}
	olderCounts, err := r.applyToStore(ctx, older)
	require.NoError(t, err)
	olderSequence := policySequence.Add(1)

	// The newer write lands in storage and in memory while the older one waits.
	require.NoError(t, r.SetRolePermissions(ctx, "tenant_a", "role_a", []types.Permission{
		{Object: "/api/new", Action: "GET"},
	}))
	require.Equal(t, []string{"p:tenant_a,role_a,/api/new,GET,allow"}, storedRules(t, store))
	require.Equal(t, []string{"p:tenant_a,role_a,/api/new,GET,allow"}, memoryRules(t, r))

	// The older memory half finally runs, out of order.
	require.NoError(t, r.applyToModel(ctx, olderSequence, older, olderCounts))

	assert.Equal(t, storedRules(t, store), memoryRules(t, r),
		"an overtaken write must not put memory back to a permission set storage has replaced")
}
