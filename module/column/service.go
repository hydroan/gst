package column

import (
	"strings"

	"github.com/hydroan/gst/types"
)

func (s *srv) Get(ctx *types.ServiceContext, req *empty) (rsp, error) {
	log := s.WithContext(ctx, ctx.GetPhase())

	table := strings.ReplaceAll(ctx.Param("id"), "-", "_")
	columns, ok := tableColumns[table]
	if !ok {
		log.Warnw("not register table", "table", table)
	}

	return new(column).QueryColumns(ctx.Query(), table, columns)
}
