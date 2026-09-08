package database_test

import (
	"context"
	"fmt"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/types"
)

// The semi-join examples: FilterExists and FilterNotExists tie the records
// described at aggregateSeed in fixture_test.go to the tags described at
// tagSeed. The SQL each one renders is quoted in MySQL spelling.

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
