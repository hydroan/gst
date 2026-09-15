// Package lock declares the example's locks.
package lock

import "github.com/hydroan/gst/lock"

// Rebuild guards the rebuild action: one run at a time across the
// deployment, whichever replica the request lands on.
var Rebuild = lock.New("rebuild")
