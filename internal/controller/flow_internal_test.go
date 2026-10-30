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
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/auditmanager"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
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

// sealedFlowSample carries a column gorm writes on insert only, which an
// update leaves as stored whatever the request carries.
type sealedFlowSample struct {
	Name string `json:"name"`
	Code string `json:"code" gorm:"size:191;<-:create"`

	modelregistry.Base
}

func (sealedFlowSample) TableName() string { return "controller_sealed_flow_samples" }

// replicaFlowSample is flowSample with its reads preferring a replica (see
// modelregistry.PrefersReplica). Its tables are where useReplica puts them,
// not on the default database.
type replicaFlowSample struct {
	Name string `json:"name"`

	modelregistry.Base
}

func (replicaFlowSample) TableName() string { return "controller_replica_flow_samples" }

func (replicaFlowSample) PreferReplica() bool { return true }

// sealedAfterHook records the code the hook after an update reads. sealedAfter
// is its one instance, registered once for the process, so a rerun of the
// test clears it and reads its own call.
type sealedAfterHook struct {
	serviceregistry.Base[*sealedFlowSample, *sealedFlowSample, *sealedFlowSample]
	seen string
}

func (h *sealedAfterHook) UpdateAfter(_ *types.ServiceContext, m *sealedFlowSample) error {
	h.seen = m.Code
	return nil
}

var sealedAfter = &sealedAfterHook{}

func init() {
	modelregistry.Register[*flowSample]()
	modelregistry.Register[*sealedFlowSample]()
}

// plainServiceContext builds the service contexts of a flow run outside any
// HTTP request, the way a call does (see call.serviceContext).
func plainServiceContext(ctx context.Context, phase apiaction.Phase) *types.ServiceContext {
	return types.NewServiceContext(nil, ctx, phase)
}

// TestCreateFlowRunsOnRequestMetadataAlone pins that a flow needs no HTTP
// request: the create flow runs on a context carrying the request metadata
// alone, and takes the audit identity of the record from that metadata.
func TestCreateFlowRunsOnRequestMetadataAlone(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.Create, apiaction.CreateBefore, apiaction.CreateAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))
	record := &flowSample{Name: "created by the flow"}

	require.NoError(t, a.createFlow(ctx, plainServiceContext, record))

	require.NotEmpty(t, record.GetID())
	require.Equal(t, "u-flow", record.GetCreatedBy())
	require.Equal(t, "u-flow", record.GetUpdatedBy())
}

