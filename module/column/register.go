package column

import (
	"maps"

	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

var tableColumns = make(map[string][]string)

// Register registers column module.
//
// m key is the table name, value is the table's columns name.
// for example: Register(map[string][]string{"user": {"name", "email"}})
//
// Models: no
//
// Routes:
//   - GET /api/column/:id
func Register(m map[string][]string) {
	maps.Copy(tableColumns, m)

	module.Use[*empty, *empty, rsp](&mod{}, consts.PHASE_GET)
}
