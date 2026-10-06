package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckLogFields runs the check over a fixture module holding one case
// each of what it reports: a key of internal/logfield written through a
// constructor and through a key-value pair, a key written as a number and as
// a string, a key-value pair whose value is of interface type, zap.Object
// outside the packages allowed to nest keys, and a constructor the check
// does not classify; left alone are a key of one type, a logfield
// constructor, util's inlined duration, a reflected struct, a key only known
// at run time, a field handed to a key-value method, and a module source
// writing a logfield key with the key's type, which a project could not
// route through logfield; a module source writing it as another type is
// reported by the type rule like any package.
func TestCheckLogFields(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/logfields/module")
	violations, err := checkLogFields(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/logfield/logfield.go",
			Message: "Log field \"status\": a number at internal/logfield/logfield.go:11, a string at internal/service/sample/mismatch.go:7, a number at internal/service/sample/sample.go:8; a key has one type, a store mapping keys by type drops the entries of every other",
		},
		{
			File:    "sample/direct.go",
			Message: "Log field 'sample/direct.go:8': the key \"threshold\" is internal/logfield's; write it through logfield.Threshold, not a key-value pair of Infow",
		},
		{
			File:    "sample/direct.go",
			Message: "Log field 'sample/direct.go:9': the key \"status\" is internal/logfield's; write it through logfield.Status, not zap.String",
		},
		{
			File:    "sample/dynamic.go",
			Message: "Log field 'sample/dynamic.go:6': the value of \"payload\" is of interface type any, so the type of the field is decided at run time; render it with a typed constructor, zap.String with fmt.Sprint for a value of any type",
		},
		{
			File:    "sample/mixed.go",
			Message: "Log field \"size\": a number at sample/mixed.go:8, a string at sample/mixed.go:9; a key has one type, a store mapping keys by type drops the entries of every other",
		},
		{
			File:    "sample/nested.go",
			Message: "Log field 'sample/nested.go:13': zap.Object nests keys under the entry; internal/logfield and util alone, whose key sets are fixed, may nest",
		},
		{
			File:    "sample/nested.go",
			Message: "Log field 'sample/nested.go:16': zap.Weird is a field constructor this check does not classify; add it to logfields.go",
		},
	}, violations)
}
