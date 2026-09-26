package controller_test

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// The tests of the call functions run them the way a project's generated
// handlers will: behind a real listener, through the server's chain and an
// auth interceptor naming the caller. The sample service stands for a
// generated service: every rpc takes a Struct holding what the request
// message of the action would carry, runs the call function of the action
// and answers with a Struct holding what the response message would.

// TestCreateCallCreatesTheRecordForTheCaller pins the create call: it
// creates the record the message carried, for the caller the auth
// interceptor named, and answers it as stored; what the hooks find on their
// service context is what an HTTP request's hooks find; a message failing
// the binding tags is refused the way a body is; a hook's refusal answers
// with the hook's status and creates nothing.
func TestCreateCallCreatesTheRecordForTheCaller(t *testing.T) {
	conn := sampleServer(t)
	name := uniqueName("call-create")

	created, err := invoke(t, conn, "Create", map[string]any{"record": map[string]any{"name": name}})
	require.NoError(t, err)
	require.Equal(t, name, created["name"])
	stored := new(sampleRecord)
	require.NoError(t, database.Database[*sampleRecord](context.Background()).Get(stored, stringOf(created["id"])))
	require.Equal(t, name, stored.Name)
	require.Equal(t, "alice", stored.GetCreatedBy())
	require.Equal(t, "alice", stored.GetUpdatedBy())

	t.Run("the hooks find the call on their service context", func(t *testing.T) {
		_, err := invoke(t, conn, "ObservedCreate", map[string]any{
			"params": map[string]any{"box": "b-1"},
			"record": map[string]any{"name": uniqueName("call-observed")},
		})
		require.NoError(t, err)

		observedMu.Lock()
		defer observedMu.Unlock()
		require.Equal(t, "b-1", lastObserved.Box)
		require.Equal(t, "alice", lastObserved.Username)
		require.Equal(t, "POST", lastObserved.Method)
		require.Equal(t, "/gst.test.Samples/ObservedCreate", lastObserved.Route)
		require.True(t, lastObserved.RequiresAuth)
	})

	t.Run("a message failing validation is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "ValidatedCreate", map[string]any{"record": map[string]any{}})

		requireStatus(t, err, codes.InvalidArgument, "invalid request body")
	})

	t.Run("a before hook refusal creates nothing", func(t *testing.T) {
		name := uniqueName("call-create-refused")

		_, err := invoke(t, conn, "RefusedCreate", map[string]any{"record": map[string]any{"name": name}})

		requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})
}

// TestGetCallAnswersTheRecordOrNotFound pins the get call: the record the id
// names, NotFound for an id no record carries or the integer key cannot
// hold, and a refusal for a message naming no id at all.
func TestGetCallAnswersTheRecordOrNotFound(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-get")

	got, err := invoke(t, conn, "Get", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	require.Equal(t, record.GetID(), got["id"])
	require.Equal(t, "call-get", got["name"])

	t.Run("an id no record carries", func(t *testing.T) {
		_, err := invoke(t, conn, "Get", map[string]any{"id": "missing"})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		_, err := invoke(t, conn, "CounterGet", map[string]any{"id": "first"})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("no id at all", func(t *testing.T) {
		_, err := invoke(t, conn, "Get", map[string]any{})
		requireStatus(t, err, codes.InvalidArgument, "not found router param")
	})
}

// TestListCallListsLikeTheHTTPQuery pins the list call: the query of the
// message is read the way the HTTP query is, a filter with an operator as
// field[op]=value, one without as the bare key, the orderings and the
// paging under their parameters; what the HTTP listener refuses, a filter on
// a field the model lacks, a page on a model that does not page, a filter
// given twice, is refused the same; and a hook's refusal answers with the
// hook's status.
func TestListCallListsLikeTheHTTPQuery(t *testing.T) {
	conn := sampleServer(t)
	prefix := uniqueName("call-list")
	first := createSample(t, prefix+"-a")
	second := createSample(t, prefix+"-b")

	listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
		"Filters": []map[string]any{{"Field": "name", "Op": "like", "Values": []string{prefix}}},
		"SortBy":  []string{"name desc"},
	}})
	require.NoError(t, err)
	require.EqualValues(t, 2, listed["total"])
	require.Equal(t, []string{second.GetID(), first.GetID()}, ids(listed["items"]))

	t.Run("a filter without an operator is the bare key", func(t *testing.T) {
		listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Values": []string{prefix + "-a"}}},
		}})
		require.NoError(t, err)
		require.Equal(t, []string{first.GetID()}, ids(listed["items"]))
	})

	t.Run("a page of a model that pages", func(t *testing.T) {
		listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "startswith", "Values": []string{prefix}}},
			"SortBy":  []string{"name"},
			"Page":    2,
			"Size":    1,
		}})
		require.NoError(t, err)
		require.EqualValues(t, 2, listed["total"])
		require.Equal(t, []string{second.GetID()}, ids(listed["items"]))
	})

	for _, tt := range []struct {
		name  string
		rpc   string
		query map[string]any
	}{
		{name: "a filter on a field the model lacks", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "missing", "Op": "eq", "Values": []string{"value"}}},
		}},
		{name: "a page on a model that does not page", rpc: "CounterList", query: map[string]any{"Page": 2}},
		{name: "a filter given twice", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "eq", "Values": []string{"a"}}, {"Field": "name", "Op": "eq", "Values": []string{"b"}}},
		}},
		{name: "several values under an operator taking one", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "eq", "Values": []string{"a", "b"}}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := invoke(t, conn, tt.rpc, map[string]any{"query": tt.query})
			requireStatus(t, err, codes.InvalidArgument, "")
		})
	}

	t.Run("a hook refusal answers with the hook's status", func(t *testing.T) {
		for _, rpc := range []string{"RefusedList", "FilterRefusedList"} {
			_, err := invoke(t, conn, rpc, map[string]any{})
			requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		}
	})
}

