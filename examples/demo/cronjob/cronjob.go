// Package cronjob registers the application's scheduled work: a job is a
// function taking a context, in a file of its own beside its schedule and
// name, and the database chain works on that context the way it does on a
// request's. Each instant of a job runs once across the replicas of the
// deployment.
package cronjob

import "github.com/hydroan/gst/cronjob"

func init() {
	cronjob.Register(purgeAudits, purgeAuditsSpec, purgeAuditsName)
	cronjob.Register(reportAudits, reportAuditsSpec, reportAuditsName)
}
