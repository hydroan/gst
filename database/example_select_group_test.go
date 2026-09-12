package database_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
)

// The grouped examples: group keys, measures, conditional measures, time
// buckets and HAVING, over the six seeded rows described at aggregateSeed in
// fixture_test.go. The SQL each one renders is quoted in MySQL spelling.

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
