package database

import (
	"database/sql"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
)

// The select builder: the specification a Select call assembles, its
// terminals, and the shape the renderer and the validator agree on. The
// grouped side — measures, group keys, HAVING — lives in select_group.go, the
// window side in select_window.go, the constants in select_literal.go, and
// the side a union reads in union.go.

// This file holds the projection's validation: the pass that turns the terms
// a select was built with into the shape the statement is rendered from, and
// everything that decides whether a term, a condition, an ordering or a
// result row is one the query can answer. The rendering itself is in
// select.go and the feature files beside it.

// validate checks the projection against the model schema and the result row
// before any SQL is built, and returns what the renderer needs to know about
// the projection's shape. The mode says whether the projection is read on its
// own or as a member of a union, which changes two rules: a member may be a
// plain projection of columns, and it carries no ordering or paging.
func (a *selector[M, R]) validate(mode buildMode) (projectionShape, error) {
	shape := projectionShape{}
	if len(a.terms) == 0 {
		return shape, ErrEmptyProjection
	}
	if mode.branch() && (len(a.orders) > 0 || a.hasLimit) {
		return shape, ErrNestedSelectOrdered
	}
	columns, err := modelschema.Columns(a.db.typ)
	if err != nil {
		return shape, errors.Wrapf(err, "resolve columns of %s", a.db.typ)
	}
	shape.columns = columnsByName(columns)
	shape.main = a.db.outerTableName()
	shape.mainInfo = tableInfoOf(columns)
	if err = a.resolveJoins(&shape); err != nil {
		return shape, err
	}
	if shape.derived, err = a.derivedTerms(shape); err != nil {
		return shape, err
	}
	// A joined select's term altered — passed under another alias, or with
	// a window or conditions of its own — is not the term the select
	// projects: it would read as a measure or a column of the select's model
	// and fail on some other term of the projection, far from the mistake,
	// so it is named here. A term the query could compute itself is the
	// query's own under an alias of its own; under the select's alias it
	// would pass for the select's term, and is refused; one a select projects
	// alike was refused above.
	for _, t := range a.terms {
		if _, derived := a.derivedOf(t, shape); derived {
			continue
		}
		own := a.ownTerm(t)
		// Every joined select is asked, whichever of them projects the term.
		for _, jt := range shape.joins {
			if jt.sub == nil {
				continue
			}
			alias, projected := jt.sub.projectsAs(t)
			switch {
			case !projected:
			case !own:
				return shape, errors.Wrapf(ErrJoinSelectColumn, "%q is the term %q of the joined select over %q altered, under another alias or with a window or conditions of its own; pass the very term the select projects", termAlias(t), alias, jt.table)
			case termAlias(t) == alias:
				return shape, errors.Wrapf(ErrDuplicateAlias, "%q is the term the joined select over %q projects, with a window or conditions of its own: neither the select's term read through nor the query's own kept apart; pass the very term to read it through, or alias the query's own term differently", alias, jt.table)
			}
		}
	}

	// The projection takes one of two shapes, and which one decides what the
	// keys mean. An aggregate or an explicit group key makes it grouped: every
	// key-shaped term is a group key, a window reads the groups, and a plain
	// column has no place, because a column next to an aggregate is either
	// grouped by or aggregated. Window functions alone make it row-level:
	// plain columns and time buckets are projected as they are.
	windowed, measures := 0, 0
	for _, t := range a.terms {
		if _, derived := a.derivedOf(t, shape); derived {
			// A joined select's term is a column of the derived table, whatever
			// function it applied inside the select: it neither groups nor
			// windows the query.
			continue
		}
		if isWindowFn(types.TermFnOf(t)) && !t.IsWindowed() {
			return shape, errors.Wrapf(ErrWindowFnWithoutWindow, "%q", termAlias(t))
		}
		switch {
		case t.IsWindowed() && !t.IsMeasure():
			return shape, errors.Wrapf(ErrWindowOnKey, "%q", termAlias(t))
		case t.IsWindowed() && types.TermFnOf(t) == types.FnCountDistinct:
			return shape, errors.Wrapf(ErrWindowCountDistinct, "%q", termAlias(t))
		case t.IsWindowed():
			windowed++
		case t.IsMeasure():
			measures++
			shape.grouped = true
		case t.IsGroupKey() && types.TermBucketOf(t) == types.TimeBucketNone:
			shape.grouped = true
		}
	}
	// A projection of plain columns or keys alone is a plain read wearing a
	// select's clothes, and List already does that better. Rejecting it keeps
	// one official path for reading rows. Stacked into a union it is a
	// report again, and across a join it reads what List cannot, so a member
	// and a joining select are allowed to be one.
	if measures == 0 && windowed == 0 && !mode.branch() && len(shape.joins) == 0 {
		return shape, ErrPlainSelect
	}
	for _, t := range a.terms {
		if _, derived := a.derivedOf(t, shape); derived {
			continue
		}
		switch {
		case shape.grouped && t.IsPlain():
			return shape, errors.Wrapf(ErrPlainColumnInGroupedSelect, "%q", termAlias(t))
		case shape.grouped && t.IsGroupKey():
			shape.keys = append(shape.keys, t)
		}
	}
	if err = a.groupDerivedTerms(&shape); err != nil {
		return shape, err
	}

	aliases := make(map[string]struct{}, len(a.terms))
	for _, t := range a.terms {
		if err = a.validateTerm(t, shape); err != nil {
			return shape, err
		}
		alias := termAlias(t)
		if len(alias) == 0 {
			return shape, errors.Wrapf(ErrInvalidAlias, "%s carries no alias; a term without a column is named with As", termLabel(t))
		}
		if !aliasPattern.MatchString(alias) {
			return shape, errors.Wrapf(ErrInvalidAlias, "%q on %s", alias, termLabel(t))
		}
		if _, dup := aliases[alias]; dup {
			return shape, errors.Wrapf(ErrDuplicateAlias, "%q", alias)
		}
		aliases[alias] = struct{}{}
	}
	// HAVING and ORDER BY are rendered from the term they carry, not from the
	// alias, so matching the alias alone is not enough: a term with the same
	// alias but a different expression would filter or sort by something the
	// projection never declared. Requiring the whole term to match makes the
	// two agree by construction.
	if err = a.validateHaving(shape); err != nil {
		return shape, err
	}
	if err = a.validateQualify(shape); err != nil {
		return shape, err
	}
	for _, o := range a.orders {
		if err = a.validateOrdering(o, shape); err != nil {
			return shape, err
		}
	}
	if shape.resultOrder, err = a.validateResultRow(aliases, shape); err != nil {
		return shape, err
	}
	return shape, nil
}

