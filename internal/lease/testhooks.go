package lease

import "time"

// This file holds the seams the framework's own tests reach for, and nothing
// else. Both replace state a deployment never chooses — every process of a
// deployment runs the one protocol, and a process that loses a lease its
// work will not let go of ends — so neither is a knob, and neither belongs
// beside the protocol it replaces. The package is internal, so nothing
// outside the framework can call them, and no path the framework runs does,
// which is what keeps them out of a built binary.
//
// A seam whose only users are its own package's tests needs none of this:
// it stays an unexported variable next to the code it replaces, the
// scheduler's clock and the component runner's failure among them, and the
// test assigns it directly.

// SetTimings replaces the protocol's timings and returns the function that
// restores them. The tests of the capabilities built on leases play the
// protocol out in milliseconds; a deployment runs what the declarations in
// lease.go say.
func SetTimings(lease, renew, deadline, grace time.Duration) (restore func()) {
	originalLease, originalRenew, originalDeadline, originalGrace := leaseDuration, renewInterval, localDeadline, stepDownGrace
	leaseDuration, renewInterval, localDeadline, stepDownGrace = lease, renew, deadline, grace
	return func() {
		leaseDuration, renewInterval, localDeadline, stepDownGrace = originalLease, originalRenew, originalDeadline, originalGrace
	}
}

// SetFail replaces what Run does once work under a lost lease will not stop,
// and returns the function that restores it. The tests record the failure
// instead of ending the test process: the process-wide failure is one-way,
// so a test that tripped it could not run twice.
func SetFail(fn func(error)) (restore func()) {
	original := fail
	fail = fn
	return func() { fail = original }
}
