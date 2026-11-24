package urlquery

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/modelschema"
)

// Orders returns the ORDER BY terms of the request, ready to be passed to
// Database.WithOrder.
//
// A model opts in to sorting by embedding model.Query; any other model yields
// no order at all. Column names are validated against the model's filterable
// columns, so an unknown column fails here with an error the caller turns into
// a client error, instead of reaching the database and failing the whole page
// with a SQL error. The value carried in each Order is the database column
// name, while the URL names the column by its query name.
func Orders(q url.Values, m gst.Model) ([]gst.Order, error) {
	if !model.IsQueryable(m) {
		return nil, nil
	}
	expression := strings.TrimSpace(q.Get(gst.QuerySortBy))
	if len(expression) == 0 {
		return nil, nil
	}
	columns, err := modelschema.FilterableIndex(reflect.TypeOf(m))
	if err != nil {
		return nil, err
	}

	orders := make([]gst.Order, 0)
	for term := range strings.SplitSeq(expression, ",") {
		fields := strings.Fields(term)
		if len(fields) == 0 || len(fields) > 2 {
			return nil, errors.Newf("invalid sort term %q: expected \"column\" or \"column asc|desc\"", strings.TrimSpace(term))
		}
		column, ok := columns[fields[0]]
		if !ok {
			return nil, errors.Newf("unknown sort column %q", fields[0])
		}
		direction := gst.OrderAsc
		if len(fields) == 2 {
			switch {
			case strings.EqualFold(fields[1], string(gst.OrderAsc)):
				direction = gst.OrderAsc
			case strings.EqualFold(fields[1], string(gst.OrderDesc)):
				direction = gst.OrderDesc
			default:
				return nil, errors.Newf("invalid sort direction %q for column %q: expected asc or desc", fields[1], fields[0])
			}
		}
		orders = append(orders, gst.NewOrder("", column.DBName, direction))
	}
	return orders, nil
}
