package database_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// statementErrorCaptureLogger records the error gorm hands the SQL log with
// each statement it traces.
type statementErrorCaptureLogger struct {
	gormlogger.Interface
	mu   sync.Mutex
	errs []error
}

func (l *statementErrorCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.mu.Lock()
	l.errs = append(l.errs, err)
	l.mu.Unlock()
	l.Interface.Trace(ctx, begin, fc, err)
}

// entryCountLogger stands in for the database stream's logger and counts the
// entries logged through it, at any level.
type entryCountLogger struct {
	gst.Logger
	entries *int
}

func (l entryCountLogger) WithContext(context.Context, apiaction.Phase) gst.Logger { return l }

func (l entryCountLogger) Debugz(string, ...zap.Field) { *l.entries++ }

func (l entryCountLogger) Infoz(string, ...zap.Field) { *l.entries++ }

func (l entryCountLogger) Warnz(string, ...zap.Field) { *l.entries++ }

func (l entryCountLogger) Errorz(string, ...zap.Field) { *l.entries++ }

// TestAWriteRefusedOverItsDataIsNoFailureOfTheDatabase pins, on every
// dialect, the three records of a write the data gets refused: the caller is
// answered the sentinel of what refused it, with what the database said of
// the refusal kept in the error, the key or the constraint it names among
// it; the SQL log is handed that error with the statement refused, one it
// logs as a refusal (see dberror.IsDataIntegrityViolation), and no failure
// beside it; and the database layer logs nothing, whoever answers the client
// reporting the refusal. The refusals are a key another record holds, a
// value too long for its column, a value the table's check refuses, a value
// a column requires left NULL, a foreign key no record satisfies, and a
// tenant the record cannot have. Sqlite declares no length for a column, so
// it refuses no value as too long.
func TestAWriteRefusedOverItsDataIsNoFailureOfTheDatabase(t *testing.T) {
	ctx := context.Background()
	sqlite := config.App.Database.Type == config.DBSqlite
	// said picks what the dialect under test says of one refusal.
	said := func(mysql, postgres, sqlite string) string {
		switch config.App.Database.Type {
		case config.DBPostgres:
			return postgres
		case config.DBSqlite:
			return sqlite
		default:
			return mysql
		}
	}

	// requireRefusal runs write on a handle whose SQL log is captured, with
	// the database stream counted meanwhile, and pins the three records; says
	// is what the database said of the refusal, empty for a refusal that is
	// not the database's.
	requireRefusal := func(t *testing.T, sentinel error, says string, write func(session *gorm.DB) error) {
		t.Helper()

		capture := &statementErrorCaptureLogger{Interface: database.DB().Logger}
		var entries int
		original := logger.Database
		logger.Database = entryCountLogger{Logger: original, entries: &entries}
		err := write(database.DB().Session(&gorm.Session{Logger: capture}))
		logger.Database = original

		require.ErrorIs(t, err, sentinel)
		require.ErrorContains(t, err, says, "the error keeps what the database said")
		var refusals int
		for _, traced := range capture.errs {
			if traced == nil {
				continue
			}
			require.ErrorIs(t, traced, sentinel, "the SQL log is handed another error than the caller")
			require.True(t, dberror.IsDataIntegrityViolation(traced), "the SQL log would log a failure: %v", traced)
			refusals++
		}
		require.Equal(t, 1, refusals, "the SQL log is handed the refusal with the one statement refused")
		require.Zero(t, entries, "the database layer logs nothing of a refusal")
	}

	t.Run("a key another record holds", func(t *testing.T) {
		first := &TestUniqueItem{UniqueCode: "refused-over-its-data", Name: "first"}
		require.NoError(t, database.Database[*TestUniqueItem](ctx).Create(first))
		t.Cleanup(func() { _ = database.Database[*TestUniqueItem](ctx).Delete(first) })

		requireRefusal(t, database.ErrDuplicatedKey, "unique_code", func(session *gorm.DB) error {
			return database.DatabaseOn[*TestUniqueItem](ctx, session).Create(&TestUniqueItem{UniqueCode: "refused-over-its-data", Name: "second"})
		})
	})

	t.Run("a value too long for its column", func(t *testing.T) {
		if sqlite {
			t.Skip("sqlite stores a value of any length")
		}
		requireRefusal(t, database.ErrValueTooLong, said("column 'code'", "character varying(4)", ""), func(session *gorm.DB) error {
			return database.DatabaseOn[*TestConstrainedRecord](ctx, session).Create(&TestConstrainedRecord{Code: "toolong", Level: 3})
		})
	})

	t.Run("a value the table's check refuses", func(t *testing.T) {
		requireRefusal(t, database.ErrCheckConstraintViolated, "chk_test_constrained_records_level", func(session *gorm.DB) error {
			return database.DatabaseOn[*TestConstrainedRecord](ctx, session).Create(&TestConstrainedRecord{Code: "ok", Level: 9})
		})
	})

	t.Run("a value a column requires", func(t *testing.T) {
		requireRefusal(t, database.ErrNotNullViolated, "label", func(session *gorm.DB) error {
			return database.DatabaseOn[*TestRequiredRecord](ctx, session).Create(&TestRequiredRecord{})
		})
	})

	t.Run("a foreign key no record satisfies", func(t *testing.T) {
		requireRefusal(t, database.ErrForeignKeyViolated, said("fk_test_constrained_entries_record", "fk_test_constrained_entries_record", "FOREIGN KEY constraint failed"), func(session *gorm.DB) error {
			return database.DatabaseOn[*TestConstrainedEntry](ctx, session).Create(&TestConstrainedEntry{RecordID: "no-such-record"})
		})
	})

	t.Run("a tenant the record cannot have", func(t *testing.T) {
		require.NoError(t, database.DB().AutoMigrate(&TestTenantSoftDeleteItem{}))
		t.Cleanup(func() { _ = database.DB().Exec("DELETE FROM test_tenant_soft_delete_items").Error })
		inA := tenant.In(ctx, "tenant-a")
		row := &TestTenantSoftDeleteItem{Name: "written-in-a"}
		require.NoError(t, database.Database[*TestTenantSoftDeleteItem](inA).Create(row))

		t.Run("another one than the caller acts in", func(t *testing.T) {
			moved := &TestTenantSoftDeleteItem{Name: "moved"}
			moved.ID = row.ID
			moved.TenantID = "tenant-b"
			requireRefusal(t, tenant.ErrImmutable, "", func(session *gorm.DB) error {
				return database.DatabaseOn[*TestTenantSoftDeleteItem](inA, session).Update(moved)
			})
		})

		t.Run("none from a caller acting in every tenant", func(t *testing.T) {
			requireRefusal(t, tenant.ErrRequired, "", func(session *gorm.DB) error {
				return database.DatabaseOn[*TestTenantSoftDeleteItem](tenant.Across(ctx), session).Create(&TestTenantSoftDeleteItem{Name: "no-tenant"})
			})
		})
	})
}

