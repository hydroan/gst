package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	gstsqlite "github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/auditmanager"
	"github.com/hydroan/gst/internal/dbruntime"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// These tests pin how a flow reads: a batch reads its records in one
// statement, and a delete reads the record it deletes only for the
// operation log, so a disabled audit costs no query, and from the primary,
// so a record a replica has not caught up on is deleted and logged all the
// same.

// captureSelects records the SELECT statements run against table from now
// until the test ends, through a query callback on the shared handle, and
// returns the function that reads them.
func captureSelects(t *testing.T, table string) func() []string {
	t.Helper()
	var (
		mu         sync.Mutex
		statements []string
	)
	db := database.DB()
	name := "test:capture-" + table
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if !strings.Contains(sql, table) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		statements = append(statements, sql)
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(name)) })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(statements)
	}
}

// useAudit swaps the package's audit manager for one built from cfg until
// the test ends, with the operation log table in place.
func useAudit(t *testing.T, cfg config.Audit) {
	t.Helper()
	require.NoError(t, database.DB().AutoMigrate(&modellogmgmt.OperationLog{}))
	previous := audit
	audit = auditmanager.New(&cfg)
	t.Cleanup(func() { audit = previous })
}

// useReplica points the database layer at a primary with a replica behind
// it until the test ends, each a database file of its own: the primary
// holds the table of replicaFlowSample and, once useAudit runs, the
// operation log, and the replica the sample table without a row, a replica
// that has not caught up on anything the test stores.
func useReplica(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	replica, err := gstsqlite.New(config.Sqlite{Path: filepath.Join(dir, "replica.db")})
	require.NoError(t, err)
	require.NoError(t, replica.AutoMigrate(&replicaFlowSample{}))
	primary, err := gstsqlite.New(config.Sqlite{Path: filepath.Join(dir, "primary.db")})
	require.NoError(t, err)
	handle, err := dbruntime.AttachResolver(primary, []gorm.Dialector{replica.Dialector})
	require.NoError(t, err)
	require.NoError(t, handle.AutoMigrate(&replicaFlowSample{}))
	previous := dbruntime.DB
	dbruntime.DB = handle
	t.Cleanup(func() {
		dbruntime.DB = previous
		for _, node := range dbruntime.NodesFor(handle) {
			require.NoError(t, node.DB.Close())
		}
		dbruntime.AttachNodes(handle, nil)
		replicaPool, err := replica.DB()
		require.NoError(t, err)
		require.NoError(t, replicaPool.Close())
	})
}

// createFlowSamples stores n flow samples named after prefix and returns
// them with their ids.
func createFlowSamples(ctx context.Context, t *testing.T, prefix string, n int) []*flowSample {
	t.Helper()
	records := make([]*flowSample, 0, n)
	for i := range n {
		record := &flowSample{Name: prefix + " " + strconv.Itoa(i)}
		require.NoError(t, database.Database[*flowSample](ctx).Create(record))
		records = append(records, record)
	}
	return records
}

// flowItem returns the item of a batch naming the record id, with name as
// the value it carries.
func flowItem(id, name string) *flowSample {
	item := &flowSample{Name: name}
	item.SetID(id)
	return item
}

// uniqueFlowName returns a name no other test run used.
func uniqueFlowName(prefix string) string {
	return prefix + " " + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// operationLogEntries returns the entries of op on model whose Record
// mentions text.
func operationLogEntries(ctx context.Context, t *testing.T, model string, op types.Operation, text string) []*modellogmgmt.OperationLog {
	t.Helper()
	var entries []*modellogmgmt.OperationLog
	require.NoError(t, database.Database[*modellogmgmt.OperationLog](ctx).
		WithQuery(&modellogmgmt.OperationLog{Model: model, OP: op}).List(&entries))
	return slices.DeleteFunc(entries, func(entry *modellogmgmt.OperationLog) bool { return !strings.Contains(entry.Record, text) })
}

func TestUpdateManyFlowBackfillsTheCreationAuditInOneStatement(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.UpdateMany, apiaction.UpdateManyBefore, apiaction.UpdateManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))
	stored := createFlowSamples(ctx, t, uniqueFlowName("update many"), 3)
	req := batch[*flowSample]{}
	for _, record := range stored {
		req.Items = append(req.Items, flowItem(record.GetID(), record.Name+" updated"))
	}

	selects := captureSelects(t, flowSample{}.TableName())
	require.NoError(t, a.updateManyFlow(ctx, plainServiceContext, &req))

	require.Len(t, selects(), 1, "the batch reads its rows back in one statement")
	for i, item := range req.Items {
		require.Equal(t, "u-flow", item.GetUpdatedBy())
		require.True(t, item.GetCreatedAt().Equal(stored[i].GetCreatedAt()), "the creation audit is backfilled as stored")
	}
}

