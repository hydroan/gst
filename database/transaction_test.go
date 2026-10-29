package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestDatabaseTransactionModelHook(t *testing.T) {
	t.Run("CreateAfter rollback", func(t *testing.T) {
		defer cleanupTestData()

		ctx := context.Background()
		config := &TestHookConfig{
			Value: "initial",
			ID:    "hook-config-create-after",
		}
		box := &TestHookBox{
			ConfigID: config.ID,
			Value:    "updated",
			ID:       "hook-box-create-after",
		}

		require.NoError(t, database.Database[*TestHookConfig](ctx).Create(config))

		err := database.Database[*TestHookBox](ctx).Create(box)
		require.ErrorIs(t, err, errTestHookBoxCreateAfter)

		storedBox := new(TestHookBox)
		err = database.Database[*TestHookBox](ctx).Get(storedBox, box.ID)
		require.ErrorIs(t, err, database.ErrRecordNotFound)

		storedConfig := new(TestHookConfig)
		require.NoError(t, database.Database[*TestHookConfig](ctx).Get(storedConfig, config.ID))
		require.Equal(t, "initial", storedConfig.Value)
	})

	t.Run("transaction context propagates to CreateAfter", func(t *testing.T) {
		defer cleanupTestData()

		ctx := context.Background()
		config := &TestHookConfig{
			Value: "initial",
			ID:    "hook-config-tx-ctx",
		}
		box := &TestHookBox{
			ConfigID: config.ID,
			Value:    "updated",
			ID:       "hook-box-tx-ctx",
		}

		require.NoError(t, database.Database[*TestHookConfig](ctx).Create(config))

		err := database.Transaction(ctx, func(ctx context.Context) error {
			return database.Database[*TestHookBox](ctx).Create(box)
		})
		require.ErrorIs(t, err, errTestHookBoxCreateAfter)

		storedBox := new(TestHookBox)
		err = database.Database[*TestHookBox](ctx).Get(storedBox, box.ID)
		require.ErrorIs(t, err, database.ErrRecordNotFound)

		storedConfig := new(TestHookConfig)
		require.NoError(t, database.Database[*TestHookConfig](ctx).Get(storedConfig, config.ID))
		require.Equal(t, "initial", storedConfig.Value)
	})

	t.Run("a hook's chain leaves out the preloads of the create around it", func(t *testing.T) {
		ctx := context.Background()
		configs := seedHookedSampleConfigs(t)

		// TestHookConfig has no Config association: a preload carried into the
		// hook would fail its read.
		hookedSampleSeen.Store(-1)
		require.NoError(t, database.Database[*TestHookedSample](ctx).WithExpand([]string{"Config"}).Create(&TestHookedSample{Value: "y"}))
		require.EqualValues(t, configs, hookedSampleSeen.Load(), "the hook reads every config")
	})
}

// seedHookedSampleConfigs migrates the TestHookedSample table, writes three
// TestHookConfig rows for its hooks to read, and returns how many config rows
// there are in all; the test drops the table and deletes its rows when done.
func seedHookedSampleConfigs(t *testing.T) int {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, database.DB().AutoMigrate(&TestHookedSample{}))
	ids := []string{"hooked-sample-config-a", "hooked-sample-config-b", "hooked-sample-config-c"}
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&TestHookedSample{}))
		require.NoError(t, database.DB().Exec("DELETE FROM test_hook_configs WHERE id IN ?", ids).Error)
	})
	for _, id := range ids {
		require.NoError(t, database.Database[*TestHookConfig](ctx).Create(&TestHookConfig{Value: id, ID: id}))
	}
	all := make([]*TestHookConfig, 0)
	require.NoError(t, database.Database[*TestHookConfig](ctx).List(&all))
	return len(all)
}

// TestDatabaseWithLock verifies row-level locking inside database.Transaction
// across lock modes and read operations.
func TestDatabaseWithLock(t *testing.T) {
	defer cleanupTestData()

	t.Run("with LockUpdate", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// Get and lock user with FOR UPDATE
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock(consts.LockUpdate).Get(user, u1.ID))
			require.Equal(t, u1.ID, user.ID)
			require.Equal(t, u1.Name, user.Name)

			// Update the locked user
			user.Name = "locked_update"
			return database.Database[*TestUser](ctx).Update(user)
		})
		require.NoError(t, err)

		// Verify update was successful
		user := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(user, u1.ID))
		require.Equal(t, "locked_update", user.Name)
	})

	t.Run("with default lock mode", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// Get user with default lock (FOR UPDATE)
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock().Get(user, u1.ID))
			require.Equal(t, u1.ID, user.ID)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("with LockUpdateNoWait", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// Get and lock user with FOR UPDATE NOWAIT
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock(consts.LockUpdateNoWait).Get(user, u1.ID))
			require.Equal(t, u1.ID, user.ID)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("with LockShare", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// Get user with FOR SHARE lock
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock(consts.LockShare).Get(user, u1.ID))
			require.Equal(t, u1.ID, user.ID)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("with LockUpdateSkipLocked", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// Get and lock user with FOR UPDATE SKIP LOCKED
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock(consts.LockUpdateSkipLocked).Get(user, u1.ID))
			require.Equal(t, u1.ID, user.ID)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("with LockShareSkipLocked", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// Get user with FOR SHARE SKIP LOCKED
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock(consts.LockShareSkipLocked).Get(user, u1.ID))
			require.Equal(t, u1.ID, user.ID)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("with List", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			// List users with lock
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](ctx).WithLock(consts.LockUpdate).List(&users))
			require.Len(t, users, 3)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("outside transaction is refused", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// The lock would be released as the statement finished, handing back
		// rows nothing holds. That is decidable before running the query, so it
		// is refused instead of served with a warning.
		users := make([]*TestUser, 0)
		err := database.Database[*TestUser](context.Background()).
			WithLock(consts.LockUpdate).List(&users)
		require.ErrorIs(t, err, database.ErrLockOutsideTransaction)
		require.Empty(t, users, "a refused lock must not return rows")
	})
}

