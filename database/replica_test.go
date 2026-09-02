package database_test

// Read/write splitting contract tests.
//
// These pin the routing SEMANTICS the framework promises — which node serves
// which statement — as observable behavior, deliberately not the resolver's
// internals: a dbresolver upgrade that changes anything user-visible turns
// one of these red, while an internal refactor passes silently. The topology
// is two non-replicating MySQL servers: the shared test database acts as the
// primary, a standalone container stands in for the replica, and a row's
// presence on exactly one side is the routing proof — deterministic, with no
// actual replication to wait for.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	gstmysql "github.com/hydroan/gst/database/mysql"
	gstpostgres "github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// roleCaptureLogger records the serving-node role each traced statement's
// context carries, the same value the SQL logger writes as db_role.
type roleCaptureLogger struct {
	gormlogger.Interface
	mu    sync.Mutex
	roles []string
}

func (l *roleCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.mu.Lock()
	l.roles = append(l.roles, dbruntime.RoleFromContext(ctx))
	l.mu.Unlock()
	l.Interface.Trace(ctx, begin, fc, err)
}

func (l *roleCaptureLogger) last() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.roles) == 0 {
		return ""
	}
	return l.roles[len(l.roles)-1]
}

// routedRecord is the plain routing fixture: no read preference declared, so
// its reads follow the global default (primary) unless a call site says
// otherwise.
type routedRecord struct {
	Note string `json:"note" gorm:"size:191"`

	model.Base
}

func (*routedRecord) TableName() string { return "routed_records" }

// replicaFirstRecord declares the model-level read preference: its reads
// default to the replica, and a call site takes one back to the primary with
// WithReplica(false).
type replicaFirstRecord struct {
	Note string `json:"note" gorm:"size:191"`

	model.Base
}

func (*replicaFirstRecord) TableName() string { return "replica_first_records" }

func (*replicaFirstRecord) PreferReplica() bool { return true }

// replicaTopology is the two-server test bed: handle is the system under
// test (a replica-configured, Write-pinned connection), replica is a plain
// probe connection onto the stand-in replica for seeding and assertions, and
// the shared test database doubles as the primary probe via database.DB().
type replicaTopology struct {
	handle  *gorm.DB
	replica *gorm.DB
}

func setupReplicaTopology(t *testing.T) replicaTopology {
	t.Helper()

	// The routing semantics are dialect-independent, so the same nine
	// contracts run against whichever replica-capable dialect the test
	// database is; the CI dialect matrix covers both.
	var handle, replicaHandle *gorm.DB
	switch config.App.Database.Type {
	case config.DBMySQL:
		primaryCfg := config.App.MySQL
		standaloneCfg, terminate, err := testcontainer.SetupStandaloneMySQL(primaryCfg.Database, primaryCfg.Username, primaryCfg.Password)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, terminate()) })

		replicaHandle, err = gstmysql.New(standaloneCfg)
		require.NoError(t, err)

		cfg := primaryCfg
		cfg.Replicas = []string{fmt.Sprintf("%s:%d", standaloneCfg.Host, standaloneCfg.Port)}
		handle, err = gstmysql.New(cfg)
		require.NoError(t, err)
	case config.DBPostgres:
		primaryCfg := config.App.Postgres
		standaloneCfg, terminate, err := testcontainer.SetupStandalonePostgres(primaryCfg.Database, primaryCfg.Username, primaryCfg.Password)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, terminate()) })

		replicaHandle, err = gstpostgres.New(standaloneCfg)
		require.NoError(t, err)

		cfg := primaryCfg
		cfg.Replicas = []string{fmt.Sprintf("%s:%d", standaloneCfg.Host, standaloneCfg.Port)}
		handle, err = gstpostgres.New(cfg)
		require.NoError(t, err)
	default:
		t.Skipf("replica routing tests run on mysql and postgres, the test database is %s", config.App.Database.Type)
	}

	// Migration pinning: DDL and the information_schema probes of a
	// replica-configured handle must reach only the primary. Were HasTable
	// routed to the (empty, lagging) replica, a freshly migrated deployment
	// would fail its startup verification right after "gg migrate".
	require.NoError(t, handle.AutoMigrate(&routedRecord{}, &replicaFirstRecord{}))
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&routedRecord{}, &replicaFirstRecord{}))
	})
	require.False(t, replicaHandle.Migrator().HasTable("routed_records"),
		"AutoMigrate over the replica-configured handle must not reach the replica")

	// The stand-in replica gets the same tables through its own plain handle.
	require.NoError(t, replicaHandle.AutoMigrate(&routedRecord{}, &replicaFirstRecord{}))

	return replicaTopology{handle: handle, replica: replicaHandle}
}