func TestPatchManyFlowReadsTheBatchInOneStatement(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))
	stored := createFlowSamples(ctx, t, uniqueFlowName("patch many"), 3)
	req := batch[*flowSample]{}
	fieldSets := make([]patchFieldSet, 0, len(stored))
	for _, record := range stored {
		req.Items = append(req.Items, flowItem(record.GetID(), record.Name+" patched"))
		fieldSets = append(fieldSets, patchFieldSet{"Name": {}})
	}

	selects := captureSelects(t, flowSample{}.TableName())
	rsp, err := a.patchManyFlow(ctx, plainServiceContext, &req, fieldSets)
	require.NoError(t, err)

	require.Len(t, selects(), 1, "the batch reads its records in one statement")
	require.Len(t, rsp.Items, 3)
	for i, item := range rsp.Items {
		require.Equal(t, stored[i].Name+" patched", item.Name)
		require.Equal(t, "u-flow", item.GetUpdatedBy())
	}
}

func TestPatchManyFlowRefusesAnItemWithoutAnIDBeforeReading(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))
	stored := createFlowSamples(ctx, t, uniqueFlowName("patch many without id"), 1)
	req := batch[*flowSample]{Items: []*flowSample{
		flowItem(stored[0].GetID(), "renamed"),
		{Name: "no id"},
	}}

	selects := captureSelects(t, flowSample{}.TableName())
	_, err := a.patchManyFlow(ctx, plainServiceContext, &req, []patchFieldSet{{"Name": {}}, {"Name": {}}})

	var serviceErr *types.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, http.StatusBadRequest, serviceErr.Status())
	require.Empty(t, selects(), "a defective batch is refused before any record is read")
}

func TestDeleteFlowReadsTheRecordOnlyForTheOperationLog(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.Delete, apiaction.DeleteBefore, apiaction.DeleteAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))

	t.Run("a disabled audit reads nothing", func(t *testing.T) {
		useAudit(t, config.Audit{})
		record := createFlowSamples(ctx, t, uniqueFlowName("delete unaudited"), 1)[0]

		selects := captureSelects(t, flowSample{}.TableName())
		require.NoError(t, a.deleteFlow(ctx, plainServiceContext, record.GetID()))

		require.Empty(t, selects())
	})
	t.Run("an enabled audit reads the record once and logs it", func(t *testing.T) {
		useAudit(t, config.Audit{Enabled: true})
		name := uniqueFlowName("delete audited")
		record := createFlowSamples(ctx, t, name, 1)[0]

		selects := captureSelects(t, flowSample{}.TableName())
		require.NoError(t, a.deleteFlow(ctx, plainServiceContext, record.GetID()))

		require.Len(t, selects(), 1)
		entries := operationLogEntries(ctx, t, a.name, types.OperationDelete, record.Name)
		require.Len(t, entries, 1)
		require.Equal(t, record.GetID(), entries[0].RecordID)
	})
	t.Run("a record preferring replicas is read from the primary", func(t *testing.T) {
		useReplica(t)
		useAudit(t, config.Audit{Enabled: true})
		ra := newAction[*replicaFlowSample, *replicaFlowSample, *replicaFlowSample]("replica-flow-samples", apiaction.Delete, apiaction.DeleteBefore, apiaction.DeleteAfter)
		record := &replicaFlowSample{Name: uniqueFlowName("delete from the primary")}
		require.NoError(t, database.Database[*replicaFlowSample](ctx).Create(record))

		require.NoError(t, ra.deleteFlow(ctx, plainServiceContext, record.GetID()), "a record the replica has not caught up on is the primary's to delete")

		require.ErrorIs(t, database.Database[*replicaFlowSample](ctx).WithReplica(false).Get(&replicaFlowSample{}, record.GetID()), database.ErrRecordNotFound)
		entries := operationLogEntries(ctx, t, ra.name, types.OperationDelete, record.Name)
		require.Len(t, entries, 1, "the log carries the record as the primary holds it")
	})
}

