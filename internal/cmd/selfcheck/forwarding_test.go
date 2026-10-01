package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckForwarding runs the check over a fixture module with one package
// per case. Twelve functions are reported. Five only forward and have one use:
// to a function of another package (single), to a variadic one (variadic), to
// another method of the receiver (method), to a function taking the receiver
// (recvarg), and from an external test (exttest). Two forward to a function
// nothing else uses: right away (merged), and after one straight-line
// statement of their own (lead). Five wrap one call, shaping the arguments,
// and have one use: of a function of their own package (adapter's pair), of
// another package's (adapter's join), of a method of a package-level value
// of another package's type (valuemethod), and of a method of a parameter,
// in a method several types of the package declare alike, which a shared
// name does not exempt (family's two). Nothing is reported where
// the forwarding function is exported and the other one has more uses
// (exported), where the forwarding function has a second use (multiuse), in
// an internal test (testuse) or in a file the build leaves out (tagged),
// where an interface of the package declares the method (sealed), where the
// forwarding function is generated (generated), where a type changes on the way
// (converted), where the one call a function wraps takes a function literal
// (funclit) or is laid out over several lines (multiline), and where more
// than a straight-line statement runs before the forwarding: two of them
// (longlead), a branch (branch), or a statement carrying a function literal
// (closure).
func TestCheckForwarding(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/forwarding/module")
	violations, err := checkForwarding(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "adapter/adapter.go",
			Message: "Function 'join' at adapter/adapter.go:12 only wraps a call of strings.Join and has one use, at adapter/adapter.go:10: write the call there instead",
		},
		{
			File:    "adapter/adapter.go",
			Message: "Function 'pair' at adapter/adapter.go:17 only wraps a call of concat and has one use, at adapter/adapter.go:15: write the call there instead",
		},
		{
			File:    "exttest/exttest_test.go",
			Message: "Function 'newSample' at exttest/exttest_test.go:15 only forwards to exttest.New and has one use, at exttest/exttest_test.go:10: call exttest.New there instead",
		},
		{
			File:    "family/family.go",
			Message: "Method 'Header.write' at family/family.go:20 only wraps a call of Sink.add and has one use, at family/family.go:27: write the call there instead",
		},
		{
			File:    "family/family.go",
			Message: "Method 'Footer.write' at family/family.go:22 only wraps a call of Sink.add and has one use, at family/family.go:28: write the call there instead",
		},
		{
			File:    "lead/lead.go",
			Message: "Function 'Open' at lead/lead.go:12 forwards to open, which nothing else uses: merge open into Open",
		},
		{
			File:    "merged/merged.go",
			Message: "Function 'Total' at merged/merged.go:9 forwards to total, which nothing else uses: merge total into Total",
		},
		{
			File:    "method/method.go",
			Message: "Method 'Store.size' at method/method.go:14 only forwards to Store.length and has one use, at method/method.go:9: call Store.length there instead",
		},
		{
			File:    "recvarg/recvarg.go",
			Message: "Method 'Item.rename' at recvarg/recvarg.go:13 only forwards to setName and has one use, at recvarg/recvarg.go:11: call setName there instead",
		},
		{
			File:    "single/single.go",
			Message: "Function 'format' at single/single.go:9 only forwards to strconv.Itoa and has one use, at single/single.go:7: call strconv.Itoa there instead",
		},
		{
			File:    "valuemethod/valuemethod.go",
			Message: "Function 'normalize' at valuemethod/valuemethod.go:12 only wraps a call of Regexp.ReplaceAllString and has one use, at valuemethod/valuemethod.go:10: write the call there instead",
		},
		{
			File:    "variadic/variadic.go",
			Message: "Function 'describef' at variadic/variadic.go:10 only forwards to fmt.Sprintf and has one use, at variadic/variadic.go:8: call fmt.Sprintf there instead",
		},
	}, violations)
}
