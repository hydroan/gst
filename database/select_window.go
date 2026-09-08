package database

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file is the window side of the select builder: how a windowed term
// renders its OVER clause, how a window is validated against the shape of the
// projection it sits in, and how Qualify filters the windowed rows. The rest
// of the builder lives in select.go.

// Errors reported while a window or a Qualify is built; see the select errors
// for why they fail fast.
var (
	ErrWindowFnWithoutWindow = errors.New("window function needs a window, declare one with Over")
	ErrWindowWithoutOrder    = errors.New("window function needs an ordered window, there is no first row without an order")
	ErrWindowOnKey           = errors.New("a group key, time bucket or literal cannot be windowed, only a function can")
	ErrWindowCountDistinct   = errors.New("COUNT DISTINCT cannot be windowed on any supported dialect")
	ErrWindowOverGroups      = errors.New("AVG, LAG and LEAD cannot be windowed over a grouped projection, only SUM, COUNT, MIN, MAX and the ranking functions can")
	ErrWindowTermNotSelected = errors.New("window references a key or term the projection does not declare")
	ErrWindowNested          = errors.New("a window cannot be ordered by another window function")
	ErrQualifyTermNotWindow  = errors.New("qualify references a term that is not a window function of the projection")
)

// qualifiedAlias is the alias of the derived table a Qualify wraps the
// projection in; the outer conditions read the window columns through it.
const qualifiedAlias = "q"

// windowExpr completes the function expression of a windowed term with its
// window. Over a grouped projection the window reads the groups, so an
// aggregate is applied to the group measure it names: SUM over the window of
// the per-group sums, the count of groups, the largest of the group maxima.
// validateWindow keeps AVG out, whose nesting would answer an average of
// averages.
func (a *selector[M, R]) windowExpr(t types.Term, sql string, args []any, shape projectionShape) (string, []any, error) {
	if shape.grouped && isAggregateFn(t.Fn) {
		sql = string(t.Fn) + "(" + sql + ")"
	}
	over, overArgs, err := a.overExpr(t, shape)
	if err != nil {
		return "", nil, err
	}
	return sql + " " + over, append(args, overArgs...), nil
}

// qualifyWrap applies the Qualify conditions. A window function is computed
// after WHERE, GROUP BY and HAVING, so a condition on it cannot join them:
// the projection is wrapped in a derived table and that is filtered, the one
// portable spelling of the QUALIFY clause some databases offer natively;
// ordering and paging then apply to the filtered rows, outside the wrap. A
// select without Qualify is handed back as it is.
func (a *selector[M, R]) qualifyWrap(tx *gorm.DB) *gorm.DB {
	if len(a.qualifies) == 0 {
		return tx
	}
	outer := a.db.annotate(a.db.ins.Session(&gorm.Session{NewDB: true})).
		Table("(?) AS "+qualifiedAlias, tx)
	for _, q := range a.qualifies {
		outer = outer.Where(clause.Expr{
			SQL:  a.db.quoteTableColumn(qualifiedAlias, a.alias(q.Term)) + " " + compareOperator(q.Op) + " ?",
			Vars: []any{q.Value},
		})
	}
	return outer
}

// validateQualify checks the Qualify conditions: every one names a window
// term the projection declares and compares against a value SQL can order.
func (a *selector[M, R]) validateQualify() error {
	for _, q := range a.qualifies {
		if !a.isSelected(q.Term) || !q.Term.IsWindowed() {
			return errors.Wrapf(ErrQualifyTermNotWindow, "%q", a.alias(q.Term))
		}
		if err := validateConditionValue(q, a.alias(q.Term)); err != nil {
			return err
		}
	}
	return nil
}

