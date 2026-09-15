package dbruntime

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// plainRecord is a minimal model for table preparation tests.
type plainRecord struct {
	Name string

	modelregistry.Base
}

func (*plainRecord) TableName() string { return "plain_records" }

func TestEnsureTableCreatesTableWhenAutoMigrateEnabled(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)

	require.NoError(t, ensureTable(db, &plainRecord{}))
	require.True(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTableFailsFastWhenDisabledAndTableMissing(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, false)
	withSqlite(t, false)

	err := ensureTable(db, &plainRecord{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "gg migrate")
	require.False(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTableMigratesInMemorySqliteWhenDisabled(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, false)
	withSqlite(t, true)

	require.NoError(t, ensureTable(db, &plainRecord{}))
	require.True(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTablePassesWhenDisabledAndTableExists(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)
	require.NoError(t, ensureTable(db, &plainRecord{}))

	withAutoMigrate(t, false)
	require.NoError(t, ensureTable(db, &plainRecord{}))
}

// undeclaredRecord omits an explicit table name; table preparation must
// reject it instead of falling back to gorm's naming strategy.
type undeclaredRecord struct {
	Name string

	modelregistry.Base
}

func TestEnsureTableRejectsUndeclaredTableName(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)

	err := ensureTable(db, &undeclaredRecord{})
	require.ErrorContains(t, err, "must declare an explicit table name")
	require.False(t, db.Migrator().HasTable("undeclared_records"))
}

// raceRecord is the model several processes prepare at once; its index makes
// the custom index step part of the race. The indexed column carries a size:
// without one gorm maps it to TEXT on MySQL, which cannot be indexed whole.
type raceRecord struct {
	Name string `gorm:"size:191"`

	modelregistry.Base
}

func (*raceRecord) TableName() string { return "race_records" }

func (*raceRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Name"}}}
}

// TestMigrateTableCreatesOnceAcrossProcesses proves replicas starting
// together prepare a table without tripping over each other: every one of
// several handles — each a pool of its own, the way separate processes look
// to the server — migrates the same fresh table at once, and all of them
// succeed, on the dialects with an advisory lock and on the one without.
func TestMigrateTableCreatesOnceAcrossProcesses(t *testing.T) {
	withAutoMigrate(t, true)

	sqliteFile := filepath.Join(t.TempDir(), "race.db")
	for _, dialect := range []struct {
		name config.DBType
		open func(t *testing.T) *gorm.DB
	}{
		{name: config.DBMySQL, open: newMySQLDB},
		{name: config.DBPostgres, open: newPostgresDB},
		{name: config.DBSqlite, open: func(t *testing.T) *gorm.DB {
			t.Helper()
			return openSQLiteDB(t, sqliteFile)
		}},
	} {
		t.Run(string(dialect.name), func(t *testing.T) {
			const processes = 4
			handles := make([]*gorm.DB, 0, processes)
			for range processes {
				handles = append(handles, dialect.open(t))
			}
			require.NoError(t, handles[0].Migrator().DropTable(&raceRecord{}))

			errs := make([]error, len(handles))
			var wg sync.WaitGroup
			for i, handle := range handles {
				wg.Go(func() { errs[i] = ensureTable(handle, &raceRecord{}) })
			}
			wg.Wait()

			for i, err := range errs {
				require.NoErrorf(t, err, "process %d must prepare the table beside the others", i)
			}
			require.True(t, handles[0].Migrator().HasTable("race_records"))
			require.True(t, handles[0].Migrator().HasIndex(&raceRecord{}, "idx_race_records_name"))
		})
	}
}

// uniqueRecord is a model whose single-column unique index is declared the
// framework's way, through Indexes() with no tag on the column: gorm's own
// migration knows nothing of the index.
type uniqueRecord struct {
	Code string `gorm:"size:191"`

	modelregistry.Base
}

func (*uniqueRecord) TableName() string { return "unique_records" }

func (*uniqueRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code"}, Unique: true}}
}

// TestMigrateTableLeavesTheIndexesAlone proves a process starting against a
// table another process prepared leaves the framework's indexes as they
// are. gorm migrates the unique constraints its tags declare, and its MySQL
// driver takes any other single-column unique index for a leftover and
// drops it: an index declared through Indexes() was dropped and created
// again on every start, and a start must not drop what the framework
// created.
func TestMigrateTableLeavesTheIndexesAlone(t *testing.T) {
	withAutoMigrate(t, true)

	sqliteFile := filepath.Join(t.TempDir(), "unique.db")
	for _, dialect := range []struct {
		name config.DBType
		open func(t *testing.T) *gorm.DB
	}{
		{name: config.DBMySQL, open: newMySQLDB},
		{name: config.DBPostgres, open: newPostgresDB},
		{name: config.DBSqlite, open: func(t *testing.T) *gorm.DB {
			t.Helper()
			return openSQLiteDB(t, sqliteFile)
		}},
	} {
		t.Run(string(dialect.name), func(t *testing.T) {
			first, second := dialect.open(t), dialect.open(t)
			require.NoError(t, first.Migrator().DropTable(&uniqueRecord{}))
			require.NoError(t, ensureTable(first, &uniqueRecord{}))

			statements := recordStatements(second)
			require.NoError(t, ensureTable(second, &uniqueRecord{}))
			for _, statement := range statements.all() {
				upper := strings.ToUpper(statement)
				require.Falsef(t, strings.Contains(upper, "INDEX") && (strings.Contains(upper, "DROP") || strings.Contains(upper, "CREATE")),
					"a start against a prepared table must leave its indexes alone, ran: %s", statement)
			}
			plans, err := modelregistry.ParseIndexPlans(second, &uniqueRecord{})
			require.NoError(t, err)
			require.Len(t, plans, 1)
			matches, err := indexMatchesPlan(second, "unique_records", plans[0])
			require.NoError(t, err)
			require.True(t, matches, "the unique index stays as declared")
		})
	}
}

// statementLog is a gorm logger that keeps every statement the handle runs.
type statementLog struct {
	logger.Interface
	mu         sync.Mutex
	statements []string
}

// recordStatements makes handle log its statements to a statementLog and
// returns it.
func recordStatements(handle *gorm.DB) *statementLog {
	log := &statementLog{Interface: logger.Discard}
	handle.Logger = log
	return log
}

func (l *statementLog) LogMode(logger.LogLevel) logger.Interface { return l }

func (l *statementLog) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	statement, _ := fc()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statements = append(l.statements, statement)
}

func (l *statementLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.statements)
}

