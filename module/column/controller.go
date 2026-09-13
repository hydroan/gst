package column

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	. "github.com/hydroan/gst/internal/response"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/urlquery"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type column struct{}

// func (cs *column) Get(c *gin.Context) {
// 	columnUser := []string{
// 		"name",
// 		"email",
// 	}
//
// 	switch c.Param(consts.PARAM_ID) {
// 	case "user":
// 		// cs.Sample(c)
// 		cs.GetColumns(c, "samples", columnUser)
// 	default:
// 		zap.S().Warn("unknow id: ", c.Param(consts.PARAM_ID))
// 		ResponseJSON(c, CodeSuccess)
// 	}
// }

func (cs *column) QueryColumns(query map[string][]string, tableName string, columns []string, db ...*gorm.DB) (map[string][]string, error) {
	return queryColumnsWithQuery(tableName, columns, query, db...)
}

func (cs *column) GetColumns(c *gin.Context, tableName string, columns []string, db ...*gorm.DB) {
	columnRes, err := queryColumnsWithQuery(tableName, columns, c.Request.URL.Query(), db...)
	if err != nil {
		zap.S().Error(err)
		JSON(c, CodeFailure)
		return
	}
	JSON(c, CodeSuccess, columnRes)
}

// queryColumns only queries which distinct values each column has.
//
// select group_id from samples group by group_id;
// +----------+
// | group_id |
// +----------+
// | BJ       |
// | NU       |
// | XS       |
// | ZJ       |
// +----------+
//
//nolint:unused,unparam
func queryColumns(table string, columns []string, db ...*gorm.DB) (map[string][]string, error) {
	_db := database.DB()
	if len(db) > 0 {
		if db[0] != nil {
			_db = db[0]
		}
	}
	cr := make(map[string][]string)
	sql := "SELECT `%s` FROM `%s` WHERE `%s` IS NOT NULL AND `deleted_at` IS NULL GROUP BY `%s`"

	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(len(columns))
	for _, column := range columns {
		go func(column string) {
			defer wg.Done()
			statement := fmt.Sprintf(sql, column, table, column, column)
			rows, err := _db.Raw(statement).Rows()
			if err != nil {
				zap.S().Error(err)
				return
			}
			if rows == nil {
				zap.S().Warnw("rows is nil for column "+column, "sql", statement)
				return
			}
			defer rows.Close()
			results := make([]string, 0)
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					zap.S().Error(err)
					return
				}
				// An empty value is useless as a frontend filter option: it either
				// matches nothing or filters nothing, so skip it.
				if len(name) == 0 {
					zap.S().Warnf("empty name for column: %s", column)
					continue
				}
				results = append(results, name)
			}

			mu.Lock()
			cr[column] = results
			mu.Unlock()
		}(column)
	}
	wg.Wait()
	return cr, nil
}

// deletedAtColumn is the soft-delete timestamp a framework model carries; a
// row that has it set is not an answer to what values a column holds now.
const deletedAtColumn = "deleted_at"

// queryColumnsWithQuery answers which distinct values each of the named
// columns has, narrowed by the filters the request carries:
//
//	SELECT `group_id` FROM `samples` WHERE `group_id` IS NOT NULL
//	  AND `deleted_at` IS NULL AND `region` IN ('east') GROUP BY `group_id`
//
// The filters come from a URL query string, so none of it may reach the
// statement as text: a filter may only name one of the columns the module was
// registered with, its values bind as statement parameters, and gorm quotes
// every identifier for the dialect in use. The filters are applied in column
// order, so one request always renders one statement.
func queryColumnsWithQuery(table string, columns []string, query map[string][]string, db ...*gorm.DB) (map[string][]string, error) {
	cr := make(map[string][]string, len(columns))
	if len(columns) == 0 {
		return cr, nil
	}
	filters, err := filterConditions(columns, query)
	if err != nil {
		return nil, err
	}

	_db := database.DB()
	if len(db) > 0 {
		if db[0] != nil {
			_db = db[0]
		}
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed error
	)
	wg.Add(len(columns))
	for _, column := range columns {
		go func(column string) {
			defer wg.Done()
			results, err := distinctValues(_db, table, column, filters)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// The caller asked for every column's values; answering with
				// the columns that happened to succeed would read as "these
				// are all the values there are".
				if failed == nil {
					failed = err
				}
				return
			}
			cr[column] = results
		}(column)
	}
	wg.Wait()
	if failed != nil {
		return nil, failed
	}
	return cr, nil
}

// filterConditions turns the request's query string into the conditions every
// column query carries. A parameter that names no registered column is
// reported rather than dropped: a filter nobody applies would widen the answer
// without saying so. Framework parameters, the "_" prefix namespace, are not
// column filters and are skipped, as they are wherever a query string is read.
func filterConditions(columns []string, query map[string][]string) ([]clause.Expression, error) {
	registered := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		registered[column] = struct{}{}
	}

	conditions := make([]clause.Expression, 0, len(query))
	unsupported := make([]string, 0)
	for _, k := range slices.Sorted(maps.Keys(query)) { // v eg: [process,package,]
		if strings.HasPrefix(k, "_") {
			continue
		}
		if _, ok := registered[k]; !ok {
			unsupported = append(unsupported, k)
			continue
		}
		items := filterValues(query[k])
		if len(items) == 0 {
			continue
		}
		conditions = append(conditions, clause.IN{Column: clause.Column{Name: k}, Values: items})
	}
	if len(unsupported) > 0 {
		return nil, urlquery.UnsupportedParameterError(unsupported)
	}
	return conditions, nil
}

