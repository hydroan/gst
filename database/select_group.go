package database

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file is the grouped side of the select builder: how a measure and a
// group key render, how GROUP BY is derived from the keys and HAVING applied,
// and the rules a grouped projection is validated by. The builder itself
// lives in select.go, the window side in select_window.go.

// Errors reported while a grouped projection is built; see the select
// errors for why they fail fast.
var (
	ErrPlainColumnInGroupedSelect = errors.New("a column next to an aggregate must be a group key, Cols.X.Group(), or be aggregated")
	ErrAggregateType              = errors.New("aggregate function does not accept this column type")
	ErrConditionOnGroupKey        = errors.New("a group key or literal cannot carry conditions, they only restrict a measure")
	ErrBucketOnMeasure            = errors.New("a measure cannot carry a time bucket, it only truncates a group key")
	ErrHavingTermNotSelected      = errors.New("having references a measure the projection does not declare")
	ErrHavingWithoutGroups        = errors.New("having needs an aggregate to restrict, a row-level select has none")
)

// groupClauses adds the GROUP BY derived from the group keys and the HAVING
// conditions to the statement.
func (a *selector[M, R]) groupClauses(tx *gorm.DB, shape projectionShape) (*gorm.DB, error) {
	// Group keys and HAVING render the full expression rather than the output
	// alias. An output alias is accepted in GROUP BY and HAVING by MySQL, SQLite
	// and ClickHouse but rejected in HAVING by PostgreSQL, and re-rendering
	// costs nothing, so one portable spelling replaces a per-dialect branch.
	//
	// Both go through gorm's own clause building so their values bind as
	// statement parameters. Rendering them with Dialector.Explain would be
	// wrong twice over: Explain exists to format SQL for the log, so it inlines
	// values instead of binding them, and it has no case for a nested
	// clause.Expression, so a conditional measure would reach the query as the
	// Go formatting of a struct rather than as its predicate.
	for _, t := range shape.keys {
		sql, args, termErr := a.termExpr(t, shape)
		if termErr != nil {
			return nil, termErr
		}
		if len(args) > 0 {
			// Unreachable today: only measures carry conditions, and a group
			// key renders to a column or a bucket expression, neither of which
			// binds a value. Fail loudly rather than drop the values if a
			// future group key gains any.
			return nil, errors.Newf("group key %q renders bound values", termAlias(t))
		}
		// Raw keeps gorm from quoting an already quoted expression: the
		// MySQL, PostgreSQL and SQLite quoters are idempotent, but the
		// ClickHouse one is not and would emit ""col"".
		tx.Statement.AddClause(clause.GroupBy{
			Columns: []clause.Column{{Name: sql, Raw: true}},
		})
	}
	for _, h := range a.havings {
		sql, args, termErr := a.termExpr(types.TermConditionTermOf(h), shape)
		if termErr != nil {
			return nil, termErr
		}
		tx = tx.Having(clause.Expr{
			SQL:  sql + " " + compareOperator(types.TermConditionOpOf(h)) + " ?",
			Vars: append(append([]any(nil), args...), types.TermConditionValueOf(h)),
		})
	}
	return tx, nil
}

// keyExpr renders a group key or plain column: the column itself, or its time
// bucket.
func (a *selector[M, R]) keyExpr(t types.Term, shape projectionShape) string {
	if jt, derived := a.derivedOf(t, shape); derived {
		return a.derivedExpr(jt, t)
	}
	column := a.columnExpr(types.TermTableOf(t), types.TermColumnOf(t), shape)
	if types.TermBucketOf(t) == types.TimeBucketNone {
		return column
	}
	return a.db.timeBucketExpr(column, types.TermBucketOf(t))
}

