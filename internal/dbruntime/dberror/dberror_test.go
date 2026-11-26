package dberror_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestIsDataIntegrityViolationNamesTheWritesTheDataBringsOn pins the errors
// that count as a write refused over its data, each as it is and behind the
// text a dialect's translation or a caller wraps it in: the sentinels gorm
// and this package translate a dialect's error to, and an error a package
// declares for a rule of its own. An outcome of another kind, a missing row,
// a canceled or an overdue statement, a failure of the database, is none of
// them.
func TestIsDataIntegrityViolationNamesTheWritesTheDataBringsOn(t *testing.T) {
	for name, err := range map[string]error{
		"a key another row holds":           gorm.ErrDuplicatedKey,
		"a foreign key no row satisfies":    gorm.ErrForeignKeyViolated,
		"a value the table's check refuses": gorm.ErrCheckConstraintViolated,
		"a value longer than its column":    dberror.ErrValueTooLong,
		"a column the table requires":       dberror.ErrNotNullViolated,
		"a rule a package declared":         dberror.NewDataIntegrityViolation("sample: a rule the data breaks"),
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, dberror.IsDataIntegrityViolation(err))
			require.True(t, dberror.IsDataIntegrityViolation(errors.Wrap(err, "create sample")))
		})
	}

	for name, err := range map[string]error{
		"no error":                  nil,
		"a missing row":             gorm.ErrRecordNotFound,
		"a canceled statement":      context.Canceled,
		"an overdue statement":      context.DeadlineExceeded,
		"a failure of the database": errors.New("Error 1146: Table 'sample' doesn't exist"),
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, dberror.IsDataIntegrityViolation(err))
		})
	}
}

// TestADeclaredViolationIsTheSentinelOfItsRule pins what a package relies on
// when it declares a rule's error here: the error says the text it was given
// and nothing more, and matches itself behind what a caller wraps it in and
// no other rule's error.
func TestADeclaredViolationIsTheSentinelOfItsRule(t *testing.T) {
	rule := dberror.NewDataIntegrityViolation("sample: a rule the data breaks")
	other := dberror.NewDataIntegrityViolation("sample: another rule the data breaks")

	require.EqualError(t, rule, "sample: a rule the data breaks")
	require.ErrorIs(t, errors.Wrap(rule, "create sample"), rule)
	require.NotErrorIs(t, errors.Wrap(rule, "create sample"), other)
}