// validateConditionValue checks the operator and value of a having or qualify
// condition. The comparison is rendered with the value bound, so anything SQL
// cannot order either fails at the database or, for nil, compares against
// NULL and quietly answers with no rows at all; and a value of another kind
// than the term yields — text against a count, a number against a name — is
// a comparison some dialects answer with no rows rather than an error, so it
// is refused where both kinds are known.
func validateConditionValue(c types.TermCondition, alias string, yields valueKind) error {
	if !types.TermConditionOpOf(c).Valid() {
		return errors.Wrapf(ErrUnknownCompareOp, "%q", types.TermConditionOpOf(c))
	}
	if types.TermConditionValueOf(c) == nil {
		return errors.Wrapf(ErrHavingValue, "%q compares against nil", alias)
	}
	if k := reflect.ValueOf(types.TermConditionValueOf(c)).Kind(); k == reflect.Slice || k == reflect.Array || k == reflect.Map {
		return errors.Wrapf(ErrHavingValue, "%q compares against a %s", alias, k)
	}
	// A typed nil pointer slips past the untyped nil check above but binds
	// the same way: the driver dereferences non-nil pointers and turns a
	// nil one at any depth into NULL, which quietly answers with no rows.
	v := reflect.ValueOf(types.TermConditionValueOf(c))
	for ; v.Kind() == reflect.Pointer; v = v.Elem() {
		if v.IsNil() {
			return errors.Wrapf(ErrHavingValue, "%q compares against a nil %s", alias, v.Type())
		}
	}
	if given := valueKindOf(v); yields != kindUnknown && given != kindUnknown && given != yields {
		return errors.Wrapf(ErrHavingValueType, "%q yields %s, the value %v is %s", alias, yields, types.TermConditionValueOf(c), given)
	}
	return nil
}

// valueKind is the kind of value a term yields or a condition compares
// against, as far as a comparison between the two can be judged before the
// query runs: a number, text, or an instant. Anything else is unknown and
// left to the database.
type valueKind int

