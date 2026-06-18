// Package modelregistry contains the internal model infrastructure behind the
// public model package.
//
// Application code should use github.com/hydroan/gst/model. This package keeps
// the framework-owned implementations for Base, Empty, and Any, the type rules
// used by controllers and schema generation, and the database initialization
// queues consumed by the database runtime.
package modelregistry
