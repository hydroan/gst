package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// probeStatus runs one probe endpoint and reports the status it answered.
func probeStatus(t *testing.T, endpoint func(*gin.Context), path string) int {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	endpoint(c)
	return recorder.Code
}

// TestReadyzAnswersOKWhileServing covers the ordinary answer of a process
// that is running and waiting on nothing.
func TestReadyzAnswersOKWhileServing(t *testing.T) {
	p := new(probe)

	require.Equal(t, http.StatusOK, probeStatus(t, p.Readyz, "/-/readyz"))
}

// TestReadyzFailsWhileDraining pins what Drain buys: the process stops
// claiming it should receive traffic, before anything is torn down.
func TestReadyzFailsWhileDraining(t *testing.T) {
	p := new(probe)
	p.Drain()

	require.Equal(t, http.StatusServiceUnavailable, probeStatus(t, p.Readyz, "/-/readyz"))
}

// TestHealthzStaysOKWhileDraining is the other half of the split: liveness
// must keep answering during a deliberate shutdown, or an orchestrator would
// read the drain as a fault and restart the container it is trying to stop.
func TestHealthzStaysOKWhileDraining(t *testing.T) {
	p := new(probe)
	p.Drain()

	require.Equal(t, http.StatusOK, probeStatus(t, p.Healthz, "/-/healthz"))
}