// overExpr renders the OVER clause of a windowed term: the partition keys, the
// order completed with a tie breaker, and for an ordered aggregate the frame
// that makes it accumulate row by row.
//
// The tie breaker is what keeps a row number or a running total stable when
// two rows sort equal: SQL leaves their order to the executor, so the same
// query could number them differently on two runs. A row-level window gets
// the primary key, a grouped one every group key not already ordered on.
//
// The frame replaces SQL's default for an ordered aggregate, RANGE up to the
// current row, under which rows sorting equal all show the total of the
// whole tie instead of stepping through it. ROWS up to the current row steps,
// and with the tie breaker the steps are stable.
func (a *selector[M, R]) overExpr(t types.Term, shape projectionShape) (string, []any, error) {
	parts := make([]string, 0, 3)
	if len(t.Window.Partition) > 0 {
		keys := make([]string, 0, len(t.Window.Partition))
		for _, key := range t.Window.Partition {
			keys = append(keys, a.keyExpr(a.windowKey(key, shape)))
		}
		parts = append(parts, "PARTITION BY "+strings.Join(keys, ", "))
	}
	args := make([]any, 0)
	if len(t.Window.Orders) > 0 {
		items := make([]string, 0, len(t.Window.Orders)+len(shape.keys))
		ordered := make(map[string]struct{}, len(t.Window.Orders))
		for _, o := range t.Window.Orders {
			sql, orderArgs, direction, err := a.windowOrderExpr(o, shape)
			if err != nil {
				return "", nil, err
			}
			items = append(items, sql+" "+string(direction))
			args = append(args, orderArgs...)
			ordered[sql] = struct{}{}
		}
		for _, breaker := range a.tieBreakers(t, shape) {
			sql := a.keyExpr(breaker)
			if _, done := ordered[sql]; done {
				continue
			}
			items = append(items, sql+" "+string(types.OrderAsc))
			ordered[sql] = struct{}{}
		}
		parts = append(parts, "ORDER BY "+strings.Join(items, ", "))
		if isAggregateFn(t.Fn) {
			parts = append(parts, "ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW")
		}
	}
	return "OVER (" + strings.Join(parts, " ") + ")", args, nil
}

// windowKey resolves a partition key to the term it renders as: in a grouped
// projection the projection's own group key, so the partition and the GROUP BY
// spell the expression identically; in a row-level projection the key as
// given.
func (a *selector[M, R]) windowKey(key types.Term, shape projectionShape) types.Term {
	if !shape.grouped {
		return key
	}
	for _, k := range shape.keys {
		if k.Column == key.Column && k.Bucket == key.Bucket {
			return k
		}
	}
	// Unreachable: validateWindow has matched every partition key against the
	// group keys. Rendering the key as given is the harmless answer.
	return key
}

// windowOrderExpr renders one ordering of a window: a column as the column, a
// term as its full expression, never as an alias, which no dialect accepts
// inside OVER.
func (a *selector[M, R]) windowOrderExpr(o types.Ordering, shape projectionShape) (string, []any, types.OrderDirection, error) {
	switch o := o.(type) {
	case types.Order:
		if shape.grouped {
			return a.keyExpr(a.selectedColumnTerm(o.Column)), nil, orderDirection(o.Direction), nil
		}
		return a.db.quoteIdent(o.Column), nil, orderDirection(o.Direction), nil
	case types.TermOrder:
		sql, args, err := a.termExpr(o.Term, shape)
		if err != nil {
			return "", nil, "", err
		}
		return sql, args, orderDirection(o.Direction), nil
	default:
		// Unreachable: Ordering is sealed to the two types above.
		return "", nil, "", errors.Wrapf(ErrUnknownOrderDirection, "%T", o)
	}
}

// tieBreakers returns the terms an ordered window is completed with: the
// group keys of a grouped projection, the primary key of a row-level one.
// RANK and DENSE_RANK get none: they rank peers equally by definition, and
// a breaker would turn every tie into distinct ranks. A model without the
// framework's primary key column gets none either, which leaves the window
// as the caller ordered it.
func (a *selector[M, R]) tieBreakers(t types.Term, shape projectionShape) []types.Term {
	if t.Fn == types.FnRank || t.Fn == types.FnDenseRank {
		return nil
	}
	if shape.grouped {
		return shape.keys
	}
	if _, ok := shape.columns[modelregistry.DefaultCursorColumn]; !ok {
		return nil
	}
	return []types.Term{{Column: modelregistry.DefaultCursorColumn, Plain: true}}
}

// isAggregateFn reports whether the function accumulates rows, which is what
// gives an ordered window a frame; the ranking functions and LAG and LEAD
// read positions and take none.
func isAggregateFn(fn types.TermFn) bool {
	switch fn {
	case types.FnCount, types.FnCountDistinct, types.FnSum, types.FnAvg, types.FnMin, types.FnMax:
		return true
	default:
		return false
	}
}

// isWindowFn reports whether the function only exists over a window.
func isWindowFn(fn types.TermFn) bool {
	switch fn {
	case types.FnRowNumber, types.FnRank, types.FnDenseRank, types.FnLag, types.FnLead:
		return true
	default:
		return false
	}
}

