package column

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	. "github.com/hydroan/gst/internal/response"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
// 		// cs.Asset(c)
// 		cs.GetColumns(c, "assets", columnUser)
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

// queryColumns 只查询字段有多少种
//
// select category_level2_id from assets group by category_level2_id;
// +--------------------+
// | category_level2_id |
// +--------------------+
// | BJ                 |
// | NU                 |
// | XS                 |
// | ZJ                 |
// +--------------------+
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
	return cr, nil
}

func queryColumnsWithQuery(table string, columns []string, query map[string][]string, db ...*gorm.DB) (map[string][]string, error) {
	cr := make(map[string][]string)
	sql := "SELECT `%s` FROM `%s` WHERE `%s` IS NOT NULL AND `deleted_at` IS NULL %s GROUP BY `%s`"

	var queryBuilder strings.Builder
	for k, v := range query { // v eg: [process,package,]
		if len(k) > 0 && len(strings.Join(v, "")) > 0 {
			items := make([]string, 0)
			for _, item := range v {
				if len(item) > 0 && strings.TrimSpace(item) != "," {
					for _item := range strings.SplitSeq(item, ",") {
						if len(strings.TrimSpace(_item)) > 0 {
							items = append(items, strings.TrimSpace(_item))
						}
					}
				}
			}

			var out strings.Builder
			for i, item := range items {
				switch i {
				case 0:
					if len(items) == 1 {
						fmt.Fprintf(&out, `('%s')`, regexp.QuoteMeta(strings.TrimSpace(item)))
					} else {
						fmt.Fprintf(&out, `('%s'`, regexp.QuoteMeta(strings.TrimSpace(item)))
					}
				case len(items) - 1:
					fmt.Fprintf(&out, `,'%s')`, regexp.QuoteMeta(strings.TrimSpace(item)))
				default:
					fmt.Fprintf(&out, `,'%s'`, regexp.QuoteMeta(strings.TrimSpace(item)))
				}
			}
			if len(strings.TrimSpace(out.String())) > 0 {
				fmt.Fprintf(&queryBuilder, " AND `%s` IN %s", k, strings.TrimSpace(out.String()))
			}
		}
	}

	_db := database.DB()
	if len(db) > 0 {
		if db[0] != nil {
			_db = db[0]
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(len(columns))
	for _, column := range columns {
		go func(column string) {
			defer wg.Done()
			statement := fmt.Sprintf(sql, column, table, column, queryBuilder.String(), column)
			// fmt.Println("--------------------- statement: ", statement)
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
					zap.S().Debugf("empty name for column: %s", column)
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

// queryColumns 只查询字段有多少种, 并且计算每种字段值的个数
//
// select category_level2_id, count(*) as category_count from assets group by category_level2_id;
// +--------------------+----------------+
// | category_level2_id | category_count |
// +--------------------+----------------+
// | BJ                 |            110 |
// | NU                 |            800 |
// | XS                 |            328 |
// | ZJ                 |            215 |
// +--------------------+----------------+
//
// select department_level2_id, count(*) as department_count from assets group by department_level2_id;
// +-------------------------------------+------------------+
// | department_level2_id                | department_count |
// +-------------------------------------+------------------+
// |                                     |             1236 |
// | od-ea0ed19af82622a997edf6c2aab262bc |               28 |
// | od-9011520298e3aca4f245e075dd873d02 |               10 |
// | od-3a87018f46f9d37fa811503745fc0b05 |                5 |
// | od-60e10a8929373b1ac0aff828dd5cacf8 |               30 |
// | od-198eb3d20e4783518acee52b1bc48356 |               20 |
// | od-ed452e84ca58c26719ea0ca8b8acecdd |                4 |
// | od-1d7f4ac953b109f2a7e2a2366f5f315e |               72 |
// | od-c6bbbc7f089b356cd45396e3443d1558 |                2 |
// | od-39c14e77f3504a8ca05f3681e9d0470b |                3 |
// | od-095e7e716c0a8262b3dad7888eb4776b |               42 |
// | od-7e8d4fb875bed78400bc5bbca88eed0c |                1 |
// +-------------------------------------+------------------+
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
