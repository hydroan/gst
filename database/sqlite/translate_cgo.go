//go:build cgo

package sqlite

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// errCodes are the errors of the constraints a client's data breaks that the
// driver's translation leaves as they are, keyed by sqlite's extended error
// code: a check constraint the row fails, and a column the table requires a
// value for left NULL; the driver translates the duplicated and the foreign
// keys itself, and sqlite declares no length for a column.
var errCodes = map[sqlite3.ErrNoExtended]error{
	sqlite3.ErrConstraintCheck:   gorm.ErrCheckConstraintViolated,
	sqlite3.ErrConstraintNotNull: dbruntime.ErrNotNullViolated,
}

// translate translates a driver error whose extended code errCodes lists to
// its sentinel, the driver's text kept as the message, and nil for any
// other error, which stays as the driver's translation left it (see
// dbruntime.InstallErrorTranslation).
//
// The driver declares its error type and codes only when built with cgo, so
// this file is built only with cgo; translate_nocgo.go stands in without it.
func translate(err error) error {
	var driverErr sqlite3.Error
	if !errors.As(err, &driverErr) {
		return nil
	}
	sentinel, found := errCodes[driverErr.ExtendedCode]
	if !found {
		return nil
	}
	return errors.Wrap(sentinel, driverErr.Error())
}
