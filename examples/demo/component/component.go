// Package component registers the application's long-running work: loops
// that run on every replica for the life of the process, each in a file of
// its own. A component is a consumer loop, a poller or a watcher of its own
// process — here the one draining a redis channel and the one reporting
// the process's figures; work on a schedule belongs in cronjob, work the
// deployment must do once in a job registered there with Register.
package component

import "github.com/hydroan/gst/component"

func init() {
	component.Register(watchNotices, "notice_watch")
	component.Register(reportRuntime, "runtime_report")
}