// TestUpdateCallReplacesTheRecord pins the update call: the record the id
// names is replaced by the message's, its creation audit kept, and an id no
// record carries answers NotFound.
func TestUpdateCallReplacesTheRecord(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-update")

	updated, err := invoke(t, conn, "Update", map[string]any{"id": record.GetID(), "record": map[string]any{"name": "call-updated"}})
	require.NoError(t, err)
	require.Equal(t, "call-updated", updated["name"])
	require.Equal(t, "alice", updated["updated_by"])
	requireSampleName(t, record.GetID(), "call-updated")

	_, err = invoke(t, conn, "Update", map[string]any{"id": "missing", "record": map[string]any{"name": "call-updated"}})
	requireStatus(t, err, codes.NotFound, "")
}

// TestPatchCallAppliesTheMaskedFields pins the patch call: only the fields
// the mask names are copied onto the stored record, an empty mask changes
// nothing, a versioned record patched without its version is refused, and
// an id no record carries answers NotFound.
func TestPatchCallAppliesTheMaskedFields(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-patch")

	patched, err := invoke(t, conn, "Patch", map[string]any{
		"id": record.GetID(), "record": map[string]any{"name": "call-patched"}, "mask": []string{"name"},
	})
	require.NoError(t, err)
	require.Equal(t, "call-patched", patched["name"])
	requireSampleName(t, record.GetID(), "call-patched")

	t.Run("a field the mask does not name stays", func(t *testing.T) {
		_, err := invoke(t, conn, "Patch", map[string]any{
			"id": record.GetID(), "record": map[string]any{"name": "call-unmasked"}, "mask": []string{},
		})
		require.NoError(t, err)
		requireSampleName(t, record.GetID(), "call-patched")
	})

	t.Run("a versioned record without its version", func(t *testing.T) {
		_, err := invoke(t, conn, "VersionedPatch", map[string]any{
			"id": "missing", "record": map[string]any{"name": "renamed"}, "mask": []string{"name"},
		})
		requireStatus(t, err, codes.InvalidArgument, "")
	})

	t.Run("an id no record carries", func(t *testing.T) {
		_, err := invoke(t, conn, "Patch", map[string]any{
			"id": "missing", "record": map[string]any{"name": "renamed"}, "mask": []string{"name"},
		})
		requireStatus(t, err, codes.NotFound, "")
	})
}

