package answer

import (
	"testing"

	"example.com/module/internal/gst"
)

// tested builds a service error in a test file, which the check leaves
// alone.
func tested() error {
	return gst.NewError(418, "the sample is a test")
}

func TestSentence(t *testing.T) {
	if err := tested(); err != nil {
		_ = gst.NewErrorWithCause(500, "the test failed", err)
	}
}
