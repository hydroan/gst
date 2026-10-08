package grpc_test

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestTimestampLeavesTheZeroTimeUnset pins the two ways of a time through
// a message: a time comes back as it went in, in UTC, and the zero time
// travels as no Timestamp at all.
func TestTimestampLeavesTheZeroTimeUnset(t *testing.T) {
	at := time.Date(2026, 9, 26, 8, 30, 0, 123456789, time.FixedZone("east", 8*3600))

	back, err := gstgrpc.Time("at", gstgrpc.Timestamp(at))
	require.NoError(t, err)
	require.Equal(t, at.UTC(), back)
	require.Nil(t, gstgrpc.Timestamp(time.Time{}))
	zero, err := gstgrpc.Time("at", nil)
	require.NoError(t, err)
	require.True(t, zero.IsZero())
}

// TestTimeRefusesATimestampOutOfRange pins that a Timestamp no time can be
// made of, past the year 9999 or before the year 1, is refused with
// InvalidArgument naming the field, the way the HTTP listener refuses a
// time it cannot parse, rather than read as whatever AsTime makes of it.
func TestTimeRefusesATimestampOutOfRange(t *testing.T) {
	for _, tt := range []struct {
		name string
		ts   *timestamppb.Timestamp
		want string
	}{
		{name: "after 9999", ts: &timestamppb.Timestamp{Seconds: 253402300800}, want: "timestamp (seconds:253402300800) after 9999-12-31"},
		{name: "before 0001", ts: &timestamppb.Timestamp{Seconds: -62135596801}, want: "timestamp (seconds:-62135596801) before 0001-01-01"},
		{name: "nanos out of range", ts: &timestamppb.Timestamp{Nanos: 1_000_000_000}, want: "timestamp (nanos:1000000000) has out-of-range nanos"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gstgrpc.Time("due", tt.ts)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Equal(t, "invalid value for field 'due'", status.Convert(err).Message())
			requireFieldViolation(t, err, "due", tt.want)
		})
	}
}

// TestDocumentRefusesWhatIsNoJSON pins Document, what a generated
// FromProto reads a JSON document through: a document comes back as its
// bytes, no bytes as nil, and bytes that are no JSON document are refused
// with InvalidArgument naming the field, the way encoding/json refuses a
// body that is no JSON over HTTP.
func TestDocumentRefusesWhatIsNoJSON(t *testing.T) {
	doc, err := gstgrpc.Document("raw", []byte(`{"id":9007199254740993}`))
	require.NoError(t, err)
	require.Equal(t, []byte(`{"id":9007199254740993}`), doc)
	none, err := gstgrpc.Document("raw", nil)
	require.NoError(t, err)
	require.Nil(t, none)
	none, err = gstgrpc.Document("raw", []byte{})
	require.NoError(t, err)
	require.Nil(t, none)

	_, err = gstgrpc.Document("raw", []byte(`{"id":`))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field 'raw'", status.Convert(err).Message())
	requireFieldViolation(t, err, "raw", "not a JSON document")
}

// TestFiniteRefusesNaNAndTheInfinities pins Finite, what a generated
// FromProto reads a floating-point field through: a number comes back as
// the field's type, a named one included, and NaN and the infinities,
// which JSON has no spelling for, are refused with InvalidArgument naming
// the field.
func TestFiniteRefusesNaNAndTheInfinities(t *testing.T) {
	type ratio float64
	r, err := gstgrpc.Finite[ratio]("ratio", 1.5)
	require.NoError(t, err)
	require.InDelta(t, 1.5, float64(r), 0)
	f, err := gstgrpc.Finite[float32]("ratio", float32(2.5))
	require.NoError(t, err)
	require.InDelta(t, 2.5, float64(f), 0)

	for _, tt := range []struct {
		v    float64
		want string
	}{
		{v: math.NaN(), want: "NaN is not a finite number"},
		{v: math.Inf(1), want: "+Inf is not a finite number"},
		{v: math.Inf(-1), want: "-Inf is not a finite number"},
	} {
		_, err := gstgrpc.Finite[float64]("ratio", tt.v)
		require.Equal(t, codes.InvalidArgument, status.Code(err), tt.want)
		require.Equal(t, "invalid value for field 'ratio'", status.Convert(err).Message())
		requireFieldViolation(t, err, "ratio", tt.want)
	}
}