// TestDeleteCallDeletesTheRecord pins the delete call: the record the id
// names is gone afterwards, and a hook's refusal keeps it.
func TestDeleteCallDeletesTheRecord(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-delete")

	_, err := invoke(t, conn, "Delete", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	require.Zero(t, countSamplesNamed(t, "call-delete"))

	kept := createSample(t, "call-delete-refused")
	_, err = invoke(t, conn, "RefusedDelete", map[string]any{"id": kept.GetID()})
	requireStatus(t, err, codes.AlreadyExists, refusedMsg)
	requireSampleName(t, kept.GetID(), "call-delete-refused")
}

// TestBatchCallsWriteAllOrNothing pins the four batch calls: the items are
// created, replaced, patched under their masks and deleted as one batch, a
// hook's refusal writes nothing, and a delete naming an empty id is refused
// before anything is deleted.
func TestBatchCallsWriteAllOrNothing(t *testing.T) {
	conn := sampleServer(t)
	prefix := uniqueName("call-batch")

	created, err := invoke(t, conn, "CreateMany", map[string]any{"items": []map[string]any{{"name": prefix + "-a"}, {"name": prefix + "-b"}}})
	require.NoError(t, err)
	createdIDs := ids(created["items"])
	require.Len(t, createdIDs, 2)
	require.Equal(t, 1, countSamplesNamed(t, prefix+"-a"))

	updated, err := invoke(t, conn, "UpdateMany", map[string]any{"items": []map[string]any{
		{"id": createdIDs[0], "name": prefix + "-a2"}, {"id": createdIDs[1], "name": prefix + "-b2"},
	}})
	require.NoError(t, err)
	require.Equal(t, createdIDs, ids(updated["items"]))
	requireSampleName(t, createdIDs[0], prefix+"-a2")

	patched, err := invoke(t, conn, "PatchMany", map[string]any{
		"items": []map[string]any{{"id": createdIDs[0], "name": prefix + "-a3"}, {"id": createdIDs[1], "name": prefix + "-b3"}},
		"masks": [][]string{{"name"}, {}},
	})
	require.NoError(t, err)
	require.Equal(t, createdIDs, ids(patched["items"]))
	requireSampleName(t, createdIDs[0], prefix+"-a3")
	requireSampleName(t, createdIDs[1], prefix+"-b2")

	_, err = invoke(t, conn, "DeleteMany", map[string]any{"ids": createdIDs})
	require.NoError(t, err)
	require.Zero(t, countSamplesNamed(t, prefix+"-a3"))
	require.Zero(t, countSamplesNamed(t, prefix+"-b2"))

	t.Run("a hook refusal writes nothing", func(t *testing.T) {
		name := uniqueName("call-batch-refused")
		_, err := invoke(t, conn, "RefusedCreateMany", map[string]any{"items": []map[string]any{{"name": name}}})
		requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a delete naming an empty id", func(t *testing.T) {
		kept := createSample(t, "call-batch-kept")
		_, err := invoke(t, conn, "DeleteMany", map[string]any{"ids": []string{kept.GetID(), ""}})
		requireStatus(t, err, codes.InvalidArgument, "")
		requireSampleName(t, kept.GetID(), "call-batch-kept")
	})
}

// TestServiceCallDelegatesToThePhaseService pins the call of an action with
// a payload and result of its own: the phase service's method runs with the
// payload, on a service context answering the parameters, the query, the
// caller and the method the call carries; its service error answers with
// its status, any other error answers Internal, and so does a response the
// service tried to write, which no call can carry; a payload failing the
// binding tags is refused; and an action described public runs for a call
// naming no caller.
func TestServiceCallDelegatesToThePhaseService(t *testing.T) {
	conn := sampleServer(t)

	result, err := invoke(t, conn, "Action", map[string]any{
		"params": map[string]any{"box": "b-2"}, "payload": map[string]any{"note": "hello"},
	})
	require.NoError(t, err)
	require.Equal(t, "hello", result["Note"])
	require.Equal(t, "b-2", result["Box"])
	require.Equal(t, "alice", result["Username"])
	require.Equal(t, "POST", result["Method"])
	require.Equal(t, "/gst.test.Samples/Action", result["Route"])
	require.Equal(t, true, result["RequiresAuth"])

	t.Run("a List reads the query", func(t *testing.T) {
		result, err := invoke(t, conn, "ActionList", map[string]any{"query": map[string]any{"Page": 2, "Expand": []string{"children"}}})
		require.NoError(t, err)
		require.Equal(t, map[string]any{"_page": []any{"2"}, "_expand": []any{"children"}}, result["Query"])
	})

	t.Run("a public action names no caller", func(t *testing.T) {
		result, err := invokeAs(t, conn, "OpenAction", map[string]any{"payload": map[string]any{"note": "hello"}}, "")
		require.NoError(t, err)
		require.Equal(t, false, result["RequiresAuth"])
		require.Empty(t, result["Username"])
	})

	t.Run("a payload failing validation is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{}})
		requireStatus(t, err, codes.InvalidArgument, "invalid request body")
	})

	t.Run("the service's error answers with its status", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionRefuse}})
		requireStatus(t, err, codes.PermissionDenied, "not yours")
	})

	t.Run("any other error answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionBreak}})
		requireStatus(t, err, codes.Internal, "internal server error")
	})

	t.Run("a response the service tried to write answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionWrite}})
		requireStatus(t, err, codes.Internal, "internal server error")
	})
}

