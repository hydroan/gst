package interceptor

import (
	"context"

	gstgrpc "github.com/hydroan/gst/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// actor names the caller in the x-actor header. Registered with
// RegisterAuth, it runs behind the session check alone, where the caller is
// known; a public method never sees it.
func actor(ctx context.Context) (context.Context, error) {
	if username := gstgrpc.CallerOf(ctx).Username; username != "" {
		if err := grpc.SetHeader(ctx, metadata.Pairs("x-actor", username)); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}
