// Package postgres is the PostgreSQL dialect of the framework's database
// layer: Init opens the default connection when the configuration selects
// PostgreSQL, and New opens a handle from a configuration of the caller's.
package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/hydroan/gst/internal/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Init opens the default PostgreSQL connection when database.type selects
// PostgreSQL, and makes it the default database that database.Database and
// database.DB use; otherwise it does nothing.
func Init() error {
	cfg := config.App.Postgres
	if config.App.Database.Type != config.DBPostgres {
		return nil
	}

	db, err := New(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to connect to postgres")
	}
	zap.S().Infow("successfully connect to postgres", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database, "sslmode", cfg.SSLMode)
	return dbruntime.InitDatabase(db)
}

// New creates and returns a new PostgreSQL database connection with the given
// configuration. With tracing configured on, the returned handle carries the
// GORM OpenTelemetry tracing plugin, so application-held instances passed to
// DatabaseOn, SelectOn, UnionAllOn, TransactionOn, and CleanupOn are traced
// like the default database. The pool runs under the connection limits of the
// [database] configuration, the same ones the default handle runs under.
func New(cfg config.Postgres) (*gorm.DB, error) {
	// Statements run over pgx's simple protocol — no gorm PrepareStmt, no
	// pgx statement cache, no server-side statement state; buildDSN explains
	// why.
	// The write failures a client's data brings on come back as the gorm
	// sentinels through the translation installed below, the framework's own
	// table (see translate) first and the driver's after it; gorm's
	// TranslateError stays off, dbruntime.InstallErrorTranslation says why.
	dialector, err := open(cfg)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Gorm, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	if err = dbruntime.InstallErrorTranslation(db, translate); err != nil {
		return nil, err
	}
	pool, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get postgres db")
	}
	dbruntime.ConfigurePool(pool)
	dbruntime.InstallTracing(db)
	return attachReplicas(db, cfg)
}

// attachReplicas wires the configured read replicas into the handle and pins
// its default route to the primary; without replicas the handle is returned
// untouched. Same contract as the MySQL twin: replicas share every DSN
// setting with the primary and differ by address only, the Write pin keeps
// reads on the primary by default, and reads move only through PreferReplica
// models or WithReplica call sites. See WithReplica for the routing
// precedence.
func attachReplicas(db *gorm.DB, cfg config.Postgres) (*gorm.DB, error) {
	if len(cfg.Replicas) == 0 {
		return db, nil
	}
	dialectors := make([]gorm.Dialector, 0, len(cfg.Replicas))
	for _, endpoint := range cfg.Replicas {
		host, port, err := dbruntime.ParseReplicaEndpoint(endpoint)
		if err != nil {
			return nil, errors.Wrap(err, "postgres replicas")
		}
		replicaCfg := cfg
		replicaCfg.Host, replicaCfg.Port = host, port
		replicaCfg.Replicas = nil
		dialector, err := open(replicaCfg)
		if err != nil {
			return nil, errors.Wrap(err, "postgres replicas")
		}
		dialectors = append(dialectors, dialector)
	}
	return dbruntime.AttachResolver(db, dialectors)
}

// open returns the dialector of the database cfg names, over a connection
// pool of its own: the pool is opened here, not by the dialector, so that
// every connection of it is set up by readTimesInUTC.
func open(cfg config.Postgres) (gorm.Dialector, error) {
	connConfig, err := pgx.ParseConfig(buildDSN(cfg))
	if err != nil {
		return nil, errors.Wrap(err, "postgres connection string")
	}
	pool := stdlib.OpenDB(*connConfig, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		readTimesInUTC(conn.TypeMap())
		return nil
	}))
	return postgres.New(postgres.Config{Conn: pool}), nil
}

// readTimesInUTC makes the connection whose types are given hand back every
// time it reads, a timestamptz and a timestamp alike, located in UTC, the
// framework's one time base, the way the MySQL DSN's loc=UTC does. The
// driver hands a timestamptz back in the zone of the process, the same
// instant under another wall clock, so a time read on a host outside UTC and
// written out as it stands, formatted or encoded, would carry that host's
// offset.
func readTimesInUTC(types *pgtype.Map) {
	types.RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
	types.RegisterType(&pgtype.Type{Name: "timestamp", OID: pgtype.TimestampOID, Codec: &pgtype.TimestampCodec{ScanLocation: time.UTC}})
}

// buildDSN assembles the pgx connection string.
//
// default_query_exec_mode=simple_protocol sends every statement as one text
// query — one round trip, no server-side statement state. The framework's
// SQL comments carry the request's trace id (see the database package's
// comment.go), making every statement text request-unique, so a text-keyed
// statement cache — gorm's PrepareStmt or pgx's default statement caching —
// would never be reused and only grow.
//
// TimeZone=UTC pins the session time zone to the framework's one time base
// across dialects, the way the MySQL DSN pins loc=UTC. PostgreSQL stores a
// timestamptz as an instant but reads the session zone wherever text meets
// time: a timestamp string without an offset — the form URL time filters and
// time cursors bind, see gst.FilterTimeLayout — is taken as time in that
// zone, and to_char renders time buckets in it. Under any other zone, filters
// and cursors would shift by the offset and buckets would split days at that
// zone's midnight, so the zone is not configurable. The zone a time read
// comes back in is the driver's to pick, and readTimesInUTC picks it.
//
// Every configured value is written quoted (see dsnValue): the driver reads
// the string as keywords and values, and a space ends a value that is not
// quoted, so a password holding one would be cut there and the words after it
// read as further keywords.
func buildDSN(cfg config.Postgres) string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC default_query_exec_mode=simple_protocol",
		dsnValue(cfg.Host), dsnValue(cfg.Username), dsnValue(cfg.Password), dsnValue(cfg.Database), cfg.Port, dsnValue(cfg.SSLMode),
	)
}

// dsnValue writes s as a value of a keyword/value connection string: in
// single quotes, a backslash before every backslash and single quote inside.
// For example, p w'd becomes 'p w\'d'.
func dsnValue(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// errCodes are the errors of the constraints a client's data breaks that the
// driver's translation leaves as they are, keyed by the SQLSTATE code: a
// value too long for its column (22001) and a column the table requires a
// value for left NULL (23502); the driver's own table holds the duplicated
// key, the foreign key and the check constraint.
var errCodes = map[string]error{
	"22001": dberror.ErrValueTooLong,
	"23502": dberror.ErrNotNullViolated,
}

// translate translates a driver error whose code errCodes lists to its
// sentinel, the driver's text kept as the message, and nil for any other
// error, which goes to the driver's own translation (see
// dbruntime.InstallErrorTranslation).
func translate(err error) error {
	var driverErr *pgconn.PgError
	if !errors.As(err, &driverErr) {
		return nil
	}
	sentinel, found := errCodes[driverErr.Code]
	if !found {
		return nil
	}
	return errors.Wrap(sentinel, driverErr.Message)
}
