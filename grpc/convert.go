package grpc

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file holds the conversions the generated handlers apply to what a
// message cannot carry as the Go value is: a time, a dynamic value, a
// JSON object, and the filters of a List request (a JSON document, a
// json.RawMessage or a datatypes.JSON, travels as its bytes and needs
// none). Each maps the unset value of one side to the unset value of the
// other, the zero time to no Timestamp and nil to nil, so a value comes
// back from a message as it went in. It also holds what a generated
// FromProto reads a value through when the message's type is wider than
// the model's, Narrow and Number, which refuse what the model's type cannot
// hold, the way encoding/json refuses it over HTTP.

// Narrow returns v as the narrower integer type T a model field holds, int8
// for the int32 its message carries, and refuses a value T cannot hold, 300
// folded into an int8 would come back as 44, with invalidValue naming
// field, "300 is out of range" the detail. The generated FromProto reads
// every int8, int16, uint8 and uint16 field through it.
func Narrow[T ~int8 | ~int16 | ~uint8 | ~uint16, V ~int32 | ~uint32](field string, v V) (T, error) {
	narrowed := T(v)
	if V(narrowed) != v {
		return 0, invalidValue(field, fmt.Sprintf("%d is out of range", v))
	}
	return narrowed, nil
}

// Number returns s as the json.Number a model field holds, "" for the unset
// field, and refuses with invalidValue naming field, `"abc" is not a JSON
// number` the detail, a string that is no JSON number literal, text or a
// quoted number among them: the message carries the number as a string, and
// one that is not a number would be written out as one. encoding/json
// judges the literal, so the two listeners refuse the same strings. The
// generated FromProto reads every json.Number field through it.
func Number(field, s string) (json.Number, error) {
	if s == "" {
		return "", nil
	}
	var n json.Number
	if err := json.Unmarshal([]byte(s), &n); err != nil || string(n) != s {
		return "", invalidValue(field, fmt.Sprintf("%q is not a JSON number", s))
	}
	return n, nil
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
