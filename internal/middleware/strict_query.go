package middleware

import (
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/urlquery"
)

// strictQuery returns a middleware that rejects, before any handler runs, a
// query string the framework cannot take at its word: a query that fails to
// parse, one that repeats a key, or one whose paging parameter is no count
// its parameter takes.
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
// carry a count, zero or more, the size urlquery.MaxPageSize at most, and a
// list reads them through helpers that have no error to answer with (see
// urlquery.Pagination), so one that is no such count is refused at the door
// the way urlquery.ParseCount words it, _page=abc with "invalid query
// parameter "_page": expect a numeric value, got "abc"", _size=-5 with
// "invalid query parameter "_size": expect a non-negative value, got "-5""
// and _size=500 with "invalid query parameter "_size": expect a value from 1
// to 100, got "500"", rather than taken for the parameter left out, which an
// empty value is, or brought down to the cap.
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
			response.Abort(c, gst.NewError(http.StatusBadRequest, malformedQueryMsg))
			return
		}

		var duplicated []string
		for key, values := range query {
			if len(values) > 1 {
				duplicated = append(duplicated, key)
			}
		}
		if len(duplicated) > 0 {
			response.Abort(c, gst.NewError(http.StatusBadRequest, urlquery.DuplicateParameterError(duplicated).Error()))
			return
		}

		for _, count := range [...]struct {
			key  string
			most int
		}{{gst.QueryPage, math.MaxInt}, {gst.QuerySize, urlquery.MaxPageSize}} {
			raw := query.Get(count.key)
			if raw == "" {
				continue
			}
			if _, err := urlquery.ParseCount(count.key, raw, count.most); err != nil {
				response.Abort(c, gst.NewError(http.StatusBadRequest, err.Error()))
				return
			}
		}
	}
}