const (
	kindUnknown valueKind = iota
	kindNumeric
	kindText
	kindTime
)

func (k valueKind) String() string {
	switch k {
	case kindNumeric:
		return "a number"
	case kindText:
		return "text"
	case kindTime:
		return "an instant"
	default:
		return "an unknown kind"
	}
}

// valueKindOf classifies a condition's value by its Go type, the pointer
// already dereferenced.
func valueKindOf(v reflect.Value) valueKind {
	if v.Type() == timeType {
		return kindTime
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return kindNumeric
	case reflect.String:
		return kindText
	default:
		return kindUnknown
	}
}

// termKind reports the kind of value a term yields: the counts and ranks
// are numbers whatever they count, AVG and SUM are numbers over the numeric
// columns they exist on, a time bucket and a literal are text, and every
// other term yields what its column stores. A joined select's term is
// classified by the select that projects it, which is out of reach here.
func (a *selector[M, R]) termKind(t types.Term, shape projectionShape) valueKind {
	if _, derived := a.derivedOf(t, shape); derived {
		return kindUnknown
	}
	switch {
	case t.IsLiteral(), types.TermBucketOf(t) != types.TimeBucketNone:
		return kindText
	case types.TermFnOf(t) == types.FnCount, types.TermFnOf(t) == types.FnCountDistinct, types.TermFnOf(t) == types.FnRowNumber,
		types.TermFnOf(t) == types.FnRank, types.TermFnOf(t) == types.FnDenseRank, types.TermFnOf(t) == types.FnSum, types.TermFnOf(t) == types.FnAvg:
		return kindNumeric
	}
	column, err := a.columnOf(types.TermTableOf(t), types.TermColumnOf(t), shape)
	if err != nil {
		return kindUnknown
	}
	switch modelschema.ClassifyColumn(column.Type) {
	case modelschema.ColumnClassNumeric:
		return kindNumeric
	case modelschema.ColumnClassTime:
		return kindTime
	}
	typ := column.Type
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.String {
		return kindText
	}
	return kindUnknown
}

// validateOrdering checks one ordering of the select: a term order must name a
// projected term, a column order a projected column or group key, so the
// output can never be sorted by something it does not carry.
func (a *selector[M, R]) validateOrdering(o types.Ordering, shape projectionShape) error {
	switch o := o.(type) {
	case types.TermOrder:
		if !types.TermOrderDirectionOf(o).Valid() {
			return errors.Wrapf(ErrUnknownOrderDirection, "%q", types.TermOrderDirectionOf(o))
		}
		if !a.isSelected(types.TermOrderTermOf(o)) {
			return errors.Wrapf(ErrOrderTermNotSelected, "%q", termAlias(types.TermOrderTermOf(o)))
		}
	case types.Order:
		if !types.OrderDirectionOf(o).Valid() {
			return errors.Wrapf(ErrUnknownOrderDirection, "%q", types.OrderDirectionOf(o))
		}
		if _, ok := a.selectedColumn(o.Table(), o.Column(), shape.main); !ok {
			return errors.Wrapf(ErrOrderTermNotSelected, "column %q, which a plain name matches by column name rather than by alias; order by the term itself to sort by its alias", o.Column())
		}
	default:
		return errors.Wrapf(ErrUnknownOrderDirection, "%T", o)
	}
	return nil
}

// selectedColumn finds the projected term that carries a column as it is: a
// group key or a plain column, never a bucket, a measure or a window over it.
// The table narrows the match to that table's column, which tells two joined
// tables' columns of one name apart; an ordering built from a plain name
// carries none and names the queried model's column, as a plain name does
// everywhere else, never whichever table's column happens to be projected
// first.
func (a *selector[M, R]) selectedColumn(table, column, main string) (types.Term, bool) {
	if len(table) == 0 {
		table = main
	}
	for _, t := range a.terms {
		if types.TermFnOf(t) != types.FnNone || types.TermBucketOf(t) != types.TimeBucketNone || types.TermColumnOf(t) != column {
			continue
		}
		if termTable := types.TermTableOf(t); len(termTable) > 0 && termTable != table || len(termTable) == 0 && table != main {
			continue
		}
		return t, true
	}
	return types.Term{}, false
}

