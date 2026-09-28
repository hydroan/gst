package grpc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hydroan/gst/internal/grpcserver"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file holds the conversions the generated handlers apply to what a
// message cannot carry as the Go value is: a time, a dynamic value, a JSON
// object, and the filters of a List request (a JSON document, a
// json.RawMessage or a datatypes.JSON, travels as its bytes and needs
// none). Each maps the unset value of one side to the unset value of the
// other, the zero time to no Timestamp and nil to nil, so a value comes
// back from a message as it went in. It also holds what a generated
// FromProto reads a value through when the message's type is wider than
// the model's, Narrow and Number, which refuse what the model's type cannot
// hold.

// Narrow returns v as the narrower integer type T a model field holds, int8
// for the int32 its message carries, and refuses with InvalidArgument,
// naming field, a value T cannot hold, the way encoding/json refuses an
// out-of-range number over HTTP: 300 folded into an int8 would come back
// as 44. The generated FromProto reads every int8, int16, uint8 and uint16
// field through it.
func Narrow[T ~int8 | ~int16 | ~uint8 | ~uint16, V ~int32 | ~uint32](field string, v V) (T, error) {
	return grpcserver.Narrow[T](field, v)
}

// Number returns s as the json.Number a model field holds, "" for the unset
// field, and refuses with InvalidArgument, naming field, a string that is no
// JSON number literal, the way encoding/json refuses it over HTTP: the
// message carries the number as a string. The generated FromProto reads
// every json.Number field through it.
func Number(field, s string) (json.Number, error) {
	return grpcserver.Number(field, s)
}

// Timestamp returns the Timestamp of t, and nil for the zero time, which a
// message leaves unset.
func Timestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Time returns the time ts holds, in UTC, and the zero time for nil.
func Time(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
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

// Map returns the object s holds as a map, and nil for nil.
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
