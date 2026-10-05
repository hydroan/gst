package controller

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/pkg/auditmanager"
	"github.com/stretchr/testify/require"
)

// flowSample is the table model the flow tests read and write. It is
// registered as the package initializes, before the TestMain of the handler
// tests brings the framework up, so its table exists beside the fixture
// tables.
type flowSample struct {
	Name string `json:"name"`

	modelregistry.Base
}

func (flowSample) TableName() string { return "controller_flow_samples" }

func init() { modelregistry.Register[*flowSample]() }

// plainServiceContext builds the service contexts of a flow run outside any
// HTTP request, the way a transport other than HTTP will.
func plainServiceContext(ctx context.Context, phase consts.Phase) *types.ServiceContext {
	return types.NewServiceContext(nil, ctx, phase)
}

// TestCreateFlowRunsOnRequestMetadataAlone pins what the flows are extracted
// for: the create flow needs no HTTP request, only a context carrying the
// request metadata, and takes the audit identity of the record from that
// metadata.
func TestCreateFlowRunsOnRequestMetadataAlone(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.Create, consts.CreateBefore, consts.CreateAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))
	record := &flowSample{Name: "created by the flow"}

	require.NoError(t, a.createFlow(ctx, plainServiceContext, record))

	require.NotEmpty(t, record.GetID())
	require.Equal(t, "flow-user", record.GetCreatedBy())
	require.Equal(t, "flow-user", record.GetUpdatedBy())
}

// TestGetFlowAnswersNotFoundAsAServiceError pins how a flow reports a
// request it cannot serve: the error carries the service error the transport
// answers with, 404 with the not-found message here for an id naming no
// record.
func TestGetFlowAnswersNotFoundAsAServiceError(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.Get, consts.GetBefore, consts.GetAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{}))

	_, err := a.getFlow(ctx, plainServiceContext, "missing")

	var serviceErr *types.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, http.StatusNotFound, serviceErr.Status())
	require.Equal(t, notFoundMsg, serviceErr.Msg())
}

// TestFlowRecordsTheRequestMethodAndURIInTheOperationLog pins that the
// operation log entry of a flow names the request by the method and URI of
// the request metadata as the transport filled them: over gRPC the action's
// HTTP method and the full method of the call, PATCH and
// /app.FlowSampleService/PatchFlowSample for a patch, so an entry reads the
// same whichever listener served the action.
func TestFlowRecordsTheRequestMethodAndURIInTheOperationLog(t *testing.T) {
	require.NoError(t, database.DB().AutoMigrate(&modellogmgmt.OperationLog{}))
	previous := audit
	audit = auditmanager.New(&config.Audit{Enabled: true})
	t.Cleanup(func() { audit = previous })

	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.Patch, consts.PatchBefore, consts.PatchAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{
		Username: "flow-user", Method: http.MethodPatch, RequestURI: "/app.FlowSampleService/PatchFlowSample",
	}))
	record := &flowSample{Name: "before the logged patch"}
	require.NoError(t, database.Database[*flowSample](ctx).Create(record))

	name := "after the logged patch " + strconv.FormatInt(time.Now().UnixNano(), 36)
	_, err := a.patchFlow(ctx, plainServiceContext, record.GetID(), &flowSample{Name: name}, patchFieldSet{"Name": {}})
	require.NoError(t, err)

	var entries []*modellogmgmt.OperationLog
	require.NoError(t, database.Database[*modellogmgmt.OperationLog](ctx).
		WithQuery(&modellogmgmt.OperationLog{Model: a.name, OP: consts.OP_PATCH}).List(&entries))
	entries = slices.DeleteFunc(entries, func(entry *modellogmgmt.OperationLog) bool { return !strings.Contains(entry.Request, name) })
	require.Len(t, entries, 1)
	require.Equal(t, http.MethodPatch, entries[0].Method)
	require.Equal(t, "/app.FlowSampleService/PatchFlowSample", entries[0].URI)
}

// TestPatchFlowRecordsTheRecordIDInTheOperationLog pins which id the audit
// entry of a patch names: the patched record's, which the route gave. The
// body of a patch carries the fields to change and need not carry an id, so
// taking the id from the body would leave the entry without one.
func TestPatchFlowRecordsTheRecordIDInTheOperationLog(t *testing.T) {
	require.NoError(t, database.DB().AutoMigrate(&modellogmgmt.OperationLog{}))
	previous := audit
	audit = auditmanager.New(&config.Audit{Enabled: true})
	t.Cleanup(func() { audit = previous })

	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.Patch, consts.PatchBefore, consts.PatchAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))
	record := &flowSample{Name: "before the patch"}
	require.NoError(t, database.Database[*flowSample](ctx).Create(record))

	name := "after the patch " + strconv.FormatInt(time.Now().UnixNano(), 36)
	patched, err := a.patchFlow(ctx, plainServiceContext, record.GetID(), &flowSample{Name: name}, patchFieldSet{"Name": {}})
	require.NoError(t, err)
	require.Equal(t, name, patched.Name)

	var entries []*modellogmgmt.OperationLog
	require.NoError(t, database.Database[*modellogmgmt.OperationLog](ctx).
		WithQuery(&modellogmgmt.OperationLog{Model: a.name, OP: consts.OP_PATCH}).List(&entries))
	entries = slices.DeleteFunc(entries, func(entry *modellogmgmt.OperationLog) bool { return !strings.Contains(entry.Request, name) })
	require.Len(t, entries, 1)
	require.Equal(t, record.GetID(), entries[0].RecordID)
}
