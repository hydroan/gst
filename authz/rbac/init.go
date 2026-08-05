package rbac

import (
	"context"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types/consts"
)

var defaultSystemRootSubjects = []string{
	consts.AUTHZ_USER_ROOT,
}

var defaultSystemRole = consts.AUTHZ_SYSTEM_ROLE_ROOT

// newEnforcer builds the enforcer modelData describes, with the invariants the
// package depends on already in place.
//
// Construction, autosave and the matcher functions are one step on purpose. The
// enforcer compiles its matcher once and caches it together with the function
// map it was built from, so a function registered after the first Enforce is
// never seen by the cached expression, and the symptom is a matcher that cannot
// resolve pathMatch at all rather than a slow one. Leaving that ordering to
// each construction site to remember is the kind of convention the second site
// gets wrong.
func newEnforcer(store persist.ContextAdapter) (*casbin.ContextEnforcer, error) {
	model, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create casbin model")
	}
	contextEnforcer, err := casbin.NewContextEnforcer(model, store)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create casbin enforcer")
	}
	enforcer, ok := contextEnforcer.(*casbin.ContextEnforcer)
	if !ok {
		return nil, errors.New("failed to create context casbin enforcer")
	}

	enforcer.AddFunction(matcherFuncPathMatch, pathMatchFunc)
	// Writes go through mutate, which drives the adapter itself so it can split
	// the database half from the in-memory half. Casbin's own persistence would
	// do both at once, which is what leaves memory ahead of a rolled back
	// transaction.
	enforcer.EnableAutoSave(false)

	return enforcer, nil
}

// Init initializes the tenant-aware Casbin enforcer when RBAC is enabled.
//
// The enforcer is built and made ready before it is published, and published
// together with its adapter. A reader reaches RBAC as soon as either package
// variable is assigned, so assigning them as they are produced would hand out an
// enforcer that still has enforcement off, or one whose writes have no adapter
// to go through.
func Init() error {
	if !config.App.Auth.RBACEnabled {
		return nil
	}

	// The adapter is told not to migrate: the policy table belongs to the
	// registered CasbinRule model, so it is created and indexed by the same
	// migration path as every other table. Letting the adapter migrate too would
	// mean two definitions of one table, and would issue DDL at startup even
	// where the framework deliberately leaves schema changes to gg migrate.
	policyAdapter := newAdapter(database.DB(), policyTable)
	policyEnforcer, err := newEnforcer(policyAdapter)
	if err != nil {
		return err
	}

	// No logger is given to the enforcer, and none should be. Of the events
	// Casbin reports, the only one it raises along any path this package takes
	// is the enforcement event, once per decision and so once per request —
	// carrying neither the tenant, nor what allowed the request, nor the trace
	// it belongs to, all of which the authz middleware already writes for the
	// same decision. The rest are raised from entry points this package never
	// calls: policy writes go through mutate rather than the enforcer, and the
	// reload goes through LoadPolicyCtx, which reports nothing at all.
	policyEnforcer.EnableEnforce(true)
	installEnforcer(policyEnforcer, policyAdapter)

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

// installEnforcer publishes the enforcer and the adapter its writes go through
// as one step, so that no reader observes one without the other. See RBAC.
func installEnforcer(policyEnforcer *casbin.ContextEnforcer, store *adapter) {
	enforcerMu.Lock()
	defer enforcerMu.Unlock()

	enforcer = policyEnforcer
	policyStore = store
}
