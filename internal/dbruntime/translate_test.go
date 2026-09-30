package dbruntime_test

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestInstallErrorTranslationRewritesTheErrorOfEveryOperation pins that an
// error translate recognizes comes back as the translation from each of the
// six operations gorm runs through its callbacks, create, query, update,
// delete, row and raw, and that an error it does not recognize comes back
// as it is.
func TestInstallErrorTranslationRewritesTheErrorOfEveryOperation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
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
