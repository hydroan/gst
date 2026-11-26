package dbruntime_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestInstallErrorTranslationRewritesTheErrorOfEveryOperation pins that an
// error translate recognizes comes back as the translation from each of the
// six operations gorm runs through its callbacks, create, query, update,
// delete, row and raw, and that an error it does not recognize comes back
// as it is.
func TestInstallErrorTranslationRewritesTheErrorOfEveryOperation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	missing := errors.New("the table is missing")
	require.NoError(t, dbruntime.InstallErrorTranslation(db, func(err error) error {
		if strings.Contains(err.Error(), "no such table") {
			return errors.Wrap(missing, err.Error())
		}
		return nil
	}))

	// No table is ever created for sample, so every statement on it fails
	// with the error translate recognizes.
	type sample struct{ ID int }
	for name, run := range map[string]func() error{
		"create": func() error { return db.Create(&sample{ID: 1}).Error },
		"query":  func() error { return db.First(&sample{}).Error },
		"update": func() error { return db.Model(&sample{ID: 1}).Update("id", 2).Error },
		"delete": func() error { return db.Delete(&sample{ID: 1}).Error },
		"row": func() error {
			rows, err := db.Raw("SELECT id FROM samples").Rows()
			if err == nil {
				_ = rows.Close()
			}
			return err
		},
		"raw": func() error { return db.Exec("INSERT INTO samples (id) VALUES (1)").Error },
	} {
		t.Run(name+"_answers_the_translation", func(t *testing.T) {
			require.ErrorIs(t, run(), missing)
		})
	}

	t.Run("an_error_translate_does_not_recognize_stays_as_it_is", func(t *testing.T) {
		err := db.Exec("SELEC 1").Error
		require.Error(t, err)
		require.NotErrorIs(t, err, missing)
		require.ErrorContains(t, err, "syntax error")
	})
}

// openConstrained opens a database of the test's own, foreign keys enforced,
// holding parents and children, each child referring to a parent through a
// foreign key checked at commit and carrying a code no two children share.
// With translation it installs the framework's over a translate that
// recognizes nothing, which leaves every error to the driver's own.
func openConstrained(t *testing.T, translation bool) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "constrained.db")+"?_foreign_keys=on"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE parents (id integer PRIMARY KEY)").Error)
	require.NoError(t, db.Exec("CREATE TABLE children (id integer PRIMARY KEY, code text UNIQUE, parent_id integer REFERENCES parents (id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, db.Exec("INSERT INTO parents (id) VALUES (1)").Error)
	require.NoError(t, db.Exec("INSERT INTO children (id, code, parent_id) VALUES (1, 'taken', 1)").Error)
	if translation {
		require.NoError(t, dbruntime.InstallErrorTranslation(db, func(error) error { return nil }))
	}
	return db
}

// child is a row of the children table openConstrained creates.
type child struct {
	ID       int
	Code     string
	ParentID int
}

// TestTheDriversTranslationKeepsWhatTheDatabaseSaid pins what becomes of an
// error translate does not recognize: it goes to the driver's own
// translation, and the sentinel the driver answers, which carries nothing of
// the database's error, comes back with what the database said in front of
// it, so that the error still names the key the write collided with.
func TestTheDriversTranslationKeepsWhatTheDatabaseSaid(t *testing.T) {
	db := openConstrained(t, true)

	err := db.Exec("INSERT INTO children (id, code, parent_id) VALUES (2, 'taken', 1)").Error

	require.ErrorIs(t, err, gorm.ErrDuplicatedKey)
	require.ErrorContains(t, err, "UNIQUE constraint failed: children.code")
}

// TestTheCommitOfAWritesOwnTransactionIsTranslated pins that the translation
// reaches the error of the commit gorm issues for a write outside any
// transaction: the foreign key is checked at commit, so the insert passes
// and the commit is what fails.
func TestTheCommitOfAWritesOwnTransactionIsTranslated(t *testing.T) {
	db := openConstrained(t, true)

	err := db.Create(&child{ID: 2, Code: "free", ParentID: 404}).Error

	require.ErrorIs(t, err, gorm.ErrForeignKeyViolated)
	require.ErrorContains(t, err, "FOREIGN KEY constraint failed")
}

// TestTranslateErrorTranslatesAnErrorNoStatementCarries pins the entry for an
// error the callbacks never see, the one a transaction's commit fails with:
// it comes back translated the way a statement's error does on the handle,
// while no error, an error of another kind, and any error on a handle with
// no translation installed come back as they are.
func TestTranslateErrorTranslatesAnErrorNoStatementCarries(t *testing.T) {
	db := openConstrained(t, true)
	tx := db.Begin()
	require.NoError(t, tx.Create(&child{ID: 2, Code: "free", ParentID: 404}).Error, "the foreign key is checked at commit, so the insert passes")
	committed := tx.Commit().Error
	require.Error(t, committed)
	require.NotErrorIs(t, committed, gorm.ErrForeignKeyViolated, "no callback runs over the commit of a transaction opened on the handle")

	translated := dbruntime.TranslateError(db, committed)
	require.ErrorIs(t, translated, gorm.ErrForeignKeyViolated)
	require.ErrorContains(t, translated, "FOREIGN KEY constraint failed")

	require.NoError(t, dbruntime.TranslateError(db, nil))
	other := errors.New("sample failure")
	require.Same(t, other, dbruntime.TranslateError(db, other))
	require.Equal(t, committed, dbruntime.TranslateError(openConstrained(t, false), committed))
}
