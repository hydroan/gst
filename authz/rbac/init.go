package rbac

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types/consts"
)

var defaultSystemRootSubjects = []string{
	consts.AUTHZ_USER_ROOT,
}

var defaultSystemRole = consts.AUTHZ_SYSTEM_ROLE_ROOT

// Init loads the stored policy set and installs it, when RBAC is enabled.
//
// The set is read and made whole before it is published, and published
// together with its adapter and its derived state. A reader reaches RBAC as
// soon as the set is assigned, so assigning the pieces as they are produced
// would hand out a policy set whose writes have no adapter to go through, or
// decisions with no index to answer from.
func Init() error {
	if !config.App.Auth.RBACEnabled {
		return nil
	}

	// The adapter never migrates: the policy table belongs to the registered
	// AuthzRule model, so it is created and indexed by the same migration
	// path as every other table, and schema changes stay an explicit
	// "gg migrate" decision instead of a startup side effect.
	policyAdapter := newAdapter(database.DB(), policyTable)
	set, err := policyAdapter.loadPolicies(context.Background())
	if err != nil {
		return err
	}
	installPolicySet(set, policyAdapter)

	for _, subject := range defaultSystemRootSubjects {
		if err := RBAC().AssignSystemRole(context.Background(), subject, defaultSystemRole); err != nil {
			return errors.Wrapf(err, "failed to add default system role for %s", subject)
		}
	}

	// From here the process reconciles itself with storage on a schedule, which
	// is what bounds every staleness it cannot see: another replica's writes, a
	// manual repair, a restore. It runs for the rest of the process's life.
	startPeriodicReload()
	return nil
}

// installPolicySet publishes the policy set, the adapter its changes persist
// through and the state derived from it as one step, so that no reader
// observes one without the others. See RBAC.
func installPolicySet(set *policySet, store *adapter) {
	policyMu.Lock()
	defer policyMu.Unlock()

	policyRules = set
	policyStore = store
	rebuildDerived(set)
}
