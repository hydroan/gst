package database_test

import (
	"context"
	"fmt"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
)

// The union examples stack the six seeded records (see select_test.go) with
// the three seeded tags (see tagSeed), and are written the way project code
// is written, through the generated Cols vars the fixture mirrors. The SQL
// each one renders is quoted in MySQL spelling; the other dialects differ
// only in the identifier quotes.

// UnionAll stacks the rows of several selects into one result: every record
// and every tag as one feed, with Literal telling the two apart. Each branch
// is an ordinary select scanning into the same row type, and the framework
// renders every branch's SELECT list in that row type's field order, so the
// columns line up by name rather than by the position they were written in.
// Rendered:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'record' AS `kind`, `id` AS `id`, `category` AS `category`
//	                 FROM `test_aggregate_records` WHERE `test_aggregate_records`.`deleted_at` IS NULL) AS b0
//	  UNION ALL
//	  SELECT * FROM (SELECT 'tag' AS `kind`, `id` AS `id`, `category` AS `category`
//	                 FROM `test_record_tags` WHERE `test_record_tags`.`deleted_at` IS NULL) AS b1
//	) AS u ORDER BY `category` ASC,`id` ASC
func ExampleUnionAll() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type feed struct {
		Kind     string
		ID       string
		Category string
	}
	records := database.Select[*TestAggregateRecord, feed](context.Background(),
		types.Literal("record").As("kind"), TestAggregateRecordCols.ID, TestAggregateRecordCols.Category)
	tags := database.Select[*TestRecordTag, feed](context.Background(),
		types.Literal("tag").As("kind"), TestRecordTagCols.ID, TestRecordTagCols.Category)

	rows := make([]feed, 0)
	if err := database.UnionAll[feed](context.Background(), records, tags).
		OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.ID.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.Kind, r.ID)
	}
	// Output:
	// alpha record a1
	// alpha record a2
	// alpha record a3
	// alpha tag t1
	// alpha tag t2
	// beta record a4
	// beta record a5
	// beta tag t3
	// gamma record a6
}

// A union pages like a select, and with a Limit the framework pushes the
// ordering and offset plus limit into every branch: each branch reads its
// first six rows by its own index, and the union sorts twelve rows at most
// to take the page. Rendered, with 6 bound inside each member and 3 and 3
// outside:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'record' AS `kind`, ... FROM `test_aggregate_records` WHERE ...
//	                 ORDER BY `category` ASC,`id` ASC LIMIT ?) AS b0
//	  UNION ALL
//	  SELECT * FROM (SELECT 'tag' AS `kind`, ... FROM `test_record_tags` WHERE ...
//	                 ORDER BY `category` ASC,`id` ASC LIMIT ?) AS b1
//	) AS u ORDER BY `category` ASC,`id` ASC LIMIT ? OFFSET ?
func ExampleUnionAll_pagination() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type feed struct {
		Kind     string
		ID       string
		Category string
	}
	records := database.Select[*TestAggregateRecord, feed](context.Background(),
		types.Literal("record").As("kind"), TestAggregateRecordCols.ID, TestAggregateRecordCols.Category)
	tags := database.Select[*TestRecordTag, feed](context.Background(),
		types.Literal("tag").As("kind"), TestRecordTagCols.ID, TestRecordTagCols.Category)

	page := make([]feed, 0)
	if err := database.UnionAll[feed](context.Background(), records, tags).
		OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.ID.Asc()).
		Limit(3).Offset(3).
		Scan(&page); err != nil {
		panic(err)
	}
	for _, r := range page {
		fmt.Println(r.Category, r.Kind, r.ID)
	}
	// Output:
	// alpha tag t1
	// alpha tag t2
	// beta record a4
}

