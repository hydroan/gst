package controller_test

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"maps"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/datatypes"
)

// The tests of the call functions run them the way a project's generated
// handlers will: behind a real listener, through the server's chain and an
// auth interceptor naming the caller. The sample service stands for a
// generated service: every rpc takes a Struct holding what the request
// message of the action would carry, runs the call function of the action
// and answers with a Struct holding what the response message would.

// TestCallsRefuseAnEmptyParentParameterWhateverItsName pins that only the
// parameter carrying the id of a standard item action, the last one of its
// route, is left to the call's own refusal: a parent parameter named id, the
// default name of a model declaring no Param, is refused when empty on a
// collection action and on an item action alike, and the item action refuses
// an empty id by the field's name. A custom action on an item route has no id
// field to refuse an empty one by, so it refuses that parameter like any
// other.
func TestCallsRefuseAnEmptyParentParameterWhateverItsName(t *testing.T) {
	conn := sampleServer(t)

	_, err := invoke(t, conn, "ParentList", map[string]any{"params": map[string]string{"id": ""}})
	requireStatus(t, err, codes.InvalidArgument, `route parameter "id" is required`)

	_, err = invoke(t, conn, "ParentGet", map[string]any{"params": map[string]string{"id": ""}, "id": "s-1"})
	requireStatus(t, err, codes.InvalidArgument, `route parameter "id" is required`)

	_, err = invoke(t, conn, "ParentGet", map[string]any{"params": map[string]string{"id": "p-1"}})
	requireStatus(t, err, codes.InvalidArgument, "id is required")

	_, err = invoke(t, conn, "ActionGet", map[string]any{"params": map[string]string{"id": ""}})
	requireStatus(t, err, codes.InvalidArgument, `route parameter "id" is required`)
}

