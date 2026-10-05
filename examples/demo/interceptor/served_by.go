package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// servedBy answers every call that reaches the project's interceptors, the
// health and reflection calls excepted, with the x-served-by header. An
// interceptor runs before the handler, may add to the context it returns,
// and refuses the call by returning an error, a gRPC status.
func servedBy(ctx context.Context) (context.Context, error) {
	if err := grpc.SetHeader(ctx, metadata.Pairs("x-served-by", "demo")); err != nil {
		return nil, err
	}
	return ctx, nil
}