// selectedColumnTerm is selectedColumn for a column validation has already
// matched.
func (a *selector[M, R]) selectedColumnTerm(table, column, main string) types.Term {
	term, _ := a.selectedColumn(table, column, main)
	return term
}

// validateTerm checks that a term names a real column and that the column's
// type accepts the function. The type check only bites on minted references:
// a term built from a generated column reference cannot reach a function its
// type rejects, because the reference does not carry the method, while a
// reference minted by hand names whatever type its author chose.
func (a *selector[M, R]) validateTerm(t types.Term, shape projectionShape) error {
	// The renderer composes SQL from these constants, so a value from outside
	// the closed set would reach the statement as text.
	if !types.TermFnOf(t).Valid() {
		return errors.Wrapf(ErrUnknownTermFn, "%q", types.TermFnOf(t))
	}
	if !types.TermBucketOf(t).Valid() {
		return errors.Wrapf(ErrUnknownTimeBucket, "%q", types.TermBucketOf(t))
	}
	if _, derived := a.derivedOf(t, shape); derived {
		// The joined select validated the term against its own model; here
		// it is a column of the derived table, with nothing left to check.
		return nil
	}
	if err := a.validateGrouping(t); err != nil {
		return err
	}
	if len(types.TermConditionsOf(t)) > 0 {
		// A measure's conditions render inside its CASE, which Count leaves
		// out of its statement; they are rendered here once so a predicate
		// the renderer cannot place fails whichever terminal runs first.
		if _, err := a.db.renderFilters(types.TermConditionsOf(t), false, a.whereScope(shape)); err != nil {
			return errors.Wrapf(err, "%q", termAlias(t))
		}
	}
	if err := a.validateWindow(t, shape); err != nil {
		return err
	}
	if t.IsLiteral() {
		// A constant names no column and reads no schema.
		return validateLiteral(t)
	}
	if len(types.TermColumnOf(t)) == 0 {
		// COUNT(*) and the ranking functions are the terms without a column.
		if t.IsMeasure() && (types.TermFnOf(t) == types.FnCount || types.TermFnOf(t) == types.FnRowNumber || types.TermFnOf(t) == types.FnRank || types.TermFnOf(t) == types.FnDenseRank) {
			return nil
		}
		return errors.Wrapf(ErrUnknownColumn, "term %q has no column", types.TermFnOf(t))
	}
	// A column reference carries the table it was built for, which columnOf
	// checks before the name: the queried model's, or a joined model's.
	column, err := a.columnOf(types.TermTableOf(t), types.TermColumnOf(t), shape)
	if err != nil {
		return err
	}
	// Under GROUP BY a joined row is shared by every row of the group that
	// matched it, so only the aggregates that answer the same over repeats
	// may read a joined column.
	if _, joined := shape.joined[types.TermTableOf(t)]; joined && shape.grouped && t.IsMeasure() && !joinedMeasureAllowed(types.TermFnOf(t)) {
		return errors.Wrapf(ErrJoinMeasure, "%s over %q of %q", types.TermFnOf(t), types.TermColumnOf(t), types.TermTableOf(t))
	}
	return a.validateColumnClass(t, column)
}

// validateResultRow matches the projection aliases against the fields of R in
// both directions, and returns the fields in declaration order. gorm leaves an
// unmatched field at its zero value and drops an unmatched column, so without
// this check a renamed alias shows up as a column of zeros on a report rather
// than as an error.
func (a *selector[M, R]) validateResultRow(aliases map[string]struct{}, shape projectionShape) ([]string, error) {
	typ, fields, err := resultRowFields[R]()
	if err != nil {
		return nil, err
	}
	order := make([]string, 0, len(fields))
	byName := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		order = append(order, f.DBName)
		byName[f.DBName] = struct{}{}
	}
	// The aliases are walked in order, so a result row missing several of them
	// always names the same one instead of whichever the map yielded first.
	for _, alias := range slices.Sorted(maps.Keys(aliases)) {
		if _, ok := byName[alias]; !ok {
			return nil, errors.Wrapf(ErrResultFieldMissing, "%s has no field for %q", typ, alias)
		}
	}
	// gorm leaves a non-pointer field at its zero value when it scans NULL,
	// which makes "no data" and "the answer is zero" the same number on a
	// report. Every field a NULL can actually reach must therefore be able to
	// hold it, and the requirement is enforced here rather than left to a doc
	// comment nobody reads at the call site. nullableAliases narrows the
	// demand to the terms where a NULL is reachable.
	nullable := a.nullableAliases(shape)
	for _, f := range fields {
		if _, ok := aliases[f.DBName]; !ok {
			return nil, errors.Wrapf(ErrAliasMissing, "%s.%s has no matching alias", typ, f.GoName)
		}
		if why, isNullable := nullable[f.DBName]; isNullable && !holdsNull(f.Type) {
			return nil, errors.Wrapf(ErrNullableResultField,
				"%s.%s holds %s; declare it as *%s or a sql.Null type",
				typ, f.GoName, why, f.Type)
		}
	}
	return order, nil
}

