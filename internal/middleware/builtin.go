package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
)

// Builtin returns the middleware chain router.Init mounts ahead of every
// route, in mounting order. The chain is the framework's own and is not a
// menu: projects never mount these pieces themselves — mounting one twice
// double-counts metrics, double-writes logs or answers CORS twice — so the
// individual constructors are unexported and the loggers they write to are
// the ones the logger package builds rather than knobs.
//
// Order carries the semantics: tracing and the access logger come first so
// every refusal downstream still carries a trace id and an access-log line,
// recovery turns panics from anything after it into enveloped responses, CORS
// and route-parameter capture prepare the request, the strict-query gate
// parses the query string — nothing before it does, so the gate's parse is
// the request's first and, through the memo it fills, its only one — and the
// deadline comes last: from there on the request is held to
// server.write_timeout, whatever a project mounts and the handler running
// inside it, and the 504 it answers a request out of time with (see deadline)
// keeps what the chain set on the response ahead of it, the CORS headers
// among it. A write timeout of zero or less mounts no deadline, the way it
// sets the server none on its writes.
func Builtin() []gin.HandlerFunc {
	chain := []gin.HandlerFunc{
		requestTrace(),
		accessLogger(),
		bodyLogger(),
		recovery(),
		cors(),
		routeParams(),
		strictQuery(),
	}
	if timeout := config.App.Server.WriteTimeout; timeout > 0 {
		chain = append(chain, deadline(timeout))
	}
	return chain
}
