package database_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	gstmysql "github.com/hydroan/gst/database/mysql"
	gstpostgres "github.com/hydroan/gst/database/postgres"
	gstsqlite "github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newInstance builds a file-backed sqlite database the way an application
// would hold its own non-default instance, and prepares the TestUser table
// on it. File-backed, because a second in-memory sqlite would vanish
// between connections.
func newInstance(t *testing.T, name string) *gorm.DB {
	t.Helper()
	// A directory of its own per test, so instances never collide and nothing
	// is left behind on the machine running the tests.
	path := filepath.Join(t.TempDir(), name)
	ins, err := gstsqlite.New(config.Sqlite{Path: path, Enabled: true})
	require.NoError(t, err)
	require.NoError(t, ins.AutoMigrate(&TestUser{}))
	return ins
}

func TestDatabaseOn(t *testing.T) {
	ins := newInstance(t, "on_crud.db")
	defer cleanupTestData()

	user := &TestUser{Name: "named", Email: "named@example.com", Age: 30, ID: "named-1"}
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), ins).Create(user))

	// Visible on the application-held instance.
	got := new(TestUser)
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), ins).Get(got, "named-1"))
	require.Equal(t, "named", got.Name)

	// Invisible on the default database: the two instances are isolated.
	miss := new(TestUser)
	err := database.Database[*TestUser](context.Background()).Get(miss, "named-1")
	require.ErrorIs(t, err, database.ErrRecordNotFound)
}

func TestDatabaseOnNilInstancePanics(t *testing.T) {
	require.Panics(t, func() {
		_ = database.DatabaseOn[*TestUser](context.Background(), nil)
	})
}

// TestDatabaseOnParsesTheRegisteredModelsBeforeTheFirstChain pins that a
// handle has every registered model parsed in its schema cache before its
// first chain runs. gorm parses a model on its first use, and parsing one
// that holds another writes into the held model's cached schema, which every
// write of the held model reads; parsed up front, the holder's first read
// runs beside writes of the held model without writing into what they read.
// The race detector, which make test runs under, reports the read otherwise.
func TestDatabaseOnParsesTheRegisteredModelsBeforeTheFirstChain(t *testing.T) {
	ins := newInstance(t, "on_parsed.db")
	ctx := context.Background()

	// The instance's first chain writes the held model alone.
	first := make([]types.SQLStatement, 0)
	require.NoError(t, database.DatabaseOn[*TestHeldEntry](ctx, ins).WithDryRun(&first).Create(&TestHeldEntry{Name: "first"}))
	stmt := &gorm.Statement{DB: ins}
	require.NoError(t, stmt.Parse(&TestHeldEntry{}))
	holders := make([]string, 0)
	for _, rel := range stmt.Schema.Relationships.Relations {
		if rel.Schema != stmt.Schema {
			holders = append(holders, rel.Schema.Name)
		}
	}
	require.Equal(t, []string{"TestHolderRecord"}, holders, "the holder was parsed with the instance's first chain")

	stop := make(chan struct{})
	written := make(chan struct{})
	go func() {
		defer close(written)
		for {
			select {
			case <-stop:
				return
			default:
			}
			beside := make([]types.SQLStatement, 0)
			_ = database.DatabaseOn[*TestHeldEntry](ctx, ins).WithDryRun(&beside).Create(&TestHeldEntry{Name: "beside"})
		}
	}()
	read := make([]types.SQLStatement, 0)
	rows := make([]*TestHolderRecord, 0)
	require.NoError(t, database.DatabaseOn[*TestHolderRecord](ctx, ins).WithDryRun(&read).List(&rows))
	time.Sleep(50 * time.Millisecond)
	close(stop)
	<-written
}

func TestSelectOn(t *testing.T) {
	ins := newInstance(t, "on_aggregate.db")

	users := []*TestUser{
		{Name: "agg1", Email: "agg1@example.com", Age: 10, ID: "agg-1"},
		{Name: "agg2", Email: "agg2@example.com", Age: 20, ID: "agg-2"},
	}
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), ins).Create(users...))

	type row struct {
		Total int64 `json:"total"`
	}
	age := types.NewNumericColumn[*TestUser, int64]("age")
	var out row
	require.NoError(t, database.SelectOn[*TestUser, row](context.Background(), ins, age.Sum().As("total")).
		ScanOne(&out))
	require.EqualValues(t, 30, out.Total)
}

func TestTransactionOn(t *testing.T) {
	ins := newInstance(t, "on_tx.db")
	defer cleanupTestData()

	rollback := errors.New("force rollback")
	err := database.TransactionOn(context.Background(), ins, func(txCtx context.Context) error {
		u := &TestUser{Name: "txu", Email: "txu@example.com", Age: 1, ID: "tx-1"}
		if err := database.DatabaseOn[*TestUser](txCtx, ins).Create(u); err != nil {
			return err
		}
		// A default-database chain inside the closure must NOT join this
		// instance's transaction: it commits independently.
		d := &TestUser{Name: "default", Email: "default@example.com", Age: 2, ID: "tx-default-1"}
		if err := database.Database[*TestUser](txCtx).Create(d); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)

	// Rolled back on the application-held instance.
	miss := new(TestUser)
	require.ErrorIs(t, database.DatabaseOn[*TestUser](context.Background(), ins).Get(miss, "tx-1"), database.ErrRecordNotFound)

	// Committed on the default database, untouched by the rollback.
	kept := new(TestUser)
	require.NoError(t, database.Database[*TestUser](context.Background()).Get(kept, "tx-default-1"))
	require.Equal(t, "default", kept.Name)
}

func TestTransactionOnNilInstancePanics(t *testing.T) {
	require.Panics(t, func() {
		_ = database.TransactionOn(context.Background(), nil, func(ctx context.Context) error { return nil })
	})
}

// TestNewInstancesRunUnderTheConfiguredPoolLimits pins that an instance a
// dialect's New hands out runs under the same pool limits as the default
// handle: the [database] open-connection cap on the SQL dialect under test,
// and the single connection sqlite pins its pool to.
func TestNewInstancesRunUnderTheConfiguredPoolLimits(t *testing.T) {
	t.Run("sqlite pins the pool to one connection", func(t *testing.T) {
		pool, err := newInstance(t, "pool.db").DB()
		require.NoError(t, err)
		require.Equal(t, 1, pool.Stats().MaxOpenConnections)
	})

	t.Run("the SQL dialect carries the configured cap", func(t *testing.T) {
		var handle *gorm.DB
		var err error
		switch config.App.Database.Type {
		case config.DBMySQL:
			handle, err = gstmysql.New(config.App.MySQL)
		case config.DBPostgres:
			handle, err = gstpostgres.New(config.App.Postgres)
		default:
			t.Skipf("pool limits are asserted on mysql and postgres, the test database is %s", config.App.Database.Type)
		}
		require.NoError(t, err)
		pool, err := handle.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = pool.Close() })
		require.Equal(t, config.App.Database.MaxOpenConns, pool.Stats().MaxOpenConnections)
	})
}
