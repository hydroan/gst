// Package client is the official HTTP client for gst backends, designed as
// the client-side pairing of the framework's DSL: every interface shape a
// model's Design() can declare has a first-class counterpart here.
//
//	DSL declaration                       client entry
//	-----------------------------------   ------------------------------------------
//	Create/Update/Patch/Delete/Get/List   verb methods (Post/Put/Patch/Delete/Get)
//	Payload[*XxxReq]()                    the verb method payload argument
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
// API shape: every entry is a method on Client. The verbs whose result needs
// a type parameter (Get/Post/Put/Patch/Delete) are parameterized methods, so
// a call reads cli.Get[XxxRsp](path); entries that need no type parameter
// (Do, Download, Upload, Stream) are plain methods.
package client
