package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckMsgReach runs the check over a fixture module in which Serve of
// internal/answer, a function nothing calls, wraps what most of the
// functions it calls return. Reported are the service errors of the
// functions whose error is dropped or wrapped whichever way it goes: one
// called by the if statement wrapping it (wrapped), one called by the
// statement before, which also compares the error, tests it with errors.Is
// and answers one input without it (assigned), one handed on as it stands by
// a function whose caller wraps it (inner), one exported by a package no
// project imports, built and wrapped with the constructors a public package
// forwards (Shared), one returned by the function literal of a transaction
// whose result the caller of the function wraps (saved), one of a function
// that literal hands the error of on, worded as the request is served
// (deep), one returned by the function literal of a transaction whose result
// the function calling it wraps itself (wrote), one dropped by a continue
// with the variable declared for it (skipped), and one wrapped by a function
// whose deferred function literal only compares the named result holding it
// (counted). Nothing is reported for a service error returned as it stands
// by one caller, whether that caller always does (handed), another caller
// wraps it (either), the same caller wraps it for the other inputs (sorted)
// or the caller returns it by its named result (bare); wrapped in an error
// that is no service error (prefixed); taken out with errors.As by a caller
// answering with its status and its sentence (rebuilt); handed to another
// call before it is wrapped (logged), be it one named as errors.Is is, of
// another package (matched), or wrapped by a function of the package that is
// no constructor (helped); tested by an if statement that does not leave
// (lingering), or by one testing another variable (mismatched); kept in a
// variable before it is wrapped (stored), or held by a variable of the
// package (beyond); put into a list (kept); left in a variable declared
// before the loop a break leaves (looped), or in one a goto leaves the test
// with (landed); held by a variable a function literal returns (captured);
// built by a function handed around as a value though a caller wraps it too
// (called), called as the package initializes (early), or called from a
// function literal that is no transaction's (nested); returned by such a
// literal, or kept in a variable first, in a function whose caller wraps
// (closed); built by a function whose error reaches Serve unwrapped (wrote);
// built by a method nothing of the module calls (service.Create) or by an
// exported function of a public package (api.Check); nor for a test file
// (answer_test.go).
func TestCheckMsgReach(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/msgreach/module")
	violations, err := checkMsgReach(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:152 never reaches a client: whichever way the error of wrapped goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:162 never reaches a client: whichever way the error of assigned goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:175 never reaches a client: whichever way the error of inner goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:185 never reaches a client: whichever way the error of Shared goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:196 never reaches a client: whichever way the error of the transaction of saved goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:203 never reaches a client: whichever way the error of deep goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:211 never reaches a client: whichever way the error of the transaction of wrote goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:237 never reaches a client: whichever way the error of skipped goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
		{
			File:    "internal/answer/answer.go",
			Message: "Service error at internal/answer/answer.go:256 never reaches a client: whichever way the error of counted goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
		},
	}, violations)
}
