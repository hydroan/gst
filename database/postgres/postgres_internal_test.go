package postgres

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestBuildDSNPinsUTCAndTheSimpleProtocol(t *testing.T) {
	cfg := config.Postgres{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "sample",
		Username: "postgres",
		Password: "secret",
		SSLMode:  "disable",
	}
	require.Equal(t,
		"host='127.0.0.1' user='postgres' password='secret' dbname='sample' port=5432 sslmode='disable' TimeZone=UTC default_query_exec_mode=simple_protocol",
		buildDSN(cfg))
}

// TestBuildDSNCarriesEveryValueThroughTheParser pins that a configured value
// holding a space, a single quote or a backslash, a password above all,
// reaches the driver as configured: the driver reads the connection string as
// keywords and values, where a space ends a value that is not quoted.
func TestBuildDSNCarriesEveryValueThroughTheParser(t *testing.T) {
	cfg := config.Postgres{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "sample db",
		Username: "app user",
		Password: `p w'd\`,
		SSLMode:  "disable",
	}
	parsed, err := pgconn.ParseConfig(buildDSN(cfg))
	require.NoError(t, err)
	require.Equal(t, cfg.Host, parsed.Host)
	require.Equal(t, cfg.Database, parsed.Database)
	require.Equal(t, cfg.Username, parsed.User)
	require.Equal(t, cfg.Password, parsed.Password)
}
