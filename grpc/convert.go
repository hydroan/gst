package grpc

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file holds the conversions the generated handlers apply to what a
// message cannot carry as the Go value is: a time, a JSON value, a JSON
// object, and the filters of a List request. Each maps the unset value of
// one side to the unset value of the other, the zero time to no Timestamp
// and nil to nil, so a value comes back from a message as it went in.

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
	return JSONValue(encoded)
}

// JSONValue returns the Value holding the JSON document raw, and nil for an
// empty one, which a JSON body would not carry either. It panics on
// malformed JSON, the way encoding/json refuses to encode it over HTTP; the
// recovery interceptor answers Internal and logs it.
func JSONValue(raw []byte) *structpb.Value {
	if len(raw) == 0 {
		return nil
	}
	value := new(structpb.Value)
	if err := value.UnmarshalJSON(raw); err != nil {
		panic(fmt.Sprintf("grpc: encode JSON as a Value: %v", err))
	}
	return value
}

// JSON returns the JSON document v holds, and nil for nil.
func JSON(v *structpb.Value) []byte {
	if v == nil {
		return nil
	}
	encoded, err := v.MarshalJSON()
	if err != nil {
		panic(fmt.Sprintf("grpc: decode a Value as JSON: %v", err))
	}
	return encoded
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