func TestDeleteManyFlowRecordsTheDeletedRecordsWhenAudited(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.DeleteMany, apiaction.DeleteManyBefore, apiaction.DeleteManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))

	t.Run("a disabled audit reads nothing", func(t *testing.T) {
		useAudit(t, config.Audit{})
		stored := createFlowSamples(ctx, t, uniqueFlowName("delete many unaudited"), 2)
		req := batch[*flowSample]{IDs: []string{stored[0].GetID(), stored[1].GetID()}}

		selects := captureSelects(t, flowSample{}.TableName())
		require.NoError(t, a.deleteManyFlow(ctx, plainServiceContext, &req))

		require.Empty(t, selects())
	})
	t.Run("an enabled audit reads the batch once and logs the records", func(t *testing.T) {
		useAudit(t, config.Audit{Enabled: true})
		name := uniqueFlowName("delete many audited")
		stored := createFlowSamples(ctx, t, name, 2)
		req := batch[*flowSample]{IDs: []string{stored[0].GetID(), stored[1].GetID()}}

		selects := captureSelects(t, flowSample{}.TableName())
		require.NoError(t, a.deleteManyFlow(ctx, plainServiceContext, &req))

		require.Len(t, selects(), 1, "the batch reads the records it deletes in one statement")
		entries := operationLogEntries(ctx, t, a.name, types.OperationDeleteMany, name)
		require.Len(t, entries, 1)
		for _, record := range stored {
			require.Contains(t, entries[0].Record, record.GetID())
			require.Contains(t, entries[0].Record, record.Name, "the log carries the records as they were, not only their ids")
		}
	})
	t.Run("records preferring replicas are read from the primary", func(t *testing.T) {
		useReplica(t)
		useAudit(t, config.Audit{Enabled: true})
		ra := newAction[*replicaFlowSample, *replicaFlowSample, *replicaFlowSample]("replica-flow-samples", apiaction.DeleteMany, apiaction.DeleteManyBefore, apiaction.DeleteManyAfter)
		name := uniqueFlowName("delete many from the primary")
		stored := []*replicaFlowSample{{Name: name + " 0"}, {Name: name + " 1"}}
		require.NoError(t, database.Database[*replicaFlowSample](ctx).Create(stored...))
		req := batch[*replicaFlowSample]{IDs: []string{stored[0].GetID(), stored[1].GetID()}}

		require.NoError(t, ra.deleteManyFlow(ctx, plainServiceContext, &req))

		entries := operationLogEntries(ctx, t, ra.name, types.OperationDeleteMany, name)
		require.Len(t, entries, 1, "the log carries the records as the primary holds them")
		for _, record := range stored {
			require.Contains(t, entries[0].Record, record.Name)
		}
	})
}

// TestBatchFlowsRecordTheirItemsWhenAudited pins the operation log of the
// batch writes beside DeleteMany: CreateMany and UpdateMany log the batch as
// written as its record, request and response alike, and PatchMany logs the
// batch it was sent as the record and request and the records as patched as
// the response.
func TestBatchFlowsRecordTheirItemsWhenAudited(t *testing.T) {
	useAudit(t, config.Audit{Enabled: true})
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user", UserID: "u-flow"}))

	t.Run("CreateMany", func(t *testing.T) {
		a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.CreateMany, apiaction.CreateManyBefore, apiaction.CreateManyAfter)
		name := uniqueFlowName("create many audited")
		req := batch[*flowSample]{Items: []*flowSample{{Name: name + " 0"}, {Name: name + " 1"}}}

		require.NoError(t, a.createManyFlow(ctx, plainServiceContext, &req))

		entries := operationLogEntries(ctx, t, a.name, types.OperationCreateMany, name)
		require.Len(t, entries, 1)
		requireLoggedAs(t, req, entries[0].Record)
		require.Equal(t, entries[0].Record, entries[0].Request)
		require.Equal(t, entries[0].Record, entries[0].Response)
	})
	t.Run("UpdateMany", func(t *testing.T) {
		a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.UpdateMany, apiaction.UpdateManyBefore, apiaction.UpdateManyAfter)
		name := uniqueFlowName("update many audited")
		req := batch[*flowSample]{}
		for _, record := range createFlowSamples(ctx, t, name, 2) {
			req.Items = append(req.Items, flowItem(record.GetID(), record.Name+" updated"))
		}

		require.NoError(t, a.updateManyFlow(ctx, plainServiceContext, &req))

		entries := operationLogEntries(ctx, t, a.name, types.OperationUpdateMany, name)
		require.Len(t, entries, 1)
		requireLoggedAs(t, req, entries[0].Record)
		require.Equal(t, entries[0].Record, entries[0].Request)
		require.Equal(t, entries[0].Record, entries[0].Response)
	})
	t.Run("PatchMany", func(t *testing.T) {
		a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
		name := uniqueFlowName("patch many audited")
		req := batch[*flowSample]{}
		var fieldSets []patchFieldSet
		for _, record := range createFlowSamples(ctx, t, name, 2) {
			req.Items = append(req.Items, flowItem(record.GetID(), record.Name+" patched"))
			fieldSets = append(fieldSets, patchFieldSet{"Name": {}})
		}

		patched, err := a.patchManyFlow(ctx, plainServiceContext, &req, fieldSets)
		require.NoError(t, err)

		entries := operationLogEntries(ctx, t, a.name, types.OperationPatchMany, name)
		require.Len(t, entries, 1)
		requireLoggedAs(t, req, entries[0].Record)
		require.Equal(t, entries[0].Record, entries[0].Request)
		requireLoggedAs(t, patched, entries[0].Response)
	})
}

// requireLoggedAs requires the logged column to hold value as JSON.
func requireLoggedAs(t *testing.T, value any, logged string) {
	t.Helper()
	want, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, string(want), logged)
}
