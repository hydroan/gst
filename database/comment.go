package database

import (
	"context"
	"net/url"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
	"gorm.io/gorm/clause"
	"gorm.io/hints"
)

// SQL statement comments.
//
// Every statement a request issues can carry a /*key='value'*/ comment naming
// where it came from, closing the reverse direction of observability: the
// application-side SQL log already maps a statement to its trace, but an
// operator starting FROM the database — SHOW PROCESSLIST, the slow query
// log, an audit plugin — held only bare SQL until now. The content is
// decided by database.sql_comment (see config.SQLCommentMode for the modes
// and the statement-cache trade the trace mode makes).
//
// The comment sits after the statement verb (SELECT /*...*/ ... FROM),
// rendered through gorm's own hints clauses — a deliberate trade against the
// sqlcommenter convention of trailing comments: both positions reach every
// database-side view, and the verb position needs no reliance on gorm build
// internals. Values are URL-encoded, which both matches the sqlcommenter
// escaping convention and keeps a value from ever closing the comment.
//
// Outside a request — cron jobs, startup, tests without request metadata —
// there is nothing to report and statements stay clean.

// sqlCommentFor renders the comment content for one chain's statements, and
// "" when the mode or the context yields nothing to annotate.
func sqlCommentFor(ctx context.Context) string {
	mode := config.App.Database.SQLComment
	if mode != config.SQLCommentRoute && mode != config.SQLCommentTrace {
		return ""
	}

	meta := requestctx.FromContext(ctx)
	// The sqlcommenter convention orders the serialized keys ascending, so
	// the appends below run in that order rather than in importance order:
	// method, route, trace_id. Keep any new key in its sorted position.
	parts := make([]string, 0, 3)
	if method := meta.Method(); len(method) > 0 {
		parts = append(parts, "method='"+encodeCommentValue(method)+"'")
	}
	if route := meta.Route(); len(route) > 0 {
		parts = append(parts, "route='"+encodeCommentValue(route)+"'")
	}
	if mode == config.SQLCommentTrace {
		if traceID := meta.TraceID(); len(traceID) > 0 {
			parts = append(parts, "trace_id='"+encodeCommentValue(traceID)+"'")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

// encodeCommentValue renders one value the way the sqlcommenter convention
// requires: percent-encoded, spaces included.
//
// url.QueryEscape reserves exactly the RFC 3986 unreserved set, which covers
// the convention's escaping and leaves a value unable to close the comment
// ("*" becomes "%2A", so "*/" cannot form) or to break out of its quotes
// ("'" becomes "%27"). Its single departure is the form-urlencoded space,
// rendered as "+" where the convention wants "%20". Rewriting that back is
// unambiguous: a literal plus is already encoded as "%2B", so no "+" the
// escape produces means anything but a space, and a consumer
// percent-decoding the value recovers it exactly.
func encodeCommentValue(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// commentClauses covers every statement verb with the comment; a chain runs
// exactly one verb, and the three hints that never match cost nothing at
// build time.
func commentClauses(comment string) []clause.Expression {
	return []clause.Expression{
		hints.CommentAfter("select", comment),
		hints.CommentAfter("insert", comment),
		hints.CommentAfter("update", comment),
		hints.CommentAfter("delete", comment),
	}
}
