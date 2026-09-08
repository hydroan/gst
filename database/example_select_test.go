package database_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
)

// The examples below are the runnable reference for the Select builder: one
// per keyword, each reading the six seeded rows described at the top of
// select_test.go and printing its result rows. They are written the way
// project code is written: through the Cols vars gg gen generates next to a
// model, which the fixture mirrors under the same names. They print rows rather than
// SQL because the rendered statement differs between dialects only in its
// identifier quotes while the rows must not differ at all; the SQL each one
// renders is quoted in its comment in MySQL spelling, and locked per dialect
// by the dry-run tests in select_test.go.
//
// Examples run without a testing.T, so the seed helpers panic on failure: an
// example that cannot seed its rows has nothing to demonstrate.

// seedAggregateExample resets the aggregate fixture to the seed every example
// reads.
func seedAggregateExample() {
	cleanupAggregateData()
	if err := database.Database[*TestAggregateRecord](context.Background()).Create(aggregateSeed()...); err != nil {
		panic(err)
	}
}

// seedTagExample resets the related-row fixture the semi-join examples read.
func seedTagExample() {
	cleanupTagData()
	if err := database.Database[*TestRecordTag](context.Background()).Create(tagSeed()...); err != nil {
		panic(err)
	}
}

// Group makes a column a group key; the framework derives GROUP BY from the
// group keys, so the projection and its GROUP BY cannot disagree. Sum and the
// package-level Count are measures. Rendered:
//
//	SELECT `category` AS `category`, COALESCE(SUM(`amount`), 0) AS `amount`, COUNT(*) AS `count`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category` ORDER BY `category` ASC
func ExampleSelect_group() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category string
		Amount   int64
		Count    int64
	}
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.Amount.Sum(),
		types.Count(),
	).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.Amount, r.Count)
	}
	// Output:
	// alpha 600 3
	// beta 900 2
	// gamma 600 1
}

// ScanOne reads an ungrouped projection, which is always exactly one row. SUM
// is coalesced to zero; AVG, MIN and MAX yield NULL over an empty set, so
// their result fields must be pointers. Rendered:
//
//	SELECT COALESCE(SUM(`amount`), 0) AS `total`, COUNT(*) AS `records`,
//	       MIN(`amount`) AS `smallest`, MAX(`amount`) AS `largest`, AVG(`amount`) AS `average`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
func ExampleSelect_scanOne() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type totals struct {
		Total    int64
		Records  int64
		Smallest *int64
		Largest  *int64
		Average  *float64
	}
	got := totals{}
	if err := database.Select[*TestAggregateRecord, totals](context.Background(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		types.Count().As("records"),
		TestAggregateRecordCols.Amount.Min().As("smallest"),
		TestAggregateRecordCols.Amount.Max().As("largest"),
		TestAggregateRecordCols.Amount.Avg().As("average"),
	).ScanOne(&got); err != nil {
		panic(err)
	}
	fmt.Printf("total=%d records=%d smallest=%d largest=%d average=%.1f\n",
		got.Total, got.Records, *got.Smallest, *got.Largest, *got.Average)
	// Output:
	// total=2100 records=6 smallest=100 largest=600 average=350.0
}

// The package-level Count counts rows, COUNT(*); a column's Count counts the
// rows whose value is not NULL; CountDistinct counts distinct non-NULL values.
// No seeded row has a closed_at, which is what tells the first two apart.
// Rendered:
//
//	SELECT COUNT(*) AS `rows`, COUNT(`closed_at`) AS `closed`, COUNT(DISTINCT `category`) AS `categories`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
func ExampleSelect_count() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type counts struct {
		Rows       int64
		Closed     int64
		Categories int64
	}
	got := counts{}
	if err := database.Select[*TestAggregateRecord, counts](context.Background(),
		types.Count().As("rows"),
		TestAggregateRecordCols.ClosedAt.Count().As("closed"),
		TestAggregateRecordCols.Category.CountDistinct().As("categories"),
	).ScanOne(&got); err != nil {
		panic(err)
	}
	fmt.Printf("rows=%d closed=%d categories=%d\n", got.Rows, got.Closed, got.Categories)
	// Output:
	// rows=6 closed=0 categories=3
}

