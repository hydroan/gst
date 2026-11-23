// Package requestctx carries what names a request below its handler. The
// Metadata of a request holds its immutable fields — the route, the path and
// the method, the caller, the session and the tenant, and the connection it
// arrived on — which logging and the storage layer read off a context (see
// WithMetadata and FromContext). Beside it stand the keys those fields are
// stored under on a gin context, with the accessors memoizing what gin would
// parse anew at every call (see GinQuery and GinClientIP); the trace id a
// request may bring along (see TraceIDOf); and the stance on forwarding
// headers both listeners resolve the client address by (see TrustedProxies
// and ClientIP).
//
// The public requestctx package forwards the part the middleware of a
// project and the copied module files use.
package requestctx
