package answer

import (
	"testing"

	"example.com/module/internal/gst"
)

// TestSentence answers with a sentence of a test file, which the check
// leaves alone.
func TestSentence(t *testing.T) {
	if gst.NewError(418, "the sample is a test") == nil {
		t.Fatal("no error")
	}
}
