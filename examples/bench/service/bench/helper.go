package bench

import (
	"strconv"

	"github.com/hydroan/gst/types"
)

func isDryRun(ctx *types.ServiceContext) bool {
	isDryRunStr := ctx.Query().Get("dry_run")
	isDryRun, _ := strconv.ParseBool(isDryRunStr)

	return isDryRun
}
