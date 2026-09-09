package database

import (
	"reflect"
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
	ErrWindowTermNotSelected = errors.New("window references a key or term the projection does not declare: a partition key is a group key or a joined select's term, and a window orders by a key or a projected measure")
	ErrWindowNested          = errors.New("a window cannot be ordered by another window function")
	ErrQualifyTermNotWindow  = errors.New("qualify references a term that is not a window function of the projection")
	ErrHavingWindowTerm      = errors.New("having cannot read a window function, which is computed after HAVING, filter it with Qualify")
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
// select without Qualify is handed back as it is. The wrap is the statement
// now, so it is the wrap that carries the operation's comment, once, and
// build leaves the inner select bare; a member of a union carries none.
func (a *selector[M, R]) qualifyWrap(tx *gorm.DB, mode buildMode) *gorm.DB {
	if len(a.qualifies) == 0 {
		return tx
	}
	outer := a.db.ins.Session(&gorm.Session{NewDB: true})
	if !mode.branch() {
		outer = a.db.annotate(outer)
	}
	outer = outer.Table("(?) AS "+a.db.quoteIdent(qualifiedAlias), tx)
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
func (a *selector[M, R]) validateQualify(shape projectionShape) error {
	for _, q := range a.qualifies {
		if !a.isSelected(q.Term) || !q.Term.IsWindowed() {
			return errors.Wrapf(ErrQualifyTermNotWindow, "%q", a.alias(q.Term))
		}
		if err := validateConditionValue(q, a.alias(q.Term), a.termKind(q.Term, shape)); err != nil {
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
			keys = append(keys, a.keyExpr(a.windowKey(key, shape), shape))
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
			sql := a.keyExpr(breaker, shape)
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
	if k, ok := a.groupKey(key, shape); ok {
		return k
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
			return a.keyExpr(a.selectedColumnTerm(o.Table, o.Column, shape.main), shape), nil, orderDirection(o.Direction), nil
		}
		return a.columnExpr(o.Table, o.Column, shape), nil, orderDirection(o.Direction), nil
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
	// The breaker names the queried table: a joined select may project a
	// term under the primary key's name, and the breaker must not read as it.
	return []types.Term{{Table: shape.main, Column: modelregistry.DefaultCursorColumn, Plain: true}}
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
		// A key carries no conditions and a measure no bucket, here as in
		// the projection: a condition the renderer never reads would
		// otherwise vanish without a word. The constants are checked the
		// same way, so a key from outside the closed sets cannot fall
		// through to a rendering it never named.
		if err := a.validateGrouping(key); err != nil {
			return errors.Wrapf(err, "%q partitions by", a.alias(t))
		}
		if !key.Fn.Valid() {
			return errors.Wrapf(ErrUnknownTermFn, "%q partitions by %q", a.alias(t), key.Fn)
		}
		if !key.Bucket.Valid() {
			return errors.Wrapf(ErrUnknownTimeBucket, "%q partitions by %q", a.alias(t), key.Bucket)
		}
		// A joined select's term the projection reads is a column of the
		// derived table, a key in either shape of the projection, passed as
		// it is; a constant is the same on every row, and a measure of the
		// projection's own is a key in neither shape.
		if _, derived := a.derivedOf(key, shape); derived {
			continue
		}
		if key.IsLiteral() {
			return errors.Wrapf(ErrWindowTermNotSelected, "%q partitions by the constant '%s', which is the same on every row", a.alias(t), key.Literal)
		}
		if key.IsMeasure() {
			if a.readsDerived(key) {
				return errors.Wrapf(ErrWindowTermNotSelected, "%q partitions by %q, a term of a joined select the projection does not read; project it to partition by it", a.alias(t), a.alias(key))
			}
			return errors.Wrapf(ErrWindowTermNotSelected, "%q partitions by a measure %q", a.alias(t), a.alias(key))
		}
		if shape.grouped {
			if _, ok := a.groupKey(key, shape); ok {
				continue
			}
			return errors.Wrapf(ErrWindowTermNotSelected, "%q partitions by %q, which is not a group key; %s", a.alias(t), a.alias(key), a.groupKeysClause(shape))
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
				if _, ok := a.selectedColumn(o.Table, o.Column, shape.main); !ok {
					return errors.Wrapf(ErrWindowTermNotSelected, "%q orders by column %q, which is not a group key; %s", a.alias(t), o.Column, a.groupKeysClause(shape))
				}
				continue
			}
			if _, err := a.columnOf(o.Table, o.Column, shape); err != nil {
				return errors.Wrapf(err, "%q orders by", a.alias(t))
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

// groupKey finds the projection's group key a window key names: the same
// column, bucket and table, an empty table naming the queried model's, and
// no function, because a measure over a key's column is not the key. The
// callers refuse a measure before asking; the function is checked here as
// well, so the rule stands whichever order a later change puts them in.
// The alias and the plain flag are the caller's spelling and do not decide. The
// table does: two tables of the query may share a column name, and a key of
// the wrong one would partition by a column the caller never named. A
// joined select's term is a group key under its own alias, not under the
// column its measure read, so only the term itself names it.
func (a *selector[M, R]) groupKey(key types.Term, shape projectionShape) (types.Term, bool) {
	for _, k := range shape.keys {
		if _, derived := a.derivedOf(k, shape); derived {
			if reflect.DeepEqual(k, key) {
				return k, true
			}
			continue
		}
		if key.Fn == types.FnNone && k.Column == key.Column && k.Bucket == key.Bucket && shape.tableOf(k) == shape.tableOf(key) {
			return k, true
		}
	}
	return types.Term{}, false
}

// groupKeysClause spells what the projection groups by, for an error
// message, so the caller sees what a window may partition or order by.
func (a *selector[M, R]) groupKeysClause(shape projectionShape) string {
	if len(shape.keys) == 0 {
		return "the projection declares no group key"
	}
	names := make([]string, 0, len(shape.keys))
	for _, k := range shape.keys {
		names = append(names, a.alias(k))
	}
	return "the projection groups by " + strings.Join(names, ", ")
}

// validateRowLevelKey checks a partition key of a row-level window against
// the tables the select reads: the column must exist on its table, and a
// bucket must sit on a time column, exactly as a group key would be checked.
func (a *selector[M, R]) validateRowLevelKey(key types.Term, shape projectionShape) error {
	column, err := a.columnOf(key.Table, key.Column, shape)
	if err != nil {
		return err
	}
	if key.Bucket != types.TimeBucketNone && modelschema.ClassifyColumn(column.Type) != modelschema.ColumnClassTime {
		return errors.Wrapf(ErrAggregateType, "time bucket over non-time column %q", key.Column)
	}
	return nil
}
