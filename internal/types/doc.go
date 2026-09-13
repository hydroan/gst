// Package types defines the contracts between the framework and business
// projects: the Model, Service, Database, Selector, Cache, RBAC, and Logger
// interfaces, the query building blocks they exchange (Filter, Order, Cursor,
// Column, Term, Window), and the per-request ServiceContext. Framework code
// imports this package directly; business projects reach its contracts
// through the public gst package, which forwards them under the same names.
package types