// TestTransactionWithOTELEnabled verifies that enabling real OTel tracing does
// not change commit/rollback behavior, and that the closure context carries the
// transaction span so inner operation spans nest under it.
func TestTransactionWithOTELEnabled(t *testing.T) {
	oteltest.Enable(t)
	defer cleanupTestData()

	t.Run("commit", func(t *testing.T) {
		defer cleanupTestData()
		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			require.True(t, trace.SpanContextFromContext(ctx).IsValid(),
				"closure ctx must carry the transaction span so inner spans nest under it")
			return database.Database[*TestUser](ctx).Create(ul...)
		})
		require.NoError(t, err, "transaction should succeed")
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
		require.Len(t, users, 3, "should have 3 records after successful transaction")
	})

	t.Run("rollback", func(t *testing.T) {
		defer cleanupTestData()
		errTest := errors.New("test error")
		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
			return errTest
		})
		require.ErrorIs(t, err, errTest)
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
		require.Empty(t, users, "should have 0 records after rollback")
	})
}

// TestTransactionFailureIsRecordedOnSpan proves the transaction span carries
// the failure of a rolled-back transaction: error status, the error text as
// its description and the exception event. The span is where this package
// records the failure; logging the error belongs to the boundary that owns
// it, such as the controller fallback for a request or the cronjob runner for
// a job.
func TestTransactionFailureIsRecordedOnSpan(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	defer cleanupTestData()

	errTest := errors.New("sample failure")
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
		return errTest
	})
	require.ErrorIs(t, err, errTest)

	span := oteltest.EndedNamed(t, recorder, gstotel.OperationSpanName("database", "Transaction"))
	require.Equal(t, codes.Error, span.Status().Code)
	require.Equal(t, errTest.Error(), span.Status().Description)
	exception := false
	for _, event := range span.Events() {
		exception = exception || event.Name == "exception"
	}
	require.True(t, exception, "the transaction span must carry the exception event")
}

// TestTransaction covers the package-level context-injecting transaction entry:
// nil fn, commit, multi-model rollback, panic rollback, a context that ends
// before the commit, and joining an outer transaction without opening a new
// one. A nil context is covered with the other entry points by
// TestNilContextIsRejected.
func TestTransaction(t *testing.T) {
	defer cleanupTestData()

	// nil fn is rejected.
	err := database.Transaction(context.Background(), nil)
	require.ErrorIs(t, err, database.ErrNilTransaction)

	// Commit: chains started from the closure ctx join the transaction
	// automatically.
	err = database.Transaction(context.Background(), func(ctx context.Context) error {
		return database.Database[*TestUser](ctx).Create(ul...)
	})
	require.NoError(t, err, "transaction should succeed")
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 3, "should have 3 records after successful transaction")
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))

	// Rollback: writes to different models roll back together.
	errTest := errors.New("test error")
	item := &TestItem{Name: "sample", Score: 1}
	err = database.Transaction(context.Background(), func(ctx context.Context) error {
		require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
		require.NoError(t, database.Database[*TestItem](ctx).Create(item))
		return errTest
	})
	require.ErrorIs(t, err, errTest)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "user writes should roll back")
	items := make([]*TestItem, 0)
	require.NoError(t, database.Database[*TestItem](context.Background()).List(&items))
	require.Empty(t, items, "item writes should roll back in the same transaction")

	// Panic: the underlying GORM transaction rolls back before the panic
	// propagates.
	require.PanicsWithValue(t, "transaction panic", func() {
		_ = database.Transaction(context.Background(), func(ctx context.Context) error {
			require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
			panic("transaction panic")
		})
	})
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "should have 0 records after panic rollback")

	// Context ended before the commit: the standard library rolls the
	// transaction back as the context ends and then refuses the commit as
	// done, yet the caller — work told to stop — gets its context's ending.
	canceled, cancel := context.WithCancel(context.Background())
	err = database.Transaction(canceled, func(ctx context.Context) error {
		require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
		cancel()
		// Time for the standard library to see the context end and roll back
		// before the commit comes.
		time.Sleep(200 * time.Millisecond)
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "should have 0 records once the ended context rolled the transaction back")

	// Join: a nested Transaction call joins the outer transaction instead of
	// opening a new one, so the outer rollback also reverts the inner write.
	err = database.Transaction(context.Background(), func(ctx context.Context) error {
		if innerErr := database.Transaction(ctx, func(ctx context.Context) error {
			return database.Database[*TestUser](ctx).Create(ul...)
		}); innerErr != nil {
			return innerErr
		}
		return errTest
	})
	require.ErrorIs(t, err, errTest)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "inner write must roll back with the outer transaction")
}
