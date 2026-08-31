package controller

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// probe answers the two questions an orchestrator asks about a process:
// whether it is alive at all, and whether it should be sent traffic.
type probe struct {
	// draining records that shutdown began. It is written once, from the
	// signal handler, and read by every readiness request after that.
	draining atomic.Bool
}

var Probe = new(probe)

// Healthz reports that the process is alive.
//
// It answers OK for as long as the process runs at all, shutdown included: an
// orchestrator restarts a container whose liveness probe fails, and a process
// that is deliberately draining must not be restarted for draining. Nor does
// it check any dependency — restarting a process cannot fix a database, and
// tying liveness to one turns a recoverable outage into every replica
// restarting at once.
func (*probe) Healthz(c *gin.Context) {
	c.Status(http.StatusOK)
}

// Readyz reports whether this process should receive traffic.
//
// It answers on one question only: is this instance shutting down. What it
// deliberately does not do is probe shared dependencies. Readiness decides
// whether to take one replica out of the pool, which only helps when the
// remaining replicas can carry the load — a shared database or cache fails
// for all of them at once, so probing it here would empty the pool entirely
// and hold it empty until every replica independently recovers. Dependency
// health belongs in metrics, where an alert can weigh it, not in a signal
// that reroutes traffic. Probes also run every few seconds on every replica,
// and must stay cheap enough to be answered that often.
func (p *probe) Readyz(c *gin.Context) {
	if p.draining.Load() {
		c.String(http.StatusServiceUnavailable, "not ready")
		return
	}
	c.Status(http.StatusOK)
}

// Drain marks the process as shutting down, which fails Readyz from the next
// request on. Bootstrap calls it when a termination signal arrives, and then
// holds the process here for server.shutdown_delay: the mark is only worth
// anything if something has time to observe it before the listener closes.
func (p *probe) Drain() {
	p.draining.Store(true)
}