// TestACommitRefusedOverItsDataAnswersItsSentinel pins the translation of the
// error a transaction's commit fails with, which no statement carries: with
// the foreign key checked at commit the insert passes, and the commit that
// fails over it answers the sentinel a refused statement does, what the
// database said kept in the error. MySQL checks every constraint as the
// statement runs, so no commit of its own fails this way.
func TestACommitRefusedOverItsDataAnswersItsSentinel(t *testing.T) {
	var deferral, says string
	switch config.App.Database.Type {
	case config.DBPostgres:
		// The constraint is declared checked with its statement; it has to be
		// deferrable before a transaction can put it off.
		require.NoError(t, database.DB().Exec("ALTER TABLE test_constrained_entries ALTER CONSTRAINT fk_test_constrained_entries_record DEFERRABLE").Error)
		t.Cleanup(func() {
			_ = database.DB().Exec("ALTER TABLE test_constrained_entries ALTER CONSTRAINT fk_test_constrained_entries_record NOT DEFERRABLE").Error
		})
		deferral, says = "SET CONSTRAINTS ALL DEFERRED", "fk_test_constrained_entries_record"
	case config.DBSqlite:
		deferral, says = "PRAGMA defer_foreign_keys = ON", "FOREIGN KEY constraint failed"
	default:
		t.Skip("mysql checks every constraint as the statement runs")
	}

	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		tx, ok := dbruntime.TxFromContext(ctx, database.DB())
		require.True(t, ok)
		if err := tx.Exec(deferral).Error; err != nil {
			return err
		}
		return database.Database[*TestConstrainedEntry](ctx).Create(&TestConstrainedEntry{RecordID: "no-such-record"})
	})

	require.ErrorIs(t, err, database.ErrForeignKeyViolated)
	require.ErrorContains(t, err, says, "the error keeps what the database said")
}

// TestADuplicatedKeyTellsTheColumnsOfItsKey pins what a write colliding with
// a key answers beyond ErrDuplicatedKey: the columns of the key, whichever
// way the dialect's error tells them — sqlite and PostgreSQL by the columns
// themselves, MySQL by the name of the key, which the framework finds among
// the keys of the model — for the primary key, for a unique index a gorm tag
// declares and for one the model's Indexes declare.
func TestADuplicatedKeyTellsTheColumnsOfItsKey(t *testing.T) {
	ctx := context.Background()
	first := &TestMixedUniqueItem{Code: "duplicated-code", Ref: "duplicated-ref", Name: "first"}
	require.NoError(t, database.Database[*TestMixedUniqueItem](ctx).Create(first))
	t.Cleanup(func() { _ = database.Database[*TestMixedUniqueItem](ctx).Delete(first) })

	sameID := &TestMixedUniqueItem{Code: "another-code", Ref: "another-ref", Name: "second"}
	sameID.ID = first.ID
	for _, tt := range []struct {
		name    string
		second  *TestMixedUniqueItem
		columns []string
	}{
		{name: "the primary key", second: sameID, columns: []string{"id"}},
		{name: "a unique index a gorm tag declares", second: &TestMixedUniqueItem{Code: "duplicated-code", Ref: "a-third-ref", Name: "second"}, columns: []string{"code"}},
		{name: "a unique index the model declares", second: &TestMixedUniqueItem{Code: "a-third-code", Ref: "duplicated-ref", Name: "second"}, columns: []string{"ref"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := database.Database[*TestMixedUniqueItem](ctx).Create(tt.second)
			require.ErrorIs(t, err, database.ErrDuplicatedKey)
			var duplicated *dberror.DuplicatedKeyError
			require.ErrorAs(t, err, &duplicated)
			require.Equal(t, tt.columns, duplicated.Columns, "the key the database named: %q", duplicated.Constraint)
		})
	}
}
