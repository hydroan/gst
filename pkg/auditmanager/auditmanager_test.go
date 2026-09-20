package auditmanager_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/pkg/auditmanager"
	"github.com/stretchr/testify/require"
)

// TestMain brings the framework up because the manager writes the entries
// itself: what the table holds afterwards is what these tests read back.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{})
}

// auditSample is the resource an audit entry is recorded for.
type auditSample struct {
	modelregistry.Base
}

func (auditSample) TableName() string { return "audit_samples" }

// TestRecordOperationSkipsBuildWhenDisabled pins the reason the entry is built
// by a callback at all: auditing is disabled by default, and a disabled audit
// must not pay for an entry it goes on to discard.
func TestRecordOperationSkipsBuildWhenDisabled(t *testing.T) {
	manager := auditmanager.New(&config.Audit{Enabled: false})

	var built int
	require.NoError(t, manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_CREATE,
		func() *modellogmgmt.OperationLog {
			built++
			return &modellogmgmt.OperationLog{}
		}))

	require.Zero(t, built, "a disabled audit must not build the entry")
}

// TestRecordOperationSkipsBuildWhenOperationExcluded pins that the exclusion
// check runs before the build too, which is why the operation is a parameter
// instead of a field the build would have to produce first.
func TestRecordOperationSkipsBuildWhenOperationExcluded(t *testing.T) {
	manager := auditmanager.New(&config.Audit{
		Enabled:           true,
		ExcludeOperations: []consts.OP{consts.OP_LIST},
	})

	var built int
	require.NoError(t, manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_LIST,
		func() *modellogmgmt.OperationLog {
			built++
			return &modellogmgmt.OperationLog{}
		}))

	require.Zero(t, built, "an excluded operation must not build the entry")
}

// TestRecordOperationStampsOperationAndTable pins what the manager fills in
// itself: the caller names the operation once, in the argument, and never
// names the table.
func TestRecordOperationStampsOperationAndTable(t *testing.T) {
	prepareOperationLogTable(t)
	manager := auditmanager.New(&config.Audit{Enabled: true})

	var built int
	require.NoError(t, manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_CREATE,
		func() *modellogmgmt.OperationLog {
			built++
			return &modellogmgmt.OperationLog{Model: t.Name(), User: "operator"}
		}))
	require.Equal(t, 1, built, "an enabled audit must build the entry exactly once")

	entry := recordedEntry(t, t.Name())
	require.Equal(t, consts.OP_CREATE, entry.OP, "the manager stamps the operation it was given")
	require.Equal(t, "audit_samples", entry.Table, "the manager stamps the model's own table")
	require.Equal(t, "operator", entry.User, "fields the build set must survive untouched")
}

// TestRecordOperationWritesOnceTheClientWentAway pins the context the write
// runs on: the operation the entry records has already happened, so a client
// that went away — or a write timeout that tripped — must not take the record
// with it.
func TestRecordOperationWritesOnceTheClientWentAway(t *testing.T) {
	prepareOperationLogTable(t)
	manager := auditmanager.New(&config.Audit{Enabled: true})

	clientGone, goAway := context.WithCancel(context.Background())
	goAway()

	require.NoError(t, manager.RecordOperation(clientGone, &auditSample{}, consts.OP_DELETE,
		func() *modellogmgmt.OperationLog {
			return &modellogmgmt.OperationLog{Model: t.Name(), User: "operator"}
		}))

	entry := recordedEntry(t, t.Name())
	require.Equal(t, consts.OP_DELETE, entry.OP, "the entry of an operation that happened is written anyway")
}

// TestRecordOperationRefusesNilBuild pins that a missing build is refused
// rather than skipped, and that the refusal does not wait for the audit to be
// enabled: an entry asked for and never written is a gap in a security record.
func TestRecordOperationRefusesNilBuild(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		manager := auditmanager.New(&config.Audit{Enabled: enabled})

		err := manager.RecordOperation(context.Background(), &auditSample{}, consts.OP_CREATE, nil)
		require.Error(t, err, "a nil build is refused whether the audit is on or off")
	}
}

// prepareOperationLogTable creates the table the entries are written into.
// The module that owns it is not registered here: these tests are about the
// manager, not about the module's routes.
func prepareOperationLogTable(t *testing.T) {
	t.Helper()

	require.NoError(t, database.DB().AutoMigrate(&modellogmgmt.OperationLog{}))
}

// recordedEntry reads back the single entry a test recorded, found by the
// model name the test stamped on it.
func recordedEntry(t *testing.T, model string) *modellogmgmt.OperationLog {
	t.Helper()

	entries := make([]*modellogmgmt.OperationLog, 0, 1)
	require.NoError(t, database.Database[*modellogmgmt.OperationLog](context.Background()).
		WithQuery(&modellogmgmt.OperationLog{Model: model}).List(&entries))
	require.Len(t, entries, 1, "the operation must have left exactly one entry")
	return entries[0]
}
