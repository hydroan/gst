package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
)

// strictQuery returns a middleware that rejects, before any handler runs, a
// query string the framework cannot take at its word: a query that fails to
// parse, one that repeats a key, or one whose paging parameter is no count.
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
// The paging parameters are held to their type here as well: _page and _size
// carry a count, zero or more, and a list reads them through helpers that
// have no error to answer with (see urlquery.Pagination), so one that is no
// count, _page=abc or _size=-5, is refused at the door, in the words the
// model decode answers a number it cannot read, "invalid query parameter
// "_page": expect a numeric value, got "abc"", rather than taken for the
// parameter left out, which an empty value is.
//
// The gate performs the request's one query parse and memoizes it for the
// framework's request metadata to reuse, so it adds no parse over a chain that
// parses the query anyway; requests without a query string skip it entirely.
func strictQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.URL == nil || c.Request.URL.RawQuery == "" {
			return
		}

		query, err := requestctx.GinQueryStrict(c)
		if err != nil {
			response.Abort(c, types.NewError(http.StatusBadRequest, "malformed query string"))
			return
		}

		var duplicated []string
		for key, values := range query {
			if len(values) > 1 {
				duplicated = append(duplicated, key)
			}
		}
		if len(duplicated) > 0 {
			response.Abort(c, types.NewError(http.StatusBadRequest, urlquery.DuplicateParameterError(duplicated).Error()))
			return
		}

		for _, key := range [...]string{types.QueryPage, types.QuerySize} {
			raw := query.Get(key)
			if raw == "" {
				continue
			}
			// A count fits the non-negative half of an int, the type its
			// readers hold it in.
			if _, err := strconv.ParseUint(raw, 10, strconv.IntSize-1); err != nil {
				response.Abort(c, types.NewError(http.StatusBadRequest, fmt.Sprintf("invalid query parameter %q: expect a numeric value, got %q", key, raw)))
				return
			}
		}
	}
}
