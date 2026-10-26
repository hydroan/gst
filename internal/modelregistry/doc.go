// Package modelregistry contains the internal model infrastructure behind the
// public model package.
//
// Application code should use github.com/hydroan/gst/model. This package keeps
// the framework-owned implementations for Base, AutoBase and Empty, the query
// parameter structs Query, Pagination and Cursor, the type rules used by
// controllers and schema generation, the detection of the hooks a model
// overrides and of the replica its reads prefer, and the database
// initialization queues consumed by the database runtime.
package modelregistry
