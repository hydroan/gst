package middleware

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/response"
)

// strictQuery returns a middleware that rejects ambiguous query strings before
// any handler runs: a query that fails to parse, or one that repeats a key.
//
// The framework's query contract is one value per key — the framework itself
// never consumes multiple values for a key, and neither the standard library's
// first-value accessors nor the framework's last-value decoding defines what a
// repeated key means. Letting such requests through is the classic HTTP
// parameter pollution setup: any two components that resolve the repetition
// differently (an authorization check and the query builder, a proxy and the
// application) can be driven apart by one crafted URL. Rejecting at the door
// makes every accessor — gin's, the standard library's and the framework's —
// provably agree on what each parameter's value is.
//
// Malformed query strings (stray percent signs, semicolon separators) are
// rejected for the same reason: url.URL.Query silently keeps whatever parsed
// before the error, so two parsers can disagree about what the client sent.
//
// The gate performs the request's one query parse and memoizes it for the
// framework's request metadata to reuse, so it adds no parse over a chain that
// parses the query anyway; requests without a query string skip it entirely.
func strictQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.URL == nil || c.Request.URL.RawQuery == "" {
			c.Next()
			return
		}

		query, err := requestctx.ParseGinQuery(c)
		if err != nil {
			response.Abort(c, http.StatusBadRequest, "malformed query string")
			return
		}

		var duplicated []string
		for key, values := range query {
			if len(values) > 1 {
				duplicated = append(duplicated, key)
			}
		}
		if len(duplicated) > 0 {
			// Map iteration order is random; sorting keeps the refusal stable
			// for clients and tests.
			sort.Strings(duplicated)
			response.Abort(c, http.StatusBadRequest,
				"duplicate query parameter: "+strings.Join(duplicated, ", "))
			return
		}

		c.Next()
	}
}
