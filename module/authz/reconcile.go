package authz

import (
	"context"

	serviceauthz "github.com/hydroan/gst/internal/service/authz"
)

// PolicyDrift is one disagreement between the stored authorization rules and
// the records they are derived from.
type PolicyDrift = serviceauthz.PolicyDrift

// PolicyReport is the outcome of ReconcilePolicies.
type PolicyReport = serviceauthz.PolicyReport

// ReconcilePolicies compares the stored authorization rules against the roles,
// menus, and role bindings they are derived from, and reports the
// disagreements without repairing them.
//
// Use it as a diagnostic: a scheduled check whose drift count is worth alerting
// on, or a manual run when the stored rules are suspected of having been
// changed outside the framework. Repair stays a human decision, because an
// orphaned rule is equally consistent with a projection bug and with a record
// deleted out from under access that is still wanted.
func ReconcilePolicies(ctx context.Context) (PolicyReport, error) {
	return serviceauthz.ReconcilePolicies(ctx)
}
