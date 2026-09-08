package database_test

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
)

// The window examples read the same six seeded rows as the aggregate examples
// (see aggregateSeed in fixture_test.go) and are written the way project code is written,
// through the generated Cols vars the fixture mirrors. The SQL each one
// renders is quoted in MySQL spelling; the other dialects differ only in the
// identifier quotes.

// RowNumber numbers the rows of each partition in the window's order, and
// Qualify keeps the rows the window function selects: here the latest row of
// every category. The framework completes the order with the primary key, so
// the two beta rows sharing a time number the same way on every run.
// Rendered:
//
//	SELECT * FROM (
//	  SELECT `id` AS `id`, `category` AS `category`, `amount` AS `amount`,
//	         ROW_NUMBER() OVER (PARTITION BY `category` ORDER BY `occurred_at` DESC, `id` ASC) AS `rn`
//	  FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	) AS q WHERE `q`.`rn` = ? ORDER BY `category` ASC
func ExampleSelect_rowNumber() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		ID       string
		Category string
		Amount   int64
		Rn       int64
	}
	rn := types.RowNumber().
		Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Desc())).
		As("rn")
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount, rn,
	).
		Qualify(rn.Eq(1)).
		OrderBy(TestAggregateRecordCols.Category.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.ID, r.Amount)
	}
	// Output:
	// alpha a3 300
	// beta a4 400
	// gamma a6 600
}

// An aggregate over an ordered window accumulates row by row: SUM becomes a
// running total within each partition. The frame is fixed to the rows from
// the partition's first up to the current one, so a4 and a5, which share a
// time, step 400 then 900 instead of both showing 900. Rendered:
//
//	SELECT `id` AS `id`, `amount` AS `amount`,
//	       COALESCE(SUM(`amount`) OVER (PARTITION BY `category` ORDER BY `occurred_at` ASC, `id` ASC
//	                ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW), 0) AS `running`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL ORDER BY `id` ASC
func ExampleSelect_runningTotal() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		ID      string
		Amount  int64
		Running int64
	}
	running := TestAggregateRecordCols.Amount.Sum().
		Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Asc())).
		As("running")
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Amount, running,
	).
		OrderBy(TestAggregateRecordCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.ID, r.Amount, r.Running)
	}
	// Output:
	// a1 100 100
	// a2 200 300
	// a3 300 600
	// a4 400 400
	// a5 500 900
	// a6 600 600
}

// A window without an order reads the whole partition, so every row carries
// its partition's figures next to its own: a row's share of its category
// without grouping the rows away. Rendered:
//
//	SELECT `id` AS `id`, `amount` AS `amount`,
//	       COUNT(*) OVER (PARTITION BY `category`) AS `rows`,
//	       COALESCE(SUM(`amount`) OVER (PARTITION BY `category`), 0) AS `total`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL ORDER BY `id` ASC
func ExampleSelect_partitionTotal() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		ID     string
		Amount int64
		Rows   int64
		Total  int64
	}
	byCategory := types.PartitionBy(TestAggregateRecordCols.Category)
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Amount,
		types.Count().Over(byCategory).As("rows"),
		TestAggregateRecordCols.Amount.Sum().Over(byCategory).As("total"),
	).
		OrderBy(TestAggregateRecordCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.ID, r.Amount, r.Rows, r.Total)
	}
	// Output:
	// a1 100 3 600
	// a2 200 3 600
	// a3 300 3 600
	// a4 400 2 900
	// a5 500 2 900
	// a6 600 1 600
}

