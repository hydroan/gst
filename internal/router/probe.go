package router

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// The probes answer the two questions an orchestrator asks about a process:
// whether it is alive at all (see healthz), and whether it should be sent
// traffic (see readyz), which it should not once its shutdown began (see
// Drain).

// draining records that shutdown began. It is written once, by Drain, and
// read by every readiness request after that.
var draining atomic.Bool

// healthz reports that the process is alive.
//
// It answers OK for as long as the process runs at all, shutdown included: an
// orchestrator restarts a container whose liveness probe fails, and a process
// that is deliberately draining must not be restarted for draining. Nor does
// it check any dependency — restarting a process cannot fix a database, and
// tying liveness to one turns a recoverable outage into every replica
// restarting at once.
func healthz(c *gin.Context) {
	c.Status(http.StatusOK)
}

// readyz reports whether this process should receive traffic.
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
func readyz(c *gin.Context) {
	if draining.Load() {
		c.String(http.StatusServiceUnavailable, "not ready")
		return
	}
	c.Status(http.StatusOK)
}

// Drain marks the process as shutting down, which fails readyz from the next
// request on. Bootstrap calls it when the process is told to stop — by a
// termination signal, or by a listener that failed — and then holds the
// process here for server.shutdown_delay: the mark is only worth anything if
// something has time to observe it before the listener closes.
func Drain() {
	draining.Store(true)
}