// TestUpdateFlowsAnswerTheColumnsAnUpdateLeavesAsStored pins what the update
// flows answer for a column gorm writes on insert only: the value stored, not
// the one the request carried, which the database did not take. The hook
// after the write reads the stored value too, and a patch never applies the
// column at all.
func TestUpdateFlowsAnswerTheColumnsAnUpdateLeavesAsStored(t *testing.T) {
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))
	stored := func(t *testing.T) *sealedFlowSample {
		t.Helper()
		record := &sealedFlowSample{Name: uniqueFlowName("sealed"), Code: "original"}
		require.NoError(t, database.Database[*sealedFlowSample](ctx).Create(record))
		return record
	}
	forged := func(id, name string) *sealedFlowSample {
		item := &sealedFlowSample{Name: name, Code: "forged"}
		item.SetID(id)
		return item
	}

	t.Run("Update", func(t *testing.T) {
		registerTestService[*sealedFlowSample, *sealedFlowSample, *sealedFlowSample](apiaction.Update, "sealed-flow-samples", sealedAfter)
		sealedAfter.seen = ""
		a := newAction[*sealedFlowSample, *sealedFlowSample, *sealedFlowSample]("sealed-flow-samples", apiaction.Update, apiaction.UpdateBefore, apiaction.UpdateAfter)
		record := stored(t)
		req := &sealedFlowSample{Name: "updated", Code: "forged"}

		require.NoError(t, a.updateFlow(ctx, plainServiceContext, record.GetID(), req))
		require.Equal(t, "updated", req.Name)
		require.Equal(t, "original", req.Code, "the update left the code as stored, and the answer says so")
		require.Equal(t, "original", sealedAfter.seen, "the hook after the write reads the stored code")
		require.True(t, req.GetCreatedAt().Equal(record.GetCreatedAt()))
	})

	t.Run("UpdateMany", func(t *testing.T) {
		a := newAction[*sealedFlowSample, *sealedFlowSample, *sealedFlowSample]("sealed-flow-samples", apiaction.UpdateMany, apiaction.UpdateManyBefore, apiaction.UpdateManyAfter)
		record := stored(t)
		req := batch[*sealedFlowSample]{Items: []*sealedFlowSample{forged(record.GetID(), "updated")}}

		require.NoError(t, a.updateManyFlow(ctx, plainServiceContext, &req))
		require.Equal(t, "updated", req.Items[0].Name)
		require.Equal(t, "original", req.Items[0].Code)
	})

	t.Run("Patch", func(t *testing.T) {
		a := newAction[*sealedFlowSample, *sealedFlowSample, *sealedFlowSample]("sealed-flow-samples", apiaction.Patch, apiaction.PatchBefore, apiaction.PatchAfter)
		record := stored(t)

		patched, err := a.patchFlow(ctx, plainServiceContext, record.GetID(), forged("", "patched"), patchFieldSet{"Name": {}, "Code": {}})
		require.NoError(t, err)
		require.Equal(t, "patched", patched.Name)
		require.Equal(t, "original", patched.Code)
	})

	t.Run("PatchMany", func(t *testing.T) {
		a := newAction[*sealedFlowSample, *sealedFlowSample, *sealedFlowSample]("sealed-flow-samples", apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
		record := stored(t)
		req := batch[*sealedFlowSample]{Items: []*sealedFlowSample{forged(record.GetID(), "patched")}}

		rsp, err := a.patchManyFlow(ctx, plainServiceContext, &req, []patchFieldSet{{"Name": {}, "Code": {}}})
		require.NoError(t, err)
		require.Equal(t, "patched", rsp.Items[0].Name)
		require.Equal(t, "original", rsp.Items[0].Code)
	})
}

// TestGetFlowAnswersNotFoundAsAServiceError pins how a flow reports a
// request it cannot serve: the error carries the service error the transport
// answers with, 404 with the not-found message here for an id naming no
// record.
func TestGetFlowAnswersNotFoundAsAServiceError(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.Get, apiaction.GetBefore, apiaction.GetAfter)
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

	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.Patch, apiaction.PatchBefore, apiaction.PatchAfter)
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
		WithQuery(&modellogmgmt.OperationLog{Model: a.name, OP: types.OperationPatch}).List(&entries))
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

	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.Patch, apiaction.PatchBefore, apiaction.PatchAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))
	record := &flowSample{Name: "before the patch"}
	require.NoError(t, database.Database[*flowSample](ctx).Create(record))

	name := "after the patch " + strconv.FormatInt(time.Now().UnixNano(), 36)
	patched, err := a.patchFlow(ctx, plainServiceContext, record.GetID(), &flowSample{Name: name}, patchFieldSet{"Name": {}})
	require.NoError(t, err)
	require.Equal(t, name, patched.Name)

	var entries []*modellogmgmt.OperationLog
	require.NoError(t, database.Database[*modellogmgmt.OperationLog](ctx).
		WithQuery(&modellogmgmt.OperationLog{Model: a.name, OP: types.OperationPatch}).List(&entries))
	entries = slices.DeleteFunc(entries, func(entry *modellogmgmt.OperationLog) bool { return !strings.Contains(entry.Request, name) })
	require.Len(t, entries, 1)
	require.Equal(t, record.GetID(), entries[0].RecordID)
}
