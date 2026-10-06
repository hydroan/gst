package sample_test

import "testing"

// TestOther pins what another test pins: the comment of a neighbor, left on
// this test by an insertion.
func TestWrongName(t *testing.T) {}

// TestRightName pins its own behavior.
func TestRightName(t *testing.T) {}

// A fixture the tests share, whose comment names no declaration.
var sharedFixture = 1

// The fixtures below are read by every test; a comment on the group names
// none of them.
var (
	first = 1
	// second is the second fixture.
	second = 2
	// Third fixture, the comment of a spec that does not open with its name.
	third = 3
)

// Nothing to register: the comment of init is free text.
func init() {}

// Demonstrates the sample API, as the comment of an Example does.
func ExampleSample() {}

// Deprecated: use TestRightName.
//
// TestOldName was the earlier form.
func TestOldName(t *testing.T) {}

// Blank identifiers carry no name to open with.
var _ = first + second + third + sharedFixture
