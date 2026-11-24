package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckTranslations runs the check over a fixture module. Reported are
// the five sentences internal/answer/answer.go answers with that are
// translated nowhere, a constant handed to a constructor, a string written
// at the call of one, a constant reaching one through a function of the
// package, a string handed to the variable a public package forwards a
// constructor in, and a string handed to the translation, each for its own
// package; the sentence a constant of internal/shared holds, handed over in
// internal/answer and translated nowhere, for the package declaring it; the
// sentence of a module source, internal/moduleservice/sample, that registers
// through the public forwarder and leaves it out; the registration
// answer.go writes outside translations.go; and, in
// internal/answer/translations.go, the translations registered under a
// constant another file of the package declares, under a string, and under
// a constant of another package, the one registered under a constant
// nothing else uses, and the registration handed a variable in place of a
// map literal. Nothing is reported for the sentences translated under the
// constant handed over, whether the call names it, writes its string out or
// passes it through a function of the package; for those the package
// declaring the constant handed over translates, by that constant or by one
// renaming it; for a message put together as the request is served; for the
// result of a function, whose translation is registered under the constant
// the function returns; nor for a test file (answer_test.go), or for the two
// packages handing the registration on.
func TestCheckTranslations(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/translations/module")
	violations, err := checkTranslations(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/answer/answer.go",
			Message: "Registration at internal/answer/answer.go:69 is written outside translations.go: a package registers its translations in the file of that name",
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample broke" at internal/answer/answer.go:31 is answered to clients and has no zh translation: register one in the translations.go of internal/answer, the package it belongs to`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample is asleep" at internal/answer/answer.go:34 is answered to clients and has no zh translation: register one in the translations.go of internal/answer, the package it belongs to`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample is locked" at internal/answer/answer.go:33 is answered to clients and has no zh translation: register one in the translations.go of internal/answer, the package it belongs to`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample is lost" at internal/answer/answer.go:15 is answered to clients and has no zh translation: register one in the translations.go of internal/answer, the package it belongs to`,
		},
		{
			File:    "internal/answer/answer.go",
			Message: `Sentence "the sample is wrapped" at internal/answer/answer.go:18 is answered to clients and has no zh translation: register one in the translations.go of internal/answer, the package it belongs to`,
		},
		{
			File:    "internal/answer/translations.go",
			Message: "Registration at internal/answer/translations.go:37 is not handed a constant language and a map literal: write the translations at the call, where the check reads them",
		},
		{
			File:    "internal/answer/translations.go",
			Message: `Translation into zh of "the sample is astray" at internal/answer/translations.go:33 is not registered under a constant its file declares: declare the sentence in that file and register the translation under the constant`,
		},
		{
			File:    "internal/answer/translations.go",
			Message: `Translation into zh of "the sample is early" at internal/answer/translations.go:34 is not registered under a constant its file declares: declare the sentence in that file and register the translation under the constant`,
		},
		{
			File:    "internal/answer/translations.go",
			Message: `Translation into zh of "the sample is gone" at internal/answer/translations.go:32 is registered under goneMsg, which nothing else uses: drop the constant and its translations, no client is answered with the sentence`,
		},
		{
			File:    "internal/answer/translations.go",
			Message: `Translation into zh of "the sample is quiet" at internal/answer/translations.go:35 is not registered under a constant its file declares: declare the sentence in that file and register the translation under the constant`,
		},
		{
			File:    "internal/moduleservice/sample/translations.go",
			Message: `Sentence "the sample is unpaid" at internal/moduleservice/sample/translations.go:9 is answered to clients and has no zh translation: register one in the translations.go of internal/moduleservice/sample, the package it belongs to`,
		},
		{
			File:    "internal/shared/translations.go",
			Message: `Sentence "the sample is quiet" at internal/shared/translations.go:11 is answered to clients and has no zh translation: register one in the translations.go of internal/shared, the package it belongs to`,
		},
	}, violations)
}