// TestCallsRunInTheControllerSpan pins the spans a call runs in: the
// controller span of the action, named and attributed the way a request's
// is, POST and the full method standing for the method and path, and the
// service span of a delegated action inside it.
func TestCallsRunInTheControllerSpan(t *testing.T) {
	conn := sampleServer(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	record := createSample(t, "call-span")

	_, err := invoke(t, conn, "Get", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	_, err = invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": "traced"}})
	require.NoError(t, err)

	got := oteltest.EndedNamed(t, recorder, gstotel.FrameworkSpanName("controller", "sampleRecord", "Get"))
	require.ElementsMatch(t, []attribute.KeyValue{
		attribute.String("component", "controller"),
		attribute.String("controller.operation", "Get"),
		attribute.String("controller.model", "sampleRecord"),
		attribute.String("controller.method", "POST"),
		attribute.String("controller.path", "/gst.test.Samples/Get"),
	}, got.Attributes())
	service := oteltest.EndedNamed(t, recorder, gstotel.FrameworkSpanName("service", "sampleRecord", "Create"))
	controllerSpan := oteltest.EndedNamed(t, recorder, gstotel.FrameworkSpanName("controller", "sampleRecord", "Create"))
	require.Equal(t, controllerSpan.SpanContext().SpanID(), service.Parent().SpanID(), "the service span runs inside the controller span")
}

// The sample service stands for the gRPC service the generator writes for a
// model: gst.test.Samples, with an rpc per call function under test, each
// taking a Struct holding what the request message of the action would
// carry and answering a Struct holding what the response message would. One
// listener serves the whole test binary, behind an auth interceptor naming
// alice as the caller of every call presenting sampleCredential.
var (
	sampleServerOnce sync.Once
	sampleServerAddr string
	errSampleServer  error
)

// sampleCredential is the authorization a call presents to be named alice.
const sampleCredential = "Bearer alice"

// sampleServer returns a connection to the sample service, starting its
// listener on first use.
func sampleServer(t *testing.T) *grpc.ClientConn {
	t.Helper()
	sampleServerOnce.Do(func() {
		grpcserver.UseAuth(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			if values := md.Get("authorization"); len(values) == 1 && values[0] == sampleCredential {
				return handler(grpcserver.WithCaller(ctx, grpcserver.Caller{Username: "alice", UserID: "u-1"}), req)
			}
			return nil, status.Error(codes.Unauthenticated, "who are you")
		})
		handlers := sampleHandlers()
		descs := make([]grpc.MethodDesc, 0, len(handlers))
		methods := make([]grpcserver.Method, 0, len(handlers))
		for name, handle := range handlers {
			fullMethod := "/gst.test.Samples/" + name
			descs = append(descs, grpc.MethodDesc{
				MethodName: name,
				Handler: func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
					in := new(structpb.Struct)
					if err := dec(in); err != nil {
						return nil, err
					}
					handler := func(ctx context.Context, _ any) (any, error) {
						out, err := handle(ctx, in.AsMap())
						if err != nil {
							return nil, err
						}
						return encode(out), nil
					}
					return interceptor(ctx, in, &grpc.UnaryServerInfo{FullMethod: fullMethod}, handler)
				},
			})
			methods = append(methods, grpcserver.Method{Name: fullMethod, Public: name == "OpenAction"})
		}
		grpcserver.Register(func(r grpc.ServiceRegistrar) {
			r.RegisterService(&grpc.ServiceDesc{
				ServiceName: "gst.test.Samples",
				HandlerType: (*any)(nil),
				Methods:     descs,
				Metadata:    "gst/test/samples.proto",
			}, nil)
		}, methods...)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			errSampleServer = err
			return
		}
		addr, _ := listener.Addr().(*net.TCPAddr)
		_ = listener.Close()
		config.App.GRPC = config.GRPC{Listen: "127.0.0.1", Port: addr.Port}
		sampleServerAddr = addr.String()
		go func() { _ = grpcserver.Run() }()
	})
	require.NoError(t, errSampleServer)
	conn, err := grpc.NewClient(sampleServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// sampleHandlers builds the rpcs of the sample service, each running the
// call function of its action the way a generated handler does, with what
// the request message carries: the route parameters under params, the id,
// the record or the items, the update mask or masks, the query and the
// payload.
func sampleHandlers() map[string]func(ctx context.Context, in map[string]any) (any, error) {
	create := controller.CreateCall[*sampleRecord](sampleRoute)
	get := controller.GetCall[*sampleRecord](sampleRoute)
	list := controller.ListCall[*sampleRecord](sampleRoute)
	update := controller.UpdateCall[*sampleRecord](sampleRoute)
	patch := controller.PatchCall[*sampleRecord](sampleRoute)
	del := controller.DeleteCall[*sampleRecord](sampleRoute)
	createMany := controller.CreateManyCall[*sampleRecord](sampleRoute)
	updateMany := controller.UpdateManyCall[*sampleRecord](sampleRoute)
	patchMany := controller.PatchManyCall[*sampleRecord](sampleRoute)
	deleteMany := controller.DeleteManyCall[*sampleRecord](sampleRoute)
	refusedCreate := controller.CreateCall[*sampleRecord](refusalRoute)
	refusedList := controller.ListCall[*sampleRecord](refusalRoute)
	refusedDelete := controller.DeleteCall[*sampleRecord](refusalRoute)
	refusedCreateMany := controller.CreateManyCall[*sampleRecord](refusalRoute)
	filterRefusedList := controller.ListCall[*sampleRecord](filterRefusalRoute)
	observedCreate := controller.CreateCall[*sampleRecord](observedRoute)
	counterGet := controller.GetCall[*sampleCounter](counterRoute)
	counterList := controller.ListCall[*sampleCounter](counterRoute)
	versionedPatch := controller.PatchCall[*versionedSample](versionedRoute)
	validatedCreate := controller.CreateCall[*validatedSample](validatedRoute)
	action := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.PHASE_CREATE, actionRoute)
	actionList := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.PHASE_LIST, actionRoute)

	return map[string]func(ctx context.Context, in map[string]any) (any, error){
		"Create": func(ctx context.Context, in map[string]any) (any, error) {
			return create(ctx, params(in), field[*sampleRecord](in, "record"))
		},
		"Get": func(ctx context.Context, in map[string]any) (any, error) {
			return get(ctx, params(in), field[string](in, "id"), field[controller.Query](in, "query"))
		},
		"List": func(ctx context.Context, in map[string]any) (any, error) {
			return listing(list(ctx, params(in), field[controller.Query](in, "query")))
		},
		"Update": func(ctx context.Context, in map[string]any) (any, error) {
			return update(ctx, params(in), field[string](in, "id"), field[*sampleRecord](in, "record"))
		},
		"Patch": func(ctx context.Context, in map[string]any) (any, error) {
			return patch(ctx, params(in), field[string](in, "id"), field[*sampleRecord](in, "record"), field[[]string](in, "mask"))
		},
		"Delete": func(ctx context.Context, in map[string]any) (any, error) {
			return done(del(ctx, params(in), field[string](in, "id")))
		},
		"CreateMany": func(ctx context.Context, in map[string]any) (any, error) {
			return batch(createMany(ctx, params(in), field[[]*sampleRecord](in, "items")))
		},
		"UpdateMany": func(ctx context.Context, in map[string]any) (any, error) {
			return batch(updateMany(ctx, params(in), field[[]*sampleRecord](in, "items")))
		},
		"PatchMany": func(ctx context.Context, in map[string]any) (any, error) {
			return batch(patchMany(ctx, params(in), field[[]*sampleRecord](in, "items"), field[[][]string](in, "masks")))
		},
		"DeleteMany": func(ctx context.Context, in map[string]any) (any, error) {
			return done(deleteMany(ctx, params(in), field[[]string](in, "ids")))
		},
		"RefusedCreate": func(ctx context.Context, in map[string]any) (any, error) {
			return refusedCreate(ctx, params(in), field[*sampleRecord](in, "record"))
		},
		"RefusedList": func(ctx context.Context, in map[string]any) (any, error) {
			return listing(refusedList(ctx, params(in), field[controller.Query](in, "query")))
		},
		"RefusedDelete": func(ctx context.Context, in map[string]any) (any, error) {
			return done(refusedDelete(ctx, params(in), field[string](in, "id")))
		},
		"RefusedCreateMany": func(ctx context.Context, in map[string]any) (any, error) {
			return batch(refusedCreateMany(ctx, params(in), field[[]*sampleRecord](in, "items")))
		},
		"FilterRefusedList": func(ctx context.Context, in map[string]any) (any, error) {
			return listing(filterRefusedList(ctx, params(in), field[controller.Query](in, "query")))
		},
		"ObservedCreate": func(ctx context.Context, in map[string]any) (any, error) {
			return observedCreate(ctx, params(in), field[*sampleRecord](in, "record"))
		},
		"CounterGet": func(ctx context.Context, in map[string]any) (any, error) {
			return counterGet(ctx, params(in), field[string](in, "id"), field[controller.Query](in, "query"))
		},
		"CounterList": func(ctx context.Context, in map[string]any) (any, error) {
			return listing(counterList(ctx, params(in), field[controller.Query](in, "query")))
		},
		"VersionedPatch": func(ctx context.Context, in map[string]any) (any, error) {
			return versionedPatch(ctx, params(in), field[string](in, "id"), field[*versionedSample](in, "record"), field[[]string](in, "mask"))
		},
		"ValidatedCreate": func(ctx context.Context, in map[string]any) (any, error) {
			return validatedCreate(ctx, params(in), field[*validatedSample](in, "record"))
		},
		"Action": func(ctx context.Context, in map[string]any) (any, error) {
			return action(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
		},
		"OpenAction": func(ctx context.Context, in map[string]any) (any, error) {
			return action(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
		},
		"ActionList": func(ctx context.Context, in map[string]any) (any, error) {
			return actionList(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
		},
	}
}

// params returns the route parameters the message carries under params.
func params(in map[string]any) map[string]string {
	return field[map[string]string](in, "params")
}

// field converts in[key] into T through its JSON shape, the way a generated
// handler converts a field of the request message into the Go value the
// call takes; a key the message lacks is the zero value.
func field[T any](in map[string]any, key string) T {
	var value T
	raw, ok := in[key]
	if !ok {
		return value
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		panic(err)
	}
	return value
}

// listing, batch and done shape what a list, a batch and a call answering
// nothing return into what their response messages would carry.
func listing[M any](items []M, total int, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total}, nil
}

func batch[M any](items []M, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items}, nil
}

