package middleware

import "github.com/gin-gonic/gin"

// Builtin returns the middleware chain router.Init mounts ahead of every
// route, in mounting order. The chain is the framework's own and is not a
// menu: projects never mount these pieces themselves — mounting one twice
// double-counts metrics, double-writes logs or answers CORS twice — so the
// individual constructors are unexported and the file targets of the two
// loggers are fixed here rather than exposed as knobs.
//
// Order carries the semantics: tracing and the access logger come first so
// every refusal downstream still carries a trace id and an access-log line,
// recovery turns panics from anything after it into enveloped responses, CORS
// and route-parameter capture prepare the request, and the strict-query gate
// runs last — nothing before it parses the query string, so the gate's parse
// is the request's first and, through the memo it fills, its only one.
func Builtin() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		tracing(),
		accessLogger("api.log"),
		bodyLogger(),
		recovery("recovery.log"),
		cors(),
		routeParams(),
		strictQuery(),
	}
}