// TestBatchCallsWriteAllOrNothing pins the four batch calls: the items are
// created, replaced, patched under their masks and deleted as one batch; a
// hook's refusal writes nothing; an item failing its binding tags refuses
// the batch; a batch patch carries a record in every item, and the call it
// runs one mask per item; a batch update or patch names each record once;
// and a delete naming an empty id is refused before anything is deleted.
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

	patched, err := invoke(t, conn, "PatchMany", map[string]any{"items": []map[string]any{
		{"id": createdIDs[0], "record": map[string]any{"name": prefix + "-a3"}, "mask": []string{"name"}},
		{"id": createdIDs[1], "record": map[string]any{"name": prefix + "-b3", "note": "masked"}, "mask": []string{"note"}},
	}})
	require.NoError(t, err)
	require.Equal(t, createdIDs, ids(patched["items"]))
	requireSampleName(t, createdIDs[0], prefix+"-a3")
	second := loadSample(t, createdIDs[1])
	require.Equal(t, prefix+"-b2", second.Name, "the second mask does not name the name")
	require.Equal(t, "masked", second.Note)

	t.Run("a batch patch call with fewer masks than items", func(t *testing.T) {
		// The generated handler reads one mask off every item; the call
		// refuses a caller of its own handing it fewer.
		first, second := &sampleRecord{Name: prefix + "-a4"}, &sampleRecord{Name: prefix + "-b4"}
		first.SetID(createdIDs[0])
		second.SetID(createdIDs[1])
		_, patchErr := controller.PatchManyCall[*sampleRecord](sampleRoute)(t.Context(), nil, []*sampleRecord{first, second}, [][]string{{"name"}})
		requireStatus(t, patchErr, codes.InvalidArgument, "2 items carry 1 update masks; each item names the fields to apply in a mask of its own")
		requireSampleName(t, createdIDs[0], prefix+"-a3")
	})

	t.Run("a batch patch with an item carrying no record", func(t *testing.T) {
		_, patchErr := invoke(t, conn, "PatchMany", map[string]any{"items": []map[string]any{
			{"id": createdIDs[0], "record": map[string]any{"name": prefix + "-a4"}, "mask": []string{"name"}},
			{"id": createdIDs[1], "mask": []string{"name"}},
		}})
		requireStatus(t, patchErr, codes.InvalidArgument, "items[1] carries no record")
		requireSampleName(t, createdIDs[0], prefix+"-a3")
	})

	t.Run("a batch update naming a record twice", func(t *testing.T) {
		_, updateErr := invoke(t, conn, "UpdateMany", map[string]any{"items": []map[string]any{
			{"id": createdIDs[0], "name": prefix + "-a5"}, {"id": createdIDs[0], "name": prefix + "-a6"},
		}})
		requireStatus(t, updateErr, codes.InvalidArgument, `items[1] names the record "`+createdIDs[0]+`", which items[0] already names`)
		requireSampleName(t, createdIDs[0], prefix+"-a3")
	})

	t.Run("a batch patch naming a record twice", func(t *testing.T) {
		_, patchErr := invoke(t, conn, "PatchMany", map[string]any{"items": []map[string]any{
			{"id": createdIDs[0], "record": map[string]any{"name": prefix + "-a5"}, "mask": []string{"name"}},
			{"id": createdIDs[0], "record": map[string]any{"note": "twice"}, "mask": []string{"note"}},
		}})
		requireStatus(t, patchErr, codes.InvalidArgument, `items[1] names the record "`+createdIDs[0]+`", which items[0] already names`)
		requireSampleName(t, createdIDs[0], prefix+"-a3")
		require.Empty(t, loadSample(t, createdIDs[0]).Note)
	})

	t.Run("an item failing validation refuses the batch", func(t *testing.T) {
		_, createErr := invoke(t, conn, "ValidatedCreateMany", map[string]any{"items": []map[string]any{{"name": "valid"}, {}}})
		requireStatus(t, createErr, codes.InvalidArgument, "items[1].name is a required field")
		var total int
		require.NoError(t, database.Database[*validatedSample](context.Background()).WithQuery(&validatedSample{Name: "valid"}).Count(&total))
		require.Zero(t, total)
	})

	_, err = invoke(t, conn, "DeleteMany", map[string]any{"ids": createdIDs})
	require.NoError(t, err)
	require.Zero(t, countSamplesNamed(t, prefix+"-a3"))
	require.Zero(t, countSamplesNamed(t, prefix+"-b2"))

	t.Run("a hook refusal writes nothing", func(t *testing.T) {
		name := uniqueName("call-batch-refused")
		_, err := invoke(t, conn, "RefusedCreateMany", map[string]any{"items": []map[string]any{{"name": name}}})
		requireStatus(t, err, codes.Aborted, refusedMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a delete naming an empty id", func(t *testing.T) {
		kept := createSample(t, "call-batch-kept")
		_, err := invoke(t, conn, "DeleteMany", map[string]any{"ids": []string{kept.GetID(), ""}})
		requireStatus(t, err, codes.InvalidArgument, "")
		requireSampleName(t, kept.GetID(), "call-batch-kept")
	})
}

// TestPatchItemReadiesTheRecordOfABatchItem pins PatchItem, what the
// generated handler of a PatchMany rpc reads each item through: the item
// names its record by id, written onto the record it carries whatever id
// the record names, the way a patch call writes the record its message
// names; an item naming no id and an item whose route parameter contradicts
// the request's are refused with InvalidArgument; a parameter the item
// leaves empty is the request's; and an item carrying no record is left to
// the call to refuse.
func TestPatchItemReadiesTheRecordOfABatchItem(t *testing.T) {
	readied, err := controller.PatchItem(0, nil, nil, "r1", &sampleRecord{Name: "named"})
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())
	require.Equal(t, "named", readied.Name)

	other := &sampleRecord{}
	other.SetID("r2")
	readied, err = controller.PatchItem(1, nil, nil, "r1", other)
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID(), "the id of the item names the record, whatever id the record carries")

	_, err = controller.PatchItem(2, nil, nil, "", &sampleRecord{})
	requireStatus(t, err, codes.InvalidArgument, "items[2] names no id")

	_, err = controller.PatchItem(3, map[string]string{"record": "a"}, map[string]string{"record": "b"}, "r1", &sampleRecord{})
	requireStatus(t, err, codes.InvalidArgument, `items[3] names the record parameter "b", the request names "a"`)

	readied, err = controller.PatchItem(4, map[string]string{"record": "a"}, map[string]string{"record": ""}, "r1", &sampleRecord{})
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())

	var absent *sampleRecord
	readied, err = controller.PatchItem(5, nil, nil, "r1", absent)
	require.NoError(t, err)
	require.Nil(t, readied)
}