// TestStartupLockNameIsOnePerPurposeAndDatabase pins the lock's scope:
// deployments on two databases of one MySQL server hold two locks, the
// table preparation and the seeding of one deployment hold two, and every
// name fits MySQL's 64-character limit whatever the database is called.
func TestStartupLockNameIsOnePerPurposeAndDatabase(t *testing.T) {
	require.NotEqual(t, startupLockName("migrate", "app"), startupLockName("migrate", "app_staging"))
	require.NotEqual(t, startupLockName("migrate", "app"), startupLockName("seed", "app"))
	require.True(t, strings.HasPrefix(startupLockName("migrate", "app"), "gst:migrate:"))
	require.LessOrEqual(t, len(startupLockName("migrate", strings.Repeat("d", 64))), 64)
	require.NotEqual(t, startupLockKey(startupLockName("migrate", "app")), startupLockKey(startupLockName("migrate", "app_staging")))
}

// TestSerializedRunsOneProcessAtATime proves the startup lock serializes a
// step across the processes of a deployment: several handles — each a pool
// of its own, the way separate processes look to the server — run the step
// at once, and never two of them inside it together, on the dialects that
// offer the lock.
func TestSerializedRunsOneProcessAtATime(t *testing.T) {
	for _, dialect := range []struct {
		name config.DBType
		open func(t *testing.T) *gorm.DB
	}{
		{name: config.DBMySQL, open: newMySQLDB},
		{name: config.DBPostgres, open: newPostgresDB},
	} {
		t.Run(string(dialect.name), func(t *testing.T) {
			const processes = 4
			handles := make([]*gorm.DB, 0, processes)
			for range processes {
				handles = append(handles, dialect.open(t))
			}

			var inside, overlaps atomic.Int32
			errs := make([]error, len(handles))
			var wg sync.WaitGroup
			for i, handle := range handles {
				wg.Go(func() {
					errs[i] = serialized(context.Background(), handle, "sample", func() error {
						if inside.Add(1) > 1 {
							overlaps.Add(1)
						}
						time.Sleep(50 * time.Millisecond)
						inside.Add(-1)
						return nil
					})
				})
			}
			wg.Wait()

			for i, err := range errs {
				require.NoErrorf(t, err, "process %d must take its turn", i)
			}
			require.Zero(t, overlaps.Load(), "no two processes may be inside the step at once")
		})
	}
}

