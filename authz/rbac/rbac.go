package rbac

import (
	"strings"
	"sync"

	"github.com/casbin/casbin/v3"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
)

var (
	enforcer    *casbin.ContextEnforcer
	policyStore *adapter
	enforcerMu  sync.RWMutex
)

type rbac struct {
	enforcer *casbin.ContextEnforcer

	// adapter is held directly rather than reached through the enforcer:
	// mutate drives the database half of a write on its own so it can defer the
	// in-memory half until the transaction commits.
	adapter policyStorage

	mu *sync.RWMutex
}

// RBAC returns the authorization entry point this process decides from.
//
// Before Init has installed an enforcer — RBAC disabled, or a process that never
// bootstrapped — it answers with noop, which denies every request and refuses
// every write.
//
// The two package variables are read together under the lock they are installed
// under. Read outside it, a caller could be handed an enforcer whose adapter has
// not been assigned yet, and the first write through it would dereference a nil
// adapter.
func RBAC() types.RBAC {
	enforcerMu.RLock()
	policyEnforcer, store := enforcer, policyStore
	enforcerMu.RUnlock()

	if policyEnforcer == nil {
		return noop{}
	}
	return &rbac{
		enforcer: policyEnforcer,
		adapter:  store,
		mu:       &enforcerMu,
	}
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
