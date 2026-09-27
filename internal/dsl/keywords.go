package dsl

// This file declares the keywords of the DSL, the functions a model's
// Design() calls: each does nothing, a Design() being read as source by the
// parser (see Parse) and never run. The public dsl package forwards every
// keyword to its declaration here and documents what it declares; methodList
// (see design.go) is how the parser knows them by name.

// Endpoint is the Endpoint keyword; the public dsl.Endpoint forwards to it.
func Endpoint(string) {}

// Param is the Param keyword; the public dsl.Param forwards to it.
func Param(string) {}

// Route is the Route keyword; the public dsl.Route forwards to it.
func Route(string, func()) {}

// Migrate is the Migrate keyword; the public dsl.Migrate forwards to it.
func Migrate() {}

// GRPC is the GRPC keyword; the public dsl.GRPC forwards to it.
func GRPC() {}

// Service is the Service keyword; the public dsl.Service forwards to it.
func Service(...string) {}

// Flatten is the Flatten keyword; the public dsl.Flatten forwards to it.
func Flatten() {}

// Public is the Public keyword; the public dsl.Public forwards to it.
func Public() {}

// Exact is the Exact keyword; the public dsl.Exact forwards to it.
func Exact() {}

// Payload is the Payload keyword; the public dsl.Payload forwards to it.
func Payload[T any]() {}

// Result is the Result keyword; the public dsl.Result forwards to it.
func Result[T any]() {}

// Create is the Create keyword; the public dsl.Create forwards to it.
func Create(func()) {}

// Delete is the Delete keyword; the public dsl.Delete forwards to it.
func Delete(func()) {}

// Update is the Update keyword; the public dsl.Update forwards to it.
func Update(func()) {}

// Patch is the Patch keyword; the public dsl.Patch forwards to it.
func Patch(func()) {}

// List is the List keyword; the public dsl.List forwards to it.
func List(func()) {}

// Get is the Get keyword; the public dsl.Get forwards to it.
func Get(func()) {}

// CreateMany is the CreateMany keyword; the public dsl.CreateMany forwards to it.
func CreateMany(func()) {}

// DeleteMany is the DeleteMany keyword; the public dsl.DeleteMany forwards to it.
func DeleteMany(func()) {}

// UpdateMany is the UpdateMany keyword; the public dsl.UpdateMany forwards to it.
func UpdateMany(func()) {}

// PatchMany is the PatchMany keyword; the public dsl.PatchMany forwards to it.
func PatchMany(func()) {}

// Import is the Import keyword; the public dsl.Import forwards to it.
func Import(func()) {}

// Export is the Export keyword; the public dsl.Export forwards to it.
func Export(func()) {}

// SSE is the SSE keyword; the public dsl.SSE forwards to it.
func SSE(func()) {}

// Stream is the Stream keyword; the public dsl.Stream forwards to it.
func Stream(func()) {}

// StreamingPayload is the StreamingPayload keyword; the public dsl.StreamingPayload forwards to it.
func StreamingPayload[T any]() {}

// StreamingResult is the StreamingResult keyword; the public dsl.StreamingResult forwards to it.
func StreamingResult[T any]() {}
