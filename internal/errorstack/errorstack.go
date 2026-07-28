// Package errorstack extracts stack traces embedded in errors, shared by
// tracing and logging so both report the same error origin stack format.
package errorstack

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
)

// Origin extracts the deepest stack trace embedded in the unwrap chain of
// err and formats it like a Go stack trace with the innermost (error
// creation) frame first. It returns "" when no error in the chain carries a
// stack trace.
//
// A stack captured during package initialization is treated as no stack at
// all: it belongs to a package-level sentinel's construction and points at
// runtime.doInit, never at the failure site. Skipping it lets the deepest
// run-time stack in the chain win, typically the site that wrapped the
// sentinel.
func Origin(err error) string {
	var deepest *errors.ReportableStackTrace
	for cur := err; cur != nil; cur = errors.UnwrapOnce(cur) {
		if st := errors.GetReportableStackTrace(cur); st != nil && !isInitTimeStack(st) {
			deepest = st
		}
	}
	if deepest == nil || len(deepest.Frames) == 0 {
		return ""
	}

	// Sentry orders frames oldest first; iterate in reverse so the error
	// creation frame comes first, matching Go stack trace conventions.
	var sb strings.Builder
	for _, frame := range slices.Backward(deepest.Frames) {
		function := frame.Function
		if frame.Module != "" && frame.Module != "unknown" {
			function = frame.Module + "." + frame.Function
		}
		file := frame.AbsPath
		if file == "" {
			file = frame.Filename
		}
		fmt.Fprintf(&sb, "%s\n\t%s:%d\n", function, file, frame.Lineno)
	}
	return sb.String()
}

// isInitTimeStack reports whether the stack trace was captured while the
// program was initializing packages, recognized by a runtime.doInit frame.
// Regular call stacks never contain that frame.
func isInitTimeStack(st *errors.ReportableStackTrace) bool {
	for i := range st.Frames {
		if st.Frames[i].Module == "runtime" && strings.HasPrefix(st.Frames[i].Function, "doInit") {
			return true
		}
	}
	return false
}