// TestItemErrorNamesTheItemOfARefusedRecord pins ItemError, what the
// generated handler of a batch rpc answers the refusal of an item's record
// through: the field a FromProto refused is named as the item's,
// items[1].rank, in the message and in the BadRequest detail alike, the
// description kept; a status naming no field and an error that is no status
// are answered as they are.
func TestItemErrorNamesTheItemOfARefusedRecord(t *testing.T) {
	refused := grpcserver.StatusError(types.NewInvalidValue("rank", "300 is out of range", nil))

	err := controller.ItemError(1, refused)

	requireStatus(t, err, codes.InvalidArgument, "invalid value for field 'items[1].rank'")
	details := status.Convert(err).Details()
	require.Len(t, details, 1)
	bad, ok := details[0].(*errdetails.BadRequest)
	require.True(t, ok, "%T", details[0])
	require.Len(t, bad.GetFieldViolations(), 1)
	require.Equal(t, "items[1].rank", bad.GetFieldViolations()[0].GetField())
	require.Equal(t, "300 is out of range", bad.GetFieldViolations()[0].GetDescription())

	plain := status.Error(codes.InvalidArgument, "nothing named")
	require.Equal(t, plain, controller.ItemError(2, plain))
	other := errors.New("not a status")
	require.Equal(t, other, controller.ItemError(3, other))
}

// TestPathErrorNamesTheFieldBehindThePath pins PathError, Element and Entry,
// what the generated conversions name a refused value of a nested message,
// a container element or a map value through: the field a FromProto refused
// is named behind the path, meta.score, the path of an element carries its
// index, points[1], the path of a map value its key, costs.k1, a key of
// digits alone in brackets like an index, by_rank[7], and the prefixes
// compose from the inside out, items[1].meta.score.
func TestPathErrorNamesTheFieldBehindThePath(t *testing.T) {
	refused := grpcserver.StatusError(types.NewInvalidValue("score", "NaN is not finite", nil))

	requireStatus(t, controller.PathError("meta", refused), codes.InvalidArgument, "invalid value for field 'meta.score'")
	require.Equal(t, "points[1]", controller.Element("points", 1))
	require.Equal(t, "costs.k1", controller.Entry("costs", "k1"))
	require.Equal(t, "by_rank[7]", controller.Entry("by_rank", int8(7)))
	require.Equal(t, "by_rank.-1", controller.Entry("by_rank", int8(-1)))
	requireStatus(t, controller.ItemError(1, controller.PathError("meta", refused)), codes.InvalidArgument, "invalid value for field 'items[1].meta.score'")
	requireStatus(t, controller.PathError(controller.Element("points", 2), refused), codes.InvalidArgument, "invalid value for field 'points[2].score'")

	plain := status.Error(codes.InvalidArgument, "nothing named")
	require.Equal(t, plain, controller.PathError("meta", plain))
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
	require.Equal(t, http.MethodPost, result["Method"])
	require.Equal(t, "/api/controller-sample-action", result["Route"], "the route of the action, not the full method")
	require.Equal(t, true, result["RequiresAuth"])

	t.Run("a List reads the query", func(t *testing.T) {
		result, err := invoke(t, conn, "ActionList", map[string]any{"query": map[string]any{"Page": 2, "Expand": []string{"children"}}})
		require.NoError(t, err)
		require.Equal(t, http.MethodGet, result["Method"], "the method of the action, not the POST every call is on the wire")
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
		requireStatus(t, err, codes.InvalidArgument, "note is a required field")
	})

	t.Run("the service's error answers with its status", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionRefuse}})
		requireStatus(t, err, codes.PermissionDenied, "not yours")
	})

	t.Run("any other error answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionBreak}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
	})

	t.Run("a canceled call is answered as canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", sampleCredential)
		before := len(accessLog.FilterMessage("/gst.test.Samples/Action").All())
		done := make(chan error, 1)
		go func() {
			done <- conn.Invoke(ctx, "/gst.test.Samples/Action", encode(map[string]any{"payload": map[string]any{"note": actionHang}}), new(structpb.Struct), grpc.WaitForReady(true))
		}()
		<-actionEntered
		cancel()
		require.Equal(t, codes.Canceled, status.Code(<-done))
		require.Eventually(t, func() bool {
			entries := accessLog.FilterMessage("/gst.test.Samples/Action").All()
			return len(entries) > before && entries[len(entries)-1].ContextMap()["grpc_code"] == codes.Canceled.String()
		}, 10*time.Second, 20*time.Millisecond, "the access log records the call as canceled, not as a failure of the service")
	})

	t.Run("a response the service tried to write answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionWrite}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
	})

	t.Run("a form value the service tried to read answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionRead}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
	})
}

