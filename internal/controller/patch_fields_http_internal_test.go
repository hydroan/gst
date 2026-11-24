package controller

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

func TestPatchFieldSetFromJSONBodyUsesJSONTags(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"enabled":false,"count":0}`))

	require.NoError(t, err)
	require.Contains(t, fields, "Enabled")
	require.Contains(t, fields, "Count")
	require.NotContains(t, fields, "Name")
}

// TestPatchFieldSetFromJSONBodyMatchesKeysLikeTheDecoder pins that a key
// names its field the way encoding/json binds the value under it: the field
// encoding to the key, else the first field in declaration order whose key
// differs from it only in case, so the field the decoder bound is the field
// the patch applies.
func TestPatchFieldSetFromJSONBodyMatchesKeysLikeTheDecoder(t *testing.T) {
	fields, err := patchFieldSetFromJSONBody(reflect.TypeFor[patchFieldsRecord](), []byte(`{"Enabled":false,"COUNT":0}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Enabled": {}, "Count": {}}, fields)

	cased := reflect.TypeFor[patchFieldsCasedRecord]()
	fields, err = patchFieldSetFromJSONBody(cased, []byte(`{"Name":"x"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Upper": {}}, fields, "an exact key wins")

	var bound patchFieldsCasedRecord
	require.NoError(t, json.Unmarshal([]byte(`{"NAME":"x"}`), &bound))
	require.Equal(t, patchFieldsCasedRecord{Lower: "x"}, bound, "encoding/json binds the first field in declaration order")
	fields, err = patchFieldSetFromJSONBody(cased, []byte(`{"NAME":"x"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Lower": {}}, fields, "the patch applies the field the decoder bound")
}

// TestPatchFieldSetFromJSONBodyWrapsDecodeError pins that patch-path body
// decoding failures carry the client-safe message instead of the raw decoder
// text, matching the bindJSONRequest contract.
func TestPatchFieldSetFromJSONBodyWrapsDecodeError(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	_, err := patchFieldSetFromJSONBody(typ, []byte(`{"enabled":`))

	var serviceErr *gst.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "request body is not valid JSON", serviceErr.Msg())
}

func TestPatchManyFieldSetsFromJSONBodyKeepItemFieldsSeparate(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	fieldSets, err := patchManyFieldSetsFromJSONBody(typ, []byte(`{"items":[{"enabled":false},{"name":""}]}`))

	require.NoError(t, err)
	require.Len(t, fieldSets, 2)
	require.Contains(t, fieldSets[0], "Enabled")
	require.NotContains(t, fieldSets[0], "Name")
	require.Contains(t, fieldSets[1], "Name")
	require.NotContains(t, fieldSets[1], "Enabled")
}

// TestPatchManyFieldSetsFromJSONBodyWrapsDecodeError is the batch-path
// counterpart: a type mismatch on the items envelope must name the field
// through the client-safe message.
func TestPatchManyFieldSetsFromJSONBodyWrapsDecodeError(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	_, err := patchManyFieldSetsFromJSONBody(typ, []byte(`{"items":3}`))

	var serviceErr *gst.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "invalid value for field 'items'", serviceErr.Msg())
}
