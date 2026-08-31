// Package database provides the framework database facade built on top of GORM.
//
// The package exposes a model-scoped Database handle for CRUD operations, query
// options, dry-run SQL generation, SQL capture, cleanup, health checks, and
// transactions. Each independent operation must start from Database[M](ctx);
// reusing a handle after a terminal operation can retain GORM clauses from the
// previous chain.
//
// Tables used by Database[M](ctx) are expected to exist before an operation
// chain runs. Framework startup prepares registered tables through the
// internal database runtime.
//
// A non-default database instance is a plain *gorm.DB the application builds
// once, typically with a dialect New function such as clickhouse.New, and
// holds itself. Chains reach it through DatabaseOn, AggregateOn, and
// TransactionOn; the entry point that opens a chain decides the instance,
// never a mid-chain option. Transactions never cross instances, and the
// application owns the instance's schema: the framework does not create
// tables on it.
//
// # Dialect support
//
// MySQL, PostgreSQL, and SQLite carry the full surface described above with
// identical behavior; the test suite runs against all three. The one stored
// time base is the UTC wall clock, and the two documented per-dialect splits
// are Upsert's conflict target (see Upsert) and row locks on SQLite (see
// WithLock).
//
// # Error-stack contract
//
// GORM and the SQL drivers hand back errors that carry no run-time stack
// trace, and the framework sentinels (ErrRecordNotFound, ErrDuplicatedKey)
// only carry the useless init-time stack of their package-level definition.
// This package therefore embeds the run-time stack at every first-hand exit
// of such an error — the first line where the error enters framework code —
// via errors.WithStack. The stack captured there still holds every caller
// frame, so the error_stack log field written by the logging layer (which
// reports the deepest run-time stack in the unwrap chain) locates the exact
// call site in any caller: service code, model hooks, DAOs, cron jobs — with
// no logging or wrapping required at the call sites themselves.
//
// The rules, so a change keeps exactly one capture point per error chain:
//
//   - a stack-less GORM/driver/sentinel error is wrapped once, at its
//     first-hand exit;
//   - errors this package builds itself (errors.New / errors.Wrap of a
//     sentinel) already capture the stack at construction and are left alone;
//   - forwarding sites return errors unchanged: re-wrapping adds a shallower
//     stack the deepest-stack rule ignores, at pure cost.
//
// Wrapping preserves the unwrap chain, so errors.Is/As checks against
// ErrRecordNotFound, ErrDuplicatedKey, and friends behave exactly as before.
//
// ClickHouse is an analytical instance (see clickhouse.New), never the
// default database. Supported on it:
//
//   - the read path: List, Get, Count, First, Last, Take, WithQuery, the
//     filter operators, ordering, paging, and cursor pagination;
//   - the whole aggregate path: grouping, measures, conditional measures,
//     time buckets, HAVING, ordering, paging, and CountGroups;
//   - a write path with a deliberately weaker contract — no model hooks, no
//     transaction boundary: Create is plain batch INSERTs (no
//     ErrDuplicatedKey; ClickHouse has no unique constraints), Delete is a
//     lightweight DELETE by primary key and always physical (no soft
//     delete), Update and UpdateByID are asynchronous ALTER TABLE ... UPDATE
//     mutations for low-frequency data correction (accepted, not awaited; no
//     ErrRecordNotFound). Each entry point's doc states the details.
//
// Not carried by ClickHouse, failing closed to an empty result: correlated
// EXISTS subqueries (FilterExists) and JSON containment (jsoncontains).
// Not carried, answering ErrUnsupportedOnDialect: Upsert (no conflict
// semantics), Cleanup (no soft-delete regime), Transaction/TransactionOn,
// and WithLock. The instance's schema — engine, ORDER BY, partitioning —
// is hand-written DDL owned by the application: neither bootstrap nor
// "gg migrate" creates or alters ClickHouse tables, bootstrap only verifies
// that a registered model's table exists.
package database
