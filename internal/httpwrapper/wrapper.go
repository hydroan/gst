// Package httpwrapper provides JSON round-trip wrappers around
// *http.Request and *http.Response. Bodies must be valid JSON (or empty):
// they are embedded verbatim in the marshaled form to keep it readable.
package httpwrapper

import (
	"bytes"
	"encoding/json"
)

// jsonNull is the encoding produced for an empty body or absent TLS state.
var jsonNull = []byte("null")

// isJSONNull reports whether raw holds nothing or the JSON null literal.
func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(raw, jsonNull)
}