// TestStartupLockWaitsForTheHolderAndSaysSo proves a process waits for the
// holder of a startup lock however long it takes, says so while it waits,
// and stops waiting when told to: with the lock held by another handle the
// step does not start, a warning is logged once the report interval passes,
// a waiter whose context ends returns with that ending instead of the
// lock, and the step runs as soon as the holder lets go.
func TestStartupLockWaitsForTheHolderAndSaysSo(t *testing.T) {
	for _, dialect := range []struct {
		name config.DBType
		open func(t *testing.T) *gorm.DB
	}{
		{name: config.DBMySQL, open: newMySQLDB},
		{name: config.DBPostgres, open: newPostgresDB},
	} {
		t.Run(string(dialect.name), func(t *testing.T) {
			logs := withObservedGlobalLogger(t)
			withFastStartupLock(t)

			holder, waiter, quitter := dialect.open(t), dialect.open(t), dialect.open(t)
			unlock, err := lockStartup(context.Background(), holder, "sample")
			require.NoError(t, err)
			unlocked := false
			t.Cleanup(func() {
				if !unlocked {
					unlock()
				}
			})

			entered := make(chan struct{})
			returned := make(chan error, 1)
			go func() {
				returned <- serialized(context.Background(), waiter, "sample", func() error {
					close(entered)
					return nil
				})
			}()

			select {
			case <-entered:
				t.Fatal("the step must not start while another process holds the lock")
			case <-time.After(200 * time.Millisecond):
			}
			require.NotEmpty(t, logs.FilterMessage("still waiting for the startup lock held by another process").All(),
				"a process waiting past the report interval must say so")

			quitCtx, quit := context.WithCancelCause(context.Background())
			quitReturned := make(chan error, 1)
			go func() {
				quitReturned <- serialized(quitCtx, quitter, "sample", func() error {
					t.Error("a waiter told to stop must not run the step")
					return nil
				})
			}()
			// Told to stop once it is waiting, not before it starts to.
			<-time.After(2 * startupLockWaitReport)
			quit(errors.New("sample stop"))
			select {
			case err := <-quitReturned:
				require.ErrorContains(t, err, "sample stop", "the waiter reports why its wait ended")
			case <-time.After(5 * time.Second):
				t.Fatal("a waiter told to stop must return")
			}

			unlock()
			unlocked = true
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("the step must start once the holder lets go")
			}
			require.NoError(t, <-returned)
		})
	}
}

// withFastStartupLock shortens the lock's poll and report intervals for the
// test and restores them afterwards.
func withFastStartupLock(t *testing.T) {
	t.Helper()

	poll, report := startupLockPoll, startupLockWaitReport
	startupLockPoll, startupLockWaitReport = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { startupLockPoll, startupLockWaitReport = poll, report })
}

// withObservedGlobalLogger routes the global logger into an observer for the
// test and restores the previous one afterwards.
func withObservedGlobalLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()

	core, logs := observer.New(zapcore.WarnLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(restore)
	return logs
}

// withAutoMigrate overrides the auto-migrate option and restores it on cleanup.
func withAutoMigrate(t *testing.T, enabled bool) {
	t.Helper()
	old := config.App.Database.AutoMigrate
	config.App.Database.AutoMigrate = enabled
	t.Cleanup(func() { config.App.Database.AutoMigrate = old })
}

// withSqlite selects sqlite as the database type and marks whether it is the
// in-memory variant, restoring both options on cleanup.
func withSqlite(t *testing.T, inMemory bool) {
	t.Helper()
	oldType, oldIsMemory := config.App.Database.Type, config.App.Sqlite.IsMemory
	config.App.Database.Type, config.App.Sqlite.IsMemory = config.DBSqlite, inMemory
	t.Cleanup(func() {
		config.App.Database.Type, config.App.Sqlite.IsMemory = oldType, oldIsMemory
	})
}

func TestWaitReturnsOnceTheQueueDrains(t *testing.T) {
	// Wait reports back immediately unless InitDatabase started the processing
	// goroutine; this stands in for that start without a database.
	tablePreparationStarted.Store(1)
	t.Cleanup(func() { tablePreparationStarted.Store(0) })

	// Queue a model and take it off the way the processing goroutine does, so
	// it counts as pending until its TableDone lands.
	modelregistry.RegisterTable[*plainRecord]()
	<-modelregistry.TableChan
	require.Equal(t, 1, modelregistry.TablesPending())

	returned := make(chan struct{})
	go func() {
		Wait()
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("Wait returned while a table was still pending")
	case <-time.After(50 * time.Millisecond):
	}

	modelregistry.TableDone()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the last table finished")
	}
}