// resultRowFields resolves the result row type R to its struct type and its
// fields in declaration order, the order a union member's SELECT list
// follows. A pointer row type is read through.
func resultRowFields[R any]() (reflect.Type, []modelschema.Column, error) {
	typ := reflect.TypeFor[R]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return typ, nil, errors.Newf("result row %s is not a struct", typ)
	}
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return typ, nil, errors.Wrapf(err, "resolve fields of result row %s", typ)
	}
	// Columns answers in column-name order from a shared cache; the union
	// wants the row as it was declared, so a copy is put back into field
	// order, embedded structs included, by the index path.
	fields := slices.Clone(columns)
	slices.SortFunc(fields, func(a, b modelschema.Column) int { return slices.Compare(a.Index, b.Index) })
	return typ, fields, nil
}

// isSelected reports whether the projection declares this exact term. Equality
// covers the whole term rather than its alias, because HAVING and ORDER BY are
// rendered from the term itself: an alias match alone would let a condition
// filter by an expression the projection never selected. The alias is
// compared as projected, so a term spelled without one is the term under
// its default.
func (a *selector[M, R]) isSelected(t types.Term) bool {
	t = t.As(termAlias(t))
	for _, selected := range a.terms {
		selected = selected.As(termAlias(selected))
		if reflect.DeepEqual(selected, t) {
			return true
		}
	}
	return false
}