// Lag and Lead read a column from the previous and the next row of the
// window; the edge rows of a partition have none and yield NULL, so the
// fields are pointers. Rendered for Lag:
//
//	LAG(`amount`) OVER (PARTITION BY `category` ORDER BY `occurred_at` ASC, `id` ASC) AS `previous`
func ExampleSelect_lagLead() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		ID       string
		Amount   int64
		Previous *int64
		Next     *int64
	}
	window := types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Asc())
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Amount,
		TestAggregateRecordCols.Amount.Lag().Over(window).As("previous"),
		TestAggregateRecordCols.Amount.Lead().Over(window).As("next"),
	).
		OrderBy(TestAggregateRecordCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	show := func(v *int64) string {
		if v == nil {
			return "-"
		}
		return strconv.FormatInt(*v, 10)
	}
	for _, r := range rows {
		fmt.Println(r.ID, show(r.Previous), r.Amount, show(r.Next))
	}
	// Output:
	// a1 - 100 200
	// a2 100 200 300
	// a3 200 300 -
	// a4 - 400 500
	// a5 400 500 -
	// a6 - 600 -
}

// A window over a grouped projection reads the groups: the ranking orders by
// a measure, alpha and gamma tie on it, and the three ranking functions
// answer the tie differently. RANK and DENSE_RANK keep their peers; only
// ROW_NUMBER takes the group key as a tie breaker. Rendered for RANK:
//
//	RANK() OVER (ORDER BY COALESCE(SUM(`amount`), 0) DESC) AS `rank`
func ExampleSelect_rank() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category  string
		Total     int64
		Rank      int64
		DenseRank int64
		RowNumber int64
	}
	total := TestAggregateRecordCols.Amount.Sum().As("total")
	byTotal := types.OrderBy(total.Desc())
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(), total,
		types.Rank().Over(byTotal),
		types.DenseRank().Over(byTotal),
		types.RowNumber().Over(byTotal),
	).
		OrderBy(types.RowNumber().Over(byTotal).Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.Total, r.Rank, r.DenseRank, r.RowNumber)
	}
	// Output:
	// beta 900 1 1 1
	// alpha 600 2 2 2
	// gamma 600 2 2 3
}

// Over a grouped projection an aggregate window adds up the group measures:
// SUM over the partition of the per-group sums is each category's total next
// to its groups' totals. Rendered:
//
//	SELECT `category` AS `category`, `status` AS `status`,
//	       COALESCE(SUM(`amount`), 0) AS `total`,
//	       COALESCE(SUM(SUM(`amount`)) OVER (PARTITION BY `category`), 0) AS `category_total`
//	FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL
//	GROUP BY `category`, `status` ORDER BY `category` ASC, `status` ASC
func ExampleSelect_windowOverGroups() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		Category      string
		Status        string
		Total         int64
		CategoryTotal int64
	}
	rows := make([]row, 0)
	if err := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Status.Group(),
		TestAggregateRecordCols.Amount.Sum().As("total"),
		TestAggregateRecordCols.Amount.Sum().Over(types.PartitionBy(TestAggregateRecordCols.Category)).As("category_total"),
	).
		OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.Status.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.Status, r.Total, r.CategoryTotal)
	}
	// Output:
	// alpha done 300 600
	// alpha failed 300 600
	// beta done 400 900
	// beta failed 500 900
	// gamma done 600 600
}

// Count on a row-level select reports the rows after Qualify, which is the
// total a paginated latest-per-group page needs; one builder serves the page
// and the total. Rendered for the count:
//
//	SELECT count(*) FROM (SELECT * FROM (SELECT ... ROW_NUMBER() OVER (...) AS `rn` FROM ...) AS q
//	  WHERE `q`.`rn` = ?) AS grouped
func ExampleSelect_qualifyCount() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type row struct {
		ID       string
		Category string
		Rn       int64
	}
	rn := types.RowNumber().
		Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Desc())).
		As("rn")
	latest := database.Select[*TestAggregateRecord, row](context.Background(),
		TestAggregateRecordCols.ID, TestAggregateRecordCols.Category, rn,
	).
		Qualify(rn.Eq(1)).
		OrderBy(TestAggregateRecordCols.Category.Asc()).
		Limit(2)

	total := 0
	if err := latest.Count(&total); err != nil {
		panic(err)
	}
	page := make([]row, 0)
	if err := latest.Scan(&page); err != nil {
		panic(err)
	}
	fmt.Println("total:", total)
	for _, r := range page {
		fmt.Println(r.Category, r.ID)
	}
	// Output:
	// total: 3
	// alpha a3
	// beta a4
}
