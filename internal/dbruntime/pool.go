package dbruntime

import (
	"database/sql"

	"github.com/hydroan/gst/config"
)

// ConfigurePool applies the framework's connection pool limits — the
// [database] max_idle_conns, max_open_conns, conn_max_lifetime, and
// conn_max_idle_time settings — to the pool of a freshly opened handle. It is
// the one step every dialect's New shares right after opening, so an
// application-held handle runs under the same pool contract as the default
// one; AttachResolver applies the same limits to every replica pool.
//
// Left at the database/sql defaults a pool keeps two idle connections and no
// lifetime: with more than two statements in flight, every connection beyond
// those two is closed on return and dialed again for the next statement, and
// a connection once opened is never retired.
//
// A dialect with a narrower contract tightens the result afterwards: sqlite
// pins its pool to a single connection, see sqlite.New.
func ConfigurePool(pool *sql.DB) {
	pool.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	pool.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	pool.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	pool.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)
}
