//go:build cgo

package sqlite

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// errCodes are the errors of the constraints a client's data breaks that the
// driver's translation leaves as they are, keyed by sqlite's extended error
// code: a check constraint the row fails, and a column the table requires a
// value for left NULL; the driver's own table holds the duplicated and the
// foreign keys, and sqlite declares no length for a column.
var errCodes = map[sqlite3.ErrNoExtended]error{
	sqlite3.ErrConstraintCheck:   gorm.ErrCheckConstraintViolated,
	sqlite3.ErrConstraintNotNull: dberror.ErrNotNullViolated,
}

// The words sqlite's message for a write colliding with a key opens the key
// with, and an index named in place of the columns.
const (
	uniqueConstraintTag      = "UNIQUE constraint failed: "
	uniqueConstraintIndexTag = "index '"
)

// translate translates a driver error whose extended code errCodes lists to
// its sentinel, the driver's text kept as the message, and nil for any
// other error, which goes to the driver's own translation (see
// dbruntime.InstallErrorTranslation).
//
// A write colliding with a key, the primary one or a unique index, is
// answered as a dberror.DuplicatedKeyError naming what sqlite's message does:
// the columns of the key, each after its table, in UNIQUE constraint failed:
// records.tenant_id, records.code, or the index, for one over an expression,
// in UNIQUE constraint failed: index 'uniq_records_code'.
//
// The driver declares its error type and codes only when built with cgo, so
// this file is built only with cgo; translate_nocgo.go stands in without it.
func translate(err error) error {
	var driverErr sqlite3.Error
	if !errors.As(err, &driverErr) {
		return nil
	}
	if driverErr.ExtendedCode == sqlite3.ErrConstraintUnique || driverErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
		duplicated := new(dberror.DuplicatedKeyError)
		_, key, _ := strings.Cut(driverErr.Error(), uniqueConstraintTag)
		if index, named := strings.CutPrefix(key, uniqueConstraintIndexTag); named {
			duplicated.Constraint = strings.TrimSuffix(index, "'")
		} else if key != "" {
			for column := range strings.SplitSeq(key, ", ") {
				duplicated.Columns = append(duplicated.Columns, column[strings.LastIndex(column, ".")+1:])
			}
		}
		return errors.Wrap(duplicated, err.Error())
	}
	sentinel, found := errCodes[driverErr.ExtendedCode]
	if !found {
		return nil
	}
	return errors.Wrap(sentinel, driverErr.Error())
}
