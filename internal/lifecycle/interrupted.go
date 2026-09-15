package lifecycle

import (
	"context"

	"github.com/cockroachdb/errors"
)

// Interrupted reports whether err is nothing but ctx's ending: the work
// returned the cancellation of the context it ran on — as is, or wrapped —
// which is what work is asked to do when the process shuts down or its lease
// is lost, not a failure of its own. An error that also carries a failure of
// the work's own, such as a join of the cancellation with another error, is
// not an interruption: the failure must not hide behind the ending.
func Interrupted(ctx context.Context, err error) bool {
	if ctx.Err() == nil || err == nil {
		return false
	}
	return onlyCause(err, ctx.Err())
}

// onlyCause reports whether every leaf of err's tree is target: a wrapper
// has one leaf, its cause; a join has one leaf per member.
func onlyCause(err, target error) bool {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		members := multi.Unwrap()
		if len(members) == 0 {
			return false
		}
		for _, member := range members {
			if !onlyCause(member, target) {
				return false
			}
		}
		return true
	}
	if next := errors.Unwrap(err); next != nil {
		return onlyCause(next, target)
	}
	return errors.Is(err, target)
}
