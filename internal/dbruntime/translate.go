package dbruntime

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// ErrValueTooLong is answered for a write carrying a value longer than its
// column holds, translated from the dialect's own error the way gorm
// translates a duplicated key; database.ErrValueTooLong forwards it.
var ErrValueTooLong = errors.New("value too long for its column")

// Translating returns dialector with translate ahead of the driver's own
// error translation: an error translate recognizes, one of the constraints
// a client's data breaks that the driver leaves as they are, becomes the
// sentinel translate returns; any other, for which translate returns nil,
// goes to the driver's translator, and through untouched when the driver
// has none. gorm looks the translator up on the dialector (see
// gorm.ErrorTranslator), so the wrapping is a dialector of its own,
// forwarding the savepoints and the driver's Apply, the settings it hands
// gorm's configuration before gorm fills in its defaults, which an embedded
// interface would otherwise hide; the migrator is part of the interface and
// forwards on its own.
func Translating(dialector gorm.Dialector, translate func(err error) error) gorm.Dialector {
	return translatingDialector{Dialector: dialector, translate: translate}
}

// translatingDialector is what Translating returns.
type translatingDialector struct {
	gorm.Dialector
	translate func(err error) error
}

func (d translatingDialector) Apply(config *gorm.Config) error {
	if applier, ok := d.Dialector.(interface{ Apply(*gorm.Config) error }); ok {
		return applier.Apply(config)
	}
	return nil
}

func (d translatingDialector) Translate(err error) error {
	if translated := d.translate(err); translated != nil {
		return translated
	}
	if translator, ok := d.Dialector.(gorm.ErrorTranslator); ok {
		return translator.Translate(err)
	}
	return err
}

func (d translatingDialector) SavePoint(tx *gorm.DB, name string) error {
	if saver, ok := d.Dialector.(gorm.SavePointerDialectorInterface); ok {
		return saver.SavePoint(tx, name)
	}
	return gorm.ErrUnsupportedDriver
}

func (d translatingDialector) RollbackTo(tx *gorm.DB, name string) error {
	if saver, ok := d.Dialector.(gorm.SavePointerDialectorInterface); ok {
		return saver.RollbackTo(tx, name)
	}
	return gorm.ErrUnsupportedDriver
}
