package database

import (
	"database/sql/driver"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gormschema "gorm.io/gorm/schema"
)

// valuedSpan is a struct carrying a time field that is a driver.Valuer as
// well.
type valuedSpan struct{ First time.Time }

func (s valuedSpan) Value() (driver.Value, error) { return s.First, nil }

// TestSQLiteTimeMirrorTypeReadsTheEmbeddedTagAsGormDoes pins that the sqlite
// stand-in reads the embedded tag the way gorm does. A semicolon escaped as
// \; keeps the word inside the comment it belongs to, so the named field is
// no embedding. A driver.Valuer is no embedding of its own fields even under
// the tag, gorm reading it through the value its Value returns, so the
// stand-in leaves it to that reading.
func TestSQLiteTimeMirrorTypeReadsTheEmbeddedTagAsGormDoes(t *testing.T) {
	type span struct{ First time.Time }
	type commented struct {
		Category string
		Span     span `gorm:"comment:a\\;embedded"`
	}
	_, mirrored := sqliteTimeMirrorType(reflect.TypeFor[commented]())
	require.False(t, mirrored)

	type stamped struct {
		Category string
		Stamp    valuedSpan `gorm:"embedded"`
	}
	sch, err := gormschema.Parse(&stamped{}, &sync.Map{}, gormschema.NamingStrategy{})
	require.NoError(t, err)
	require.Nil(t, sch.LookUpField("first"), "gorm reads no column from the Valuer's own fields")
	_, mirrored = sqliteTimeMirrorType(reflect.TypeFor[stamped]())
	require.False(t, mirrored)
}
