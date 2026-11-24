package lifecycle

import "github.com/cockroachdb/errors"

// PanicError turns a recovered panic value into an error carrying the stack
// of the panic site. It must be called from the deferred function that
// recovered, while the goroutine is still unwinding: the frames captured then
// still include the line that panicked, whereas a stack taken after recovery
// would only show the recovering package. A panic value that is already an
// error keeps its own, deeper stack if it has one — the error_stack field
// reports the deepest stack in the chain — and gains this one otherwise.
func PanicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return errors.WithStack(err)
	}
	return errors.Newf("%v", recovered)
}