// countOn reports how many rows of table carry id on one probe connection.
func countOn(t *testing.T, probe *gorm.DB, table, id string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, probe.Table(table).Where("id = ?", id).Count(&count).Error)
	return count
}

func TestReplicaRouting(t *testing.T) {
	top := setupReplicaTopology(t)
	ctx := context.Background()

	// Seed one row on each side of the plain fixture, and one on each side
	// of the preferring fixture, through the write path and the replica
	// probe respectively.
	primaryOnly := &routedRecord{Note: "primary-only"}
	primaryOnly.ID = "row-primary"
	require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.handle).Create(primaryOnly))

	replicaOnly := &routedRecord{Note: "replica-only"}
	replicaOnly.ID = "row-replica"
	require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.replica).Create(replicaOnly))

	t.Run("writes reach only the primary", func(t *testing.T) {
		require.EqualValues(t, 1, countOn(t, database.DB(), "routed_records", "row-primary"))
		require.EqualValues(t, 0, countOn(t, top.replica, "routed_records", "row-primary"))
	})

	t.Run("plain reads stay on the primary", func(t *testing.T) {
		rows := make([]*routedRecord, 0)
		require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.handle).List(&rows))
		require.Len(t, rows, 1)
		require.Equal(t, "row-primary", rows[0].ID, "a plain read must not see replica-only rows")
	})

	t.Run("WithReplica moves the read", func(t *testing.T) {
		rows := make([]*routedRecord, 0)
		require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.handle).WithReplica().List(&rows))
		require.Len(t, rows, 1)
		require.Equal(t, "row-replica", rows[0].ID, "a WithReplica read must see the replica's rows only")
	})

	t.Run("PreferReplica model defaults to the replica", func(t *testing.T) {
		onPrimary := &replicaFirstRecord{Note: "on-primary"}
		onPrimary.ID = "pref-primary"
		require.NoError(t, database.DatabaseOn[*replicaFirstRecord](ctx, top.handle).Create(onPrimary))
		onReplica := &replicaFirstRecord{Note: "on-replica"}
		onReplica.ID = "pref-replica"
		require.NoError(t, database.DatabaseOn[*replicaFirstRecord](ctx, top.replica).Create(onReplica))

		rows := make([]*replicaFirstRecord, 0)
		require.NoError(t, database.DatabaseOn[*replicaFirstRecord](ctx, top.handle).List(&rows))
		require.Len(t, rows, 1)
		require.Equal(t, "pref-replica", rows[0].ID, "the model's default read side is the replica")

		rows = rows[:0]
		require.NoError(t, database.DatabaseOn[*replicaFirstRecord](ctx, top.handle).WithReplica(false).List(&rows))
		require.Len(t, rows, 1)
		require.Equal(t, "pref-primary", rows[0].ID, "WithReplica(false) overrides the model default")
	})

	t.Run("transactions pin to the primary", func(t *testing.T) {
		require.NoError(t, database.TransactionOn(ctx, top.handle, func(txCtx context.Context) error {
			row := &routedRecord{Note: "tx-row"}
			row.ID = "row-tx"
			if err := database.DatabaseOn[*routedRecord](txCtx, top.handle).Create(row); err != nil {
				return err
			}
			// The uncommitted row is visible inside the transaction — and
			// stays visible under WithReplica, because a transaction outranks
			// every routing declaration.
			got := new(routedRecord)
			if err := database.DatabaseOn[*routedRecord](txCtx, top.handle).Get(got, "row-tx"); err != nil {
				return err
			}
			return database.DatabaseOn[*routedRecord](txCtx, top.handle).WithReplica().Get(got, "row-tx")
		}))
		txShell := new(routedRecord)
		txShell.ID = "row-tx"
		require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.handle).WithPurge().Delete(txShell))
	})

	t.Run("prepared statements keep the routes apart", func(t *testing.T) {
		// The same query shape alternates sides repeatedly, exercising each
		// pool's statement cache: a shared or misrouted prepared statement
		// would surface as a row from the wrong side.
		for range 3 {
			rows := make([]*routedRecord, 0)
			require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.handle).List(&rows))
			require.Len(t, rows, 1)
			require.Equal(t, "row-primary", rows[0].ID)

			rows = rows[:0]
			require.NoError(t, database.DatabaseOn[*routedRecord](ctx, top.handle).WithReplica().List(&rows))
			require.Len(t, rows, 1)
			require.Equal(t, "row-replica", rows[0].ID)
		}
	})

	t.Run("observability names the serving node", func(t *testing.T) {
		// Every pool is registered under the pinned handle with its role, and
		// the framework pool limits reach the replica pool too — an unlimited
		// replica pool would grow unbounded under read load.
		nodes := dbruntime.NodesFor(top.handle)
		require.Len(t, nodes, 2)
		require.ElementsMatch(t,
			[]string{dbruntime.RolePrimary, dbruntime.RoleReplica},
			[]string{nodes[0].Role, nodes[1].Role})
		for _, node := range nodes {
			require.Equal(t, config.App.Database.MaxOpenConns, node.DB.Stats().MaxOpenConnections,
				"pool limits must apply to every node")
		}

		// Each statement's context carries the role of the node that served
		// it, which the SQL logger surfaces as db_role.
		capture := &roleCaptureLogger{Interface: top.handle.Logger}
		session := top.handle.Session(&gorm.Session{Logger: capture})

		rows := make([]*routedRecord, 0)
		require.NoError(t, database.DatabaseOn[*routedRecord](ctx, session).List(&rows))
		require.Equal(t, dbruntime.RolePrimary, capture.last(), "a plain read logs as served by the primary")

		rows = rows[:0]
		require.NoError(t, database.DatabaseOn[*routedRecord](ctx, session).WithReplica().List(&rows))
		require.Equal(t, dbruntime.RoleReplica, capture.last(), "a WithReplica read logs as served by the replica")
	})

	t.Run("WithReplica on a write is refused", func(t *testing.T) {
		row := &routedRecord{Note: "never-written"}
		row.ID = "row-refused"
		require.ErrorIs(t, database.DatabaseOn[*routedRecord](ctx, top.handle).WithReplica().Create(row), database.ErrWithReplicaOnWrite)
		require.ErrorIs(t, database.DatabaseOn[*routedRecord](ctx, top.handle).WithReplica(false).Update(row), database.ErrWithReplicaOnWrite)
	})
}

