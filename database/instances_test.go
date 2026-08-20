package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/types"
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
	ins, err := sqlite.New(config.Sqlite{Path: path, Enabled: true})
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

func TestAggregateOn(t *testing.T) {
	ins := newInstance(t, "on_aggregate.db")

	users := []*TestUser{
		{Name: "agg1", Email: "agg1@example.com", Age: 10, ID: "agg-1"},
		{Name: "agg2", Email: "agg2@example.com", Age: 20, ID: "agg-2"},
	}
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), ins).Create(users...))

	type row struct {
		Total int64 `json:"total"`
	}
	age := types.NewNumericColumn[int64]("age")
	var out row
	require.NoError(t, database.AggregateOn[*TestUser, row](context.Background(), ins).
		Select(age.Sum().As("total")).
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