func done(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

// encode renders out, a record or a map, as the Struct a response message
// would carry, through its JSON shape.
func encode(out any) *structpb.Struct {
	encoded, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(encoded, &fields); err != nil {
		panic(err)
	}
	message, err := structpb.NewStruct(fields)
	if err != nil {
		panic(err)
	}
	return message
}

// invoke calls the rpc name of the sample service as alice with in as the
// request message and returns the response message as a map.
func invoke(t *testing.T, conn *grpc.ClientConn, name string, in map[string]any) (map[string]any, error) {
	t.Helper()
	return invokeAs(t, conn, name, in, sampleCredential)
}

// invokeAs calls the rpc name presenting authorization, none when empty.
func invokeAs(t *testing.T, conn *grpc.ClientConn, name string, in map[string]any, authorization string) (map[string]any, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if authorization != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authorization)
	}
	out := new(structpb.Struct)
	if err := conn.Invoke(ctx, "/gst.test.Samples/"+name, encode(in), out, grpc.WaitForReady(true)); err != nil {
		return nil, err
	}
	return out.AsMap(), nil
}

// ids returns the ids of the records items holds, in order.
func ids(items any) []string {
	var got []string
	list, _ := items.([]any)
	for _, item := range list {
		record, _ := item.(map[string]any)
		got = append(got, stringOf(record["id"]))
	}
	return got
}

// stringOf returns the string value holds, "" for any other value.
func stringOf(value any) string {
	s, _ := value.(string)
	return s
}

// requireStatus requires err to be a status of code, carrying message when
// one is given, and, for a code the framework maps a failure to, the
// ErrorInfo detail the envelope's code and status travel in.
func requireStatus(t *testing.T, err error, code codes.Code, message string) {
	t.Helper()
	require.Error(t, err)
	st := status.Convert(err)
	require.Equal(t, code, st.Code(), st.Message())
	if message != "" {
		require.Equal(t, message, st.Message())
	}
	if code == codes.Internal {
		return
	}
	var info *errdetails.ErrorInfo
	for _, detail := range st.Details() {
		if d, ok := detail.(*errdetails.ErrorInfo); ok {
			info = d
		}
	}
	require.NotNil(t, info, "a mapped failure carries the envelope's code and status")
}