// TestUTF8ReplacesInvalidBytesLikeTheJSONEncoder pins UTF8, what a generated
// ToProto writes a string through: a valid string comes back as it is, and
// every byte that is no valid UTF-8 is replaced by U+FFFD, one per byte,
// which is what encoding/json writes the string out as over HTTP.
func TestUTF8ReplacesInvalidBytesLikeTheJSONEncoder(t *testing.T) {
	require.Equal(t, "plain, \u00e9 and \U0001F600", gstgrpc.UTF8("plain, \u00e9 and \U0001F600"))

	broken := "a\xffb\xfe\xfdc"
	encoded, err := json.Marshal(broken)
	require.NoError(t, err)
	var overHTTP string
	require.NoError(t, json.Unmarshal(encoded, &overHTTP))
	require.Equal(t, "a\uFFFDb\uFFFD\uFFFDc", overHTTP)
	require.Equal(t, overHTTP, gstgrpc.UTF8(broken))
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
// its JSON object and comes back as a map, that nil stays nil on both
// ways, where AsMap alone would answer an empty map, and that a number
// JSON has no spelling for comes back as the string the Value mapping
// gives it.
func TestStructAndMapRoundTripAnObject(t *testing.T) {
	m := map[string]any{"count": 2, "tags": []string{"a"}, "nested": map[string]any{"ok": true}}

	require.Equal(t, map[string]any{"count": float64(2), "tags": []any{"a"}, "nested": map[string]any{"ok": true}}, gstgrpc.Map(gstgrpc.Struct(m)))
	require.Nil(t, gstgrpc.Struct(nil))
	require.Nil(t, gstgrpc.Map(nil))
	require.Empty(t, gstgrpc.Map(&structpb.Struct{}))
	require.NotNil(t, gstgrpc.Map(&structpb.Struct{}))
	require.Equal(t, map[string]any{"n": "NaN"}, gstgrpc.Map(&structpb.Struct{Fields: map[string]*structpb.Value{"n": structpb.NewNumberValue(math.NaN())}}), "a NaN comes back spelt as the JSON mapping of a Value spells it")
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

// TestItemErrorNamesTheItemOfARefusedRecord pins ItemError, what a generated
// batch handler answers an item's FromProto refusal through: the field
// refused is named as the item's, items[1].count, in the message and in the
// BadRequest detail alike, the description kept.
func TestItemErrorNamesTheItemOfARefusedRecord(t *testing.T) {
	_, refused := gstgrpc.Narrow[int8]("count", int32(300))

	err := gstgrpc.ItemError(1, refused)

	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "invalid value for field 'items[1].count'", status.Convert(err).Message())
	requireFieldViolation(t, err, "items[1].count", "300 is out of range")
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
// back as it is, and anything else, text, a quoted number, a literal with
// trailing space, the empty string, is refused with InvalidArgument naming
// the field, the way encoding/json refuses it over HTTP; the empty string
// of an unset field never reaches it.
func TestNumberRefusesWhatIsNoJSONNumber(t *testing.T) {
	amount, err := gstgrpc.Number("amount", "-12.5e3")
	require.NoError(t, err)
	require.Equal(t, json.Number("-12.5e3"), amount)
	for _, s := range []string{"abc", `"12"`, "12 ", "0x1f", "1.", "null", ""} {
		_, err := gstgrpc.Number("amount", s)
		require.Equal(t, codes.InvalidArgument, status.Code(err), s)
		require.Equal(t, "invalid value for field 'amount'", status.Convert(err).Message())
		requireFieldViolation(t, err, "amount", strconv.Quote(s)+" is not a JSON number")
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
