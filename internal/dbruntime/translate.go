package dbruntime

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// ErrValueTooLong is answered for a write carrying a value longer than its
// column holds, translated from the dialect's own error the way gorm
// translates a duplicated key; database.ErrValueTooLong forwards it.
var ErrValueTooLong = errors.New("value too long for its column")

// ErrNotNullViolated is answered for a write leaving a column the table
// requires a value for NULL, translated from the dialect's own error the
// way ErrValueTooLong is; database.ErrNotNullViolated forwards it.
var ErrNotNullViolated = errors.New("a column the table requires a value for is null")

// translateErrorCallback names the callback InstallErrorTranslation registers
// on each of gorm's operations.
const translateErrorCallback = "gst:translate_error"

// InstallErrorTranslation registers translate on db to run over the error a
// statement fails with, once the driver's own translation, which gorm runs as
// it records the error (see gorm.ErrorTranslator), is done with it: an error
// translate recognizes, one of the constraints a client's data breaks that
// the driver leaves as it is, is replaced by the sentinel translate returns;
// any other, for which translate returns nil, stays as it is. The callback
// is registered after gorm's own callbacks of each of its six operations,
// create, query, update, delete, row and raw, which is how dbresolver and
// otelgorm hook a handle too: through callbacks, with the driver's dialector
// left in place, since gorm.io/datatypes tells the dialect apart by the
// dialector's own type.
func InstallErrorTranslation(db *gorm.DB, translate func(err error) error) error {
	rewrite := func(stmt *gorm.DB) {
		if stmt.Error == nil {
			return
		}
		if translated := translate(stmt.Error); translated != nil {
			stmt.Error = translated
		}
	}
	callbacks := db.Callback()
	for _, register := range []func(name string, fn func(*gorm.DB)) error{
		callbacks.Create().After("gorm:create").Register,
		callbacks.Query().After("gorm:query").Register,
		callbacks.Update().After("gorm:update").Register,
		callbacks.Delete().After("gorm:delete").Register,
		callbacks.Row().After("gorm:row").Register,
		callbacks.Raw().After("gorm:raw").Register,
	} {
		if err := register(translateErrorCallback, rewrite); err != nil {
			return errors.Wrap(err, "failed to register the database error translation")
		}
	}
	return nil
}
