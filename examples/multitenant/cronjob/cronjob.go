// Package cronjob registers the application's scheduled work: a job is a
// function taking a context, in a file of its own beside its schedule and
// name. A job runs outside any request, so no authorization decided the
// tenant it acts in: its context acts in the default tenant until it says
// otherwise, and the digest acts in every tenant in turn through tenant.In,
// while the reconciliation reads across all of them.
package cronjob

import "github.com/hydroan/gst/cronjob"

func init() {
	cronjob.Register(digestTickets, digestTicketsSpec, digestTicketsName)
	cronjob.Register(reconcilePolicies, reconcilePoliciesSpec, reconcilePoliciesName)
}
