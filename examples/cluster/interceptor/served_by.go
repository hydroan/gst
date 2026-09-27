package interceptor

import (
	"context"

	"cluster/helper"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// servedBy answers every call with the x-served-by header naming the replica,
// the same name the rows and the HTTP answers carry, so a client sees which
// replica each connection landed on. An interceptor runs before the handler,
// may add to the context it returns, and refuses the call by returning an
// error, a gRPC status.
func servedBy(ctx context.Context) (context.Context, error) {
	if err := grpc.SetHeader(ctx, metadata.Pairs("x-served-by", helper.Replica())); err != nil {
		return nil, err
	}
	return ctx, nil
}
