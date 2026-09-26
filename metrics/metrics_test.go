package prommetrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// nopConnector satisfies sql.OpenDB without a registered driver: the stats
// collector only reads pool counters, so no connection is ever made.
type nopConnector struct{}

func (nopConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("nop connector never connects")
}
func (nopConnector) Driver() driver.Driver { return nil }

func TestRegisterDBStats(t *testing.T) {
	require.Error(t, prommetrics.RegisterDBStats(nil, "nil-db"), "a nil handle has no pool to report")

	db := sql.OpenDB(nopConnector{})
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, prommetrics.RegisterDBStats(db, "register-db-stats-test"))

	// Registering the same name again replaces the previous collector instead
	// of failing, so re-initialization stays idempotent.
	replacement := sql.OpenDB(nopConnector{})
	t.Cleanup(func() { _ = replacement.Close() })
	require.NoError(t, prommetrics.RegisterDBStats(replacement, "register-db-stats-test"))

	// The registered collector serves pool gauges labeled with the db name.
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	found := false
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "db_name" && label.GetValue() == "register-db-stats-test" {
					found = true
				}
			}
		}
	}
	require.True(t, found, "gathered metrics should carry the registered db_name label")
}

// TestInitServesTheRequestDurationHistogram pins that the latency histogram
// the access logger observes into is registered by Init and so gathered: a
// collector built but left off the registration list is observed into on
// every request and served to nobody. The name it is gathered under pins
// the naming of every metric, gst_backend_<name>, with the underscores
// Prometheus joins the parts with and no others.
func TestInitServesTheRequestDurationHistogram(t *testing.T) {
	if err := prommetrics.Init(); err != nil {
		// Init registers into the process-wide default registry, so a second
		// run in one process is refused as duplicate registration; what the
		// first run registered is still served.
		var duplicate prometheus.AlreadyRegisteredError
		require.ErrorAs(t, err, &duplicate)
	}
	prommetrics.HTTPRequestDuration.WithLabelValues(http.MethodGet, "/probe", "200").Observe(0.1)

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(families))
	for _, family := range families {
		names = append(names, family.GetName())
	}
	require.Contains(t, names, "gst_backend_http_request_duration_seconds")
}