// functionExpr renders the function call of a measure or window function
// without its window and without the COALESCE a SUM takes, which the caller
// adds around the complete expression; coalesce reports whether it must.
func (a *selector[M, R]) functionExpr(t types.Term, shape projectionShape) (sql string, args []any, coalesce bool, err error) {
	column := a.columnExpr(types.TermTableOf(t), types.TermColumnOf(t), shape)
	cond, condErr := a.db.renderFilters(types.TermConditionsOf(t), false, a.whereScope(shape))
	if condErr != nil {
		return "", nil, false, condErr
	}
	switch types.TermFnOf(t) {
	case types.FnCount:
		if cond != nil {
			// COUNT(*) and COUNT(column) both become a conditional count:
			// CASE yields NULL outside the predicate, and COUNT skips NULLs.
			if len(types.TermColumnOf(t)) == 0 {
				return "COUNT(CASE WHEN ? THEN 1 END)", []any{cond}, false, nil
			}
			return "COUNT(CASE WHEN ? THEN " + column + " END)", []any{cond}, false, nil
		}
		if len(types.TermColumnOf(t)) == 0 {
			return "COUNT(*)", nil, false, nil
		}
		return "COUNT(" + column + ")", nil, false, nil
	case types.FnCountDistinct:
		if cond != nil {
			return "COUNT(DISTINCT CASE WHEN ? THEN " + column + " END)", []any{cond}, false, nil
		}
		return "COUNT(DISTINCT " + column + ")", nil, false, nil
	case types.FnSum:
		if cond != nil {
			return "SUM(CASE WHEN ? THEN " + column + " ELSE 0 END)", []any{cond}, true, nil
		}
		return "SUM(" + column + ")", nil, true, nil
	case types.FnAvg, types.FnMin, types.FnMax:
		fn := string(types.TermFnOf(t))
		if cond != nil {
			return fn + "(CASE WHEN ? THEN " + column + " END)", []any{cond}, false, nil
		}
		return fn + "(" + column + ")", nil, false, nil
	case types.FnRowNumber, types.FnRank, types.FnDenseRank:
		return string(types.TermFnOf(t)) + "()", nil, false, nil
	case types.FnLag, types.FnLead:
		return string(types.TermFnOf(t)) + "(" + column + ")", nil, false, nil
	default:
		// validate rejects any function outside the closed set before the
		// renderer runs, so reaching this arm means the two drifted apart.
		// Composing SQL from the value would put caller text into the
		// statement, so it errors instead.
		return "", nil, false, errors.Wrapf(ErrUnknownTermFn, "%q", types.TermFnOf(t))
	}
}

// validateGrouping checks the group-side rules of one term: a condition only
// restricts a measure, a bucket only truncates a group key. Both were
// previously dropped without a word, which is how a report ends up silently
// counting the wrong rows.
func (a *selector[M, R]) validateGrouping(t types.Term) error {
	if !t.IsMeasure() && len(types.TermConditionsOf(t)) > 0 {
		return errors.Wrapf(ErrConditionOnGroupKey, "%q", termAlias(t))
	}
	if t.IsMeasure() && types.TermBucketOf(t) != types.TimeBucketNone {
		return errors.Wrapf(ErrBucketOnMeasure, "%q", termAlias(t))
	}
	return nil
}

// validateColumnClass checks that the column's type accepts what the term
// applies to it: SUM and AVG need a numeric column, a time bucket a time
// column.
func (a *selector[M, R]) validateColumnClass(t types.Term, column modelschema.Column) error {
	class := modelschema.ClassifyColumn(column.Type)
	switch {
	case types.TermFnOf(t) == types.FnSum || types.TermFnOf(t) == types.FnAvg:
		// The generated reference already blocks this at compile time for the
		// types it can classify, so the check only bites on minted
		// references. It asks ClassifyColumn rather than keeping a rule of
		// its own: a second rule admitted every struct storing itself through
		// driver.Valuer, which is also how uuid, JSON and text-backed null
		// wrappers travel, and gorm.DeletedAt is on every model. Two rules
		// disagreeing about the same type is worse than one rule being strict.
		if class != modelschema.ColumnClassNumeric {
			return errors.Wrapf(ErrAggregateType, "%s over non-numeric column %q", types.TermFnOf(t), types.TermColumnOf(t))
		}
	case !t.IsMeasure() && types.TermBucketOf(t) != types.TimeBucketNone:
		if class != modelschema.ColumnClassTime {
			return errors.Wrapf(ErrAggregateType, "time bucket over non-time column %q", types.TermColumnOf(t))
		}
	}
	return nil
}

// validateHaving checks the HAVING conditions: there are groups to restrict,
// and every condition names a measure the projection declares, not a window
// over it, which is computed after HAVING and filtered by Qualify, and
// compares against a value SQL can order.
func (a *selector[M, R]) validateHaving(shape projectionShape) error {
	if !shape.grouped && len(a.havings) > 0 {
		return ErrHavingWithoutGroups
	}
	for _, h := range a.havings {
		if !a.isSelected(types.TermConditionTermOf(h)) {
			return errors.Wrapf(ErrHavingTermNotSelected, "%q", termAlias(types.TermConditionTermOf(h)))
		}
		if types.TermConditionTermOf(h).IsWindowed() {
			return errors.Wrapf(ErrHavingWindowTerm, "%q", termAlias(types.TermConditionTermOf(h)))
		}
		if err := validateConditionValue(h, termAlias(types.TermConditionTermOf(h)), a.termKind(types.TermConditionTermOf(h), shape)); err != nil {
			return err
		}
	}
	return nil
}
