package counterstep

import (
	"net/http"
	"time"

	"cluster/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/service"
)

// watchInterval is how often a watch looks for new numbers; the leader
// writes one a second.
const watchInterval = 200 * time.Millisecond

// Watch serves the WatchCounterStep rpc, a server stream: the numbers after
// the one asked for, those already written first and then each new one as
// the leader writes it, until the client hangs up. It reads the table, the
// counter being in the database and the leader possibly another replica, so
// any replica serves it, and a client cut off by a rolling update resumes on
// another replica by asking for the numbers after the last one it saw.
type Watch struct {
	service.Base[*model.CounterStep, *model.CounterStepWatchReq, *model.CounterStep]
}

// Stream sends the numbers after req.After as they appear and returns once
// ctx is done, which is when the client has gone or the process is stopping.
func (w *Watch) Stream(ctx *gst.ServiceContext, req *model.CounterStepWatchReq, stream *grpc.ServerStream[*model.CounterStep]) error {
	after := req.After
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		steps := make([]*model.CounterStep, 0)
		err := database.Database[*model.CounterStep](ctx).
			WithQuery(&model.CounterStep{}, gst.QueryOptions{Filters: []gst.Filter{model.CounterStepCols.Seq.Gt(after)}}).
			WithOrder(model.CounterStepCols.Seq.Asc()).
			List(&steps)
		if err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "the numbers were not read", err)
		}
		for _, step := range steps {
			if err := stream.Send(step); err != nil {
				return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the number", err)
			}
			after = step.Seq
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
