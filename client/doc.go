// Package client is the official HTTP client for gst backends, designed as
// the client-side pairing of the framework's DSL: every interface shape a
// model's Design() can declare has a first-class counterpart here.
//
//	DSL declaration                       client entry
//	-----------------------------------   ------------------------------------------
//	Create/Update/Patch/Delete/Get/List   verb functions (Post/Put/Patch/Delete/Get)
//	Payload[*XxxReq]()                    the verb function payload argument
//	Result[*XxxRsp]()                     the RSP type parameter, decoded from data
//	Route("xxx/:id/action")               method + path composition
//	batch actions (items/ids bodies)      verbs + BatchItems/BatchIDs on /batch
//	Export()                              Download
//	Import()                              Upload
//	SSE responses                         Stream
//	model.Pagination / Query / Cursor     WithPage/WithSortBy/WithExpand/WithCursor
//	envelope (code/msg/data/trace_id)     Envelope on success, *Error on rejection
//
// Evolution rule: whenever the DSL grows a new action or protocol shape, this
// package must grow the matching entry in the same change; a DSL capability
// without a client counterpart is an incomplete feature.
//
// API shape: entries whose result needs a type parameter are package-level
// generic functions (Get/Post/Put/Patch/Delete), because Go methods cannot
// declare their own type parameters. Entries that need none (Do, Download,
// Upload, Stream) stay methods on Client. Do not "tidy" the verb functions
// into methods; see the TODO in verbs.go for the planned refactor once the
// language supports parameterized methods.
package client
