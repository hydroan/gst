package database_test

import (
	"context"
	"fmt"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
)

// The examples below are the runnable reference for the Select builder: one
// per keyword, each reading the six seeded rows described at aggregateSeed in
// fixture_test.go and printing its result rows. They are written the way
// project code is written: through the Cols vars gg gen generates next to a
// model, which the fixture mirrors under the same names. They print rows
// rather than SQL because the rendered statement differs between dialects only
// in its identifier quotes while the rows must not differ at all; the SQL each
// one renders is quoted in its comment in MySQL spelling, and locked per
// dialect by the dry-run tests. This file covers the builder itself; the
// grouped side is in example_select_group_test.go, the window side in
// example_select_window_test.go, the semi joins in example_filter_test.go and
// the unions in example_union_test.go.

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

// OrderBy sorts by a projection term, Limit caps the rows and Page keeps one
// page of them, which is how a report ranks groups and pages through them. A second
// order term breaks the tie between alpha and gamma so the page is stable.
// Rendered for the first page:
//
//	SELECT `category` AS `category`, COALESCE(SUM(`amount`), 0) AS `total`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category` ORDER BY `total` DESC,`category` ASC LIMIT ?
func ExampleSelect_orderByLimitPage() {
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
	if err := ranking.Page(2, 2).Scan(&second); err != nil {
		panic(err)
	}
	fmt.Println("page 1:", first)
	fmt.Println("page 2:", second)
	// Output:
	// page 1: [{beta 900} {alpha 600}]
	// page 2: [{gamma 600}]
}

// Count reports how many rows the projection produces, which is the total a
// paginated report needs; the builder's OrderBy, Limit and Page do not
// apply to it, so one builder serves both the page and the total. Rendered:
//
//	SELECT count(*) FROM (SELECT `category` AS `category` FROM `test_aggregate_records`
//	  WHERE `status` = ? AND `test_aggregate_records`.`deleted_at` IS NULL GROUP BY `category`) AS `grouped`
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
