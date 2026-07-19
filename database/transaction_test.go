package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/model"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestDatabaseTransactionModelHook(t *testing.T) {
	t.Run("CreateAfter rollback", func(t *testing.T) {
		defer cleanupTestData()

		ctx := context.Background()
		config := &TestHookConfig{
			Value: "initial",
			Base:  model.Base{ID: "hook-config-create-after"},
		}
		group := &TestHookGroup{
			ConfigID: config.ID,
			Value:    "updated",
			Base:     model.Base{ID: "hook-group-create-after"},
		}

		require.NoError(t, database.Database[*TestHookConfig](ctx).Create(config))

		err := database.Database[*TestHookGroup](ctx).Create(group)
		require.ErrorIs(t, err, errTestHookGroupCreateAfter)

		storedGroup := new(TestHookGroup)
		err = database.Database[*TestHookGroup](ctx).Get(storedGroup, group.ID)
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
			Base:  model.Base{ID: "hook-config-tx-ctx"},
		}
		group := &TestHookGroup{
			ConfigID: config.ID,
			Value:    "updated",
			Base:     model.Base{ID: "hook-group-tx-ctx"},
		}

		require.NoError(t, database.Database[*TestHookConfig](ctx).Create(config))

		err := database.Transaction(ctx, func(ctx context.Context) error {
			return database.Database[*TestHookGroup](ctx).Create(group)
		})
		require.ErrorIs(t, err, errTestHookGroupCreateAfter)

		storedGroup := new(TestHookGroup)
		err = database.Database[*TestHookGroup](ctx).Get(storedGroup, group.ID)
		require.ErrorIs(t, err, database.ErrRecordNotFound)

		storedConfig := new(TestHookConfig)
		require.NoError(t, database.Database[*TestHookConfig](ctx).Get(storedConfig, config.ID))
		require.Equal(t, "initial", storedConfig.Value)
	})
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

	t.Run("outside transaction still queries and only warns", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithLock(consts.LockUpdate).List(&users))
		require.Len(t, users, 3, "lock outside a transaction must not change query results")
	})
}

// TestTransactionWithOTELEnabled verifies that enabling real OTel tracing does
// not change commit/rollback behavior, and that the closure context carries the
// transaction span so inner operation spans nest under it.
func TestTransactionWithOTELEnabled(t *testing.T) {
	setupOTELTestForTransaction(t)
	defer cleanupTestData()

	t.Run("commit", func(t *testing.T) {
		defer cleanupTestData()
		err := database.Transaction(context.Background(), func(ctx context.Context) error {
			require.True(t, oteltrace.SpanContextFromContext(ctx).IsValid(),
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

// TestTransaction covers the package-level context-injecting transaction entry:
// nil fn, commit, multi-model rollback, panic rollback, and joining an outer
// transaction without opening a new one.
func TestTransaction(t *testing.T) {
	defer cleanupTestData()

	// nil fn is rejected.
	err := database.Transaction(context.Background(), nil)
	require.ErrorIs(t, err, database.ErrNilTransaction)

	// nil ctx falls back to context.Background and still runs the transaction.
	err = database.Transaction(nil, func(ctx context.Context) error { //nolint:staticcheck
		users := make([]*TestUser, 0)
		return database.Database[*TestUser](ctx).List(&users)
	})
	require.NoError(t, err, "nil ctx should fall back to context.Background")

	// Commit: chains started from the closure ctx join the transaction automatically.
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
	product := &TestProduct{Name: "sample", Price: 1}
	err = database.Transaction(context.Background(), func(ctx context.Context) error {
		require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
		require.NoError(t, database.Database[*TestProduct](ctx).Create(product))
		return errTest
	})
	require.ErrorIs(t, err, errTest)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "user writes should roll back")
	products := make([]*TestProduct, 0)
	require.NoError(t, database.Database[*TestProduct](context.Background()).List(&products))
	require.Empty(t, products, "product writes should roll back in the same transaction")

	// Panic: the underlying GORM transaction rolls back before the panic propagates.
	require.PanicsWithValue(t, "transaction panic", func() {
		_ = database.Transaction(context.Background(), func(ctx context.Context) error {
			require.NoError(t, database.Database[*TestUser](ctx).Create(ul...))
			panic("transaction panic")
		})
	})
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Empty(t, users, "should have 0 records after panic rollback")

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

// setupOTELTestForTransaction enables real OTel tracing for one test, mirroring
// middleware/tracing_test.go's setupTracingTestWithEndpointAndSampler. Unlike that helper,
// this only swaps config.App.OTEL rather than all of config.App, because config.App.Database
// (set up once by this package's init()) must stay intact for the transaction itself to work.
func setupOTELTestForTransaction(t *testing.T) {
	t.Helper()

	originalOTEL := config.App.OTEL
	config.App.OTEL.Enabled = true
	config.App.OTEL.ServiceName = "gst-test"
	config.App.OTEL.ExporterOTLPProtocol = config.OTLPProtocolHTTPProtobuf
	config.App.OTEL.ExporterOTLPTracesEndpoint = "http://127.0.0.1:1/v1/traces"
	config.App.OTEL.ExporterOTLPCompression = config.OTLPCompressionNone
	config.App.OTEL.TracesSampler = config.TracesSamplerParentBasedAlwaysOn
	config.App.OTEL.BSPMaxQueueSize = 100
	config.App.OTEL.BSPMaxExportBatchSize = 100
	config.App.OTEL.BSPScheduleDelay = 10 * time.Millisecond
	config.App.OTEL.BSPExportTimeout = time.Second
	t.Cleanup(func() {
		config.App.OTEL = originalOTEL
	})

	originalOTELLogger := logger.OTEL
	logger.OTEL = pkgzap.New("/dev/null")
	t.Cleanup(func() {
		logger.OTEL = originalOTELLogger
	})

	gstotel.Close()
	require.NoError(t, gstotel.Init())
	t.Cleanup(func() {
		gstotel.Close()
	})
}
