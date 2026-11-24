package column

import (
	"strings"

	"github.com/hydroan/gst/internal/gst"
)

func (s *srv) Get(ctx *gst.ServiceContext, req *empty) (rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	table := strings.ReplaceAll(ctx.Param("id"), "-", "_")
	columns, ok := tableColumns[table]
	if !ok {
		log.Warnw("not register table", "table", table)
	}

	return new(column).QueryColumns(ctx.Query(), table, columns)
}