// A measure's Where restricts it to the rows matching the filters, so one
// scan projects several measures over different subsets. Rendered:
//
//	SELECT `category` AS `category`,
//	       COALESCE(SUM(CASE WHEN `status` = ? THEN `amount` ELSE 0 END), 0) AS `done_amount`,
//	       COALESCE(SUM(CASE WHEN `status` = ? THEN `amount` ELSE 0 END), 0) AS `failed_amount`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category` ORDER BY `category` ASC
func ExampleSelect_conditionalMeasure() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category     string
		DoneAmount   int64
		FailedAmount int64
	}
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(),
		TestAggregateRecordCols.Amount.Sum().Where(TestAggregateRecordCols.Status.Eq("done")).As("done_amount"),
		TestAggregateRecordCols.Amount.Sum().Where(TestAggregateRecordCols.Status.Eq("failed")).As("failed_amount"),
	).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.DoneAmount, r.FailedAmount)
	}
	// Output:
	// alpha 300 300
	// beta 400 500
	// gamma 600 0
}

// ByHour truncates a time group key to the hour; the label is a string so the
// result row reads the same on every dialect. Rendered on MySQL:
//
//	SELECT DATE_FORMAT(`occurred_at`, '%Y-%m-%d %H:00:00') AS `hour`, COUNT(*) AS `count`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY DATE_FORMAT(`occurred_at`, '%Y-%m-%d %H:00:00') ORDER BY `hour` ASC
func ExampleSelect_byHour() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Hour  string
		Count int64
	}
	hour := TestAggregateRecordCols.OccurredAt.ByHour().As("hour")
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(), hour, types.Count()).
		OrderBy(hour.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Hour, r.Count)
	}
	// Output:
	// 2024-01-10 08:00:00 1
	// 2024-01-10 09:00:00 1
	// 2024-01-11 08:00:00 1
	// 2024-02-10 08:00:00 2
	// 2024-02-11 10:00:00 1
}

// ByDay truncates a time group key to the day. Rendered on MySQL:
//
//	SELECT DATE_FORMAT(`occurred_at`, '%Y-%m-%d') AS `day`, COUNT(*) AS `count`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY DATE_FORMAT(`occurred_at`, '%Y-%m-%d') ORDER BY `day` ASC
func ExampleSelect_byDay() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Day   string
		Count int64
	}
	day := TestAggregateRecordCols.OccurredAt.ByDay().As("day")
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(), day, types.Count()).
		OrderBy(day.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Day, r.Count)
	}
	// Output:
	// 2024-01-10 2
	// 2024-01-11 1
	// 2024-02-10 2
	// 2024-02-11 1
}

// ByMonth truncates a time group key to the month. Rendered on MySQL:
//
//	SELECT DATE_FORMAT(`occurred_at`, '%Y-%m') AS `month`, COALESCE(SUM(`amount`), 0) AS `amount`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY DATE_FORMAT(`occurred_at`, '%Y-%m') ORDER BY `month` ASC
func ExampleSelect_byMonth() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Month  string
		Amount int64
	}
	month := TestAggregateRecordCols.OccurredAt.ByMonth().As("month")
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(), month, TestAggregateRecordCols.Amount.Sum()).
		OrderBy(month.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Month, r.Amount)
	}
	// Output:
	// 2024-01 600
	// 2024-02 1500
}

