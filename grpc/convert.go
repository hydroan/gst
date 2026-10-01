package grpc

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file holds the conversions the generated handlers apply to what a
// message cannot carry as the Go value is: a time, a dynamic value, a JSON
// object, and the filters of a List request. Each maps the unset value of
// one side to the unset value of the other, the zero time to no Timestamp
// and nil to nil, so a value comes back from a message as it went in. It
// also holds what a generated FromProto reads a value through when the
// message's type is wider than the model's, or than what the HTTP listener
// lets through: Narrow and Number, Time, Document, Finite, Map and Any
// refuse what the model's field cannot hold, the way encoding/json refuses
// it over HTTP, so that a record a gRPC call wrote reads back over HTTP;
// and what a generated ToProto writes a string through, UTF8, which
// replaces what a message cannot carry the way the JSON encoder does.

// Narrow returns v as the narrower integer type T a model field holds, int8
// for the int32 its message carries, and refuses a value T cannot hold, 300
// folded into an int8 would come back as 44, with invalidValue naming
// field, "300 is out of range" the detail. The generated FromProto reads
// every int8, int16, uint8 and uint16 field, and every map key of one of
// those types, through it.
func Narrow[T ~int8 | ~int16 | ~uint8 | ~uint16, V ~int32 | ~uint32](field string, v V) (T, error) {
	narrowed := T(v)
	if V(narrowed) != v {
		return 0, invalidValue(field, fmt.Sprintf("%d is out of range", v))
	}
	return narrowed, nil
}

// Number returns s as the json.Number a model field holds, and refuses with
// invalidValue naming field, `"abc" is not a JSON number` the detail, a
// string that is no JSON number literal, text, a quoted number or the
// empty string among them: the message carries the number as a string, and
// one that is not a number would be written out as one. encoding/json
// judges the literal, so the two listeners refuse the same strings. The
// generated FromProto reads every json.Number field through it, a field the
// message leaves empty, "", standing for the unset value and read as it is.
func Number(field, s string) (json.Number, error) {
	var n json.Number
	if err := json.Unmarshal([]byte(s), &n); err != nil || string(n) != s {
		return "", invalidValue(field, fmt.Sprintf("%q is not a JSON number", s))
	}
	return n, nil
}

// Finite returns v as the floating-point type T a model field holds, a
// named type over float32 or float64 included, and refuses NaN and the
// infinities with invalidValue naming field, "NaN is not a finite number"
// the detail: a message carries them where JSON has no spelling for them,
// so the HTTP listener never lets one in and could not write one out. The
// generated FromProto reads every float32 and float64 field through it.
func Finite[T ~float32 | ~float64, V ~float32 | ~float64](field string, v V) (T, error) {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0, invalidValue(field, fmt.Sprintf("%v is not a finite number", v))
	}
	return T(v), nil
}

// Time returns the time ts holds, in UTC, and the zero time for nil; a
// Timestamp outside the range a Timestamp may hold, the years 1 to 9999,
// is refused with invalidValue naming field and timestamppb's own account
// of it, "timestamp (seconds:253402300800) after 9999-12-31", its proto
// prefix dropped, the way the HTTP listener refuses a time it cannot
// parse. The generated FromProto reads every time through it.
func Time(field string, ts *timestamppb.Timestamp) (time.Time, error) {
	if ts == nil {
		return time.Time{}, nil
	}
	if err := ts.CheckValid(); err != nil {
		return time.Time{}, invalidValue(field, timestampDetail(err))
	}
	return ts.AsTime(), nil
}

// Timestamp returns the Timestamp of t, and nil for the zero time, which a
// message leaves unset. Where a nil would stand for something else, in a
// repeated field, a map value or an optional field, the generated ToProto
// writes the zero time as the Timestamp of 0001-01-01 instead, which comes
// back as the zero time.
func Timestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Document returns b, the bytes of a JSON document a model field holds as
// a json.RawMessage or a datatypes.JSON, nil for none, and refuses bytes
// that are no JSON document with invalidValue naming field, "not a JSON
// document" the detail: the HTTP listener reads the document out of the
// body it decoded, which encoding/json refused if the document was not
// JSON, and what a message carries is written out as one. The generated
// FromProto reads every JSON document through it.
func Document(field string, b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	if !json.Valid(b) {
		return nil, invalidValue(field, "not a JSON document")
	}
	return b, nil
}