// TestCallsRunInTheControllerSpan pins the spans a call runs in: the
// controller span of the action, named and attributed the way a request's
// is, by the method and route of the action as the registration described
// it, and the service span of a delegated action inside it.
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
		attribute.String("controller.method", http.MethodGet),
		attribute.String("controller.path", "/api/controller-sample-get"),
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
// alice as the caller of every call presenting sampleCredential; the custom
// OpenAction and the rpcs named after publicPrefix are declared public, so a
// call to them runs anonymous.
var (
	sampleServerOnce sync.Once
	// accessLog collects the gRPC access log of the sample server, the entry
	// of every call it served, for the tests reading how a call ended.
	accessLog        *observer.ObservedLogs
	sampleServerAddr string
	errSampleServer  error
)

// sampleCredential is the authorization a call presents to be named alice,
// and sampleAcrossCredential the one presenting alice acting across tenants
// (see tenant.Across).
const (
	sampleCredential       = "Bearer alice"
	sampleAcrossCredential = "Bearer alice across tenants"
)

// publicPrefix names the standard rpcs of publicRoute, the sample service's
// public methods: no auth interceptor runs on them, so they serve anonymous
// calls.
const publicPrefix = "Public"

// sampleServer returns a connection to the sample service, starting its
// listener on first use.
func sampleServer(t *testing.T) *grpc.ClientConn {
	t.Helper()
	sampleServerOnce.Do(func() {
		grpcserver.UseAuth(func(ctx context.Context) (context.Context, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			if values := md.Get("authorization"); len(values) == 1 {
				alice := grpcserver.WithCaller(ctx, grpcserver.Caller{Username: "alice", UserID: "u-1"})
				switch values[0] {
				case sampleCredential:
					return alice, nil
				case sampleAcrossCredential:
					return tenant.Across(alice), nil
				}
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
			// Every rpc is described with the HTTP action it stands for, the
			// way the generated registration describes one: a route named
			// after the rpc and the method of its kind, so that what a hook
			// reads for the route and method is the action's.
			httpMethod := http.MethodPost
			if strings.Contains(name, "List") || strings.Contains(name, "Get") {
				httpMethod = http.MethodGet
			}
			public := name == "OpenAction" || strings.HasPrefix(name, publicPrefix)
			methods = append(methods, grpcserver.Method{Name: fullMethod, Public: public, HTTPMethod: httpMethod, Route: "/api/controller-sample-" + strings.ToLower(name)})
		}
		for _, name := range []string{"Watch", "Upload", "Chat", "Fork", "Silence"} {
			methods = append(methods, grpcserver.Method{Name: "/gst.test.Samples/" + name, HTTPMethod: grpcserver.MethodStream, Route: "/api/controller-sample-" + strings.ToLower(name)})
		}
		grpcserver.Register(func(r grpc.ServiceRegistrar) {
			r.RegisterService(&grpc.ServiceDesc{
				ServiceName: "gst.test.Samples",
				HandlerType: (*any)(nil),
				Methods:     descs,
				Streams:     streamDescs(),
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
		core, entries := observer.New(zap.InfoLevel)
		logger.GRPC = zap.New(core)
		accessLog = entries
		go func() { _ = grpcserver.Run() }()
	})
	require.NoError(t, errSampleServer)
	conn, err := grpc.NewClient(sampleServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// rpcHandler is an rpc of the sample service: it runs the call function of
// its action the way a generated handler does, with what the request
// message carries, and answers what the response message would.
type rpcHandler func(ctx context.Context, in map[string]any) (any, error)

// parentRoute nests the samples under a parent whose parameter is named id,
// the default name of a model declaring no Param, written the way the
// generated code writes the route of a collection action; parentItemRoute
// is the route of its item actions, ending in the parameter carrying the
// sample's id.
const (
	parentRoute     = "controller-parents/:id/samples"
	parentItemRoute = parentRoute + "/:sample"
)

// sampleHandlers builds the rpcs of the sample service: the ten standard
// actions of every fixture model on every fixture route (see standardRPCs),
// the list and get of the samples nested under a parent (see parentRoute),
// and the custom action of the sample, taking the query and the payload.
func sampleHandlers() map[string]rpcHandler {
	handlers := make(map[string]rpcHandler)
	standardRPCs[*sampleRecord](handlers, sampleRoute, "")
	standardRPCs[*sampleRecord](handlers, refusalRoute, "Refused")
	standardRPCs[*sampleRecord](handlers, filterRefusalRoute, "FilterRefused")
	standardRPCs[*sampleRecord](handlers, hangingRoute, "Hanging")
	standardRPCs[*sampleRecord](handlers, publicRoute, publicPrefix)
	standardRPCs[*sampleRecord](handlers, observedRoute, "Observed")
	standardRPCs[*sampleRecord](handlers, cookieBeforeRoute, "CookieBefore")
	standardRPCs[*sampleRecord](handlers, cookieAfterRoute, "CookieAfter")
	standardRPCs[*sampleCounter](handlers, counterRoute, "Counter")
	standardRPCs[*versionedSample](handlers, versionedRoute, "Versioned")
	standardRPCs[*shapedSample](handlers, shapedRoute, "Shaped")
	standardRPCs[*validatedSample](handlers, validatedRoute, "Validated")
	standardRPCs[*datedSample](handlers, datedRoute, "Dated")
	standardRPCs[*feedSample](handlers, feedRoute, "Feed")
	standardRPCs[*scopedSample](handlers, scopedRoute, "Scoped")
	standardRPCs[*stagedSample](handlers, stagedRoute, "Staged")
	standardRPCs[*stagedSample](handlers, stagingRoute, "Staging")
	standardRPCs[*nestedSample](handlers, nestedRoute, "Nested")

	parentList := controller.ListCall[*sampleRecord](parentRoute)
	parentGet := controller.GetCall[*sampleRecord](parentItemRoute)
	handlers["ParentList"] = func(ctx context.Context, in map[string]any) (any, error) {
		return listing(parentList(ctx, params(in), field[controller.Query](in, "query")))
	}
	handlers["ParentGet"] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return parentGet(ctx, itemParams(parentItemRoute, params(in), id), id, field[controller.Query](in, "query"))
	}

	action := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](apiaction.Create, actionRoute)
	actionList := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](apiaction.List, actionRoute)
	handlers["Action"] = func(ctx context.Context, in map[string]any) (any, error) {
		return action(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
	}
	handlers["OpenAction"] = handlers["Action"]
	handlers["ActionList"] = func(ctx context.Context, in map[string]any) (any, error) {
		return actionList(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
	}
	actionGet := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](apiaction.Get, actionItemRoute)
	handlers["ActionGet"] = func(ctx context.Context, in map[string]any) (any, error) {
		return actionGet(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
	}
	return handlers
}

// standardRPCs adds to handlers the rpcs of the ten standard actions of M
// on route, each named prefix followed by the action's name, Create and
// PatchMany for the samples, RefusedCreate for the refusals: each runs the
// call function of its action the way the generated handler does (see the
// golden record.gen.go of cmd/gg), with what the request message carries,
// the route parameters under params, the id, the record or the items
// decoded as a FromProto decodes them (see fromProto), the update mask, the
// query and the ids; an item of a PatchMany carries its id, its record and
// its mask, and is readied through PatchItem.
func standardRPCs[M types.Model](handlers map[string]rpcHandler, route, prefix string) {
	create := controller.CreateCall[M](route)
	get := controller.GetCall[M](route)
	list := controller.ListCall[M](route)
	update := controller.UpdateCall[M](route)
	patch := controller.PatchCall[M](route)
	del := controller.DeleteCall[M](route)
	createMany := controller.CreateManyCall[M](route)
	updateMany := controller.UpdateManyCall[M](route)
	patchMany := controller.PatchManyCall[M](route)
	deleteMany := controller.DeleteManyCall[M](route)

	handlers[prefix+apiaction.Create.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		record, err := fromProto[M](in["record"])
		if err != nil {
			return nil, err
		}
		return create(ctx, params(in), record)
	}
	handlers[prefix+apiaction.Get.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return get(ctx, itemParams(route, params(in), id), id, field[controller.Query](in, "query"))
	}
	handlers[prefix+apiaction.List.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return listing(list(ctx, params(in), field[controller.Query](in, "query")))
	}
	handlers[prefix+apiaction.Update.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		record, err := fromProto[M](in["record"])
		if err != nil {
			return nil, err
		}
		id := field[string](in, "id")
		return update(ctx, itemParams(route, params(in), id), id, record)
	}
	handlers[prefix+apiaction.Patch.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		record, err := fromProto[M](in["record"])
		if err != nil {
			return nil, err
		}
		id := field[string](in, "id")
		return patch(ctx, itemParams(route, params(in), id), id, record, field[[]string](in, "mask"))
	}
	handlers[prefix+apiaction.Delete.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return done(del(ctx, itemParams(route, params(in), id), id))
	}
	handlers[prefix+apiaction.CreateMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		items, err := itemsFromProto[M](in)
		if err != nil {
			return nil, err
		}
		return batch(createMany(ctx, params(in), items))
	}
	handlers[prefix+apiaction.UpdateMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		items, err := itemsFromProto[M](in)
		if err != nil {
			return nil, err
		}
		return batch(updateMany(ctx, params(in), items))
	}
	handlers[prefix+apiaction.PatchMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		raw := field[[]map[string]any](in, "items")
		models := make([]M, len(raw))
		masks := make([][]string, len(raw))
		for i, item := range raw {
			record, err := fromProto[M](item["record"])
			if err != nil {
				return nil, controller.ItemError(i, err)
			}
			m, err := controller.PatchItem(i, params(in), nil, field[string](item, "id"), record)
			if err != nil {
				return nil, err
			}
			models[i] = m
			masks[i] = field[[]string](item, "mask")
		}
		return batch(patchMany(ctx, params(in), models, masks))
	}
	handlers[prefix+apiaction.DeleteMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return done(deleteMany(ctx, params(in), field[[]string](in, "ids")))
	}
}

// params returns the route parameters the message carries under params.
func params(in map[string]any) map[string]string {
	return field[map[string]string](in, "params")
}

// itemParams returns the route parameters of an item call the way a
// generated handler builds them: the parameters the message carries plus,
// under the last parameter segment of route (see apiaction.LastRouteParam), the
// id the message names the record by, whatever it holds (see the golden
// record.gen.go of cmd/gg). A route naming no parameter, as the fixture
// routes are written, gets the parameters as they are.
func itemParams(route string, given map[string]string, id string) map[string]string {
	name := apiaction.LastRouteParam(route)
	if name == "" {
		return given
	}
	params := maps.Clone(given)
	if params == nil {
		params = make(map[string]string, 1)
	}
	params[name] = id
	return params
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

// fromProto converts raw, the record or the item a message carries, into T
// through its JSON shape, the way a generated FromProto converts a message
// into its model (see messageDecoding), refusing a value a field cannot hold
// the way one does (see grpc.Narrow, grpc.Time and grpc.Number): with the
// status of types.NewInvalidValue naming the field as encoding/json names
// it. A message carrying none, nil, is the nil model. It leaves tenant_id
// out, as a generated FromProto does: the field is output-only on gRPC (see
// TRANSPORTS.md, section 9).
func fromProto[T any](raw any) (T, error) {
	var value T
	if raw == nil {
		return value, nil
	}
	if record, ok := raw.(map[string]any); ok {
		record = maps.Clone(record)
		delete(record, "tenant_id")
		raw = record
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		panic(err)
	}
	if err = jsonv2.Unmarshal(encoded, &value, json.DefaultOptionsV1(), messageDecoding); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return value, grpcserver.StatusError(types.NewInvalidValue(typeErr.Field, typeErr.Value+" does not fit the field", err))
		}
		panic(err)
	}
	return value, nil
}

// messageDecoding is the decoding option fromProto reads a message's JSON
// shape with beside the options json.DefaultOptionsV1 names: a time of day as
// the duration it names (see timeOfDayAsDuration), and a time or a number the
// field cannot hold refused naming the field (see decodeAtPointer).
var messageDecoding = jsonv2.WithUnmarshalers(jsonv2.JoinUnmarshalers(
	timeOfDayAsDuration,
	jsonv2.UnmarshalFromFunc(decodeAtPointer[time.Time]),
	jsonv2.UnmarshalFromFunc(decodeAtPointer[json.Number]),
))

// decodeAtPointer decodes the value the decoder is at into v as json.Unmarshal
// does and refuses a value it cannot read at the value's JSON Pointer, so the
// refusal names the field; a JSON number or a null is left to the default
// decoding, which names the field of what it refuses. A generated FromProto
// reads a Timestamp through grpc.Time and the text of a number through
// grpc.Number, each refusing a value it cannot read by naming the field, while
// a record of the contract table spells a time and a number as HTTP does. It
// is a copy of the decodeAtPointer of request_bind_http.go, which this
// external test package cannot reach, so both lines of the contract table
// read a time and a number alike: a change to one is a change to the other.
func decodeAtPointer[T any](dec *jsontext.Decoder, v *T) error {
	if kind := dec.PeekKind(); kind == '0' || kind == 'n' {
		return errors.ErrUnsupported
	}
	value, err := dec.ReadValue()
	if err != nil {
		return err
	}
	if err = jsonv2.Unmarshal(value, v, json.DefaultOptionsV1()); err != nil {
		return &jsonv2.SemanticError{JSONPointer: dec.StackPointer(), JSONKind: value.Kind(), JSONValue: value.Clone(), GoType: reflect.TypeFor[T](), Err: err}
	}
	return nil
}

// timeOfDayAsDuration reads a time of day for fromProto. A message carries one
// as a duration since midnight, which a generated FromProto reads through
// grpc.TimeOfDay, refusing a duration outside a day, while a record of the
// contract table spells it as HTTP does. The spellings time.TimeOnly takes name
// exactly the durations within a day, 17:30:00.5 the 63000.5 seconds a gRPC
// client sends for it, so a spelling is read through
// modelschema.ParseTimeOfDay, and one naming a duration past the end of a day,
// 25:00:00, is refused naming the field, the way grpc.TimeOfDay refuses 25
// hours. A null is a duration the message leaves unset, read as none.
var timeOfDayAsDuration = jsonv2.UnmarshalFromFunc(func(dec *jsontext.Decoder, t *datatypes.Time) error {
	value, err := dec.ReadValue()
	if err != nil || value.Kind() == jsontext.KindNull {
		return err
	}
	text, err := jsontext.AppendUnquote(nil, value)
	if err != nil {
		return err
	}
	since, err := modelschema.ParseTimeOfDay(string(text))
	if err != nil {
		return &jsonv2.SemanticError{JSONPointer: dec.StackPointer(), JSONKind: value.Kind(), GoType: reflect.TypeFor[datatypes.Time](), Err: err}
	}
	*t = datatypes.Time(since)
	return nil
})

// itemsFromProto converts the items a batch message carries into models the
// way the generated handler of a CreateMany or UpdateMany rpc does: each
// through fromProto, the refusal of one naming the item (see
// controller.ItemError); a message carrying no items is nil items.
func itemsFromProto[M types.Model](in map[string]any) ([]M, error) {
	raw := field[[]any](in, "items")
	if raw == nil {
		return nil, nil
	}
	items := make([]M, len(raw))
	for i, item := range raw {
		m, err := fromProto[M](item)
		if err != nil {
			return nil, controller.ItemError(i, err)
		}
		items[i] = m
	}
	return items, nil
}

// listing, batch and done shape what a list, a batch and a call answering
// nothing return into what their response messages would carry.
func listing[M any](items []M, total int, hasMore bool, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total, "has_more": hasMore}, nil
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
	return invokeIn(t.Context(), t, conn, name, in, authorization)
}

// invokeIn is invokeAs for a call made in ctx, which a test ending the
// call on the client's behalf cancels; every call is given twenty seconds.
func invokeIn(ctx context.Context, t *testing.T, conn *grpc.ClientConn, name string, in map[string]any, authorization string) (map[string]any, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
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
// one is given.
func requireStatus(t *testing.T, err error, code codes.Code, message string) {
	t.Helper()
	require.Error(t, err)
	st := status.Convert(err)
	require.Equal(t, code, st.Code(), st.Message())
	if message != "" {
		require.Equal(t, message, st.Message())
	}
}
