package bench

import (
	"strconv"

	"github.com/hydroan/gst"
)

func isDryRun(ctx *gst.ServiceContext) bool {
	isDryRunStr := ctx.Query().Get("dry_run")
	isDryRun, _ := strconv.ParseBool(isDryRunStr)

	return isDryRun
}
