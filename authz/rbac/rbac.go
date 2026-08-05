package rbac

import (
	"strings"
	"sync"

	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
)

var (
	// policyRules is the policy set this process decides from, nil until Init
	// installs one. policyStore is where its changes are persisted.
	policyRules *policySet
	policyStore *adapter

	// policyMu guards the policy set and everything derived from it — the
	// decision index and the role graph — so a reader sees the three change
	// as one.
	policyMu sync.RWMutex
)

type rbac struct {
	// adapter is where mutate drives the database half of a write; the
	// in-memory half is deferred until the transaction commits.
	adapter policyStorage

	mu *sync.RWMutex
}

// RBAC returns the authorization entry point this process decides from.
//
// Before Init has installed a policy set — RBAC disabled, or a process that
// never bootstrapped — it answers with noop, which denies every request and
// refuses every write.
//
// The two package variables are read together under the lock they are
// installed under. Read outside it, a caller could be handed a policy set
// whose adapter has not been assigned yet, and the first write through it
// would dereference a nil adapter.
func RBAC() types.RBAC {
	policyMu.RLock()
	rules, store := policyRules, policyStore
	policyMu.RUnlock()

	if rules == nil {
		return noop{}
	}
	return &rbac{
		adapter: store,
		mu:      &policyMu,
	}
}

// rebuildDerived rebuilds everything decided from the policy set — the
// decision index and the role graph — from set. The caller holds the policy
// write lock, which is what keeps the set and its derivations changing as one.
func rebuildDerived(set *policySet) {
	if set == nil {
		policyIndex = nil
		policyGraph = nil
		return
	}
	policyIndex = buildDecisionIndex(set)
	policyGraph = buildRoleGraph(set)
}

// normalizeTenant resolves the authorization domain an operation acts in.
//
// An unset tenant means the default domain. That is the whole arrangement in a
// single-tenant deployment, where no resolver is configured and every rule is
// stored against tenant.Default, so an empty argument is an ordinary way of
// saying "here" rather than a mistake to refuse.
//
// Every entry point taking a tenant applies it, reads and writes alike. Reading
// and writing under different names for one domain is how a rule ends up written
// where nothing will look for it.
func normalizeTenant(id string) string {
	if id = strings.TrimSpace(id); id != "" {
		return id
	}
	return tenant.Default
}