// Having restricts the produced groups by a measure, with the six comparisons
// a measure supports. The condition renders the measure's full expression
// rather than its alias, because PostgreSQL does not accept an output alias
// in HAVING. Rendered for Gt:
//
//	SELECT `category` AS `category`, COALESCE(SUM(`amount`), 0) AS `total`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category` HAVING COALESCE(SUM(`amount`), 0) > ? ORDER BY `category` ASC
func ExampleSelect_having() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category string
		Total    int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")
	comparisons := []struct {
		name string
		cond types.TermCondition
	}{
		{"eq", total.Eq(600)},
		{"ne", total.Ne(600)},
		{"gt", total.Gt(600)},
		{"gte", total.Gte(600)},
		{"lt", total.Lt(600)},
		{"lte", total.Lte(600)},
	}
	for _, c := range comparisons {
		rows := make([]row, 0)
		if err := database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
			Having(c.cond).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
			Scan(&rows); err != nil {
			panic(err)
		}
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			names = append(names, r.Category)
		}
		if len(names) == 0 {
			names = append(names, "(none)")
		}
		fmt.Printf("total %s 600: %s\n", c.name, strings.Join(names, " "))
	}
	// Output:
	// total eq 600: alpha gamma
	// total ne 600: beta
	// total gt 600: beta
	// total gte 600: alpha beta gamma
	// total lt 600: (none)
	// total lte 600: alpha gamma
}

// OrderBy sorts by a projection term, Limit caps the rows and Offset skips
// them, which is how a report ranks groups and pages through them. A second
// order term breaks the tie between alpha and gamma so the page is stable.
// Rendered for the first page:
//
//	SELECT `category` AS `category`, COALESCE(SUM(`amount`), 0) AS `total`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category` ORDER BY `total` DESC,`category` ASC LIMIT ?
func ExampleSelect_orderByLimitOffset() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category string
		Total    int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")
	ranking := database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
		OrderBy(total.Desc(), TestAggregateRecordCols.Category.Group().Asc()).
		Limit(2)

	first := make([]row, 0)
	if err := ranking.Scan(&first); err != nil {
		panic(err)
	}
	second := make([]row, 0)
	if err := ranking.Offset(2).Scan(&second); err != nil {
		panic(err)
	}
	fmt.Println("page 1:", first)
	fmt.Println("page 2:", second)
	// Output:
	// page 1: [{beta 900} {alpha 600}]
	// page 2: [{gamma 600}]
}

// Count reports how many rows the projection produces, which is the total a
// paginated report needs; the builder's OrderBy, Limit and Offset do not
// apply to it, so one builder serves both the page and the total. Rendered:
//
//	SELECT count(*) FROM (SELECT `category` AS `category` FROM `test_aggregate_records`
//	  WHERE `status` = ? AND `test_aggregate_records`.`deleted_at` IS NULL GROUP BY `category`) AS grouped
func ExampleSelect_countRows() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category string
		Total    int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")
	report := database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), total).
		Where(TestAggregateRecordCols.Status.Eq("done")).
		OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
		Limit(2)

	groups := 0
	if err := report.Count(&groups); err != nil {
		panic(err)
	}
	page := make([]row, 0)
	if err := report.Scan(&page); err != nil {
		panic(err)
	}
	fmt.Println("groups:", groups)
	fmt.Println("page:", page)
	// Output:
	// groups: 3
	// page: [{alpha 300} {beta 400}]
}

// As renames a term, which is needed when the result field is not named after
// the column or when two measures read the same column. Rendered:
//
//	SELECT MIN(`amount`) AS `smallest`, MAX(`amount`) AS `largest`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
func ExampleSelect_as() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type bounds struct {
		Smallest *int64
		Largest  *int64
	}
	got := bounds{}
	if err := database.Select[*TestAggregateRecord, bounds](context.Background(),
		TestAggregateRecordCols.Amount.Min().As("smallest"),
		TestAggregateRecordCols.Amount.Max().As("largest"),
	).ScanOne(&got); err != nil {
		panic(err)
	}
	fmt.Printf("smallest=%d largest=%d\n", *got.Smallest, *got.Largest)
	// Output:
	// smallest=100 largest=600
}

