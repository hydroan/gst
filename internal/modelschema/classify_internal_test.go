package modelschema

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// pointerDataType declares its gorm data type on the pointer receiver, the
// way a custom wrapper with pointer-based Scan/Value methods would.
type pointerDataType struct{}

func (*pointerDataType) GormDataType() string { return "JSONB" }

func TestIsJSONType(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[datatypes.JSON](),
			reflect.TypeFor[datatypes.JSONType[map[string]string]](),
			reflect.TypeFor[datatypes.JSONSlice[string]](),
			reflect.TypeFor[datatypes.JSONMap](),
			// A pointer column stores the pointed-to document.
			reflect.TypeFor[*datatypes.JSONSlice[string]](),
			// The declaration may hang off the pointer receiver, and the
			// spelling is case-insensitive.
			reflect.TypeFor[pointerDataType](),
		} {
			require.True(t, isJSONType(typ), typ.String())
		}
	})

	t.Run("NotJSON", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[string](), reflect.TypeFor[[]string](),
			reflect.TypeFor[[]byte](), reflect.TypeFor[gorm.DeletedAt](),
			reflect.TypeFor[time.Time](),
		} {
			require.False(t, isJSONType(typ), typ.String())
		}
		require.False(t, isJSONType(nil))
	})
}
