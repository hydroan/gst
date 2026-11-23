package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckMsgConst runs the check over a fixture module. Reported are the
// four sentences internal/answer/answer.go answers with that no constant of
// the module holds, a string written at the call of a constructor, one
// handed to a function of the package reaching a constructor, one put
// together from a constant where it is handed to the variable a public
// package forwards a constructor in, and one a constant of the standard
// library holds; and three constants whose name does not end in Msg, each
// once, where it is declared: one handed to the translation and to a
// constructor (lockedText), one a translation is registered under while it
// reaches a client through the result of a function (returnedText), and one
// of another package (shared.Quiet). Nothing is reported for the constants
// named as a sentence is, of the package or of another, handed to a
// constructor, to the function of the package and to the forwarding
// variable; for a message put together as the request is served; for the
// result of a function; nor for a test file (answer_test.go).
func TestCheckMsgConst(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/msgconst/module")
	violations, err := checkMsgConst(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "GET" at internal/answer/answer.go:24 is answered with and held by no constant of the framework: declare it as a constant in the translations.go of its package and answer with the constant`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample broke" at internal/answer/answer.go:21 is answered with and held by no constant of the framework: declare it as a constant in the translations.go of its package and answer with the constant`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample is lost for good" at internal/answer/answer.go:23 is answered with and held by no constant of the framework: declare it as a constant in the translations.go of its package and answer with the constant`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample is refused" at internal/answer/answer.go:22 is answered with and held by no constant of the framework: declare it as a constant in the translations.go of its package and answer with the constant`,
		},
		{
			File:    "internal/answer/translations.go",
			Message: "Constant lockedText at internal/answer/translations.go:10 holds a sentence a client is answered with: end its name in Msg",
		},
		{
			File:    "internal/answer/translations.go",
			Message: "Constant returnedText at internal/answer/translations.go:13 holds a sentence a client is answered with: end its name in Msg",
		},
		{
			File:    "internal/shared/translations.go",
			Message: "Constant Quiet at internal/shared/translations.go:9 holds a sentence a client is answered with: end its name in Msg",
		},
	}, violations)
}
