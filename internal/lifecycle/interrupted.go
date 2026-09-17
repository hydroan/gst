package lifecycle

import (
	"context"

	"github.com/cockroachdb/errors"
)

// Interrupted reports whether err is nothing but ctx's ending: the work
// returned the ending of the context it ran on — its error or the cause it
// was ended with, as is or wrapped — which is what work is asked to do when
// the process shuts down or its lease is lost, not a failure of its own. An
// error that also carries a failure of the work's own, such as a join of the
// ending with another error, is not an interruption: the failure must not
// hide behind the ending.
func Interrupted(ctx context.Context, err error) bool {
	if ctx.Err() == nil || err == nil {
		return false
	}
	return onlyEnding(err, innermost(ctx.Err()), innermost(context.Cause(ctx)))
}

// onlyEnding reports whether every leaf of err's tree is one of endings: a
// wrapper has one leaf, its cause; a join has one leaf per member.
func onlyEnding(err error, endings ...error) bool {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		members := multi.Unwrap()
		if len(members) == 0 {
			return false
		}
		for _, member := range members {
			if !onlyEnding(member, endings...) {
				return false
			}
		}
		return true
	}
	if next := errors.Unwrap(err); next != nil {
		return onlyEnding(next, endings...)
	}
	for _, ending := range endings {
		if errors.Is(err, ending) {
			return true
		}
	}
	return false
}

// innermost returns the error at the bottom of err's chain. A leaf is matched
// against an ending's innermost error, not the ending itself: an ending built
// with a stack, as the framework's sentinels are, wraps its leaf, and the leaf
// alone does not match the wrapper.
func innermost(err error) error {
	for next := errors.Unwrap(err); next != nil; next = errors.Unwrap(err) {
		err = next
	}
	return err
}
