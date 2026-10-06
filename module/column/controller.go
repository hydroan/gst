package column

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/urlquery"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type column struct{}

func (cs *column) QueryColumns(query map[string][]string, tableName string, columns []string, db ...*gorm.DB) (map[string][]string, error) {
	return queryColumnsWithQuery(tableName, columns, query, db...)
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
