package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestReadyzAnswersOKWhileServing covers the ordinary answer of a process
// that is running and waiting on nothing.
func TestReadyzAnswersOKWhileServing(t *testing.T) {
	require.Equal(t, http.StatusOK, probeStatus(t, readyz, "/-/readyz"))
}

// TestReadyzFailsWhileDraining pins what Drain buys: the process stops
// claiming it should receive traffic, before anything is torn down.
func TestReadyzFailsWhileDraining(t *testing.T) {
	drainForTest(t)

	require.Equal(t, http.StatusServiceUnavailable, probeStatus(t, readyz, "/-/readyz"))
}

// TestHealthzStaysOKWhileDraining is the other half of the split: liveness
// must keep answering during a deliberate shutdown, or an orchestrator would
// read the drain as a fault and restart the container it is trying to stop.
func TestHealthzStaysOKWhileDraining(t *testing.T) {
	drainForTest(t)

	require.Equal(t, http.StatusOK, probeStatus(t, healthz, "/-/healthz"))
}

// drainForTest drains the process for the test and takes the mark back when
// the test ends, the mark being the process's.
func drainForTest(t *testing.T) {
	t.Helper()

	Drain()
	t.Cleanup(func() { draining.Store(false) })
}

// probeStatus runs one probe endpoint and reports the status it answered.
func probeStatus(t *testing.T, endpoint func(*gin.Context), path string) int {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	endpoint(c)
	return recorder.Code
}
