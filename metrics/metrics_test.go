package prommetrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/cockroachdb/errors"
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
	require.Error(t, RegisterDBStats(nil, "nil-db"), "a nil handle has no pool to report")

	db := sql.OpenDB(nopConnector{})
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, RegisterDBStats(db, "register-db-stats-test"))

	// Registering the same name again replaces the previous collector instead
	// of failing, so re-initialization stays idempotent.
	replacement := sql.OpenDB(nopConnector{})
	t.Cleanup(func() { _ = replacement.Close() })
	require.NoError(t, RegisterDBStats(replacement, "register-db-stats-test"))

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
