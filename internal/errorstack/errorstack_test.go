package errorstack_test

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/errorstack"
	"github.com/stretchr/testify/require"
)

// errSampleInitTimeSentinel mirrors a package-level sentinel: its stack trace is
// captured while the package initializes, so it points at runtime.doInit
// instead of any error production site.
var errSampleInitTimeSentinel = errors.New("sample init-time sentinel")

func TestOriginTreatsInitTimeStackAsNoStack(t *testing.T) {
	t.Run("wrapped sentinel reports the wrap site, not the init stack", func(t *testing.T) {
		err := errors.WithStack(errSampleInitTimeSentinel)

		stackTrace := errorstack.Origin(err)
		require.NotEmpty(t, stackTrace)
		require.NotContains(t, stackTrace, "runtime.doInit",
			"the init-time stack of the sentinel must not win over the wrap site")
		require.Contains(t, stackTrace, "TestOriginTreatsInitTimeStackAsNoStack")
	})

	t.Run("bare sentinel has no reportable origin", func(t *testing.T) {
		require.Empty(t, errorstack.Origin(errSampleInitTimeSentinel),
			"an init-time stack alone carries no useful origin")
	})

	t.Run("run-time stack is reported unchanged", func(t *testing.T) {
		err := errors.New("sample run-time failure")

		stackTrace := errorstack.Origin(err)
		require.NotEmpty(t, stackTrace)
		// The creation frame comes first, matching Go stack trace conventions.
		lines := strings.Split(stackTrace, "\n")
		require.Contains(t, lines[0], "TestOriginTreatsInitTimeStackAsNoStack")
	})
}
