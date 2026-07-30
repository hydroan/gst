// Package traceprobe traces context propagation through the service, database,
// GORM, and model hooks of every standard CRUD phase. Each phase has its own
// service struct in its own file; this file holds the tracing helpers they
// share.
//
//	curl -s -i -c ./cookies.txt \
//	  -X POST http://localhost:8090/api/login \
//	  -H 'Content-Type: application/json' \
//	  -d '{"username":"root","password":"toor"}'
//
//	curl -s -i -b ./cookies.txt \
//	  -X POST http://localhost:8090/api/trace-probes \
//	  -H 'Content-Type: application/json' \
//	  -d '{"name":"trace-probe-codex","note":"standard-crud-context"}'
//
//	curl -s -i -b ./demo-cookies.txt \
//	  'http://localhost:8090/api/trace-probes?name=trace-probe-codex'
//
//	curl -s -i -b ./cookies.txt \
//	  http://localhost:8090/api/trace-probes/019efee7-76e5-7520-a405-9d4c7bead437
package traceprobe

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// traceServiceHook logs one service hook of the current phase together with the
// live row count, so a request can be followed across the service, database and
// model layers. log is the service logger, nil before logger injection runs.
func traceServiceHook(log types.Logger, ctx *types.ServiceContext, phase consts.Phase, probe *model.TraceProbe, itemCount int) error {
	var total int
	err := database.Database[*model.TraceProbe](ctx).Count(&total)

	fields := traceProbeServiceFields(probe, phase, total, itemCount)
	if log != nil {
		entry := log.WithContext(ctx, phase)
		if err != nil {
			entry.Errorz("trace probe service hook", append(fields, zap.Error(err))...)
		} else {
			entry.Infoz("trace probe service hook", fields...)
		}
	}
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to count trace probes", err)
	}
	return nil
}

func traceProbeServiceFields(probe *model.TraceProbe, phase consts.Phase, total int, itemCount int) []zap.Field {
	fields := []zap.Field{
		zap.String("component", "service_hook"),
		zap.String("hook", phase.MethodName()),
		zap.Int("total", total),
		zap.Int("item_count", itemCount),
	}
	if probe != nil {
		fields = append(
			fields,
			zap.String("probe_id", probe.GetID()),
			zap.String("probe_name", probe.Name),
		)
	}
	return fields
}

func traceProbeListLen(probes *[]*model.TraceProbe) int {
	if probes == nil {
		return 0
	}
	return len(*probes)
}
