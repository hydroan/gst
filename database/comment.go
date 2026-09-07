package database

import (
	"context"
	"net/url"
	"strings"

	"github.com/hydroan/gst/internal/execctx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SQL statement comments.
//
// Every statement a request issues carries a /* trace_id='...' */ comment,
// closing the reverse direction of observability: the application-side SQL
// log already maps a statement to its trace, and the comment gives an
// operator starting FROM the database — SHOW PROCESSLIST, the slow query
// log, an audit plugin — the key back to the request's full trail.
//
// The trace id is deliberately the only key. Everything else about the
// request — method, route, user, parameters — is one trace-id lookup away
// in the log store, and the application-side SQL log already carries those
// as structured fields, so more keys would only duplicate them into every
// statement text and bury the SQL under an URL-encoded preamble.
//
// The per-request-unique comment rules out text-keyed statement caching
// wholesale; the dialect packages therefore run their connections on
// per-statement text protocol instead of prepared statements — see the
// mysql and postgres buildDSN for that half of the contract.
//
// The comment sits after the statement verb (SELECT /*...*/ ... FROM),
// attached through the clause map gorm builds statements from — a
// deliberate trade against the sqlcommenter convention of trailing comments:
// both positions reach every database-side view, and the verb position needs
// no reliance on gorm build internals. The value is URL-encoded, which both
// matches the sqlcommenter escaping convention and keeps a value from ever
// closing the comment; for the usual hex trace id the encoding changes
// nothing.
//
// A context carrying no execution identity — cron jobs, startup, tests —
// has nothing to report unless a span is open on it, and its statements stay
// clean; execctx defines what counts as an identity.

// sqlCommentFor renders the comment block for one chain's statements,
// delimiters included, and "" when the context carries nothing to annotate.
// The block is rendered once per chain, in a single concatenation, and
// written into every statement as is.
func sqlCommentFor(ctx context.Context) string {
	traceID := execctx.FromContext(ctx).TraceID
	if len(traceID) == 0 {
		return ""
	}
	return "/* trace_id='" + encodeCommentValue(traceID) + "' */"
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

// statementComment attaches one chain's comment to whichever statement verb
// the chain runs. One value serves all four verbs: gorm keeps a statement's
// clauses in a map keyed by verb name, so the modifier registers itself as
// the after-expression of each, and the three entries that never match cost
// only their map slots. The chain owns the value and hands gorm a pointer to
// it, so attaching the comment allocates nothing beyond the rendered text;
// the four gorm.io/hints comment hints this replaces each boxed a hint value
// and a clause-name slice of their own, per chain.
type statementComment struct {
	text string // the rendered comment block, delimiters included
}

// gorm recognizes the modifier by a run-time type assertion inside Clauses
// and would otherwise take the value for a WHERE condition, so the interface
// is pinned at compile time; the expression side is pinned alongside it.
var (
	_ gorm.StatementModifier = (*statementComment)(nil)
	_ clause.Expression      = (*statementComment)(nil)
)

// commentedVerbs are the gorm clause names of the statement verbs, in the
// form the clause map is keyed by.
var commentedVerbs = [...]string{"SELECT", "INSERT", "UPDATE", "DELETE"}

// ModifyStatement implements gorm.StatementModifier: it registers the comment
// as the after-expression of every verb clause, composing with an expression
// already there the way gorm.io/hints does.
func (c *statementComment) ModifyStatement(stmt *gorm.Statement) {
	for _, name := range commentedVerbs {
		verb := stmt.Clauses[name]
		if verb.AfterExpression == nil {
			verb.AfterExpression = c
		} else {
			verb.AfterExpression = commentExprs{verb.AfterExpression, c}
		}
		stmt.Clauses[name] = verb
	}
}

// Build implements clause.Expression, writing the comment block after the
// verb clause it is registered on. The builder writes into memory and never
// reports an error, so the result is discarded.
func (c *statementComment) Build(builder clause.Builder) {
	_, _ = builder.WriteString(c.text)
}

// commentExprs renders expressions in order, one space apart: the form an
// after-expression takes when the comment joins an expression that was
// registered before it.
type commentExprs []clause.Expression

func (exprs commentExprs) Build(builder clause.Builder) {
	for i, expr := range exprs {
		if i > 0 {
			_ = builder.WriteByte(' ')
		}
		expr.Build(builder)
	}
}
