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
func Origin(err error) string {
	var deepest *errors.ReportableStackTrace
	for cur := err; cur != nil; cur = errors.UnwrapOnce(cur) {
		if st := errors.GetReportableStackTrace(cur); st != nil {
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
