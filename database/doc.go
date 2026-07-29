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
package database
