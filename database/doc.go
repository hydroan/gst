// Package database provides the framework database facade built on top of GORM.
//
// The package exposes a model-scoped Database handle for CRUD operations, query
// options, dry-run SQL generation, SQL capture, cleanup, health checks, and
// transactions. Each independent operation must start from Database[M](ctx);
// reusing a handle after a terminal operation can retain GORM clauses from the
// previous chain.
//
// Tables used by Database[M](ctx), WithDB, and WithTable are expected to exist
// before an operation chain runs. Framework startup prepares registered tables
// through the internal database runtime; callers using custom database instances
// are responsible for preparing their schemas before using them.
package database