// UTF8 returns s with every byte that is no part of valid UTF-8 replaced by
// U+FFFD, the way encoding/json writes a string out: a message carries
// valid UTF-8 alone and would fail to encode otherwise, failing the whole
// response for one value, where the JSON encoder replaces the bytes and
// goes on. The generated ToProto writes every string through it. A valid
// string comes back as it is.
func UTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteRune(utf8.RuneError)
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// invalidValue is the InvalidArgument a generated FromProto refuses a
// message field with when the value it carries is not one the model field
// holds: the sentence HTTP answers with for a body value it cannot decode
// into a field it can name, "invalid value for field 'rank'", and a
// google.rpc.BadRequest detail naming the field with description, the way
// the fields a validator refused are detailed. HTTP names the field of a
// number it cannot decode only when encoding/json reports it, which for a
// json.Number field it does not, answering without the name there.
func invalidValue(field, description string) error {
	st := status.New(codes.InvalidArgument, "invalid value for field '"+field+"'")
	detail := &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: description}}}
	if detailed, err := st.WithDetails(detail); err == nil {
		st = detailed
	}
	return st.Err()
}

// Value returns the Value holding v, the JSON value encoding/json encodes v
// to, and nil for nil: a Go value travels in a Value as it travels in a JSON
// body. It panics on a value encoding/json cannot encode, a channel or a
// function, the way the JSON renderer panics on one over HTTP; the recovery
// interceptor answers Internal and logs it.
func Value(v any) *structpb.Value {
	if v == nil {
		return nil
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("grpc: encode %T as a Value: %v", v, err))
	}
	value := new(structpb.Value)
	if err := value.UnmarshalJSON(encoded); err != nil {
		panic(fmt.Sprintf("grpc: encode %T as a Value: %v", v, err))
	}
	return value
}

// Struct returns the Struct holding m, the JSON object encoding/json encodes
// m to, and nil for a nil map. It panics on a value encoding/json cannot
// encode, like Value.
func Struct(m map[string]any) *structpb.Struct {
	if m == nil {
		return nil
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("grpc: encode a map as a Struct: %v", err))
	}
	s := new(structpb.Struct)
	if err := s.UnmarshalJSON(encoded); err != nil {
		panic(fmt.Sprintf("grpc: encode a map as a Struct: %v", err))
	}
	return s
}

// Map returns the object s holds as a map, and nil for nil. A NaN or an
// infinity among its numbers comes back as the string "NaN", "Infinity"
// or "-Infinity", the spelling the JSON mapping of a Value gives it, which
// a JSON body carries as it is.
func Map(s *structpb.Struct) map[string]any {
	if s == nil {
		return nil
	}
	return s.AsMap()
}

// Filters returns the filters a List request carries, each a Filter
// message of the request as gg gen derives it, as the Query takes them, and
// nil for none.
func Filters[F interface {
	GetField() string
	GetOp() string
	GetValues() []string
}](filters []F) []Filter {
	if filters == nil {
		return nil
	}
	out := make([]Filter, len(filters))
	for i, f := range filters {
		out[i] = Filter{Field: f.GetField(), Op: f.GetOp(), Values: f.GetValues()}
	}
	return out
}

// timestampDetail is timestamppb's account of an invalid Timestamp without
// the package prefix every protobuf error carries, "timestamp
// (seconds:253402300800) after 9999-12-31".
func timestampDetail(err error) string {
	_, detail, found := strings.Cut(err.Error(), "proto:")
	if !found {
		return err.Error()
	}
	return strings.TrimLeft(detail, "  ")
}
