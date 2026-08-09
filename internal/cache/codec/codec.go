// Package codec encodes cache values for the backends that store bytes
// rather than live objects.
//
// It exists to keep encoding and decoding keyed on the same thing. The
// underlying util.Marshal dispatches on a value's dynamic type, taking a
// compact path for strings, byte slices and numbers, while util.Unmarshal
// dispatches on the static type of its destination. The two agree whenever
// the cached type is concrete, and disagree exactly when it is an interface:
// a string stored through a Cache[any] is written as raw bytes and then read
// back into an *any, where the compact form is not valid JSON and decoding
// fails with "unexpected value type". Routing both directions through the
// static T removes the disagreement at its source.
package codec

import (
	"reflect"

	jsoniter "github.com/json-iterator/go"

	"github.com/hydroan/gst/util"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// Marshal encodes value for storage in a byte-addressed cache.
func Marshal[T any](value T) ([]byte, error) {
	if isInterface[T]() {
		return json.Marshal(value)
	}
	return util.Marshal(value)
}

// Unmarshal decodes a value produced by Marshal for the same T.
func Unmarshal[T any](data []byte, value *T) error {
	if isInterface[T]() {
		return json.Unmarshal(data, value)
	}
	return util.Unmarshal(data, value)
}

// isInterface reports whether T is an interface type, in which case a value's
// dynamic type is not knowable from T and the compact encoding cannot be
// reversed.
func isInterface[T any]() bool {
	return reflect.TypeFor[T]().Kind() == reflect.Interface
}
