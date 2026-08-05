package rbac

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadPolicyReportsUnusableRowsInsteadOfPanicking covers a policy table
// holding a row no rule kind declares. The ptype column is NOT NULL with an
// empty default and nothing constrains it to the declared kinds, so both cases
// are reachable from storage alone — through a restore, a migration, or a
// manual repair. A rule filed under either would decide nothing while looking
// stored, so the row has to fail the load, named so it can be repaired.
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
			_, err := store.loadPolicies(ctx)
			require.Error(t, err, "an unusable row must fail the load rather than crash it")
			assert.Contains(t, err.Error(), "authz policy row",
				"the error must name the row so it can be repaired")

			require.NoError(t, dbruntimeDB().Table(store.table).Where("ptype = ?", ptype).
				Delete(map[string]any{}).Error)
		})
	}
}
