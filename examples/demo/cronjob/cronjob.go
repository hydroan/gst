// Package cronjob registers the application's scheduled work: a job is a
// function taking a context, in a file of its own beside its schedule and
// name, and the database chain works on that context the way it does on a
// request's. Each instant of a job registered with Register runs once
// across the replicas of the deployment; one registered with
// RegisterPerInstance runs on every replica, for what each process keeps
// for itself.
package cronjob

import "github.com/hydroan/gst/cronjob"

func init() {
	cronjob.Register(purgeAudits, purgeAuditsSpec, purgeAuditsName)
	cronjob.Register(reportAudits, reportAuditsSpec, reportAuditsName)
	cronjob.RegisterPerInstance(countRecords, countRecordsSpec, countRecordsName)
}
