package postgres

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestBuildDSN(t *testing.T) {
	cfg := config.Postgres{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "sample",
		Username: "postgres",
		Password: "secret",
		SSLMode:  "disable",
		TimeZone: "UTC",
	}
	require.Equal(t,
		"host=127.0.0.1 user=postgres password=secret dbname=sample port=5432 sslmode=disable TimeZone=UTC",
		buildDSN(cfg))
}
