// Package openapigen builds the OpenAPI document a running service serves.
// Route registration records every route through Set, with the types of its
// model, request and response and the phase of its action; DocumentHandler
// builds the document from what was recorded on the first request for it, so
// a process that never serves the document never pays for building it. The
// schemas are derived from the Go types by reflection, and the descriptions
// are the doc comments the apidoc package was registered with at build time.
package openapigen
