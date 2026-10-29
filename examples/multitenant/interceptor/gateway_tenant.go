package interceptor

import (
	"context"
	"strings"

	"multitenant/configx"

	"github.com/hydroan/gst/config"
	gstgrpc "github.com/hydroan/gst/grpc"
	"google.golang.org/grpc/metadata"
)

// gatewayTenant takes the tenant from the metadata a trusted gateway names
// it in, under the configured header's name, and establishes the caller
// again with it: the authorization behind it reads the caller's tenant.
// With no header configured it changes nothing.
func gatewayTenant(ctx context.Context) (context.Context, error) {
	header := config.Get[configx.Gateway]().TenantHeader
	if header == "" {
		return ctx, nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(header)
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return ctx, nil
	}
	caller := gstgrpc.CallerOf(ctx)
	caller.TenantID = strings.TrimSpace(values[0])
	return gstgrpc.WithCaller(ctx, caller), nil
}
