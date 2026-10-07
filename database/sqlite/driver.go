package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// driverName is the database/sql driver this package opens connections with:
// the stock sqlite3 driver extended with the SQL functions the framework's
// query surface needs, binding every time in UTC. Registered at package
// load, which is how database/sql drivers are installed.
const driverName = "gst_sqlite3"

// sqliteDriver is the driver registered under driverName: the stock driver
// with the REGEXP function on every connection (see registerRegexpFunc), its
// connections wrapped to bind every time in UTC (see utcConn).
var sqliteDriver = &utcDriver{SQLiteDriver: &sqlite3.SQLiteDriver{ConnectHook: registerRegexpFunc}}

func init() {
	sql.Register(driverName, sqliteDriver)
}

// utcDriver opens the stock driver's connections wrapped in utcConn.
type utcDriver struct {
	*sqlite3.SQLiteDriver
}

// Open opens a connection of the stock driver and wraps it (see utcConn).
func (d *utcDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := d.SQLiteDriver.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &utcConn{Conn: conn}, nil
}

// utcConn is a connection of the stock driver binding every time in UTC.
// SQLite stores a time as the text the driver formats, zone suffix included,
// and sorts and compares that text: a time written with the offset it came
// with sorts by the clock its writer kept rather than by its instant, and
// reads back in that offset, where MySQL and PostgreSQL store the instant.
// The framework's own timestamps are UTC already (see dbruntime.NowUTC);
// what a request or a service writes is converted here, the one place every
// statement's parameters pass through, so gorm's statements and raw SQL alike
// store the instant. The driver itself offers no way to do this: go-sqlite3
// formats a time it binds in the zone the value carries, and its _loc
// connection parameter governs reading alone, which is what the connection
// string's _loc=UTC covers, reading the text back in UTC (see buildDSN and
// memoryDSN).
//
// The stock connection is held as a driver.Conn and the optional interfaces
// database/sql looks for are forwarded one by one: embedding the stock type
// would not compile without cgo, where the driver is a stub whose connection
// type has none of these methods.
type utcConn struct {
	driver.Conn
}

// CheckNamedValue converts a parameter the way database/sql would on its own
// and binds a time as UTC.
func (c *utcConn) CheckNamedValue(nv *driver.NamedValue) error {
	value, err := driver.DefaultParameterConverter.ConvertValue(nv.Value)
	if err != nil {
		return err
	}
	if t, ok := value.(time.Time); ok {
		value = t.UTC()
	}
	nv.Value = value
	return nil
}

// Ping forwards to the stock connection.
func (c *utcConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

// ExecContext forwards to the stock connection, or has database/sql prepare
// the statement when the connection cannot run one directly.
func (c *utcConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := c.Conn.(driver.ExecerContext); ok {
		return execer.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

// QueryContext forwards to the stock connection, or has database/sql prepare
// the statement when the connection cannot run one directly.
func (c *utcConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := c.Conn.(driver.QueryerContext); ok {
		return queryer.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

// PrepareContext forwards to the stock connection, or prepares without the
// context when the connection takes none.
func (c *utcConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if preparer, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return preparer.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

// BeginTx forwards to the stock connection; a connection taking no options
// cannot honor them and refuses.
func (c *utcConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginner, ok := c.Conn.(driver.ConnBeginTx); ok {
		return beginner.BeginTx(ctx, opts)
	}
	return nil, errors.New("sqlite: the connection cannot begin a transaction with options")
}

// registerRegexpFunc makes REGEXP work on conn. SQLite parses the operator
// but ships no implementation: "value REGEXP pattern" invokes a user function
// regexp(pattern, value), and without one every regex filter fails at runtime
// with "no such function: REGEXP".
//
// The implementation is Go's regexp package, so patterns use RE2 syntax and
// match case-sensitively like the PostgreSQL ~ operator; MySQL matches
// case-insensitively only through its default collation, which is a collation
// choice rather than framework behavior. (?i) opts into case-insensitivity
// per pattern, and an invalid pattern fails the query the way every dialect
// rejects one.
//
// A NULL value or pattern never matches, mirroring how MySQL answers NULL
// with NULL and a WHERE treats that as false. Numeric values match against
// their text form the way MySQL casts them.
//
// The compiled pattern is cached per connection: the rows of one statement
// all carry the same pattern, and a connection runs statements one at a time,
// so the single-entry cache needs no lock.
func registerRegexpFunc(conn *sqlite3.SQLiteConn) error {
	var lastPattern string
	var lastRe *regexp.Regexp
	return conn.RegisterFunc("regexp", func(patternArg, valueArg any) (bool, error) {
		pattern, ok := textValue(patternArg)
		if !ok {
			return false, nil
		}
		value, ok := textValue(valueArg)
		if !ok {
			return false, nil
		}
		if lastRe == nil || pattern != lastPattern {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, err
			}
			lastPattern, lastRe = pattern, re
		}
		return lastRe.MatchString(value), nil
	}, true)
}

// textValue renders a SQLite value as the text REGEXP matches against.
// NULL and BLOB report false: neither has a text form a pattern should
// silently match.
func textValue(arg any) (string, bool) {
	switch v := arg.(type) {
	case string:
		return v, true
	case int64:
		return strconv.FormatInt(v, 10), true
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), true
	default:
		return "", false
	}
}
