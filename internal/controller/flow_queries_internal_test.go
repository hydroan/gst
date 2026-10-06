package controller

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/pkg/auditmanager"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// These tests pin how many statements a flow reads with: a batch reads its
// records in one statement, and a delete reads the record it deletes only
// for the operation log, so a disabled audit costs no query.

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

// operationLogEntries returns the entries of op whose Record mentions text.
func operationLogEntries(ctx context.Context, t *testing.T, a *action[*flowSample, *flowSample, *flowSample], op consts.OP, text string) []*modellogmgmt.OperationLog {
	t.Helper()
	var entries []*modellogmgmt.OperationLog
	require.NoError(t, database.Database[*modellogmgmt.OperationLog](ctx).
		WithQuery(&modellogmgmt.OperationLog{Model: a.name, OP: op}).List(&entries))
	return slices.DeleteFunc(entries, func(entry *modellogmgmt.OperationLog) bool { return !strings.Contains(entry.Record, text) })
}

func TestUpdateManyFlowBackfillsTheCreationAuditInOneStatement(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.UpdateMany, consts.UpdateManyBefore, consts.UpdateManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))
	stored := createFlowSamples(ctx, t, uniqueFlowName("update many"), 3)
	req := batch[*flowSample]{}
	for _, record := range stored {
		req.Items = append(req.Items, flowItem(record.GetID(), record.Name+" updated"))
	}

	selects := captureSelects(t, flowSample{}.TableName())
	require.NoError(t, a.updateManyFlow(ctx, plainServiceContext, &req))

	require.Len(t, selects(), 1, "the batch reads its rows back in one statement")
	for i, item := range req.Items {
		require.Equal(t, "flow-user", item.GetUpdatedBy())
		require.True(t, item.GetCreatedAt().Equal(stored[i].GetCreatedAt()), "the creation audit is backfilled as stored")
	}
}

func TestPatchManyFlowReadsTheBatchInOneStatement(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.PatchMany, consts.PatchManyBefore, consts.PatchManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))
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
		require.Equal(t, "flow-user", item.GetUpdatedBy())
	}
}

func TestPatchManyFlowRefusesAnItemWithoutAnIDBeforeReading(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.PatchMany, consts.PatchManyBefore, consts.PatchManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))
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
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.Delete, consts.DeleteBefore, consts.DeleteAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))

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
		entries := operationLogEntries(ctx, t, a, consts.OP_DELETE, record.Name)
		require.Len(t, entries, 1)
		require.Equal(t, record.GetID(), entries[0].RecordID)
	})
}

func TestDeleteManyFlowRecordsTheDeletedRecordsWhenAudited(t *testing.T) {
	a := newAction[*flowSample, *flowSample, *flowSample]("flow-samples", consts.DeleteMany, consts.DeleteManyBefore, consts.DeleteManyAfter)
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{Username: "flow-user"}))

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
		entries := operationLogEntries(ctx, t, a, consts.OP_DELETE_MANY, name)
		require.Len(t, entries, 1)
		for _, record := range stored {
			require.Contains(t, entries[0].Record, record.GetID())
			require.Contains(t, entries[0].Record, record.Name, "the log carries the records as they were, not only their ids")
		}
	})
}
