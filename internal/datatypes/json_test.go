package datatypes_test

import (
	"encoding/json"
	"testing"

	"github.com/hydroan/gst/internal/datatypes"
	"github.com/stretchr/testify/require"
	gormdatatypes "gorm.io/datatypes"
)

// sampleOptions is a value a JSON holds as one document.
type sampleOptions struct {
	Color string `json:"color"`
}

// TestJSONWritesAndReadsTheValueItHolds pins the JSON of a JSON: it is
// written as the value it holds, a json.RawMessage as the document it is,
// read back as it was written, and the value none was set on as what its Go
// zero value writes.
func TestJSONWritesAndReadsTheValueItHolds(t *testing.T) {
	type record struct {
		Options datatypes.JSON[sampleOptions]   `json:"options"`
		Raw     datatypes.JSON[json.RawMessage] `json:"raw"`
	}
	written, err := json.Marshal(record{
		Options: datatypes.NewJSON(sampleOptions{Color: "red"}),
		Raw:     datatypes.NewJSON(json.RawMessage(`{"a":[1,2]}`)),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"options":{"color":"red"},"raw":{"a":[1,2]}}`, string(written))

	var read record
	require.NoError(t, json.Unmarshal(written, &read))
	require.Equal(t, sampleOptions{Color: "red"}, read.Options.Data())
	require.JSONEq(t, `{"a":[1,2]}`, string(read.Raw.Data()))

	written, err = json.Marshal(record{})
	require.NoError(t, err)
	require.JSONEq(t, `{"options":{"color":""},"raw":null}`, string(written))
}

// TestJSONStoresThroughTheLibrarysJSONType pins that a JSON column takes
// and gives a JSON the way it takes and gives the JSONType of
// gorm.io/datatypes: the value it is written as, what it reads back from
// it, and the data type it declares.
func TestJSONStoresThroughTheLibrarysJSONType(t *testing.T) {
	options := sampleOptions{Color: "red"}
	document, err := datatypes.NewJSON(options).Value()
	require.NoError(t, err)
	library, err := gormdatatypes.NewJSONType(options).Value()
	require.NoError(t, err)
	require.Equal(t, library, document)

	var read datatypes.JSON[sampleOptions]
	require.NoError(t, read.Scan(document))
	require.Equal(t, options, read.Data())
	require.Equal(t, gormdatatypes.JSONType[sampleOptions]{}.GormDataType(), read.GormDataType())
}
