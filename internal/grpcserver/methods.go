package grpcserver

import (
	"slices"

	"github.com/hydroan/gst/internal/apiaction"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// Method describes one rpc of a registered service the way the generated
// registration file declares it: its full name, whether its action declares
// Public(), and the HTTP method and route the same action is served at over
// HTTP, which a call's request metadata carries and Route answers.
type Method struct {
	// Name is the full method name, "/app.RecordService/ListRecord".
	Name string
	// HTTPMethod and Route are the HTTP method and the route pattern of the
	// same action, "GET" and "/api/records/:id"; for the rpc of a Stream
	// action HTTPMethod is MethodStream and Route the path its connection
	// is opened at over HTTP, "/api/feeds/watch".
	HTTPMethod string
	Route      string
	// Public marks the action as one declaring Public(): the interceptors
	// UseAuth queued leave the method alone.
	Public bool
}

// MethodStream is what the registration describes the rpc of a Stream
// action with in place of an HTTP method, apiaction.MethodStream: the word
// a stream is listed and authorized by on both listeners, so a policy
// written for GET or POST lets no stream through. The public
// grpc.MethodStream forwards to it.
const MethodStream = apiaction.MethodStream

// methods are the rpcs Register described, keyed by their full name.
var methods map[string]Method

// nonPublicMethods lists the registered methods not declared public, in
// order of their names: the ones the auth interceptors guard, or would.
func nonPublicMethods() []string {
	names := make([]string, 0, len(methods))
	for name, m := range methods {
		if !m.Public {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// ownServices are the services the server registers for itself (see Run),
// the health service and the reflection service in its two versions, which
// the project's interceptors leave alone, the common and the auth ones
// alike: they are the framework's, not the project's actions, and the
// callers of either present no credentials — a Kubernetes gRPC probe or a
// balancer checking the health service, grpcurl listing the services
// through reflection — the way the HTTP listener's probes run outside the
// middleware a project registers and take no authentication. Reflection
// exposes the schema alone, what the committed .proto files carry; [grpc]
// reflection turns it off where that is too much.
var ownServices = map[string]bool{
	grpc_health_v1.Health_ServiceDesc.ServiceName:                    true,
	grpc_reflection_v1.ServerReflection_ServiceDesc.ServiceName:      true,
	grpc_reflection_v1alpha.ServerReflection_ServiceDesc.ServiceName: true,
}

// requiresAuth reports whether the call of fullMethod, an rpc of service,
// is one the auth interceptors run on, which is what the RequiresAuth of
// its request metadata says (see enterCall): a call of a method not
// declared public, the server's own services aside.
func requiresAuth(service, fullMethod string) bool {
	return !ownServices[service] && !methods[fullMethod].Public
}
