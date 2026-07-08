package serviceregistry

import "github.com/gorilla/schema"

// queryDecoder is the shared gorilla/schema decoder used to parse URL query
// parameters into models. A single instance is kept so gorilla/schema can
// reuse its internal struct-metadata cache across requests instead of
// rebuilding it via reflection every time.
//
// Decode is safe for concurrent use, but the option setters and converter
// registration are not; therefore the decoder is configured once here and
// callers must not mutate it afterwards.
var queryDecoder = func() *schema.Decoder {
	decoder := schema.NewDecoder()
	decoder.SetAliasTag("query")

	return decoder
}()

// QueryDecoder returns the shared query decoder. Callers should only use it to
// Decode; see queryDecoder for why it must not be reconfigured.
func QueryDecoder() *schema.Decoder {
	return queryDecoder
}
