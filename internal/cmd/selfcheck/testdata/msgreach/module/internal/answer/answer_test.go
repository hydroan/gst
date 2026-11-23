package answer

import (
	"testing"

	"example.com/module/internal/types"
)

// tested builds a service error in a test file, which the check leaves
// alone.
func tested() error {
	return types.NewError(418, "the sample is a test")
}

func TestSentence(t *testing.T) {
	if err := tested(); err != nil {
		_ = types.NewErrorWithCause(500, "the test failed", err)
	}
}
