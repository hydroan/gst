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
// MySQL, SQLite, and PostgreSQL carry the full surface described above with
// identical behavior; the test suite runs against all three. The one stored
// time base is the UTC wall clock, and the two documented per-dialect splits
// are Upsert's conflict target (see Upsert) and row locks on SQLite (see
// WithLock).
//
// ClickHouse is a read-only analytical instance (see clickhouse.New), never
// the default database. Supported on it:
//
//   - the read path: List, Get, Count, First, Last, Take, WithQuery, the
//     filter operators, ordering, paging, and cursor pagination;
//   - the whole aggregate path: grouping, measures, conditional measures,
//     time buckets, HAVING, ordering, paging, and CountGroups.
//
// Not carried by ClickHouse, failing closed to an empty result: correlated
// EXISTS subqueries (FilterExists) and JSON containment (jsoncontains).
// Not carried, answering ErrUnsupportedOnDialect: the write path (Create,
// Update, Delete, Upsert, UpdateByID, Cleanup), Transaction/TransactionOn,
// and WithLock — ClickHouse has no transactions or unique constraints for
// their contracts to build on. Feeding the instance is the application
// ingestion side's job, through plain batch INSERTs outside this package.
package database