// filterValues reads one parameter's values: a repeated key and a comma
// separated list both mean "any of these". Blanks are dropped, which is what
// an untouched filter box sends.
func filterValues(raw []string) []any {
	items := make([]any, 0, len(raw))
	for _, value := range raw {
		for item := range strings.SplitSeq(value, ",") {
			if item = strings.TrimSpace(item); len(item) > 0 {
				items = append(items, item)
			}
		}
	}
	return items
}

// distinctValues reads the values one column holds, under the filters shared
// by every column of the request.
func distinctValues(_db *gorm.DB, table, column string, filters []clause.Expression) ([]string, error) {
	tx := _db.Table(table).
		Where(clause.Neq{Column: clause.Column{Name: column}, Value: nil}).
		Where(clause.Eq{Column: clause.Column{Name: deletedAtColumn}, Value: nil})
	for _, filter := range filters {
		tx = tx.Where(filter)
	}
	tx = tx.Group(column)

	scanned := make([]string, 0)
	if err := tx.Pluck(column, &scanned).Error; err != nil {
		return nil, err
	}
	// fmt.Println("--------------------- statement: ", tx.Statement.SQL.String())

	results := make([]string, 0, len(scanned))
	for _, name := range scanned {
		// An empty value is useless as a frontend filter option: it either
		// matches nothing or filters nothing, so skip it.
		if len(name) == 0 {
			zap.S().Debugf("empty name for column: %s", column)
			continue
		}
		results = append(results, name)
	}
	return results, nil
}

// queryColumnsAndCount queries which distinct values each column has, together
// with the number of records holding each value.
//
// select group_id, count(*) as group_count from samples group by group_id;
// +----------+-------------+
// | group_id | group_count |
// +----------+-------------+
// | BJ       |         110 |
// | NU       |         800 |
// | XS       |         328 |
// | ZJ       |         215 |
// +----------+-------------+
//
// select owner_id, count(*) as owner_count from samples group by owner_id;
// +-------------------------------------+-------------+
// | owner_id                            | owner_count |
// +-------------------------------------+-------------+
// |                                     |        1236 |
// | id-ea0ed19af82622a997edf6c2aab262bc |          28 |
// | id-9011520298e3aca4f245e075dd873d02 |          10 |
// | id-3a87018f46f9d37fa811503745fc0b05 |           5 |
// | id-60e10a8929373b1ac0aff828dd5cacf8 |          30 |
// | id-198eb3d20e4783518acee52b1bc48356 |          20 |
// | id-ed452e84ca58c26719ea0ca8b8acecdd |           4 |
// | id-1d7f4ac953b109f2a7e2a2366f5f315e |          72 |
// | id-c6bbbc7f089b356cd45396e3443d1558 |           2 |
// | id-39c14e77f3504a8ca05f3681e9d0470b |           3 |
// | id-095e7e716c0a8262b3dad7888eb4776b |          42 |
// | id-7e8d4fb875bed78400bc5bbca88eed0c |           1 |
// +-------------------------------------+-------------+
//
//nolint:unused,unparam
func queryColumnsAndCount(table string, columns []string, db ...*gorm.DB) (columnResult, error) {
	_db := database.DB()
	if len(db) > 0 {
		if db[0] != nil {
			_db = db[0]
		}
	}
	cr := make(map[string][]result)
	sql := "SELECT `%s`, count(*) as count FROM `%s` where `deleted_at` IS NULL GROUP BY `%s`"
	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(len(columns))
	for _, column := range columns {
		go func(column string) {
			defer wg.Done()
			statement := fmt.Sprintf(sql, column, table, column)
			rows, err := _db.Raw(statement).Rows()
			if err != nil {
				zap.S().Error(err)
				return
			}
			if rows == nil {
				zap.S().Warnw("rows is nil for column "+column, "sql", statement)
				return
			}
			defer rows.Close()
			results := make([]result, 0)
			for rows.Next() {
				var name string
				var count uint
				if err := rows.Scan(&name, &count); err != nil {
					zap.S().Error(err)
					return
				}
				// An empty value is useless as a frontend filter option: it either
				// matches nothing or filters nothing, so skip it.
				if len(name) == 0 {
					zap.S().Warnf("empty name for column: %s", column)
					continue
				}
				results = append(results, result{name, count})
			}
			mu.Lock()
			cr[column] = results
			mu.Unlock()
		}(column)
	}
	wg.Wait()
	return cr, nil
}

//nolint:unused
type columnResult map[string][]result

//nolint:unused
type result struct {
	Name  string
	Count uint
}