// An empty set sums to zero but averages to NULL: the two answers differ on
// purpose, so an average that can be empty binds to a pointer. Rendered:
//
//	SELECT COALESCE(SUM(`amount`), 0) AS `total`, AVG(`amount`) AS `average`
//	FROM `test_aggregate_records` WHERE `category` = ? AND `test_aggregate_records`.`deleted_at` IS NULL
func ExampleSelect_emptySet() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type totals struct {
		Total   int64
		Average *float64
	}
	got := totals{}
	if err := database.Select[*TestAggregateRecord, totals](context.Background(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		TestAggregateRecordCols.Amount.Avg().As("average"),
	).
		Where(TestAggregateRecordCols.Category.Eq("delta")).
		ScanOne(&got); err != nil {
		panic(err)
	}
	fmt.Printf("total=%d average=%v\n", got.Total, got.Average)
	// Output:
	// total=0 average=<nil>
}

// FilterFalse is the predicate that matches nothing, for a hook that denies
// every row; it composes like any other filter. Rendered:
//
//	SELECT COUNT(*) AS `count` FROM `test_aggregate_records`
//	WHERE 1 = 0 AND `test_aggregate_records`.`deleted_at` IS NULL
func ExampleSelect_filterFalse() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type counts struct {
		Count int64
	}
	got := counts{}
	if err := database.Select[*TestAggregateRecord, counts](context.Background(), types.Count()).
		Where(types.FilterFalse()).
		ScanOne(&got); err != nil {
		panic(err)
	}
	fmt.Println("count:", got.Count)
	// Output:
	// count: 0
}

// FilterExists is a semi join: it keeps the rows that have at least one
// related row, and EqCol is the predicate that ties the related row to the
// queried one. A row matches at most once, so the counts never double the
// way a join to a one-to-many table would. FilterNotExists keeps the rows
// without such a related row. Rendered for the first projection:
//
//	SELECT `category` AS `category`, COUNT(*) AS `count` FROM `test_aggregate_records`
//	WHERE EXISTS (SELECT 1 FROM `test_record_tags`
//	              WHERE (`test_record_tags`.`record_id` = `test_aggregate_records`.`id` AND `test_record_tags`.`label` = ?)
//	                AND `test_record_tags`.`deleted_at` IS NULL)
//	  AND `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category` ORDER BY `category` ASC
func ExampleSelect_filterExists() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type row struct {
		Category string
		Count    int64
	}
	vip := types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))
	noVip := types.FilterNotExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))
	for _, scope := range []struct {
		name   string
		filter types.Filter
	}{{"vip", vip}, {"no vip", noVip}} {
		rows := make([]row, 0)
		if err := database.Select[*TestAggregateRecord, row](context.Background(), TestAggregateRecordCols.Category.Group(), types.Count()).
			Where(scope.filter).
			OrderBy(TestAggregateRecordCols.Category.Group().Asc()).
			Scan(&rows); err != nil {
			panic(err)
		}
		fmt.Println(scope.name+":", rows)
	}
	// Output:
	// vip: [{alpha 2}]
	// no vip: [{alpha 1} {beta 2} {gamma 1}]
}

// The same semi join narrows a List: FilterExists is a Filter operator, so
// every read that takes filters accepts it. Rendered:
//
//	SELECT * FROM `test_aggregate_records`
//	WHERE EXISTS (SELECT 1 FROM `test_record_tags`
//	              WHERE (`test_record_tags`.`record_id` = `test_aggregate_records`.`id` AND `test_record_tags`.`label` = ?)
//	                AND `test_record_tags`.`deleted_at` IS NULL)
//	  AND `test_aggregate_records`.`deleted_at` IS NULL ORDER BY `id` ASC
func ExampleDatabase_filterExists() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	vip := types.FilterExists[*TestRecordTag](TestRecordTagCols.RecordID.EqCol(TestAggregateRecordCols.ID), TestRecordTagCols.Label.Eq("vip"))
	records := make([]*TestAggregateRecord, 0)
	if err := database.Database[*TestAggregateRecord](context.Background()).
		WithQuery(nil, types.QueryOptions{Filters: []types.Filter{vip}}).
		WithOrder(types.Asc("id")).
		List(&records); err != nil {
		panic(err)
	}
	for _, r := range records {
		fmt.Println(r.ID, r.Category, r.Amount)
	}
	// Output:
	// a1 alpha 100
	// a3 alpha 300
}
