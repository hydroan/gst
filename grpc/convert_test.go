package grpc_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	require.Equal(t, structpb.NullValue_NULL_VALUE, gstgrpc.Value((*int)(nil)).GetNullValue(), "a nil pointer is the JSON null, not an unset value")
}

// TestValuePanicsOnWhatJSONCannotEncode pins that a value encoding/json
// refuses is a panic, the way gin's renderer panics on it, and not a
// silently empty Value.
func TestValuePanicsOnWhatJSONCannotEncode(t *testing.T) {
	require.PanicsWithValue(t, "grpc: encode chan int as a Value: json: unsupported type: chan int", func() {
		gstgrpc.Value(make(chan int))
	})
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

// TestNarrowRefusesWhatTheTypeCannotHold pins Narrow, what a generated
// FromProto reads a narrow integer through: a value in range comes back as
// the field's type, a named one included, and one out of range is refused
// with InvalidArgument naming the field, the way encoding/json refuses it
// over HTTP, rather than folded into the type.
func TestNarrowRefusesWhatTheTypeCannotHold(t *testing.T) {
	type level int8
	count, err := gstgrpc.Narrow[int16]("count", int32(300))
	require.NoError(t, err)
	require.Equal(t, int16(300), count)
	lvl, err := gstgrpc.Narrow[level]("level", int32(-5))
	require.NoError(t, err)
	require.Equal(t, level(-5), lvl)
	port, err := gstgrpc.Narrow[uint8]("port", uint32(255))
	require.NoError(t, err)
	require.Equal(t, uint8(255), port)

	_, err = gstgrpc.Narrow[int8]("count", int32(300))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field 'count'", status.Convert(err).Message())
	requireFieldViolation(t, err, "count", "300 is out of range")
	_, err = gstgrpc.Narrow[uint16]("port", uint32(70000))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	requireFieldViolation(t, err, "port", "70000 is out of range")
	_, err = gstgrpc.Narrow[level]("level", int32(128))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field 'level'", status.Convert(err).Message())
	requireFieldViolation(t, err, "level", "128 is out of range")
}

// requireFieldViolation asserts that err carries one google.rpc.BadRequest
// detail naming field with description.
func requireFieldViolation(t *testing.T, err error, field, description string) {
	t.Helper()
	var violations []*errdetails.BadRequest_FieldViolation
	for _, detail := range status.Convert(err).Details() {
		if bad, ok := detail.(*errdetails.BadRequest); ok {
			violations = append(violations, bad.GetFieldViolations()...)
		}
	}
	require.Len(t, violations, 1)
	require.Equal(t, field, violations[0].GetField())
	require.Equal(t, description, violations[0].GetDescription())
}

// TestNumberRefusesWhatIsNoJSONNumber pins Number, what a generated
// FromProto reads a json.Number field through: a JSON number literal comes
// back as it is, the empty string as the unset field, and anything else,
// text, a quoted number, a literal with trailing space, is refused with
// InvalidArgument naming the field, the way encoding/json refuses it over
// HTTP.
func TestNumberRefusesWhatIsNoJSONNumber(t *testing.T) {
	amount, err := gstgrpc.Number("amount", "-12.5e3")
	require.NoError(t, err)
	require.Equal(t, json.Number("-12.5e3"), amount)
	amount, err = gstgrpc.Number("amount", "")
	require.NoError(t, err)
	require.Equal(t, json.Number(""), amount)
	for _, s := range []string{"abc", `"12"`, "12 ", "0x1f", "1.", "null"} {
		_, err := gstgrpc.Number("amount", s)
		require.Equal(t, codes.InvalidArgument, status.Code(err), s)
		require.Equal(t, `field "amount": `+strconv.Quote(s)+` is not a JSON number`, status.Convert(err).Message())
	}
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
