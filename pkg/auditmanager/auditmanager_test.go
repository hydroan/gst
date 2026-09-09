package auditmanager

import (
	"context"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/ds/queue/circularbuffer"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// auditSample is the resource an audit entry is recorded for.
type auditSample struct {
	model.Base
}

func (auditSample) TableName() string { return "audit_samples" }

// TestRecordOperationSkipsBuildWhenDisabled pins the reason the entry is built
// by a callback at all: auditing is disabled by default, and a disabled audit
// must not pay for an entry it goes on to discard.
func TestRecordOperationSkipsBuildWhenDisabled(t *testing.T) {
	manager, buffer := newTestManager(t, &config.Audit{Enabled: false, AsyncWrite: true})

	var built int
	require.NoError(t, manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_CREATE,
		func() *modellogmgmt.OperationLog {
			built++
			return &modellogmgmt.OperationLog{}
		}))

	require.Zero(t, built, "a disabled audit must not build the entry")
	require.True(t, buffer.IsEmpty())
}

// TestRecordOperationSkipsBuildWhenOperationExcluded pins that the exclusion
// check runs before the build too, which is why the operation is a parameter
// instead of a field the build would have to produce first.
func TestRecordOperationSkipsBuildWhenOperationExcluded(t *testing.T) {
	manager, buffer := newTestManager(t, &config.Audit{
		Enabled:           true,
		AsyncWrite:        true,
		ExcludeOperations: []consts.OP{consts.OP_LIST},
	})

	var built int
	require.NoError(t, manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_LIST,
		func() *modellogmgmt.OperationLog {
			built++
			return &modellogmgmt.OperationLog{}
		}))

	require.Zero(t, built, "an excluded operation must not build the entry")
	require.True(t, buffer.IsEmpty())
}

// TestRecordOperationStampsOperationAndTable pins what the manager fills in
// itself: the caller names the operation once, in the argument, and never
// names the table.
func TestRecordOperationStampsOperationAndTable(t *testing.T) {
	manager, buffer := newTestManager(t, &config.Audit{Enabled: true, AsyncWrite: true})

	var built int
	require.NoError(t, manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_CREATE,
		func() *modellogmgmt.OperationLog {
			built++
			return &modellogmgmt.OperationLog{Model: "auditSample", User: "operator"}
		}))
	require.Equal(t, 1, built, "an enabled audit must build the entry exactly once")

	entry, ok := buffer.Dequeue()
	require.True(t, ok)
	require.Equal(t, consts.OP_CREATE, entry.OP, "the manager stamps the operation it was given")
	require.Equal(t, "audit_samples", entry.Table, "the manager stamps the model's own table")
	require.Equal(t, "auditSample", entry.Model, "fields the build set must survive untouched")
	require.Equal(t, "operator", entry.User)
}

// TestRecordOperationRefusesNilBuild pins that a missing build is refused
// rather than skipped, and that the refusal does not wait for the audit to be
// enabled: an entry asked for and never written is a gap in a security record.
func TestRecordOperationRefusesNilBuild(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		manager, buffer := newTestManager(t, &config.Audit{Enabled: enabled, AsyncWrite: true})

		err := manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_CREATE, nil)
		require.Error(t, err, "a nil build is refused whether the audit is on or off")
		require.True(t, buffer.IsEmpty())
	}
}

// newTestManager builds a manager writing into a buffer the test can drain,
// which keeps every case on the asynchronous path and away from a database.
func newTestManager(t *testing.T, conf *config.Audit) (*AuditManager, *circularbuffer.CircularBuffer[*modellogmgmt.OperationLog]) {
	t.Helper()

	buffer, err := circularbuffer.New[*modellogmgmt.OperationLog](8)
	require.NoError(t, err)
	return New(conf, buffer), buffer
}