// validateWindow checks the window side of a term: the functions that need a
// window have one, a key is never windowed, and the window's partition keys
// and orders name what they may — the queried model's columns in a row-level
// projection, the projection's own keys and terms in a grouped one.
func (a *selector[M, R]) validateWindow(t types.Term, shape projectionShape) error {
	if !t.IsWindowed() {
		if isWindowFn(t.Fn) {
			return errors.Wrapf(ErrWindowFnWithoutWindow, "%q", a.alias(t))
		}
		return nil
	}
	if !t.IsMeasure() {
		return errors.Wrapf(ErrWindowOnKey, "%q", a.alias(t))
	}
	if t.Fn == types.FnCountDistinct {
		return errors.Wrapf(ErrWindowCountDistinct, "%q", a.alias(t))
	}
	if shape.grouped && (t.Fn == types.FnAvg || t.Fn == types.FnLag || t.Fn == types.FnLead) {
		return errors.Wrapf(ErrWindowOverGroups, "%q", a.alias(t))
	}
	if isWindowFn(t.Fn) && len(t.Window.Orders) == 0 {
		return errors.Wrapf(ErrWindowWithoutOrder, "%q", a.alias(t))
	}
	for _, key := range t.Window.Partition {
		if key.IsMeasure() {
			return errors.Wrapf(ErrWindowTermNotSelected, "%q partitions by a measure %q", a.alias(t), a.alias(key))
		}
		if shape.grouped {
			if !a.isGroupKey(key, shape) {
				return errors.Wrapf(ErrWindowTermNotSelected, "%q partitions by %q, which is not a group key", a.alias(t), a.alias(key))
			}
			continue
		}
		if err := a.validateRowLevelKey(key, shape); err != nil {
			return errors.Wrapf(err, "%q partitions by", a.alias(t))
		}
	}
	for _, o := range t.Window.Orders {
		switch o := o.(type) {
		case types.Order:
			if !o.Direction.Valid() {
				return errors.Wrapf(ErrUnknownOrderDirection, "%q", o.Direction)
			}
			if shape.grouped {
				if _, ok := a.selectedColumn(o.Column); !ok {
					return errors.Wrapf(ErrWindowTermNotSelected, "%q orders by column %q, which is not a group key", a.alias(t), o.Column)
				}
				continue
			}
			if _, ok := shape.columns[o.Column]; !ok {
				return errors.Wrapf(ErrUnknownColumn, "%q orders by %q", a.alias(t), o.Column)
			}
		case types.TermOrder:
			if !o.Direction.Valid() {
				return errors.Wrapf(ErrUnknownOrderDirection, "%q", o.Direction)
			}
			if o.Term.IsWindowed() {
				return errors.Wrapf(ErrWindowNested, "%q orders by %q", a.alias(t), a.alias(o.Term))
			}
			if !a.isSelected(o.Term) {
				return errors.Wrapf(ErrWindowTermNotSelected, "%q orders by %q", a.alias(t), a.alias(o.Term))
			}
		default:
			return errors.Wrapf(ErrUnknownOrderDirection, "%T", o)
		}
	}
	return nil
}

// isGroupKey reports whether a window key matches one of the projection's
// group keys by column and bucket; the alias and the plain flag are the
// caller's spelling and do not decide.
func (a *selector[M, R]) isGroupKey(key types.Term, shape projectionShape) bool {
	for _, k := range shape.keys {
		if k.Column == key.Column && k.Bucket == key.Bucket {
			return true
		}
	}
	return false
}

// validateRowLevelKey checks a partition key of a row-level window against
// the queried model: the column must exist, and a bucket must sit on a time
// column, exactly as a group key would be checked.
func (a *selector[M, R]) validateRowLevelKey(key types.Term, shape projectionShape) error {
	if len(key.Table) > 0 && key.Table != a.db.outerTableName() {
		return errors.Wrapf(ErrColumnTable, "%q belongs to table %q, the select reads %q", key.Column, key.Table, a.db.outerTableName())
	}
	column, ok := shape.columns[key.Column]
	if !ok {
		return errors.Wrapf(ErrUnknownColumn, "%q", key.Column)
	}
	if key.Bucket != types.TimeBucketNone && modelschema.ClassifyColumn(column.Type) != modelschema.ColumnClassTime {
		return errors.Wrapf(ErrAggregateType, "time bucket over non-time column %q", key.Column)
	}
	return nil
}
