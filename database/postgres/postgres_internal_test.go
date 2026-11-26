package postgres

import (
	"fmt"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestReadTimesInUTCLocatesEveryTimeReadInUTC pins that a connection set up
// by readTimesInUTC hands back a timestamptz and a timestamp located in UTC,
// in the text format statements run over the simple protocol are answered in
// and in the binary one alike. Left to itself the driver locates a
// timestamptz in the zone of the process, which is UTC by name on no host.
func TestReadTimesInUTCLocatesEveryTimeReadInUTC(t *testing.T) {
	types := pgtype.NewMap()
	readTimesInUTC(types)
	instant := time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC)

	for name, tc := range map[string]struct {
		oid  uint32
		text string
	}{
		"timestamptz": {oid: pgtype.TimestamptzOID, text: "2024-01-10 10:00:00+02"},
		"timestamp":   {oid: pgtype.TimestampOID, text: "2024-01-10 08:00:00"},
	} {
		t.Run(name, func(t *testing.T) {
			var read time.Time
			require.NoError(t, types.Scan(tc.oid, pgtype.TextFormatCode, []byte(tc.text), &read))
			require.Equal(t, time.UTC, read.Location())
			require.True(t, instant.Equal(read), "%s", read)

			binary, err := types.Encode(tc.oid, pgtype.BinaryFormatCode, instant, nil)
			require.NoError(t, err)
			read = time.Time{}
			require.NoError(t, types.Scan(tc.oid, pgtype.BinaryFormatCode, binary, &read))
			require.Equal(t, time.UTC, read.Location())
			require.True(t, instant.Equal(read), "%s", read)
		})
	}

	// The driver's own types, for the contrast: the zone of the process.
	var read time.Time
	require.NoError(t, pgtype.NewMap().Scan(pgtype.TimestamptzOID, pgtype.TextFormatCode, []byte("2024-01-10 10:00:00+02"), &read))
	require.Equal(t, time.Local, read.Location())
}

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

// TestTranslateTellsTheKeyOfAUniqueViolation pins what a unique violation is
// translated to: the sentinel, with the constraint and the columns the
// detail lists; the driver's own error still behind it for a caller reading
// it; and the text the driver's own translation gives the sentinel, so the
// key is told and nothing else of the error changes. A detail naming no
// column leaves the columns out.
func TestTranslateTellsTheKeyOfAUniqueViolation(t *testing.T) {
	driverErr := &pgconn.PgError{
		Severity:       "ERROR",
		Code:           uniqueViolation,
		Message:        `duplicate key value violates unique constraint "uniq_records_code"`,
		Detail:         "Key (tenant_id, code)=(t1, a) already exists.",
		ConstraintName: "uniq_records_code",
	}

	translated := translate(driverErr)

	require.ErrorIs(t, translated, dberror.ErrDuplicatedKey)
	var duplicated *dberror.DuplicatedKeyError
	require.ErrorAs(t, translated, &duplicated)
	require.Equal(t, "uniq_records_code", duplicated.Constraint)
	require.Equal(t, []string{"tenant_id", "code"}, duplicated.Columns)
	var behind *pgconn.PgError
	require.ErrorAs(t, translated, &behind)
	require.Same(t, driverErr, behind)
	require.Equal(t, fmt.Errorf("%w: %w", gorm.ErrDuplicatedKey, driverErr).Error(), translated.Error()) //nolint:forbidigo // the wording of the driver's own translation, which the test holds the text to

	bare := &pgconn.PgError{Code: uniqueViolation, ConstraintName: "records_pkey"}
	require.ErrorAs(t, translate(bare), &duplicated)
	require.Equal(t, "records_pkey", duplicated.Constraint)
	require.Empty(t, duplicated.Columns)

	require.NoError(t, translate(errors.New("connection refused")))
}
