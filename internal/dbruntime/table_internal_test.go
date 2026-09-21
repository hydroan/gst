package dbruntime

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
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
	withFastStartupLock(t)

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
// created. automigrating is what keeps gorm's hands off them.
// TestMigrateTableCreatesMySQLTablesLikeTheMigrationDoes pins the table the
// startup path creates against the one gg migrate creates. The collation is
// what would diverge: a server default of utf8mb4_0900_ai_ci compares
// strings case-insensitively, so a unique key created at startup would
// refuse a row the migrated schema accepts, and a lookup would find rows the
// other schema does not — the same code behaving differently by where its
// tables came from.
func TestMigrateTableCreatesMySQLTablesLikeTheMigrationDoes(t *testing.T) {
	withAutoMigrate(t, true)
	withFastStartupLock(t)

	handle := newMySQLDB(t)
	require.NoError(t, handle.Migrator().DropTable(&raceRecord{}))
	require.NoError(t, ensureTable(handle, &raceRecord{}))

	var collation string
	require.NoError(t, handle.Raw(
		"SELECT table_collation FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		"race_records").Scan(&collation).Error)
	require.Equal(t, "utf8mb4_bin", collation, "a table created at startup carries the collation gg migrate writes")
}

func TestMigrateTableLeavesTheIndexesAlone(t *testing.T) {
	withAutoMigrate(t, true)
	withFastStartupLock(t)

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
				keyword, _, _ := strings.Cut(strings.ToUpper(strings.TrimSpace(statement)), " ")
				require.NotContainsf(t, []string{"ALTER", "CREATE", "DROP"}, keyword,
					"a start against a prepared table must leave it as it is, ran: %s", statement)
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

	returned := make(chan error, 1)
	go func() { returned <- Wait() }()

	select {
	case <-returned:
		t.Fatal("Wait returned while a table was still pending")
	case <-time.After(50 * time.Millisecond):
	}

	modelregistry.TableDone()

	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the last table finished")
	}
}

// TestWaitReportsATableThePreparationCouldNotGet proves where a table the
// deployment could not get reaches the caller. Preparation runs on a
// goroutine of its own, so failing there cannot end the start where it
// stands — a panic on that goroutine would take the process down with its
// buffered log lines unwritten and the entries already on disk reading like
// a start that went fine. Wait hands the failure back instead, in time for
// the start to end through the exit every other startup failure leaves
// through.
func TestWaitReportsATableThePreparationCouldNotGet(t *testing.T) {
	tablePreparationStarted.Store(1)
	t.Cleanup(func() { tablePreparationStarted.Store(0) })
	forgetPreparationFailure(t)

	db := newSQLiteDB(t)
	withAutoMigrate(t, true)

	// Queued and taken off the way the processing goroutine does, then
	// prepared here: the model declares no table name, so it is one no
	// database could have given it.
	modelregistry.RegisterTable[*undeclaredRecord]()
	<-modelregistry.TableChan
	prepareTable(db, &undeclaredRecord{})

	err := Wait()
	require.ErrorContains(t, err, "failed to prepare table")
	require.ErrorContains(t, err, "must declare an explicit table name")
}

// forgetPreparationFailure clears the failure the preparation recorded, on
// the way in and out: it is process-wide, and one test's failure is not the
// next test's.
func forgetPreparationFailure(t *testing.T) {
	t.Helper()
	forget := func() {
		prepareFailure.mu.Lock()
		defer prepareFailure.mu.Unlock()
		prepareFailure.err = nil
	}
	forget()
	t.Cleanup(forget)
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
