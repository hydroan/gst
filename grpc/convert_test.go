package grpc_test

import (
	"encoding/json"
	"testing"
	"time"

	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestTimestampLeavesTheZeroTimeUnset pins the two ways of a time through
// a message: a time comes back as it went in, in UTC, and the zero time
// travels as no Timestamp at all.
func TestTimestampLeavesTheZeroTimeUnset(t *testing.T) {
	at := time.Date(2026, 9, 26, 8, 30, 0, 123456789, time.FixedZone("east", 8*3600))

	require.Equal(t, at.UTC(), gstgrpc.Time(gstgrpc.Timestamp(at)))
	require.Nil(t, gstgrpc.Timestamp(time.Time{}))
	require.True(t, gstgrpc.Time(nil).IsZero())
}

// TestValueCarriesWhatJSONEncodes pins that a Go value travels in a Value
// as its JSON encoding: nested slices and structs, which structpb itself
// refuses, are encoded the way a JSON body encodes them, and nil is unset.
func TestValueCarriesWhatJSONEncodes(t *testing.T) {
	type link struct {
		URL string `json:"url"`
	}
	value := gstgrpc.Value(map[string]any{"tags": []string{"a", "b"}, "link": link{URL: "https://example.com"}, "n": 3})

	require.Equal(t, map[string]any{
		"tags": []any{"a", "b"},
		"link": map[string]any{"url": "https://example.com"},
		"n":    float64(3),
	}, value.AsInterface())
	require.Nil(t, gstgrpc.Value(nil))
	require.Equal(t, "null", string(gstgrpc.JSON(gstgrpc.Value((*int)(nil)))), "a nil pointer is the JSON null, not an unset value")
}

// TestValuePanicsOnWhatJSONCannotEncode pins that a value encoding/json
// refuses is a panic, the way gin's renderer panics on it, and not a
// silently empty Value.
func TestValuePanicsOnWhatJSONCannotEncode(t *testing.T) {
	require.PanicsWithValue(t, "grpc: encode chan int as a Value: json: unsupported type: chan int", func() {
		gstgrpc.Value(make(chan int))
	})
	// The protobuf runtime words its own errors, with a non-breaking space
	// after "proto:" that keeps them from being matched by hand.
	require.Contains(t, panicValue(t, func() { gstgrpc.JSONValue([]byte("{")) }), "grpc: encode JSON as a Value: proto:")
}

// panicValue runs fn and returns the value it panics with, failing the test
// when it returns.
func panicValue(t *testing.T, fn func()) string {
	t.Helper()
	value := func() (value any) {
		defer func() { value = recover() }()
		fn()
		return nil
	}()
	require.NotNil(t, value, "fn should have panicked")
	s, ok := value.(string)
	require.True(t, ok, "fn should panic with a string, not %T", value)
	return s
}

// TestJSONValueRoundTripsADocument pins that a JSON document travels in a
// Value and comes back as JSON, null included, and that an empty document
// is unset.
func TestJSONValueRoundTripsADocument(t *testing.T) {
	raw := json.RawMessage(`{"a":[1,2,{"b":null}],"c":"x"}`)

	require.JSONEq(t, string(raw), string(gstgrpc.JSON(gstgrpc.JSONValue(raw))))
	require.Equal(t, "null", string(gstgrpc.JSON(gstgrpc.JSONValue([]byte("null")))))
	require.Nil(t, gstgrpc.JSONValue(nil))
	require.Nil(t, gstgrpc.JSON(nil))
}

// TestStructAndMapRoundTripAnObject pins that a map travels in a Struct as
// its JSON object and comes back as a map, and that nil stays nil on both
// ways, where AsMap alone would answer an empty map.
func TestStructAndMapRoundTripAnObject(t *testing.T) {
	m := map[string]any{"count": 2, "tags": []string{"a"}, "nested": map[string]any{"ok": true}}

	require.Equal(t, map[string]any{"count": float64(2), "tags": []any{"a"}, "nested": map[string]any{"ok": true}}, gstgrpc.Map(gstgrpc.Struct(m)))
	require.Nil(t, gstgrpc.Struct(nil))
	require.Nil(t, gstgrpc.Map(nil))
	require.Empty(t, gstgrpc.Map(&structpb.Struct{}))
	require.NotNil(t, gstgrpc.Map(&structpb.Struct{}))
}

// filterMessage stands for the nested Filter message of a List request,
// with the getters the plugin generates for it.
type filterMessage struct {
	field, op string
	values    []string
}

func (f *filterMessage) GetField() string    { return f.field }
func (f *filterMessage) GetOp() string       { return f.op }
func (f *filterMessage) GetValues() []string { return f.values }

// TestFiltersReadTheFilterMessages pins that the filters of a request come
// out as the Query takes them, in order, and that no filter is nil rather
// than an empty slice.
func TestFiltersReadTheFilterMessages(t *testing.T) {
	filters := gstgrpc.Filters([]*filterMessage{
		{field: "status", op: "in", values: []string{"active", "archived"}},
		{field: "name", values: []string{"alice"}},
	})

	require.Equal(t, []gstgrpc.Filter{
		{Field: "status", Op: "in", Values: []string{"active", "archived"}},
		{Field: "name", Values: []string{"alice"}},
	}, filters)
	require.Nil(t, gstgrpc.Filters[*filterMessage](nil))
}
