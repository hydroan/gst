// Package dberror names the errors a write is refused with over the data it
// carries and tells them from a failure of the database, for the SQL log and
// the database layer to record the two apart. It imports no package of the
// framework: a package with a rule of its own that a write's data can break
// declares the error of that rule in its own code, with
// NewDataIntegrityViolation.
package dberror

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// dataIntegrityViolationError is the error of a write refused over the data
// it carries, as NewDataIntegrityViolation makes it.
type dataIntegrityViolationError struct{ msg string }

func (e *dataIntegrityViolationError) Error() string { return e.msg }

// NewDataIntegrityViolation returns an error saying msg that
// IsDataIntegrityViolation recognizes, for a package to declare as the
// sentinel of a rule a write's data can break.
func NewDataIntegrityViolation(msg string) error {
	return &dataIntegrityViolationError{msg: msg}
}

// The sentinels gorm declares for the writes a constraint of the table
// refuses, each translated from the dialect's own error. What the framework
// means by each is said here once: database forwards them to a project, and
// a package that cannot import database tells them by these names.
var (
	// ErrDuplicatedKey is answered for a write colliding with a primary or
	// unique key; database.ErrDuplicatedKey forwards it.
	ErrDuplicatedKey = gorm.ErrDuplicatedKey

	// ErrForeignKeyViolated is answered for a write whose foreign key no
	// record satisfies, or that would leave a record other records still
	// refer to; database.ErrForeignKeyViolated forwards it.
	ErrForeignKeyViolated = gorm.ErrForeignKeyViolated

	// ErrCheckConstraintViolated is answered for a write carrying a value a
	// check constraint of the table refuses;
	// database.ErrCheckConstraintViolated forwards it.
	ErrCheckConstraintViolated = gorm.ErrCheckConstraintViolated
)

// ErrValueTooLong is answered for a write carrying a value longer than its
// column holds, translated from the dialect's own error the way gorm
// translates a duplicated key; database.ErrValueTooLong forwards it.
var ErrValueTooLong = NewDataIntegrityViolation("value too long for its column")

// ErrNotNullViolated is answered for a write leaving a column the table
// requires a value for NULL, translated from the dialect's own error the
// way ErrValueTooLong is; database.ErrNotNullViolated forwards it.
var ErrNotNullViolated = NewDataIntegrityViolation("a column the table requires a value for is null")

// IsDataIntegrityViolation reports whether err is a write refused over the
// data it carries: a key another row holds, a foreign key no row satisfies
// or one other rows still refer to, a value the table's check refuses, a
// value longer than its column, a column the table requires a value for left
// NULL, or a rule a package declared with NewDataIntegrityViolation. The
// data brings each of them on and the database is not at fault, so a record
// of the database's failures leaves them out.
func IsDataIntegrityViolation(err error) bool {
	return errors.HasType(err, (*dataIntegrityViolationError)(nil)) ||
		errors.IsAny(err, ErrDuplicatedKey, ErrForeignKeyViolated, ErrCheckConstraintViolated)
}
