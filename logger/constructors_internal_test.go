package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamNameIsTheFileNameWithoutItsExtension(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"access.log", "access"},
		{"logs/sample.log", "sample"},
		{" cronjob.log ", "cronjob"},
		{"", "global"},
		{"/dev/stdout", "global"},
		{"/dev/stderr", "global"},
	} {
		require.Equal(t, tc.want, streamName(tc.file), "file %q", tc.file)
	}
}