func TestReplicaFallback(t *testing.T) {
	// The default test database has no replicas configured, which is exactly
	// the fallback contract: WithReplica and PreferReplica stay safe to use
	// and every read answers from the primary.
	ctx := context.Background()
	require.NoError(t, database.DB().AutoMigrate(&routedRecord{}, &replicaFirstRecord{}))
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&routedRecord{}, &replicaFirstRecord{}))
	})

	row := &routedRecord{Note: "fallback"}
	row.ID = "fallback-row"
	require.NoError(t, database.Database[*routedRecord](ctx).Create(row))
	rows := make([]*routedRecord, 0)
	require.NoError(t, database.Database[*routedRecord](ctx).WithReplica().List(&rows))
	require.Len(t, rows, 1, "WithReplica without replicas falls back to the primary")

	preferring := &replicaFirstRecord{Note: "fallback-prefer"}
	preferring.ID = "fallback-prefer"
	require.NoError(t, database.Database[*replicaFirstRecord](ctx).Create(preferring))
	got := new(replicaFirstRecord)
	require.NoError(t, database.Database[*replicaFirstRecord](ctx).Get(got, "fallback-prefer"))
	require.Equal(t, "fallback-prefer", got.ID, "a PreferReplica model reads from the primary when no replica exists")
}
