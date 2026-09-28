package grpcserver

import (
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This file holds what a generated FromProto reads a value through when the
// message's type is wider than the model's: a message carries an int32 for
// an int8 field and a string for a json.Number, and what the model's type
// cannot hold is refused rather than folded into it, the way encoding/json
// refuses it over HTTP. The public grpc.Narrow and grpc.Number forward to
// them.

// Narrow returns v as the narrower integer type T a model field holds, int8
// for the int32 its message carries, and refuses with InvalidArgument,
// naming field, a value T cannot hold: 300 folded into an int8 would come
// back as 44.
func Narrow[T ~int8 | ~int16 | ~uint8 | ~uint16, V ~int32 | ~uint32](field string, v V) (T, error) {
	narrowed := T(v)
	if V(narrowed) != v {
		return 0, status.Errorf(codes.InvalidArgument, "field %q: %d does not fit %T", field, v, narrowed)
	}
	return narrowed, nil
}

// Number returns s as the json.Number a model field holds, "" for the unset
// field, and refuses with InvalidArgument, naming field, a string that is no
// JSON number literal, text or a quoted number among them: the message
// carries the number as a string, and one that is not a number would be
// written out as one. encoding/json judges the literal, so the two
// listeners refuse the same strings.
func Number(field, s string) (json.Number, error) {
	if s == "" {
		return "", nil
	}
	var n json.Number
	if err := json.Unmarshal([]byte(s), &n); err != nil || string(n) != s {
		return "", status.Errorf(codes.InvalidArgument, "field %q: %q is not a JSON number", field, s)
	}
	return n, nil
}
