// Package types defines the public contracts between the framework and
// business projects: the Model, Service, Database, Selector, Cache, RBAC,
// and Logger interfaces, the query building blocks they exchange (Filter,
// Order, Cursor, Column, Term, Window), and the per-request ServiceContext.
package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Coder describes an API envelope code, HTTP status, and client-safe message.
type Coder = itypes.Coder

// ESDocumenter represents a document that can be indexed into Elasticsearch.
// Types implementing this interface should be able to convert themselves
// into a document format suitable for Elasticsearch indexing.
type ESDocumenter = itypes.ESDocumenter