// Count adds up the counts of the branches and materializes no stacked row;
// like a select's Count it ignores the ordering and paging set on the union,
// which is what the total of a paginated feed needs. Rendered:
//
//	SELECT n FROM (SELECT (SELECT COUNT(*) FROM (SELECT 1 AS `row_marker` FROM `test_aggregate_records` WHERE ...) AS b0)
//	                    + (SELECT COUNT(*) FROM (SELECT 1 AS `row_marker` FROM `test_record_tags` WHERE ...) AS b1) AS n) AS counts
func ExampleUnionAll_count() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type feed struct {
		Kind     string
		ID       string
		Category string
	}
	records := database.Select[*TestAggregateRecord, feed](context.Background(),
		types.Literal("record").As("kind"), TestAggregateRecordCols.ID, TestAggregateRecordCols.Category).
		Where(TestAggregateRecordCols.Category.Eq("alpha"))
	tags := database.Select[*TestRecordTag, feed](context.Background(),
		types.Literal("tag").As("kind"), TestRecordTagCols.ID, TestRecordTagCols.Category)

	feedOfAlpha := database.UnionAll[feed](context.Background(), records, tags).
		OrderBy(TestAggregateRecordCols.ID.Asc()).
		Limit(2)
	total := 0
	if err := feedOfAlpha.Count(&total); err != nil {
		panic(err)
	}
	page := make([]feed, 0)
	if err := feedOfAlpha.Scan(&page); err != nil {
		panic(err)
	}
	fmt.Println("total:", total)
	for _, r := range page {
		fmt.Println(r.Kind, r.ID)
	}
	// Output:
	// total: 6
	// record a1
	// record a2
}

// The columns of the branches line up by name, so a column spelled
// differently in one model is aligned with As: here every tag is listed under
// the record it marks, its record_id projected as ref next to the records'
// own id. A term shared between a branch and OrderBy lets the union sort by
// the literal too. Rendered:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'record' AS `kind`, `id` AS `ref`, `category` AS `category` FROM `test_aggregate_records` WHERE ...) AS b0
//	  UNION ALL
//	  SELECT * FROM (SELECT 'tag' AS `kind`, `record_id` AS `ref`, `category` AS `category` FROM `test_record_tags` WHERE ...) AS b1
//	) AS u ORDER BY `category` ASC,`ref` ASC,`kind` ASC
func ExampleUnionAll_columnAlias() {
	seedAggregateExample()
	defer cleanupAggregateData()
	seedTagExample()
	defer cleanupTagData()

	type feed struct {
		Kind     string
		Ref      string
		Category string
	}
	kind := types.Literal("record").As("kind")
	records := database.Select[*TestAggregateRecord, feed](context.Background(),
		kind, TestAggregateRecordCols.ID.As("ref"), TestAggregateRecordCols.Category)
	tags := database.Select[*TestRecordTag, feed](context.Background(),
		types.Literal("tag").As("kind"), TestRecordTagCols.RecordID.As("ref"), TestRecordTagCols.Category)

	rows := make([]feed, 0)
	if err := database.UnionAll[feed](context.Background(), records, tags).
		OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.ID.As("ref").Asc(), kind.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.Ref, r.Kind)
	}
	// Output:
	// alpha a1 record
	// alpha a1 tag
	// alpha a2 record
	// alpha a3 record
	// alpha a3 tag
	// beta a4 record
	// beta a4 tag
	// beta a5 record
	// gamma a6 record
}

// A branch can take any shape a select takes: a grouped projection stacks
// beside a plain one, so every category's total sits with its rows. The
// literal stays out of GROUP BY. Rendered:
//
//	SELECT * FROM (
//	  SELECT * FROM (SELECT 'total' AS `kind`, `category` AS `category`, COALESCE(SUM(`amount`), 0) AS `amount`
//	                 FROM `test_aggregate_records` WHERE ... GROUP BY `category`) AS b0
//	  UNION ALL
//	  SELECT * FROM (SELECT 'row' AS `kind`, `category` AS `category`, `amount` AS `amount`
//	                 FROM `test_aggregate_records` WHERE ...) AS b1
//	) AS u ORDER BY `category` ASC,`amount` ASC,`kind` ASC
func ExampleUnionAll_groupedBranch() {
	seedAggregateExample()
	defer cleanupAggregateData()

	type line struct {
		Kind     string
		Category string
		Amount   int64
	}
	kind := types.Literal("total").As("kind")
	totals := database.Select[*TestAggregateRecord, line](context.Background(),
		kind, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum())
	each := database.Select[*TestAggregateRecord, line](context.Background(),
		types.Literal("row").As("kind"), TestAggregateRecordCols.Category, TestAggregateRecordCols.Amount)

	rows := make([]line, 0)
	if err := database.UnionAll[line](context.Background(), totals, each).
		OrderBy(TestAggregateRecordCols.Category.Asc(), TestAggregateRecordCols.Amount.Asc(), kind.Asc()).
		Scan(&rows); err != nil {
		panic(err)
	}
	for _, r := range rows {
		fmt.Println(r.Category, r.Kind, r.Amount)
	}
	// Output:
	// alpha row 100
	// alpha row 200
	// alpha row 300
	// alpha total 600
	// beta row 400
	// beta row 500
	// beta total 900
	// gamma row 600
	// gamma total 600
}
