package configx

import "github.com/hydroan/gst/config"

// Jobs holds what the example's scheduled work reads from configuration.
type Jobs struct {
	// SlowSeconds is how long a round of the slow job takes. Raising it past
	// the job's period is how the deployment is made to show what a round
	// that overruns its schedule looks like: the instants that pass while it
	// runs are skipped, and the round says so while it is still running.
	SlowSeconds int `json:"slow_seconds" mapstructure:"slow_seconds" default:"20"`
}

func init() {
	config.Register[Jobs]()
}
