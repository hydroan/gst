// Package component registers the application's long-running work: loops
// that run on every replica for the life of the process, each in a file of
// its own. A component observes or serves its own process — a cache it
// keeps warm, figures it reports; work the deployment must do once belongs
// in cronjob.
package component

import "github.com/hydroan/gst/component"

func init() {
	component.Register(countRecords, "record_count")
	component.Register(reportRuntime, "runtime_report")
}
