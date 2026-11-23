package datatypes_test

import (
	"encoding/json"
	"testing"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/stretchr/testify/require"
	gormdatatypes "gorm.io/datatypes"
)

// TestJSONSliceIsTheSliceItIsDeclaredOver pins the JSON of a JSONSlice and
// its use: it is written as the array a slice is, read back as it was
// written, indexed and appended to as a slice, and one nothing was set on
// is written as null.
func TestJSONSliceIsTheSliceItIsDeclaredOver(t *testing.T) {
	type record struct {
		Tags datatypes.JSONSlice[string] `json:"tags"`
	}
	written, err := json.Marshal(record{Tags: datatypes.NewJSONSlice([]string{"a", "b"})})
	require.NoError(t, err)
	require.JSONEq(t, `{"tags":["a","b"]}`, string(written))

	var read record
	require.NoError(t, json.Unmarshal(written, &read))
	require.Equal(t, datatypes.JSONSlice[string]{"a", "b"}, read.Tags)
	require.Equal(t, "b", read.Tags[1])
	read.Tags = append(read.Tags, "c")
	require.Len(t, read.Tags, 3)

	written, err = json.Marshal(record{})
	require.NoError(t, err)
	require.JSONEq(t, `{"tags":null}`, string(written))
}

// TestJSONSliceStoresThroughTheLibrarysJSONSlice pins that a JSON column
// takes and gives a JSONSlice the way it takes and gives the JSONSlice of
// gorm.io/datatypes: the value it is written as, an empty one and one
// nothing was set on among them, what it reads back from it, and the data
// type it declares.
func TestJSONSliceStoresThroughTheLibrarysJSONSlice(t *testing.T) {
	for _, tags := range [][]string{{"a", "b"}, {}, nil} {
		array, err := datatypes.NewJSONSlice(tags).Value()
		require.NoError(t, err)
		library, err := gormdatatypes.NewJSONSlice(tags).Value()
		require.NoError(t, err)
		require.Equal(t, library, array)

		var read datatypes.JSONSlice[string]
		require.NoError(t, read.Scan(array))
		require.Equal(t, datatypes.JSONSlice[string](tags), read)
	}
	require.Equal(t, gormdatatypes.JSONSlice[string]{}.GormDataType(), datatypes.JSONSlice[string]{}.GormDataType())
}