// nullableAliases returns the aliases whose term can come back NULL, each
// keyed to a clause naming the way the NULL arrives, ready for the validation
// message. SUM is absent because the renderer coalesces it to zero, which is
// the identity of addition rather than a guess.
//
// AVG, MIN and MAX return NULL when they see no value at all. Under GROUP BY
// every group holds at least one row, so a measure only meets that fate
// through one of three doors: the projection has no group keys, and the whole
// read is a single group that is empty when the filters match no rows; the
// measure carries conditions, and no row of a group passes them; or the
// source column is nullable, and a group holds only NULLs. A grouped,
// unconditional measure over a non-nullable column can keep a plain result
// field — the alternative would demand a pointer nothing ever sets to nil.
//
// Over a window the partition always holds the current row, so only a
// condition or a nullable source column can leave AVG, MIN or MAX empty;
// LAG and LEAD are NULL on the first and last row of every partition; and a
// plain column is as nullable as the column it projects.
func (a *selector[M, R]) nullableAliases(shape projectionShape) map[string]string {
	grouped := len(shape.keys) > 0
	nullable := make(map[string]string)
	proven := a.notNullColumns()
	for _, t := range a.terms {
		// A joined select's term is NULL where a LEFT JOIN matched no group,
		// and where the select itself answered NULL.
		if jt, derived := a.derivedOf(t, shape); derived {
			switch why, isNullable := jt.derived.nullable[termAlias(t)]; {
			case jt.left:
				nullable[termAlias(t)] = fmt.Sprintf("term %q of the joined select, which is NULL when the LEFT JOIN matches no group", termAlias(t))
			case isNullable:
				nullable[termAlias(t)] = why
			}
			continue
		}
		// A column of a LEFT JOIN table is NULL on every row the join left
		// unmatched, whatever reads it, except the counts, which count no
		// row as zero, and SUM, which the renderer coalesces to zero.
		if jt, joined := shape.joined[types.TermTableOf(t)]; joined && jt.left && types.TermFnOf(t) != types.FnCount && types.TermFnOf(t) != types.FnCountDistinct && types.TermFnOf(t) != types.FnSum {
			nullable[termAlias(t)] = fmt.Sprintf("column %q of %q, which is NULL when the LEFT JOIN matches no row", types.TermColumnOf(t), types.TermTableOf(t))
			continue
		}
		source, err := a.columnOf(types.TermTableOf(t), types.TermColumnOf(t), shape)
		known := err == nil
		switch {
		case t.IsPlain():
			if known && nullableColumn(source) && !proven[a.columnKey(types.TermTableOf(t), types.TermColumnOf(t))] {
				nullable[termAlias(t)] = fmt.Sprintf("column %q, which is nullable unless a condition on it in Where keeps NULL out", types.TermColumnOf(t))
			}
			continue
		case t.IsGroupKey():
			// The rows without a value form a group of their own, keyed NULL;
			// a bucket of NULL is NULL as well.
			if known && nullableColumn(source) && !proven[a.columnKey(types.TermTableOf(t), types.TermColumnOf(t))] {
				nullable[termAlias(t)] = fmt.Sprintf("group key %q over a nullable column, NULL for the rows without one unless a condition on it in Where keeps them out", types.TermColumnOf(t))
			}
			continue
		case types.TermFnOf(t) == types.FnLag || types.TermFnOf(t) == types.FnLead:
			nullable[termAlias(t)] = fmt.Sprintf("%s, which is NULL on the edge rows of a partition", types.TermFnOf(t))
			continue
		case types.TermFnOf(t) == types.FnAvg || types.TermFnOf(t) == types.FnMin || types.TermFnOf(t) == types.FnMax:
		default:
			continue
		}
		// The unknown-column case cannot be reached — validateTerm has already
		// rejected the term — but if it ever is, requiring the pointer is the
		// safe side of the guess.
		switch {
		case !grouped && !t.IsWindowed():
			nullable[termAlias(t)] = fmt.Sprintf("%s, which is NULL when the filters match no rows", types.TermFnOf(t))
		case len(types.TermConditionsOf(t)) > 0:
			nullable[termAlias(t)] = fmt.Sprintf("a conditional %s, which is NULL for a group where no row passes its conditions", types.TermFnOf(t))
		case !known || (nullableColumn(source) && !proven[a.columnKey(types.TermTableOf(t), types.TermColumnOf(t))]):
			nullable[termAlias(t)] = fmt.Sprintf("%s over nullable column %q, NULL for a group holding only NULLs unless a condition on it in Where keeps them out", types.TermFnOf(t), types.TermColumnOf(t))
		}
	}
	return nullable
}

// notNullColumns returns the columns the WHERE keeps NULL out of, keyed by
// columnKey. A condition on a column is never true of NULL, IS NULL apart,
// so a row whose column is NULL fails it. A condition AND-ed at the top
// level, or inside an AND group, holds of every row read; one inside an OR
// group need not, and a subquery says nothing of the row's own columns.
func (a *selector[M, R]) notNullColumns() map[string]bool {
	proven := make(map[string]bool)
	var walk func(filters []types.Filter)
	walk = func(filters []types.Filter) {
		for _, f := range filters {
			switch f.Op() {
			case types.FilterOpAnd:
				if members, ok := f.Value().([]types.Filter); ok {
					walk(members)
				}
			case types.FilterOpOr, types.FilterOpExists, types.FilterOpFalse:
			case types.FilterOpIsNull:
				if isNull, ok := f.Value().(bool); ok && !isNull {
					proven[a.columnKey(f.Table(), f.Column())] = true
				}
			default:
				if len(f.Column()) > 0 {
					proven[a.columnKey(f.Table(), f.Column())] = true
				}
			}
		}
	}
	walk(a.filters)
	return proven
}

// nullableColumn reports whether a model column can hold NULL: its Go type
// tells NULL apart and the schema does not forbid it.
func nullableColumn(c modelschema.Column) bool { return holdsNull(c.Type) && !c.NotNull }

// scannerType is the interface a field implements to decode a raw database
// value itself, which is how the sql.Null wrappers represent absence.
var scannerType = reflect.TypeFor[sql.Scanner]()

// holdsNull reports whether a result row field can tell NULL apart from the
// zero value. A pointer does it by being nil; a sql.Null wrapper does it by
// carrying a Valid flag. Anything else silently reads NULL as zero, which is
// what makes "no rows" and "the answer is zero" the same number on a report.
func holdsNull(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		return true
	}
	return typ.Implements(scannerType) || reflect.PointerTo(typ).Implements(scannerType)
}
