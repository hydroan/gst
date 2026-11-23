package datatypes_test

import (
	"encoding/json"
	"testing"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/stretchr/testify/require"
	gormdatatypes "gorm.io/datatypes"
)

// TestJSONMapIsTheMapItIsDeclaredOver pins the JSON of a JSONMap and its
// use: it is written as the object a map is, read back as it was written,
// read and written by key as a map, and one nothing was set on is written
// as null.
func TestJSONMapIsTheMapItIsDeclaredOver(t *testing.T) {
	type record struct {
		Meta datatypes.JSONMap `json:"meta"`
	}
	written, err := json.Marshal(record{Meta: datatypes.JSONMap{"k": "v"}})
	require.NoError(t, err)
	require.JSONEq(t, `{"meta":{"k":"v"}}`, string(written))

	var read record
	require.NoError(t, json.Unmarshal(written, &read))
	require.Equal(t, datatypes.JSONMap{"k": "v"}, read.Meta)
	require.Equal(t, "v", read.Meta["k"])
	read.Meta["n"] = true
	require.Len(t, read.Meta, 2)

	written, err = json.Marshal(record{})
	require.NoError(t, err)
	require.JSONEq(t, `{"meta":null}`, string(written))
}

// TestJSONMapStoresThroughTheLibrarysJSONMap pins that a JSON column takes
// and gives a JSONMap the way it takes and gives the JSONMap of
// gorm.io/datatypes: the value it is written as, an empty one and one
// nothing was set on among them, what it reads back from it, and the data
// type it declares.
func TestJSONMapStoresThroughTheLibrarysJSONMap(t *testing.T) {
	for _, meta := range []map[string]any{{"k": "v"}, {}, nil} {
		object, err := datatypes.JSONMap(meta).Value()
		require.NoError(t, err)
		library, err := gormdatatypes.JSONMap(meta).Value()
		require.NoError(t, err)
		require.Equal(t, library, object)

		var read datatypes.JSONMap
		require.NoError(t, read.Scan(object))
		var libraryRead gormdatatypes.JSONMap
		require.NoError(t, libraryRead.Scan(library))
		require.Equal(t, map[string]any(libraryRead), map[string]any(read))
	}
	require.Equal(t, gormdatatypes.JSONMap{}.GormDataType(), datatypes.JSONMap{}.GormDataType())
}
