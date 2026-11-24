// Package column is the module that answers which distinct values the
// columns of a table hold, for the tables and columns a project registers,
// under the filters a request names among those columns. Register adds the
// module to a project.
package column

import (
	"maps"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/module"
)

var tableColumns = make(map[string][]string)

// Register registers column module.
//
// m key is the table name, value is the table's columns name.
// for example: Register(map[string][]string{"user": {"name", "email"}})
//
// The registered columns are also the filters the endpoint accepts: a query
// parameter naming none of them is refused, so a request never reaches the
// database with a column of its own choosing.
//
// Models: no
//
// Routes:
//   - GET /api/column/:id
func Register(m map[string][]string) {
	maps.Copy(tableColumns, m)

	module.Use[*empty, *empty, rsp](&mod{}, module.CRUD(apiaction.Get))
}
