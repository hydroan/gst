package database

import (
	"reflect"
	"strings"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

// This file holds the predicate engine: the single implementation that turns a
// types.Filter tree into SQL. It is deliberately separate from the chainable
// options in query_options.go, because a rendered predicate is no longer tied
// to the WHERE clause of a list query. The same rules serve three call sites --
// the WHERE clause, the CASE guard of a conditional aggregate, and HAVING --
// and keeping one implementation is what stops the fail-closed behavior from
// drifting apart between them.

// applyFilters appends field-level operator filters, each as an AND
// condition with its value bound as a statement parameter. Every mismatch
// fails closed with "1 = 0" instead of being dropped: silently dropping a
// filter would widen the result set, which is dangerous when the result
// feeds deletes or exports. That covers an empty column, an operator the
// switch does not recognize, and a value whose type does not match the
// operator (see the Filter type for the per-operator value contract).
//
// The caller must hold db.mu.
func (db *database[M]) applyFilters(filters []types.Filter) {
	if expr := db.renderFilters(filters, false); expr != nil {
		db.ins = db.ins.Where(expr)
	}
}

// renderFilters turns a filter list into one composable predicate rather than
// applying it to a query directly. Returning an expression is what lets a
// single implementation serve every place a predicate appears — the WHERE
// clause, the CASE guard of a conditional aggregate, and HAVING — so the
// fail-closed rules cannot drift apart between them.
//
// The filters are OR-combined with each other when or is set and AND-combined
// otherwise; the top-level call always combines with AND. gorm parenthesizes
// the nesting as it builds the expression, so a group can never absorb the
// conditions around it.
//
// An empty list yields nil, which callers read as "no condition". An empty
// group fails closed instead; see groupCondition.
//
// The caller must hold db.mu.
func (db *database[M]) renderFilters(filters []types.Filter, or bool) clause.Expression {
	if len(filters) == 0 {
		return nil
	}
	exprs := make([]clause.Expression, 0, len(filters))
	for _, f := range filters {
		exprs = append(exprs, db.renderFilter(f))
	}
	if or {
		return clause.Or(exprs...)
	}
	return clause.And(exprs...)
}

// renderFilter turns one filter into a predicate.
//
// The caller must hold db.mu.
func (db *database[M]) renderFilter(f types.Filter) clause.Expression {
	// Groups carry their children in Value and have no column of their own,
	// so they are dispatched before the empty-column check below.
	switch f.Op {
	case types.FilterOpOr:
		return db.groupCondition(f, true)
	case types.FilterOpAnd:
		return db.groupCondition(f, false)
	}
	if len(f.Column) == 0 {
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("filter has empty column, adding safety condition")
		return failClosedExpr()
	}
	column := db.quoteIdent(f.Column)
	switch f.Op {
	case types.FilterOpEq:
		return db.scalarFilter(f, column+" = ?")
	case types.FilterOpNe:
		return db.scalarFilter(f, column+" <> ?")
	case types.FilterOpGt:
		return db.scalarFilter(f, column+" > ?")
	case types.FilterOpGte:
		return db.scalarFilter(f, column+" >= ?")
	case types.FilterOpLt:
		return db.scalarFilter(f, column+" < ?")
	case types.FilterOpLte:
		return db.scalarFilter(f, column+" <= ?")
	case types.FilterOpIn:
		return db.listFilter(f, column+" IN ?")
	case types.FilterOpNotIn:
		return db.listFilter(f, column+" NOT IN ?")
	case types.FilterOpLike:
		return db.patternFilter(f, column+" LIKE ?"+likeEscapeClause, "%", "%")
	case types.FilterOpNotLike:
		return db.patternFilter(f, column+" NOT LIKE ?"+likeEscapeClause, "%", "%")
	case types.FilterOpStartsWith:
		return db.patternFilter(f, column+" LIKE ?"+likeEscapeClause, "", "%")
	case types.FilterOpEndsWith:
		return db.patternFilter(f, column+" LIKE ?"+likeEscapeClause, "%", "")
	case types.FilterOpIsNull:
		b, ok := f.Value.(bool)
		if !ok {
			return db.failClosedFilter(f, "expects a bool value")
		}
		if b {
			return clause.Expr{SQL: column + " IS NULL"}
		}
		return clause.Expr{SQL: column + " IS NOT NULL"}
	case types.FilterOpRegex:
		return db.stringFilter(f, column+" "+db.regexpOperator()+" ?")
	case types.FilterOpNotRegex:
		return db.stringFilter(f, "NOT ("+column+" "+db.regexpOperator()+" ?)")
	case types.FilterOpJSONContains:
		// datatypes handles the dialect split: JSON_CONTAINS on MySQL and
		// a json_each EXISTS subquery on SQLite. The column is passed
		// unquoted because the expression quotes it itself.
		s, ok := f.Value.(string)
		if !ok {
			return db.failClosedFilter(f, "expects a string value")
		}
		return datatypes.JSONArrayQuery(f.Column).Contains(s)
	default:
		return db.failClosedFilter(f, "is unknown")
	}
}

// groupCondition builds one filter group as a single nested predicate, so the
// conditions outside the group can never be absorbed into it. Its children are
// OR-combined when or is set and AND-combined otherwise; a child may itself be
// a group, which is how arbitrary nesting works.
//
// A group whose value is not a filter list, or that carries no children at
// all, fails closed: an empty group is a caller bug, and answering it with the
// logical identity (TRUE for AND) would widen the result set.
func (db *database[M]) groupCondition(f types.Filter, or bool) clause.Expression {
	children, ok := f.Value.([]types.Filter)
	if !ok {
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter group %q expects a filter list value, adding safety condition", f.Op)
		return failClosedExpr()
	}
	if len(children) == 0 {
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter group %q has no children, adding safety condition", f.Op)
		return failClosedExpr()
	}
	return db.renderFilters(children, or)
}

// failClosedExpr is the predicate that matches nothing. Narrowing to an empty
// result is always safe; widening it is not.
func failClosedExpr() clause.Expression { return clause.Expr{SQL: "1 = 0"} }

// failClosedFilter records why a filter cannot be applied and narrows the
// query to an empty result instead of widening it.
func (db *database[M]) failClosedFilter(f types.Filter, msg string) clause.Expression {
	logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter operator %q on column %q %s, adding safety condition", f.Op, f.Column, msg)
	return failClosedExpr()
}

// scalarFilter binds a comparison filter whose value must be a scalar; nil,
// slice, and array values fail closed.
func (db *database[M]) scalarFilter(f types.Filter, sql string) clause.Expression {
	if f.Value == nil {
		return db.failClosedFilter(f, "expects a scalar value")
	}
	if k := reflect.ValueOf(f.Value).Kind(); k == reflect.Slice || k == reflect.Array {
		return db.failClosedFilter(f, "expects a scalar value")
	}
	return clause.Expr{SQL: sql, Vars: []any{f.Value}}
}

// listFilter binds a set-membership filter whose value must be a slice or an
// array; anything else, including a comma-separated string, fails closed. An
// empty slice keeps the SQL list semantics: IN matches nothing, and the result
// never widens.
func (db *database[M]) listFilter(f types.Filter, sql string) clause.Expression {
	if f.Value == nil {
		return db.failClosedFilter(f, "expects a slice value")
	}
	if k := reflect.ValueOf(f.Value).Kind(); k != reflect.Slice && k != reflect.Array {
		return db.failClosedFilter(f, "expects a slice value")
	}
	return clause.Expr{SQL: sql, Vars: []any{f.Value}}
}

// patternFilter binds a LIKE-family filter; the value must be a string and is
// escaped so the stored value matches literally.
func (db *database[M]) patternFilter(f types.Filter, sql, prefix, suffix string) clause.Expression {
	s, ok := f.Value.(string)
	if !ok {
		return db.failClosedFilter(f, "expects a string value")
	}
	return clause.Expr{SQL: sql, Vars: []any{prefix + escapeLikePattern(s) + suffix}}
}

// stringFilter binds a filter whose value must be a plain string bound as-is
// (the regex operators).
func (db *database[M]) stringFilter(f types.Filter, sql string) clause.Expression {
	s, ok := f.Value.(string)
	if !ok {
		return db.failClosedFilter(f, "expects a string value")
	}
	return clause.Expr{SQL: sql, Vars: []any{s}}
}

// likeEscapeClause declares the LIKE escape character used by filters.
// The pipe is chosen over the conventional backslash because
// backslash inside a SQL string literal is itself an escape character in
// MySQL but a plain character in SQLite/PostgreSQL, so no single spelling of
// ESCAPE '\' parses the same way across the supported dialects.
const likeEscapeClause = " ESCAPE '|'"

// likePatternEscaper rewrites a filter value into a literal LIKE
// pattern fragment: client values are literals, not pattern language, so the
// wildcards and the escape character itself are escaped.
var likePatternEscaper = strings.NewReplacer("|", "||", "%", `|%`, "_", `|_`)

func escapeLikePattern(value string) string {
	return likePatternEscaper.Replace(value)
}
