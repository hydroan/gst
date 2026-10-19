package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckTestSeams runs the check over a fixture module with one package
// per case. Reported are the exports only tests use that are neither in
// testhooks.go nor marked //gst:api (onlytests.Exported, which another
// package's test calls, and the variable onlytests.Flag, which the package's
// own test reads), a directive without a reason (companion.Knob), a stale
// directive on an export production uses (used.Used), and a function
// declared in testhooks.go that production code uses (seamed.Leaked). Every
// other export is left alone for a reason the check has to recognize: a use
// in production code (onlytests.Kept), a directive with its reason
// (companion.Companion, companion.NewHolder), a use in a file behind a build
// constraint (used.Tagged), a seam declared where it belongs (seamed.Hook), a
// test support package (support), a package nothing imports outside tests
// (consumer, whose Sum only its own test calls, and the backend
// internal/cache/alt), a public package (pkg) and a main package (cmd/tool).
func TestCheckTestSeams(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/testseams/module")
	violations, err := checkTestSeams(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/companion/companion.go",
			Message: "//gst:api on variable 'Knob' of internal/companion gives no reason: say why it is an API after the directive",
		},
		{
			File:    "internal/onlytests/onlytests.go",
			Message: "Exported function 'Exported' of internal/onlytests is used by tests alone: declare it in internal/onlytests/testhooks.go, the file of the package's test seams, mark it //gst:api <why it is an API> in its doc comment, or drop it",
		},
		{
			File:    "internal/onlytests/onlytests.go",
			Message: "Exported variable 'Flag' of internal/onlytests is used by tests alone: declare it in internal/onlytests/testhooks.go, the file of the package's test seams, mark it //gst:api <why it is an API> in its doc comment, or drop it",
		},
		{
			File:    "internal/seamed/testhooks.go",
			Message: "Exported function 'Leaked' declared in internal/seamed/testhooks.go is used outside tests, at internal/consumer/consumer.go: it is no test seam, declare it beside the code it serves",
		},
		{
			File:    "internal/used/used.go",
			Message: "//gst:api on function 'Used' of internal/used is stale: non-test code uses it at internal/consumer/consumer.go; drop the directive",
		},
	}, violations)
}
