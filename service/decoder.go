package service

import "github.com/hydroan/gst/internal/serviceregistry"

// QueryDecoder exposes the shared query decoder to business code, so services
// can parse URL query parameters into models with the same configuration used
// by the framework. See serviceregistry.QueryDecoder for its sharing and
// concurrency semantics; in particular, do not reconfigure the returned
// decoder.
var QueryDecoder = serviceregistry.QueryDecoder
